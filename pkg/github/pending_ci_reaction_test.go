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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/issues/42/comments":
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			if page == 1 {
				comments := make([]map[string]int, 100)
				for index := range comments {
					comments[index] = map[string]int{"id": index + 1}
				}
				_ = json.NewEncoder(w).Encode(comments)

				return
			}
			_ = json.NewEncoder(w).Encode([]map[string]int{{"id": 101}})
		case "/repos/owner/repo/issues/comments/101/reactions":
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			if page == 1 {
				reactions := make([]map[string]any, 100)
				for index := range reactions {
					reactions[index] = map[string]any{
						"content": "eyes",
						"user":    map[string]any{"login": fmt.Sprintf("reviewer-%d", index)},
					}
				}
				_ = json.NewEncoder(w).Encode(reactions)

				return
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"content": "hooray", "user": map[string]any{"login": "smyklot[bot]"},
			}})
		default:
			if r.URL.Query().Get("page") == "1" {
				_ = json.NewEncoder(w).Encode([]map[string]any{})

				return
			}
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
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
