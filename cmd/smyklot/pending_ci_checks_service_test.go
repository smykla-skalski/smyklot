package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/storage/open"
)

type pendingCICheckTokensStub struct{}

func (pendingCICheckTokensStub) AppToken() (string, error) {
	return "app-token", nil
}

func (pendingCICheckTokensStub) InstallationToken(int64) (string, error) {
	return "installation-token", nil
}

type pendingCICheckAPICalls struct {
	t           *testing.T
	listCalls   atomic.Int64
	createCalls atomic.Int64
	patchMu     sync.Mutex
	patchStates []string
}

func (calls *pendingCICheckAPICalls) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/app":
		_ = json.NewEncoder(w).Encode(map[string]any{"id": int64(17)})
	case "/repos/owner/repo/commits/head/check-runs":
		calls.listCalls.Add(1)
		time.Sleep(50 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total_count": 0,
			"check_runs":  []any{},
		})
	case "/repos/owner/repo/check-runs":
		created := calls.createCalls.Add(1)
		var request struct {
			Name       string `json:"name"`
			HeadSHA    string `json:"head_sha"`
			ExternalID string `json:"external_id"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			calls.t.Error(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          int64(700) + created,
			"name":        request.Name,
			"head_sha":    request.HeadSHA,
			"external_id": request.ExternalID,
			"status":      request.Status,
			"conclusion":  request.Conclusion,
			"html_url":    fmt.Sprintf("https://github.example/checks/%d", 700+created),
			"app":         map[string]any{"id": int64(17)},
		})
	case "/repos/owner/repo/check-runs/702":
		var request struct {
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			calls.t.Error(err)
		}
		calls.patchMu.Lock()
		calls.patchStates = append(
			calls.patchStates,
			request.Status+":"+request.Conclusion,
		)
		calls.patchMu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": int64(702), "name": storage.PendingCICheckName,
			"head_sha":    "head",
			"external_id": "smyklot:merge-after-ci:repository-20:head:g2",
			"status":      request.Status,
			"conclusion":  request.Conclusion,
			"app":         map[string]any{"id": int64(17)},
		})
	case "/repos/owner/repo/pulls/42":
		_, _ = w.Write([]byte(
			`{"state":"closed","head":{"sha":"head"},"base":{"ref":"main"}}`,
		))
	default:
		calls.t.Errorf("unexpected GitHub request %s %s", r.Method, r.URL.RequestURI())
		http.Error(w, "unexpected request", http.StatusNotFound)
	}
}

func TestPendingCICheckCreationIsSerializedPerHead(t *testing.T) {
	t.Parallel()

	calls := &pendingCICheckAPICalls{t: t}
	api := httptest.NewServer(calls)
	defer api.Close()

	store, err := open.Store(t.Context(), filepath.Join(t.TempDir(), "checks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	}()
	now := time.Date(2026, time.August, 18, 12, 0, 0, 0, time.UTC)
	err = store.ReconcileInstallation(t.Context(), storage.InstallationSnapshot{
		TargetID: "installation:77", InstallationID: "77",
		Kind: storage.TargetOrganization,
		Account: storage.Account{
			ID: "account:77", Provider: "github", SubjectID: "77",
			Login: "owner", UpdatedAt: now,
		},
		Repositories: []storage.RepositorySnapshot{{
			ID: "repository-20", Name: "repo", FullName: "owner/repo", DefaultBranch: "main",
		}},
		SyncedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}

	checks := &githubPendingCIChecks{
		store: store, tokens: pendingCICheckTokensStub{}, apiBaseURL: api.URL,
		now: func() time.Time { return now }, syncer: newPendingCICoordinator(),
	}
	target := storage.Target{ID: "installation:77", InstallationID: "77"}
	repository := storage.Repository{
		ID: "repository-20", FullName: "owner/repo",
	}

	const callers = 16
	start := make(chan struct{})
	errors := make(chan error, callers)
	var group sync.WaitGroup
	for range callers {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			slot, ensureErr := checks.EnsureBaseline(
				context.Background(), target, repository, 42, "head",
			)
			if ensureErr == nil && (slot.CheckRunID == nil || *slot.CheckRunID != 701) {
				ensureErr = fmt.Errorf("unexpected check slot: %#v", slot)
			}
			errors <- ensureErr
		}()
	}
	close(start)
	group.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Error(err)
		}
	}
	if got := calls.listCalls.Load(); got != 1 {
		t.Errorf("check-run list calls = %d, want 1", got)
	}
	if got := calls.createCalls.Load(); got != 1 {
		t.Errorf("check-run create calls = %d, want 1", got)
	}

	slot, err := store.GetCheckSlotByHead(t.Context(), repository.ID, "head")
	if err != nil {
		t.Fatal(err)
	}
	if slot.State != pendingci.CheckSlotReady || slot.AppliedDigest != slot.DesiredDigest {
		t.Fatalf("check slot was not durably applied: %#v", slot)
	}
	verifyCompletedBaselineRenewal(t, checks, calls, target, repository, now)
	verifyArmFailurePreservesPriorAuthorization(t, checks, calls, target, repository, store)
	verifyClosedPullRequestSlotReassignment(t, checks, repository, target)
	verifyRerequestRefreshesAppliedCheck(t, checks, calls, repository, store)
}

func verifyRerequestRefreshesAppliedCheck(
	t *testing.T,
	checks *githubPendingCIChecks,
	calls *pendingCICheckAPICalls,
	repository storage.Repository,
	store pendingci.CheckStore,
) {
	t.Helper()
	slot, err := store.GetCheckSlotByHead(t.Context(), repository.ID, "head")
	if err != nil {
		t.Fatal(err)
	}
	calls.patchMu.Lock()
	before := len(calls.patchStates)
	calls.patchMu.Unlock()
	refreshed, err := checks.RefreshRerequest(
		t.Context(), repository.ID, slot.HeadSHA, slot.AppID, *slot.CheckRunID,
		slot.Name, slot.ExternalID, true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !refreshed {
		t.Fatal("owned rerequested check was ignored")
	}
	calls.patchMu.Lock()
	defer calls.patchMu.Unlock()
	if len(calls.patchStates) != before+1 ||
		calls.patchStates[len(calls.patchStates)-1] != "completed:success" {
		t.Fatalf("rerequest repair states = %v", calls.patchStates)
	}
}

func verifyClosedPullRequestSlotReassignment(
	t *testing.T,
	checks *githubPendingCIChecks,
	repository storage.Repository,
	target storage.Target,
) {
	t.Helper()
	slot, err := checks.EnsureBaseline(
		t.Context(), target, repository, 43, "head",
	)
	if err != nil {
		t.Fatal(err)
	}
	if slot.PullRequest != 43 {
		t.Fatalf("reassigned check pull request = %d, want 43", slot.PullRequest)
	}
}

func verifyCompletedBaselineRenewal(
	t *testing.T,
	checks *githubPendingCIChecks,
	calls *pendingCICheckAPICalls,
	target storage.Target,
	repository storage.Repository,
	now time.Time,
) {
	t.Helper()
	checks.now = func() time.Time { return now.Add(pendingCICompletedRenewAfter) }
	renewed, err := checks.EnsureBaseline(
		t.Context(), target, repository, 42, "head",
	)
	if err != nil {
		t.Fatal(err)
	}
	if renewed.Generation != 2 || renewed.CheckRunID == nil || *renewed.CheckRunID != 702 {
		t.Fatalf("completed baseline was not renewed: %#v", renewed)
	}
	if got := calls.createCalls.Load(); got != 2 {
		t.Errorf("check-run create calls after renewal = %d, want 2", got)
	}
}

func verifyArmFailurePreservesPriorAuthorization(
	t *testing.T,
	checks *githubPendingCIChecks,
	calls *pendingCICheckAPICalls,
	target storage.Target,
	repository storage.Repository,
	store pendingci.CheckStore,
) {
	t.Helper()
	slot, err := store.GetCheckSlotByHead(t.Context(), repository.ID, "head")
	if err != nil {
		t.Fatal(err)
	}
	slotID := slot.ID
	command := &pendingCICommand{
		store: pendingCICommandStoreStub{request: pendingci.Request{
			RepositoryID: repository.ID, PullRequest: 42, HeadSHA: "head",
			MergeMethod: pendingci.MergeMethodSquash, Requester: "prior",
			ArtifactKind: pendingci.ArtifactCheck, CheckSlotID: &slotID,
		}},
		checks: checks, repositoryID: repository.ID,
	}
	err = restorePendingCICheckAfterArmFailure(
		t.Context(), command, target, repository,
		pendingCIActivationRequest{pullRequest: 42, headSHA: "head"}, slot,
	)
	if err != nil {
		t.Fatal(err)
	}
	calls.patchMu.Lock()
	defer calls.patchMu.Unlock()
	if len(calls.patchStates) != 1 || calls.patchStates[0] != "in_progress:" {
		t.Fatalf("check rollback states = %v, want prior authorization", calls.patchStates)
	}
}
