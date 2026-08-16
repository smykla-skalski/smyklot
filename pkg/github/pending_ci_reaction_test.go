package github_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

func TestHasPullRequestCommentReactionFindsBotReactionOnLaterPages(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(paginatedPendingCIReactionHandler))
	defer server.Close()

	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	found, err := client.HasPullRequestCommentReaction(
		t.Context(), "owner", "repo", 42, "smyklot[bot]",
		github.ReactionPendingCIService,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("bot eyes reaction was not found on later comment and reaction pages")
	}
}

func paginatedPendingCIReactionHandler(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/repos/owner/repo/issues/42/comments":
		writePendingCICommentPage(w, r)
	case "/repos/owner/repo/issues/comments/101/reactions":
		writePendingCIReactionPage(w, r)
	default:
		writeEmptyPendingCIReactionPage(w, r)
	}
}

func writePendingCICommentPage(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page != 1 {
		_ = json.NewEncoder(w).Encode([]map[string]int{{"id": 101}})

		return
	}
	comments := make([]map[string]int, 100)
	for index := range comments {
		comments[index] = map[string]int{"id": index + 1}
	}
	_ = json.NewEncoder(w).Encode(comments)
}

func writePendingCIReactionPage(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page != 1 {
		_ = json.NewEncoder(w).Encode([]map[string]any{{
			"content": "hooray", "user": map[string]any{"login": "smyklot[bot]"},
		}})

		return
	}
	reactions := make([]map[string]any, 100)
	for index := range reactions {
		reactions[index] = map[string]any{
			"content": "eyes",
			"user":    map[string]any{"login": fmt.Sprintf("reviewer-%d", index)},
		}
	}
	_ = json.NewEncoder(w).Encode(reactions)
}

func writeEmptyPendingCIReactionPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("page") == "1" {
		_ = json.NewEncoder(w).Encode([]map[string]any{})

		return
	}
	http.Error(w, "unexpected request", http.StatusNotFound)
}

func TestHasPullRequestCommentReactionIgnoresOtherReactions(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/issues/42/comments":
			_ = json.NewEncoder(w).Encode([]map[string]int{{"id": 101}})
		case "/repos/owner/repo/issues/comments/101/reactions":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"content": "eyes", "user": map[string]any{"login": "reviewer"}},
				{"content": "+1", "user": map[string]any{"login": "smyklot[bot]"}},
			})
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	found, err := client.HasPullRequestCommentReaction(
		context.Background(), "owner", "repo", 42, "smyklot[bot]",
		github.ReactionPendingCIService,
	)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("another user's eyes and the bot's success reaction claimed service ownership")
	}
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
		github.ReactionPendingCIService,
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
