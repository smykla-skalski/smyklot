package gate

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

func TestPullRequestObservationProjectsDraftState(t *testing.T) {
	t.Parallel()
	observation := pullRequestObservation(
		github.PullRequestState{
			HeadSHA: "head", BaseBranch: "main", Open: true, Draft: true,
		},
		true,
		time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC),
	)
	if !observation.PullRequestDraft {
		t.Fatal("draft state was not projected into the pending CI observation")
	}
}

func TestPendingCICheckObservationAppliesFreshBranchScope(t *testing.T) {
	t.Parallel()
	store, target, repository, _ := gateTestStore(t)
	backend := &Backend{store: store, panelled: true}
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
	allowed, err := (&Gate{}).preparePendingCIReauthorization(
		t.Context(),
		reauthorizationCandidate{
			slot: pendingci.CheckSlot{PullRequest: 42},
			request: pendingci.Request{
				CandidateBaseBranch: "main",
			},
			client: client, owner: "owner", repository: "repository",
		},
		pendingci.Signal{Actor: "maintainer", HeadSHA: "new-head"},
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
		github.MergeMethodSquash, "main", "authorized-head", func() error { return nil },
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
		github.MergeMethodSquash, "main", "authorized-head", func() error { return nil },
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
		github.MergeMethodMerge, "main", "authorized-head", func() error { return nil },
	)
	if err == nil {
		t.Fatal("authorized merge method failure was ignored")
	}
	if mergeAttempts != 1 {
		t.Fatalf("check-mode merge attempts = %d, want exactly one", mergeAttempts)
	}
}

func TestPendingCICheckMergeReauthorizesAfterFinalStateReads(t *testing.T) {
	t.Parallel()
	steps := make([]string, 0, 3)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repository/pulls/42":
			steps = append(steps, "state")
			_, _ = w.Write([]byte(
				`{"state":"open","head":{"sha":"authorized-head"},"base":{"ref":"main"}}`,
			))
		case "/graphql":
			steps = append(steps, "queue")
			_, _ = w.Write([]byte(`{"data":{"repository":{"mergeQueue":null}}}`))
		default:
			t.Fatalf("merge continued after authorization changed: %s %s", r.Method, r.URL.RequestURI())
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
		func() error {
			steps = append(steps, "authorize")

			return pendingci.ErrStaleSourceRevision
		},
	)
	if !errors.Is(err, pendingci.ErrStaleSourceRevision) {
		t.Fatalf("merge error = %v, want stale source", err)
	}
	want := []string{"state", "queue", "authorize"}
	if !slices.Equal(steps, want) {
		t.Fatalf("final merge steps = %v, want %v", steps, want)
	}
}

func TestPendingCICheckMergeRejectsCurrentDraft(t *testing.T) {
	t.Parallel()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repository/pulls/42" {
			t.Fatalf("draft merge continued: %s %s", r.Method, r.URL.RequestURI())
		}
		_, _ = w.Write([]byte(
			`{"state":"open","draft":true,"head":{"sha":"authorized-head"},"base":{"ref":"main"}}`,
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
		func() error { return nil },
	)
	if err == nil {
		t.Fatal("check-mode merge accepted a draft pull request")
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
	backend := &Backend{}
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
	owned, err := bot.PendingCIServiceOwned(
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
	owned, err := bot.PendingCIServiceOwned(
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
		scope        cleanupScope
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
			scope: cleanupScope{label: true, sourceReaction: true},
		},
		{
			name: "replacement no longer armed", err: storage.ErrNotFound,
			scope: cleanupScope{
				label: true, sourceReaction: true, serviceFence: true,
			},
		},
		{
			name: "another terminal request retains ownership", err: storage.ErrNotFound,
			otherCleanup: true,
			scope: cleanupScope{
				label: true, sourceReaction: true, serviceFence: true,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			backend := &Backend{current: currentStoreStub{
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
	backend := &Backend{current: currentStoreStub{
		request: pendingci.Request{ID: 42, Revision: 1},
	}}
	if err := backend.requireCurrent(t.Context(), request); !errors.Is(err, storage.ErrConflict) {
		t.Fatalf("currentness error = %v, want conflict", err)
	}
	backend.current = currentStoreStub{request: request}
	if err := backend.requireCurrent(t.Context(), request); err != nil {
		t.Fatalf("matching snapshot rejected: %v", err)
	}
}

func TestPendingCICleanupIgnoresMissingGitHubArtifacts(t *testing.T) {
	t.Parallel()
	missing := &github.APIError{StatusCode: http.StatusNotFound}
	if err := bot.CleanupGitHubError("remove label", missing); err != nil {
		t.Fatalf("missing artifact cleanup failed: %v", err)
	}
	unavailable := &github.APIError{StatusCode: http.StatusServiceUnavailable}
	if err := bot.CleanupGitHubError("remove label", unavailable); err == nil {
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
	reconciler := newReconciler(
		&reconcilerTestStore{}, reconcilerTestObserver{}, &reconcilerTestEffects{},
		coordinatorStub{err: coordinationErr}, defaultTiming(),
	)
	err := reconciler.Process(context.Background(), request)
	if !errors.Is(err, coordinationErr) {
		t.Fatalf("cleanup error = %v, want coordination failure", err)
	}
}

type currentStoreStub struct {
	request      pendingci.Request
	err          error
	otherCleanup bool
}

type coordinatorStub struct {
	err error
}

func (stub coordinatorStub) Exclusive(
	context.Context,
	string,
	func() error,
) error {
	return stub.err
}

func (store currentStoreStub) GetArmed(
	context.Context,
	string,
	int,
) (pendingci.Request, error) {
	return store.request, store.err
}

func (store currentStoreStub) HasPendingCleanup(
	context.Context,
	pendingci.CleanupFilter,
) (bool, error) {
	return store.otherCleanup, nil
}
