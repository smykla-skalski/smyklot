package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	adminpanel "github.com/smykla-skalski/smyklot/internal/panel"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/storage/open"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

func TestMaintenanceQueueCoversEveryRecurringWorkload(t *testing.T) {
	t.Parallel()
	targetID := "github:installation:1"
	server := &server{panel: &adminpanel.Server{}}
	jobs := server.panelMaintenanceJobs(context.Background())
	jobs = append(jobs, server.targetMaintenanceJobs(context.Background(), targetID, 1)...)
	jobs = append(jobs, server.repositoryMaintenanceJobs(
		context.Background(), targetID, 1,
		github.Repository{ID: 1, Owner: "smykla-skalski", Name: "smyklot"},
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
