package github_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

func TestProposalPrefixSearchPaginatesAndExcludesForeignBranches(t *testing.T) {
	var endpoint *httptest.Server
	endpoint = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("base") != "main" || r.URL.Query().Get("state") != "all" {
			t.Errorf("query=%s", r.URL.RawQuery)
		}
		pull := func(number int, owner, branch, kind string) map[string]any {
			return map[string]any{"number": number, "state": "open", "user": map[string]any{"type": kind}, "head": map[string]any{"ref": branch, "repo": map[string]any{"full_name": owner}}}
		}
		if r.URL.Query().Get("page") == "2" {
			_ = json.NewEncoder(w).Encode([]any{pull(5, "OWNER/repo", "smyklot/files-new", "Bot")})
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/repos/owner/repo/pulls?page=2>; rel="next"`, endpoint.URL))
		_ = json.NewEncoder(w).Encode([]any{pull(1, "owner/repo", "smyklot/files-old", "Bot"), pull(2, "fork/repo", "smyklot/files-old", "Bot"), pull(3, "owner/repo", "other", "Bot"), pull(4, "owner/repo", "smyklot/files-human", "User")})
	}))
	defer endpoint.Close()
	client, err := github.NewClient("token", endpoint.URL)
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.ListPullRequestsByHeadPrefix(t.Context(), "owner", "repo", "smyklot/files-", "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Number != 1 || got[1].Number != 5 {
		t.Fatalf("pulls=%#v", got)
	}
}
