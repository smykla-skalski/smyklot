package apply

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

func TestClosedReusedBranchGetsNewProposalBranch(t *testing.T) {
	for _, body := range []string{proposalMarker + "smyklot/files-B -->", supersededMarker} {
		t.Run(body, func(t *testing.T) {
			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Query().Get("head") != "" {
					_, _ = w.Write([]byte(`[]`))
					return
				}
				_ = json.NewEncoder(w).Encode([]any{map[string]any{"number": 42, "state": "closed", "body": body, "user": map[string]any{"type": "Bot"}, "head": map[string]any{"ref": "smyklot/files-A", "repo": map[string]any{"full_name": "owner/repo"}}}})
			}))
			defer endpoint.Close()
			client, err := github.NewClient("token", endpoint.URL)
			if err != nil {
				t.Fatal(err)
			}
			plan := orgsync.FilePlan{Proposal: "smyklot/files-A", Actions: []orgsync.Action{{Payload: []byte(`{"path":"README.md","proposal":"smyklot/files-A"}`)}}}
			got, err := reuseFileProposal(t.Context(), client, syncTarget{Owner: "owner", Name: "repo", DefaultBranch: "main"}, "repo", orgsync.FileConfig{}, orgsync.FileOverride{}, config.DefaultFormattingPolicy(), plan)
			if err != nil || len(got.actions) != 1 {
				t.Fatalf("answer=%#v err=%v", got, err)
			}
			file, err := orgsync.DecodeFile(got.actions[0].Payload)
			if err != nil || file.Proposal != "smyklot/files-A-r42" || file.Fingerprint != "smyklot/files-A" {
				t.Fatalf("file=%#v err=%v", file, err)
			}
		})
	}
}

func TestMetadataRepairPrecedesDuplicateClosure(t *testing.T) {
	for _, failEdit := range []bool{false, true} {
		t.Run(map[bool]string{false: "repaired", true: "retry failed"}[failEdit], func(t *testing.T) { checkMetadataRepair(t, failEdit) })
	}
}

func checkMetadataRepair(t *testing.T, failEdit bool) {
	var requests []string
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /repos/owner/repo/git/ref/heads/smyklot/files-old":
			_, _ = w.Write([]byte(`{"object":{"sha":"tip"}}`))
		case "GET /repos/owner/repo/pulls":
			_ = json.NewEncoder(w).Encode(repairPulls(r.URL.Query().Get("head") != ""))
		case "PATCH /repos/owner/repo/pulls/43":
			var body struct {
				Body string `json:"body"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if !strings.Contains(body.Body, proposalMarker+"smyklot/files-new -->") {
				t.Errorf("body=%q", body.Body)
			}
			if failEdit {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{"message":"edit failed"}`))
				return
			}
			_, _ = w.Write([]byte(`{"number":43,"state":"open"}`))
		case "PATCH /repos/owner/repo/pulls/42":
			_, _ = w.Write([]byte(`{"number":42,"state":"closed"}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer endpoint.Close()
	client, err := github.NewClient("token", endpoint.URL)
	if err != nil {
		t.Fatal(err)
	}
	action := orgsync.Action{Operation: orgsync.OperationUpdate, Payload: []byte(`{"proposal":"smyklot/files-old","fingerprint":"smyklot/files-new","proposal_only":true,"consolidate":true}`)}
	_, err = applyFileActions(t.Context(), client, syncTarget{Owner: "owner", Name: "repo", DefaultBranch: "main"}, []orgsync.Action{action})
	closed := strings.Contains(strings.Join(requests, "\n"), "PATCH /repos/owner/repo/pulls/42")
	if failEdit && (err == nil || closed) {
		t.Fatalf("requests=%v err=%v", requests, err)
	}
	if !failEdit && (err != nil || !closed) {
		t.Fatalf("requests=%v err=%v", requests, err)
	}
}

func repairPulls(exact bool) []any {
	var pulls []any
	for _, number := range []int{43, 42} {
		if exact && number == 42 {
			continue
		}
		pulls = append(pulls, map[string]any{"number": number, "state": "open", "body": "Existing description\n" + proposalMarker + "old -->", "html_url": "https://github.com/owner/repo/pull/43", "user": map[string]any{"type": "Bot"}, "head": map[string]any{"ref": "smyklot/files-old", "repo": map[string]any{"full_name": "owner/repo"}}})
	}
	return pulls
}

func TestNewerAcceptanceSupersedesOldRefusal(t *testing.T) {
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("head") != "" {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		_ = json.NewEncoder(w).Encode([]any{
			map[string]any{"number": 10, "state": "closed", "body": proposalMarker + "smyklot/files-A -->", "user": map[string]any{"type": "Bot"}, "head": map[string]any{"ref": "smyklot/files-A", "repo": map[string]any{"full_name": "owner/repo"}}},
			map[string]any{"number": 11, "state": "closed", "merged_at": "2026-09-14T12:00:00Z", "body": proposalMarker + "smyklot/files-A -->", "user": map[string]any{"type": "Bot"}, "head": map[string]any{"ref": "smyklot/files-B", "repo": map[string]any{"full_name": "owner/repo"}}},
		})
	}))
	defer endpoint.Close()
	client, err := github.NewClient("token", endpoint.URL)
	if err != nil {
		t.Fatal(err)
	}
	plan := orgsync.FilePlan{Proposal: "smyklot/files-A", Actions: []orgsync.Action{{Payload: []byte(`{"path":"README.md","proposal":"smyklot/files-A"}`)}}}
	got, err := reuseFileProposal(t.Context(), client, syncTarget{Owner: "owner", Name: "repo", DefaultBranch: "main"}, "repo", orgsync.FileConfig{}, orgsync.FileOverride{}, config.DefaultFormattingPolicy(), plan)
	if err != nil || len(got.actions) != 1 {
		t.Fatalf("answer=%#v err=%v", got, err)
	}
	file, err := orgsync.DecodeFile(got.actions[0].Payload)
	if err != nil || file.Proposal != "smyklot/files-A-r10" {
		t.Fatalf("file=%#v err=%v", file, err)
	}
}
