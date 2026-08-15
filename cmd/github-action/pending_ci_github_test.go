package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

func TestPendingCIRequiredWaitRejectsUnprotectedBranch(t *testing.T) {
	t.Parallel()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repository/branches/main/protection/required_status_checks" {
			t.Fatalf("unexpected GitHub path %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Branch not protected"}`))
	}))
	defer api.Close()

	client, err := github.NewClient("installation-token", api.URL)
	if err != nil {
		t.Fatal(err)
	}
	backend := &githubPendingCIBackend{}
	_, err = backend.checks(
		context.Background(),
		client,
		pendingci.Request{RequiredChecksOnly: true},
		github.PullRequestState{BaseBranch: "main", HeadSHA: "head"},
		"owner",
		"repository",
	)
	if !errors.Is(err, errNoRequiredStatusChecks) {
		t.Fatalf("checks error = %v, want no-required-checks error", err)
	}
}

func TestActionRecognizesServicePendingCIOwnership(t *testing.T) {
	t.Parallel()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repository/pulls/42" {
			t.Fatalf("unexpected GitHub path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
			"number":42,
			"state":"open",
			"head":{"sha":"head"},
			"base":{"ref":"main"},
			"labels":[{"name":"smyklot:pending:ci:service"}]
		}`))
	}))
	defer api.Close()

	client, err := github.NewClient("installation-token", api.URL)
	if err != nil {
		t.Fatal(err)
	}
	owned, err := pendingCIServiceOwned(
		context.Background(), client, "owner", "repository", 42,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !owned {
		t.Fatal("service ownership marker was ignored")
	}
}

func TestPendingCICleanupScopePreservesReplacementArtifacts(t *testing.T) {
	t.Parallel()
	request := pendingci.Request{
		RepositoryID: "9001", PullRequest: 198,
		Label: "smyklot:pending:ci:squash", SourceCommentID: 101,
	}
	tests := []struct {
		name         string
		current      pendingci.Request
		err          error
		otherCleanup bool
		scope        pendingCICleanupScope
	}{
		{
			name: "same command source and label",
			current: pendingci.Request{
				Label: request.Label, SourceCommentID: request.SourceCommentID,
			},
		},
		{
			name: "different command source and label",
			current: pendingci.Request{
				Label: "smyklot:pending:ci:rebase", SourceCommentID: 202,
			},
			scope: pendingCICleanupScope{label: true, reaction: true},
		},
		{
			name: "replacement no longer armed", err: storage.ErrNotFound,
			scope: pendingCICleanupScope{
				label: true, reaction: true, serviceMarker: true,
			},
		},
		{
			name: "another terminal request retains ownership", err: storage.ErrNotFound,
			otherCleanup: true,
			scope:        pendingCICleanupScope{label: true, reaction: true},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			backend := &githubPendingCIBackend{current: pendingCICurrentStoreStub{
				request: test.current, err: test.err, otherCleanup: test.otherCleanup,
			}}
			scope, err := backend.cleanupScope(
				context.Background(), request,
			)
			if err != nil {
				t.Fatal(err)
			}
			if scope != test.scope {
				t.Fatalf(
					"cleanup scope = %+v, want %+v", scope, test.scope,
				)
			}
		})
	}
}

func TestPendingCIObserveRequiresCurrentSnapshotBeforeMutation(t *testing.T) {
	t.Parallel()
	request := pendingci.Request{
		ID: 41, Revision: 7, RepositoryID: "9001", PullRequest: 198,
	}
	backend := &githubPendingCIBackend{current: pendingCICurrentStoreStub{
		request: pendingci.Request{ID: 42, Revision: 1},
	}}
	if err := backend.requireCurrent(t.Context(), request); !errors.Is(err, storage.ErrConflict) {
		t.Fatalf("currentness error = %v, want conflict", err)
	}
	backend.current = pendingCICurrentStoreStub{request: request}
	if err := backend.requireCurrent(t.Context(), request); err != nil {
		t.Fatalf("matching snapshot rejected: %v", err)
	}
}

func TestPendingCICleanupIgnoresMissingGitHubArtifacts(t *testing.T) {
	t.Parallel()
	missing := &github.APIError{StatusCode: http.StatusNotFound}
	if err := cleanupGitHubError("remove label", missing); err != nil {
		t.Fatalf("missing artifact cleanup failed: %v", err)
	}
	unavailable := &github.APIError{StatusCode: http.StatusServiceUnavailable}
	if err := cleanupGitHubError("remove label", unavailable); err == nil {
		t.Fatal("transient GitHub failure was ignored")
	}
}

func TestPendingCICleanupAcquiresRepositoryOwnership(t *testing.T) {
	t.Parallel()
	coordinationErr := errors.New("coordination unavailable")
	request := reconcilerRequest(time.Now().UTC())
	request.RepositoryID = "9001"
	request.Lifecycle = pendingci.LifecycleCancelled
	request.CleanupPending = true
	reconciler := newPendingCIReconciler(
		&reconcilerTestStore{}, reconcilerTestObserver{}, &reconcilerTestEffects{},
		pendingCICoordinatorStub{err: coordinationErr}, defaultPendingCITiming(),
	)
	err := reconciler.Process(context.Background(), request)
	if !errors.Is(err, coordinationErr) {
		t.Fatalf("cleanup error = %v, want coordination failure", err)
	}
}

type pendingCICurrentStoreStub struct {
	request      pendingci.Request
	err          error
	otherCleanup bool
}

type pendingCICoordinatorStub struct {
	err error
}

func (stub pendingCICoordinatorStub) Exclusive(
	context.Context,
	string,
	func() error,
) error {
	return stub.err
}

func (store pendingCICurrentStoreStub) GetArmed(
	context.Context,
	string,
	int,
) (pendingci.Request, error) {
	return store.request, store.err
}

func (store pendingCICurrentStoreStub) HasPendingCleanup(
	context.Context,
	pendingci.CleanupFilter,
) (bool, error) {
	return store.otherCleanup, nil
}
