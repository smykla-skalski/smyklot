package apply

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

func TestFileUpdateExtendsExistingBranchAndPatchesSamePR(t *testing.T) {
	requests := map[string]json.RawMessage{}
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		if r.Method != http.MethodGet {
			var body json.RawMessage
			_ = json.NewDecoder(r.Body).Decode(&body)
			requests[key] = body
		}
		w.Header().Set("Content-Type", "application/json")
		replies := map[string]string{
			"GET /repos/owner/repo/git/ref/heads/smyklot/files-old":    `{"object":{"sha":"reviewer-tip"}}`,
			"GET /repos/owner/repo/pulls":                              `[{"number":43,"state":"open","html_url":"https://github.com/owner/repo/pull/43"}]`,
			"GET /repos/owner/repo/git/commits/reviewer-tip":           `{"tree":{"sha":"reviewer-tree"}}`,
			"GET /repos/owner/repo/git/trees/reviewer-tree":            `{"sha":"reviewer-tree","tree":[]}`,
			"POST /repos/owner/repo/git/blobs":                         `{"sha":"new-blob"}`,
			"POST /repos/owner/repo/git/trees":                         `{"sha":"new-tree"}`,
			"POST /repos/owner/repo/git/commits":                       `{"sha":"new-tip"}`,
			"PATCH /repos/owner/repo/git/refs/heads/smyklot/files-old": `{"object":{"sha":"new-tip"}}`,
			"PATCH /repos/owner/repo/pulls/43":                         `{"number":43,"state":"open"}`,
		}
		reply, ok := replies[key]
		if !ok {
			t.Errorf("unexpected request %s", key)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(reply))
	}))
	defer endpoint.Close()
	client, err := github.NewClient("token", endpoint.URL)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(orgsync.ResolvedFile{Path: "README.md", Content: []byte("updated\n"), Proposal: "smyklot/files-old", Fingerprint: "smyklot/files-new"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := applyFileActions(t.Context(), client, syncTarget{Owner: "owner", Name: "repo", DefaultBranch: "main"}, []orgsync.Action{{Operation: orgsync.OperationUpdate, Payload: payload}})
	if err != nil || got.proposalURL != "https://github.com/owner/repo/pull/43" {
		t.Fatalf("got=%#v err=%v", got, err)
	}
	var commit struct {
		Parents []string `json:"parents"`
	}
	_ = json.Unmarshal(requests["POST /repos/owner/repo/git/commits"], &commit)
	if len(commit.Parents) != 1 || commit.Parents[0] != "reviewer-tip" {
		t.Fatalf("parents=%v", commit.Parents)
	}
	var tree struct {
		Base string `json:"base_tree"`
	}
	_ = json.Unmarshal(requests["POST /repos/owner/repo/git/trees"], &tree)
	if tree.Base != "reviewer-tree" {
		t.Fatalf("base=%q", tree.Base)
	}
	var ref struct {
		Force *bool `json:"force"`
	}
	_ = json.Unmarshal(requests["PATCH /repos/owner/repo/git/refs/heads/smyklot/files-old"], &ref)
	if ref.Force == nil || *ref.Force {
		t.Fatalf("force=%v", ref.Force)
	}
	var pull struct {
		Body string `json:"body"`
	}
	_ = json.Unmarshal(requests["PATCH /repos/owner/repo/pulls/43"], &pull)
	if !strings.Contains(pull.Body, proposalMarker+"smyklot/files-new -->") {
		t.Fatalf("body=%q", pull.Body)
	}
}
