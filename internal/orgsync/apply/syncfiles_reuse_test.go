package apply

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

type reuseCase struct {
	name, base, branch, desired string
	wantActions                 int
	duplicates                  bool
}

func TestReusesOpenFileProposalAcrossConfigurationChanges(t *testing.T) {
	for _, test := range []reuseCase{
		{"changed configuration", "base\n", "old proposal\n", "new proposal\n", 1, false},
		{"reverted configuration", "base\n", "old proposal\n", "base\n", 1, false},
		{"already updated", "base\n", "new proposal\n", "new proposal\n", 0, false},
		{"duplicates already updated", "base\n", "new proposal\n", "new proposal\n", 1, true},
		{"duplicates need update", "base\n", "old proposal\n", "new proposal\n", 1, true},
		{"metadata retry", "base\n", "new proposal\n", "new proposal\n", 1, false},
		{"metadata retry original branch", "base\n", "new proposal\n", "new proposal\n", 1, false},
	} {
		t.Run(test.name, func(t *testing.T) { checkReuseCase(t, test) })
	}
}

func checkReuseCase(t *testing.T, test reuseCase) {
	branch := "smyklot/files-old"
	var fingerprint string
	endpoint := httptest.NewServer(reuseHandler(t, test, &branch, &fingerprint))
	defer endpoint.Close()
	client, err := github.NewClient("test-token", endpoint.URL)
	if err != nil {
		t.Fatal(err)
	}
	files := orgsync.FileConfig{Files: []orgsync.File{{Path: "README.md", Content: test.desired}}}
	policy := config.DefaultFormattingPolicy()
	plan, err := orgsync.PlanFiles("repo", files, orgsync.FileOverride{}, "main", map[string]orgsync.CurrentFile{"README.md": {Blob: orgsync.BlobID([]byte(test.base)), Size: len(test.base)}}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if test.name == "metadata retry original branch" {
		branch = plan.Proposal
	}
	fingerprint = plan.Proposal
	if test.name == "metadata retry" || test.name == "metadata retry original branch" {
		fingerprint = "stale"
	}
	got, err := reuseFileProposal(t.Context(), client, syncTarget{Owner: "owner", Name: "repo", DefaultBranch: "main"}, "repo", files, orgsync.FileOverride{}, policy, plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.actions) != test.wantActions {
		t.Fatalf("answer=%#v", got)
	}
	if test.wantActions == 0 {
		if got.observation != orgsync.ObservationProposed || got.proposalURL != "https://github.com/owner/repo/pull/42" {
			t.Fatalf("answer=%#v", got)
		}
		return
	}
	payload, err := orgsync.DecodeFile(got.actions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Consolidate != test.duplicates {
		t.Fatalf("payload=%#v", payload)
	}
	if test.branch == test.desired {
		if !payload.ProposalOnly || payload.Proposal != branch {
			t.Fatalf("payload=%#v", payload)
		}
		return
	}
	if payload.Proposal != branch || payload.Fingerprint != plan.Proposal || string(payload.Content) != test.desired {
		t.Fatalf("payload=%#v", payload)
	}
}

func TestReusedProposalClosureDeclinesLatestConfiguration(t *testing.T) {
	for _, merged := range []bool{false, true} {
		t.Run(map[bool]string{false: "declined", true: "merged"}[merged], func(t *testing.T) {
			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Query().Get("head") != "" {
					_, _ = w.Write([]byte(`[]`))
					return
				}
				_ = json.NewEncoder(w).Encode([]any{map[string]any{"number": 42, "state": "closed", "merged": merged, "body": proposalMarker + "smyklot/files-new -->", "html_url": "https://github.com/owner/repo/pull/42", "user": map[string]any{"type": "Bot"}, "head": map[string]any{"ref": "smyklot/files-old", "repo": map[string]any{"full_name": "owner/repo"}}}})
			}))
			defer endpoint.Close()
			client, err := github.NewClient("test-token", endpoint.URL)
			if err != nil {
				t.Fatal(err)
			}
			plan := orgsync.FilePlan{Proposal: "smyklot/files-new", Actions: []orgsync.Action{{}}}
			got, err := reuseFileProposal(t.Context(), client, syncTarget{Owner: "owner", Name: "repo", DefaultBranch: "main"}, "repo", orgsync.FileConfig{}, orgsync.FileOverride{}, config.DefaultFormattingPolicy(), plan)
			if err != nil {
				t.Fatal(err)
			}
			if !merged && (got.observation != orgsync.ObservationDeclined || len(got.actions) != 0) {
				t.Fatalf("answer=%#v", got)
			}
			if merged && len(got.actions) != 1 {
				t.Fatalf("answer=%#v", got)
			}
		})
	}
}

func TestDuplicateProposalCleanupKeepsNewestAndPreservesBranches(t *testing.T) {
	for _, newestOpen := range []bool{true, false} {
		t.Run(map[bool]string{true: "newest open", false: "newest closed concurrently"}[newestOpen], func(t *testing.T) { checkDuplicateCleanup(t, newestOpen) })
	}
}

func checkDuplicateCleanup(t *testing.T, newestOpen bool) {
	var closed []int
	endpoint := httptest.NewServer(cleanupHandler(t, newestOpen, &closed))
	defer endpoint.Close()
	client, err := github.NewClient("token", endpoint.URL)
	if err != nil {
		t.Fatal(err)
	}
	err = closeOlderFileProposals(t.Context(), client, syncTarget{Owner: "owner", Name: "repo", DefaultBranch: "main"}, github.PullRequest{Number: 43})
	if newestOpen && (err != nil || len(closed) != 1) {
		t.Fatalf("closed=%v err=%v", closed, err)
	}
	if !newestOpen && (err == nil || len(closed) != 0) {
		t.Fatalf("closed=%v err=%v", closed, err)
	}
}

func reuseHandler(t *testing.T, test reuseCase, branch *string, fingerprint *string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /repos/owner/repo/pulls":
			pulls := []any{map[string]any{"number": 42, "state": "open", "body": proposalMarker + *fingerprint + " -->", "html_url": "https://github.com/owner/repo/pull/42", "user": map[string]any{"type": "Bot"}, "head": map[string]any{"ref": *branch, "repo": map[string]any{"full_name": "owner/repo"}}}}
			if test.duplicates {
				pulls = append(pulls, map[string]any{"number": 41, "state": "open", "user": map[string]any{"type": "Bot"}, "head": map[string]any{"ref": "smyklot/files-older", "repo": map[string]any{"full_name": "owner/repo"}}})
			}
			_ = json.NewEncoder(w).Encode(pulls)
		case "GET /repos/owner/repo/git/trees/" + *branch:
			_ = json.NewEncoder(w).Encode(map[string]any{"sha": "tree", "tree": []any{map[string]any{"path": "README.md", "type": "blob", "mode": "100644", "sha": orgsync.BlobID([]byte(test.branch)), "size": len(test.branch)}}})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func cleanupHandler(t *testing.T, newestOpen bool, closed *[]int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls" {
			var pulls []any
			for _, number := range []int{42, 43} {
				state := "open"
				if number == 43 && !newestOpen {
					state = "closed"
				}
				pulls = append(pulls, map[string]any{"number": number, "state": state, "user": map[string]any{"type": "Bot"}, "head": map[string]any{"ref": "smyklot/files-old", "repo": map[string]any{"full_name": "owner/repo"}}})
			}
			_ = json.NewEncoder(w).Encode(pulls)
			return
		}
		if r.Method == http.MethodPatch && r.URL.Path == "/repos/owner/repo/pulls/42" {
			var body struct {
				State string `json:"state"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.State != "closed" {
				t.Errorf("state=%q", body.State)
			}
			*closed = append(*closed, 42)
			_, _ = w.Write([]byte(`{"number":42,"state":"closed"}`))
			return
		}
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
}
