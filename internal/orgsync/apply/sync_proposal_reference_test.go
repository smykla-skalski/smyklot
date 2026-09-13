package apply

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

func TestObservedProposalRetainsGitHubDestination(t *testing.T) {
	for _, test := range []struct {
		name, state string
		merged      bool
		want        orgsync.Observation
	}{
		{"open", "open", false, orgsync.ObservationProposed},
		{"declined", "closed", false, orgsync.ObservationDeclined},
		{"merged", "closed", true, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/repos/owner/repo/pulls" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode([]any{map[string]any{"number": 42, "state": test.state, "merged": test.merged, "html_url": "https://github.com/owner/repo/pull/42"}})
			}))
			defer endpoint.Close()
			client, err := github.NewClient("test-token", endpoint.URL)
			if err != nil {
				t.Fatal(err)
			}
			got, err := proposalObservation(t.Context(), client, syncTarget{Owner: "owner", Name: "repo", DefaultBranch: "main"}, "proposal")
			if err != nil {
				t.Fatal(err)
			}
			if got.state != test.want {
				t.Fatalf("observation = %#v", got)
			}
			expectedURL := map[bool]string{false: "https://github.com/owner/repo/pull/42", true: ""}[test.merged]
			if got.proposalURL != expectedURL {
				t.Fatalf("proposal destination = %q, want %q", got.proposalURL, expectedURL)
			}
		})
	}
}

func TestOpeningOrAdoptingProposalRetainsDestination(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "created", true: "adopted"}[existing], func(t *testing.T) {
			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				wantMethod, wantPath := http.MethodPost, "/repos/owner/repo/pulls"
				if existing {
					wantMethod, wantPath = http.MethodPatch, "/repos/owner/repo/pulls/42"
				}
				if r.Method != wantMethod || r.URL.Path != wantPath {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"number":42,"state":"open","html_url":"https://github.com/owner/repo/pull/42"}`))
			}))
			defer endpoint.Close()
			client, err := github.NewClient("test-token", endpoint.URL)
			if err != nil {
				t.Fatal(err)
			}
			var previous *github.PullRequest
			if existing {
				previous = &github.PullRequest{Number: 42, State: "open", URL: "https://github.com/owner/repo/pull/42"}
			}
			pull, err := openOrUpdateProposal(t.Context(), client, syncTarget{Owner: "owner", Name: "repo", DefaultBranch: "main"}, "proposal", previous, nil)
			if err != nil || pull.URL != "https://github.com/owner/repo/pull/42" {
				t.Fatalf("pull = %#v, %v", pull, err)
			}
		})
	}
}

func TestAppliedFileProposalReachesObservationAndActionHistory(t *testing.T) {
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		replies := map[string]string{
			"GET /repos/owner/repo/git/ref/heads/proposal": `{"object":{"sha":"commit"}}`,
			"GET /repos/owner/repo/pulls":                  `[{"number":42,"state":"open","html_url":"https://github.com/owner/repo/pull/42"}]`,
			"GET /repos/owner/repo/git/commits/commit":     `{"sha":"commit","tree":{"sha":"tree"}}`,
			"GET /repos/owner/repo/git/trees/tree":         `{"sha":"tree","tree":[],"truncated":false}`,
			"PATCH /repos/owner/repo/pulls/42":             `{"number":42,"state":"open"}`,
		}
		reply, ok := replies[r.Method+" "+r.URL.Path]
		if !ok {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(reply))
	}))
	defer endpoint.Close()
	client, err := github.NewClient("test-token", endpoint.URL)
	if err != nil {
		t.Fatal(err)
	}
	store := &syncExecutionStore{}
	engine := New(store, nil, "")
	action := orgsync.Action{ID: 1, RepositoryID: "repo", Kind: orgsync.KindFiles, Operation: orgsync.OperationDelete, State: orgsync.ActionPending, InputDigest: "original", Payload: []byte(`{"path":"retired.json","proposal":"proposal"}`)}
	work := orgsync.RepositoryWork{Kinds: []orgsync.KindWork{{Kind: orgsync.KindFiles, Actions: []orgsync.Action{action}}}}
	var outcome orgsync.Outcome
	engine.applyRepositoryWork(t.Context(), client, storage.Repository{ID: "repo", FullName: "owner/repo", DefaultBranch: "main"}, work, &outcome)
	if len(outcome.Applied) != 1 || len(store.actionNotes) != 1 {
		t.Fatalf("outcome=%#v notes=%#v", outcome, store.actionNotes)
	}
	if outcome.Applied[0].ProposalURL != "https://github.com/owner/repo/pull/42" || store.actionNotes[0].ProposalURL != outcome.Applied[0].ProposalURL {
		t.Fatalf("lost proposal: %#v, %#v", outcome.Applied, store.actionNotes)
	}
	if outcome.Applied[0].Observation != orgsync.ObservationProposed {
		t.Fatalf("proposal became agreement: %#v", outcome.Applied)
	}
}
