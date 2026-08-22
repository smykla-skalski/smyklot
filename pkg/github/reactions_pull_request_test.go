package github_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

func TestPullRequestServiceReactionLifecycle(t *testing.T) {
	t.Parallel()
	state := &pullRequestReactionState{t: t}
	server := httptest.NewServer(state)
	defer server.Close()

	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.AddPullRequestReaction(
		t.Context(), "owner", "repo", 42, github.ReactionHooray,
	); err != nil {
		t.Fatal(err)
	}
	if state.added != github.ReactionHooray {
		t.Fatalf("added reaction = %q", state.added)
	}
	found, err := client.HasPullRequestReaction(
		t.Context(), "owner", "repo", 42, "smyklot[bot]",
		github.ReactionHooray,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("service reaction was not found on the pull request")
	}
	if err := client.RemovePullRequestReactionByUser(
		t.Context(), "owner", "repo", 42, "smyklot[bot]",
		github.ReactionHooray,
	); err != nil {
		t.Fatal(err)
	}
	if state.deleted != "/repos/owner/repo/issues/42/reactions/501" {
		t.Fatalf("deleted reaction path = %q", state.deleted)
	}
}

func TestPullRequestServiceReactionRemovalRestartsAfterPageShift(t *testing.T) {
	t.Parallel()
	state := &shiftingPullRequestReactionState{}
	server := httptest.NewServer(state)
	defer server.Close()

	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.RemovePullRequestReactionByUser(
		t.Context(), "owner", "repo", 42, "smyklot[bot]",
		github.ReactionHooray,
	); err != nil {
		t.Fatal(err)
	}
	if state.firstPageReads != 2 {
		t.Fatalf("first page reads = %d, want 2", state.firstPageReads)
	}
	if state.deleted != "/repos/owner/repo/issues/42/reactions/501" {
		t.Fatalf("deleted reaction path = %q", state.deleted)
	}
}

type shiftingPullRequestReactionState struct {
	firstPageReads int
	deleted        string
}

func (state *shiftingPullRequestReactionState) ServeHTTP(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method == http.MethodDelete {
		state.deleted = r.URL.Path
		w.WriteHeader(http.StatusNoContent)

		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page == 1 {
		state.firstPageReads++
		writeShiftingPullRequestReactionPage(w, state.firstPageReads)

		return
	}
	_ = json.NewEncoder(w).Encode([]map[string]any{})
}

func writeShiftingPullRequestReactionPage(w http.ResponseWriter, scan int) {
	count := 100
	if scan > 1 {
		count = 99
	}
	reactions := make([]map[string]any, 0, count+1)
	for index := range count {
		reactions = append(reactions, map[string]any{
			"id": index + 1, "content": "eyes",
			"user": map[string]any{"login": fmt.Sprintf("reviewer-%d", index)},
		})
	}
	if scan > 1 {
		reactions = append(reactions, map[string]any{
			"id": 501, "content": "hooray",
			"user": map[string]any{"login": "smyklot[bot]"},
		})
	}
	_ = json.NewEncoder(w).Encode(reactions)
}

type pullRequestReactionState struct {
	t       *testing.T
	added   github.ReactionType
	deleted string
}

func (state *pullRequestReactionState) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/repos/owner/repo/issues/42/reactions" &&
		r.URL.Path != "/repos/owner/repo/issues/42/reactions/501" {
		http.Error(w, "unexpected path", http.StatusNotFound)

		return
	}
	switch r.Method {
	case http.MethodGet:
		writePullRequestReactionPage(w, r)
	case http.MethodPost:
		state.recordAddedReaction(w, r)
	case http.MethodDelete:
		state.deleted = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func writePullRequestReactionPage(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page != 1 {
		_ = json.NewEncoder(w).Encode([]map[string]any{{
			"id": 501, "content": "hooray",
			"user": map[string]any{"login": "smyklot[bot]"},
		}})

		return
	}
	reactions := make([]map[string]any, 100)
	for index := range reactions {
		reactions[index] = map[string]any{
			"id": index + 1, "content": "eyes",
			"user": map[string]any{"login": fmt.Sprintf("reviewer-%d", index)},
		}
	}
	_ = json.NewEncoder(w).Encode(reactions)
}

func (state *pullRequestReactionState) recordAddedReaction(
	w http.ResponseWriter,
	r *http.Request,
) {
	var payload struct {
		Content github.ReactionType `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		state.t.Error(err)
	}
	state.added = payload.Content
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"id":501}`))
}

func TestRemovePullRequestCommentReactionsByUserRemovesEveryMatch(t *testing.T) {
	t.Parallel()
	var deleted []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/owner/repo/issues/42/comments":
			_ = json.NewEncoder(w).Encode([]map[string]int{{"id": 101}, {"id": 102}})
		case r.Method == http.MethodGet &&
			r.URL.Path == "/repos/owner/repo/issues/comments/101/reactions":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": 11, "content": "hooray", "user": map[string]any{"login": "smyklot[bot]"}},
				{"id": 12, "content": "hooray", "user": map[string]any{"login": "reviewer"}},
			})
		case r.Method == http.MethodGet &&
			r.URL.Path == "/repos/owner/repo/issues/comments/102/reactions":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": 21, "content": "hooray", "user": map[string]any{"login": "smyklot[bot]"}},
			})
		case r.Method == http.MethodDelete:
			deleted = append(deleted, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.RemovePullRequestCommentReactionsByUser(
		t.Context(), "owner", "repo", 42, "smyklot[bot]",
		github.ReactionHooray,
	); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/repos/owner/repo/issues/comments/101/reactions/11",
		"/repos/owner/repo/issues/comments/102/reactions/21",
	}
	if len(deleted) != len(want) {
		t.Fatalf("deleted reactions = %v, want %v", deleted, want)
	}
	for index := range want {
		if deleted[index] != want[index] {
			t.Fatalf("deleted reactions = %v, want %v", deleted, want)
		}
	}
}
