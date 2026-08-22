package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	adminpanel "github.com/smykla-skalski/smyklot/internal/panel"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

func TestPendingCICheckObservationAppliesFreshBranchScope(t *testing.T) {
	t.Parallel()
	store, target, repository, _ := pendingCIGateTestStore(t)
	backend := &githubPendingCIBackend{server: &server{
		store: store, panel: &adminpanel.Server{},
	}}
	request := pendingci.Request{TargetID: target.ID, RepositoryID: repository.ID}
	included, err := backend.checkBranchIncluded(t.Context(), request, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !included {
		t.Fatal("default branch was excluded from pending CI checks")
	}
	included, err = backend.checkBranchIncluded(t.Context(), request, "release")
	if err != nil {
		t.Fatal(err)
	}
	if included {
		t.Fatal("out-of-scope branch remained eligible for pending CI checks")
	}
}

func TestPendingCIReauthorizationRejectsANewMergeQueue(t *testing.T) {
	t.Parallel()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repository/contents/.github/CODEOWNERS":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Not Found"}`))
		case "/repos/owner/repository/collaborators/maintainer/permission":
			_, _ = w.Write([]byte(`{"permission":"write"}`))
		case "/repos/owner/repository/pulls/42":
			_, _ = w.Write([]byte(
				`{"state":"open","head":{"sha":"new-head"},"base":{"ref":"main"}}`,
			))
		case "/graphql":
			var request struct {
				Variables map[string]string `json:"variables"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if request.Variables["branch"] != "main" {
				t.Fatalf("merge queue branch = %q, want main", request.Variables["branch"])
			}
			_, _ = w.Write([]byte(
				`{"data":{"repository":{"mergeQueue":{"id":"MQ_main"}}}}`,
			))
		default:
			t.Fatalf("unexpected GitHub request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer api.Close()

	client, err := github.NewClient("installation-token", api.URL)
	if err != nil {
		t.Fatal(err)
	}
	allowed, err := (&server{}).preparePendingCIReauthorization(
		t.Context(),
		pendingCIReauthorizationCandidate{
			slot: pendingci.CheckSlot{PullRequest: 42},
			request: pendingci.Request{
				CandidateBaseBranch: "main",
			},
			client: client, owner: "owner", repository: "repository",
		},
		webhook.PendingCISignal{Actor: "maintainer", HeadSHA: "new-head"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("reauthorization accepted after a merge queue was enabled")
	}
}

func TestPendingCIMergeRejectsANewMergeQueue(t *testing.T) {
	t.Parallel()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repository/pulls/42":
			_, _ = w.Write([]byte(
				`{"state":"open","head":{"sha":"authorized-head"},"base":{"ref":"main"}}`,
			))
		case "/graphql":
			_, _ = w.Write([]byte(
				`{"data":{"repository":{"mergeQueue":{"id":"MQ_main"}}}}`,
			))
		default:
			t.Fatalf("merge attempted after queue activation: %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer api.Close()

	client, err := github.NewClient("installation-token", api.URL)
	if err != nil {
		t.Fatal(err)
	}
	err = mergePendingPRAtHeadWithoutQueue(
		t.Context(), client, "owner", "repository", 42,
		github.MergeMethodSquash, "main", "authorized-head",
	)
	if err == nil {
		t.Fatal("merge accepted after a merge queue was enabled")
	}
}

func TestPendingCIMergeRejectsARetargetAfterCheckSuccess(t *testing.T) {
	t.Parallel()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repository/pulls/42" {
			t.Fatalf("merge continued after retarget: %s %s", r.Method, r.URL.RequestURI())
		}
		_, _ = w.Write([]byte(
			`{"state":"open","head":{"sha":"authorized-head"},"base":{"ref":"release"}}`,
		))
	}))
	defer api.Close()

	client, err := github.NewClient("installation-token", api.URL)
	if err != nil {
		t.Fatal(err)
	}
	err = mergePendingPRAtHeadWithoutQueue(
		t.Context(), client, "owner", "repository", 42,
		github.MergeMethodSquash, "main", "authorized-head",
	)
	if err == nil {
		t.Fatal("merge accepted after the base changed following check success")
	}
}

func TestPendingCICheckMergeDoesNotFallbackAfterAuthorizedMethodFails(t *testing.T) {
	t.Parallel()
	mergeAttempts := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repository/pulls/42":
			_, _ = w.Write([]byte(
				`{"state":"open","head":{"sha":"authorized-head"},"base":{"ref":"main"}}`,
			))
		case "/graphql":
			_, _ = w.Write([]byte(`{"data":{"repository":{"mergeQueue":null}}}`))
		case "/repos/owner/repository/pulls/42/merge":
			mergeAttempts++
			w.WriteHeader(http.StatusMethodNotAllowed)
			_, _ = w.Write([]byte(`{"message":"Merge commits are not allowed"}`))
		default:
			t.Fatalf("unexpected merge request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer api.Close()

	client, err := github.NewClient("installation-token", api.URL)
	if err != nil {
		t.Fatal(err)
	}
	err = mergePendingPRAtHeadWithoutQueue(
		t.Context(), client, "owner", "repository", 42,
		github.MergeMethodMerge, "main", "authorized-head",
	)
	if err == nil {
		t.Fatal("authorized merge method failure was ignored")
	}
	if mergeAttempts != 1 {
		t.Fatalf("check-mode merge attempts = %d, want exactly one", mergeAttempts)
	}
}

func TestPendingCICheckSuccessRejectsARetargetedRevision(t *testing.T) {
	t.Parallel()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repository/pulls/42" {
			t.Fatalf("unexpected preflight request %s %s", r.Method, r.URL.RequestURI())
		}
		_, _ = w.Write([]byte(
			`{"state":"open","head":{"sha":"authorized-head"},"base":{"ref":"queued"}}`,
		))
	}))
	defer api.Close()

	client, err := github.NewClient("installation-token", api.URL)
	if err != nil {
		t.Fatal(err)
	}
	err = preflightPendingCICheckMerge(
		t.Context(), client, "owner", "repository", pendingci.Request{
			PullRequest: 42, HeadSHA: "authorized-head", BaseBranch: "main",
		},
	)
	if err == nil {
		t.Fatal("required check was allowed to succeed after the base branch changed")
	}
}

func TestPendingCICheckSuccessRejectsANewMergeQueue(t *testing.T) {
	t.Parallel()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repository/pulls/42":
			_, _ = w.Write([]byte(
				`{"state":"open","head":{"sha":"authorized-head"},"base":{"ref":"main"}}`,
			))
		case "/graphql":
			_, _ = w.Write([]byte(
				`{"data":{"repository":{"mergeQueue":{"id":"MQ_main"}}}}`,
			))
		default:
			t.Fatalf("unexpected preflight request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer api.Close()

	client, err := github.NewClient("installation-token", api.URL)
	if err != nil {
		t.Fatal(err)
	}
	err = preflightPendingCICheckMerge(
		t.Context(), client, "owner", "repository", pendingci.Request{
			PullRequest: 42, HeadSHA: "authorized-head", BaseBranch: "main",
		},
	)
	if err == nil {
		t.Fatal("required check was allowed to succeed after a merge queue was enabled")
	}
}

func TestPendingCIRequiredWaitRejectsUnprotectedBranch(t *testing.T) {
	t.Parallel()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repository/branches/main/protection/required_status_checks",
			"/repos/owner/repository/rules/branches/main":
		default:
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
		switch r.URL.Path {
		case "/repos/owner/repository/pulls/42":
			_, _ = w.Write([]byte(
				`{"state":"open","head":{"sha":"head"},"base":{"ref":"main"},"labels":[]}`,
			))
		case "/repos/owner/repository/issues/42/reactions":
			_, _ = w.Write([]byte(`[{"content":"hooray","user":{"login":"smyklot[bot]"}}]`))
		default:
			t.Fatalf("unexpected GitHub path %q", r.URL.Path)
		}
	}))
	defer api.Close()

	client, err := github.NewClient("installation-token", api.URL)
	if err != nil {
		t.Fatal(err)
	}
	owned, err := pendingCIServiceOwned(
		context.Background(), client, "owner", "repository", 42, "smyklot[bot]",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !owned {
		t.Fatal("service ownership reaction was ignored")
	}
}

func TestActionRecognizesLegacyPendingCIServiceLabel(t *testing.T) {
	t.Parallel()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repository/pulls/42" {
			t.Fatalf("unexpected GitHub path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(
			`{"state":"open","head":{"sha":"head"},"base":{"ref":"main"},` +
				`"labels":[{"name":"smyklot:pending:ci:service"}]}`,
		))
	}))
	defer api.Close()

	client, err := github.NewClient("installation-token", api.URL)
	if err != nil {
		t.Fatal(err)
	}
	owned, err := pendingCIServiceOwned(
		t.Context(), client, "owner", "repository", 42, "smyklot[bot]",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !owned {
		t.Fatal("legacy service ownership label was ignored during migration")
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
			scope: pendingCICleanupScope{label: true, sourceReaction: true},
		},
		{
			name: "replacement no longer armed", err: storage.ErrNotFound,
			scope: pendingCICleanupScope{
				label: true, sourceReaction: true, serviceFence: true,
			},
		},
		{
			name: "another terminal request retains ownership", err: storage.ErrNotFound,
			otherCleanup: true,
			scope: pendingCICleanupScope{
				label: true, sourceReaction: true, serviceFence: true,
			},
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
