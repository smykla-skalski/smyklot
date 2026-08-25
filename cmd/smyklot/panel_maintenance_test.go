package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	adminpanel "github.com/smykla-skalski/smyklot/internal/panel"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/storage/open"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

type maintenanceCatalogStore struct {
	storage.Store
	target       storage.Target
	repositories []storage.Repository
	gate         storage.PendingCIRepositoryGate
	gates        []storage.PendingCIRepositoryGate
}

func (store maintenanceCatalogStore) GetPendingCIRepositoryGate(
	context.Context,
	string,
) (storage.PendingCIRepositoryGate, error) {
	return store.gate, nil
}

func (store maintenanceCatalogStore) ListTargetPendingCIRepositoryGates(
	context.Context,
	string,
) ([]storage.PendingCIRepositoryGate, error) {
	return store.gates, nil
}

func (store maintenanceCatalogStore) ListRootTargets(context.Context) ([]storage.Target, error) {
	return []storage.Target{store.target}, nil
}

func (store maintenanceCatalogStore) ListRepositories(
	context.Context,
	string,
) ([]storage.Repository, error) {
	return store.repositories, nil
}

func (store maintenanceCatalogStore) GetTarget(context.Context, string) (storage.Target, error) {
	return store.target, nil
}

func (store maintenanceCatalogStore) GetRepository(
	_ context.Context,
	_, repositoryID string,
) (storage.Repository, error) {
	for _, repository := range store.repositories {
		if repository.ID == repositoryID {
			return repository, nil
		}
	}

	return storage.Repository{}, storage.ErrNotFound
}

func TestMaintenanceQueueCoversEveryRecurringWorkload(t *testing.T) {
	t.Parallel()
	targetID := "github:installation:1"
	server := &server{panel: &adminpanel.Server{}}
	jobs := server.panelMaintenanceJobs(context.Background())
	jobs = append(jobs, server.targetMaintenanceJobs(context.Background(), targetID, 1)...)
	jobs = append(jobs, server.repositoryMaintenanceJobs(
		context.Background(), targetID, 1,
		github.Repository{ID: 1, Owner: "smykla-skalski", Name: "smyklot"},
		true, nil,
	)...)

	queued := make(map[workqueue.Kind]int, len(jobs))
	for _, job := range jobs {
		queued[job.work.kind]++
	}
	for _, kind := range workqueue.Kinds() {
		if !kind.Recurring() {
			continue
		}
		if queued[kind] != 1 {
			t.Errorf("recurring workload %q has %d maintenance queue producers", kind, queued[kind])
		}
	}
	if len(queued) != 8 {
		t.Errorf("maintenance queue has %d workload producers, want 8", len(queued))
	}
}

func TestStoredSweepRepositoryReadsScopedIdentity(t *testing.T) {
	t.Parallel()

	repository, err := storedSweepRepository(storage.Repository{
		ID:            storage.RepositoryID(31),
		FullName:      "smykla-skalski/smyklot",
		Private:       true,
		DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if repository.ID != 31 || repository.Owner != "smykla-skalski" ||
		repository.Name != "smyklot" || !repository.Private || repository.DefaultBranch != "main" {
		t.Fatalf("converted repository = %#v", repository)
	}
}

func TestMaintenanceQueueRetainsOnlyDisabledRepositoryCleanup(t *testing.T) {
	t.Parallel()
	enabled := true
	targetID := storage.InstallationID(1)
	store := maintenanceCatalogStore{
		target: storage.Target{
			ID: targetID, InstallationID: "1", Available: true,
			RepositoryDefaultEnabled: false,
		},
		repositories: []storage.Repository{
			{
				ID: storage.RepositoryID(11), TargetID: targetID,
				FullName: "smykla-skalski/disabled", Available: true,
			},
			{
				ID: storage.RepositoryID(12), TargetID: targetID,
				FullName: "smykla-skalski/enabled", Available: true,
				EnabledOverride: &enabled,
			},
		},
		gates: []storage.PendingCIRepositoryGate{{
			RepositoryID: storage.RepositoryID(11), EffectiveMode: storage.PendingCIEffectiveChecks,
		}},
	}
	service := &server{panel: &adminpanel.Server{}, store: store}
	jobs, err := service.durableMaintenanceJobs(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	disabledID := storage.RepositoryID(11)
	enabledID := storage.RepositoryID(12)
	disabledJobs := 0
	enabledJobs := 0
	for _, job := range jobs {
		if job.work.repositoryID != nil && *job.work.repositoryID == disabledID {
			disabledJobs++
			if job.work.kind != workqueue.KindPendingCIGate {
				t.Fatalf("disabled repository published %q", job.work.kind)
			}
		}
		if job.work.repositoryID != nil && *job.work.repositoryID == enabledID {
			enabledJobs++
		}
	}
	if disabledJobs != 1 {
		t.Fatalf("disabled repository jobs = %d, want cleanup only", disabledJobs)
	}
	if enabledJobs != 3 {
		t.Fatalf("enabled repository jobs = %d, want 3", enabledJobs)
	}
}

func TestDisabledRepositoryExecutionStopsBeforeGitHub(t *testing.T) {
	t.Parallel()
	targetID := storage.InstallationID(1)
	repositoryID := storage.RepositoryID(11)
	store := maintenanceCatalogStore{
		target: storage.Target{
			ID: targetID, InstallationID: "1", Available: true,
			RepositoryDefaultEnabled: false,
		},
		repositories: []storage.Repository{{
			ID: repositoryID, TargetID: targetID,
			FullName: "smykla-skalski/disabled", Available: true,
		}},
	}
	service := &server{panel: &adminpanel.Server{}, store: store}
	repository := github.Repository{
		ID: 11, Owner: "smykla-skalski", Name: "disabled",
	}

	if err := service.scanQueuedReactions(t.Context(), targetID, 1, repository); err != nil {
		t.Fatalf("disabled reaction scan = %v", err)
	}
	if err := service.migrateRepositoryConfig(t.Context(), nil, targetID, repository); err != nil {
		t.Fatalf("disabled configuration migration = %v", err)
	}
	if err := service.reconcileQueuedPendingCIGate(t.Context(), targetID, 1, repository); err != nil {
		t.Fatalf("disabled pending-CI reconciliation = %v", err)
	}
}

func TestRecurringCompletionDistinguishesBlockersAndTerminalGitHubFailures(t *testing.T) {
	t.Parallel()
	blocked := recurringCompletion("", recurringBlocker{reason: "safe operator guidance"})
	if !blocked.Blocked || blocked.Retryable || blocked.Failure != "safe operator guidance" {
		t.Fatalf("blocked completion = %#v", blocked)
	}

	terminal := recurringCompletion("", github.NewAPIError(
		errors.New("forbidden"), 403, "GET", "/repos/owner/repo/rulesets",
		errors.New("upgrade to GitHub Pro"),
	))
	if terminal.Blocked || terminal.Retryable {
		t.Fatalf("terminal completion = %#v", terminal)
	}
}

func TestPendingCIGateQueueOutcomeRetriesTemporaryProviderFailures(t *testing.T) {
	t.Parallel()
	service := &server{store: maintenanceCatalogStore{
		gate: storage.PendingCIRepositoryGate{
			Readiness: storage.PendingCIBlocked,
			Reason:    "GitHub is temporarily unavailable. Smyklot will retry.",
		},
	}}
	cause := github.NewAPIError(
		github.ErrAPIRequest, 503, "GET", "/repos/owner/repo/rulesets",
		errors.New("upstream unavailable"),
	)

	outcome := service.pendingCIGateQueueOutcome(t.Context(), "repository-1", cause)
	completion := recurringCompletion("", outcome)
	if completion.Blocked || !completion.Retryable {
		t.Fatalf("temporary provider completion = %#v", completion)
	}
}

func TestMissingMaintenanceScopeIsSupersededBeforeClaim(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	store, err := open.Store(ctx, filepath.Join(t.TempDir(), "queue.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	targetID, repositoryID := storage.InstallationID(1), storage.RepositoryID(11)
	_, err = store.EnsureRecurringWork(ctx, workqueue.RecurringClaim{
		Kind: workqueue.KindReactionScan, TargetID: &targetID,
		RepositoryID: &repositoryID, Title: "Discover pull request reactions",
		Now: time.Now().UTC().Add(-time.Hour), LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	service := &server{store: store}
	if err := service.supersedeMissingMaintenanceJobs(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := store.ClaimNextRecurringWork(ctx, workqueue.RecurringLease{
		Now: time.Now().UTC(), LeaseDuration: time.Minute,
	}); err != nil || claimed {
		t.Fatalf("claim after supersede = %t, %v", claimed, err)
	}
}

func TestMissingMaintenanceScopeIsSupersededAfterClaim(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	store, err := open.Store(ctx, filepath.Join(t.TempDir(), "queue.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	targetID, repositoryID := "github:installation:1", "repository:removed"
	_, err = store.EnsureRecurringWork(ctx, workqueue.RecurringClaim{
		Kind: workqueue.KindReactionScan, TargetID: &targetID,
		RepositoryID: &repositoryID, Title: "Discover pull request reactions",
		Now: time.Now().UTC().Add(-time.Hour), LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	service := &server{store: store}
	claimed, err := service.runNextMaintenanceJob(ctx, nil)
	if err != nil {
		t.Fatalf("retire unmatched maintenance work: %v", err)
	}
	if !claimed {
		t.Fatal("expected unmatched maintenance work to be claimed")
	}
	page, err := store.ListWorkQueue(ctx, workqueue.Filter{
		Kinds: []workqueue.Kind{workqueue.KindReactionScan},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].State != workqueue.StateSuperseded {
		t.Fatalf("unmatched maintenance work was not retired: %#v", page.Items)
	}
}
