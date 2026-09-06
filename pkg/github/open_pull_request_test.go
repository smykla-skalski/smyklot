package github_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

func TestOpenProposalLookupIncludesRetargetedPullRequests(t *testing.T) {
	for _, number := range []int{0, 42} {
		t.Run(fmt.Sprint(number), func(t *testing.T) {
			endpoint := httptest.NewServer(openProposalHandler(t, number))
			defer endpoint.Close()
			client, err := github.NewClient("test-token", endpoint.URL)
			if err != nil {
				t.Fatal(err)
			}
			pull, err := client.FindOpenPullRequestByHead(t.Context(), "acme", "web", "proposal")
			if err != nil || (pull == nil) != (number == 0) {
				t.Fatalf("open proposal = %+v (%v)", pull, err)
			}
			if pull != nil && (pull.Number != 42 || pull.URL != "https://github.com/acme/web/pull/42") {
				t.Fatalf("retargeted proposal lost its identity: %+v", pull)
			}
		})
	}
}

func openProposalHandler(t *testing.T, number int) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if r.Method != http.MethodGet || r.URL.Path != "/repos/acme/web/pulls" || query.Get("state") != "open" || query.Get("head") != "acme:proposal" || query.Has("base") || query.Get("per_page") != "1" {
			t.Errorf("proposal lookup could miss an open PR: %s %s", r.Method, r.URL)
		}
		if number == 0 {
			_, _ = fmt.Fprint(w, `[]`)
		} else {
			_, _ = fmt.Fprint(w, `[{"number":42,"state":"open","html_url":"https://github.com/acme/web/pull/42","base":{"ref":"release"}}]`)
		}
	})
}
