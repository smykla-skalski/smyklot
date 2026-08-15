package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
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
