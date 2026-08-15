package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

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

func TestPendingCICleanupScopePreservesReplacementArtifacts(t *testing.T) {
	t.Parallel()
	request := pendingci.Request{
		RepositoryID: "9001", PullRequest: 198,
		Label: "smyklot:pending:ci:squash", SourceCommentID: 101,
	}
	tests := []struct {
		name           string
		current        pendingci.Request
		err            error
		removeLabel    bool
		removeReaction bool
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
			removeLabel: true, removeReaction: true,
		},
		{
			name: "replacement no longer armed", err: storage.ErrNotFound,
			removeLabel: true, removeReaction: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			backend := &githubPendingCIBackend{current: pendingCICurrentStoreStub{
				request: test.current, err: test.err,
			}}
			removeLabel, removeReaction, err := backend.cleanupScope(
				context.Background(), request,
			)
			if err != nil {
				t.Fatal(err)
			}
			if removeLabel != test.removeLabel || removeReaction != test.removeReaction {
				t.Fatalf(
					"cleanup scope = (%t, %t), want (%t, %t)",
					removeLabel, removeReaction, test.removeLabel, test.removeReaction,
				)
			}
		})
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

type pendingCICurrentStoreStub struct {
	request pendingci.Request
	err     error
}

func (store pendingCICurrentStoreStub) GetArmed(
	context.Context,
	string,
	int,
) (pendingci.Request, error) {
	return store.request, store.err
}
