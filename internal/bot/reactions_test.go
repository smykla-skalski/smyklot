package bot

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/permissions"
)

func TestHandleReactionsAsksOncePerReactor(t *testing.T) {
	t.Parallel()

	// Given two people who reacted, and a repository with no CODEOWNERS, so
	// every permission decision costs a round trip to GitHub
	var permissionCalls atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/reactions"):
			_, _ = w.Write([]byte(`[
			  {"content":"confused","user":{"login":"first"}},
			  {"content":"confused","user":{"login":"second"}}
			]`))
		case strings.HasSuffix(r.URL.Path, "/labels"):
			_, _ = w.Write([]byte(`[]`))
		case strings.HasSuffix(r.URL.Path, "/permission"):
			permissionCalls.Add(1)
			_, _ = w.Write([]byte(`{"permission":"write"}`))
		default:
			t.Errorf("unexpected GitHub request %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer api.Close()

	client, err := github.NewClient("token", api.URL)
	if err != nil {
		t.Fatal(err)
	}
	checker, err := permissions.NewCheckerFromContent("", client)
	if err != nil {
		t.Fatal(err)
	}

	// When the reactions are handled
	runtime := &RuntimeConfig{RepoOwner: "owner", RepoName: "repo", BotUsername: DefaultBotUsername}
	defaults := config.Default()
	err = handleReactions(
		t.Context(), client, runtime, defaults, checker, 42, 900, CommandEnvironment{},
	)
	if err != nil {
		t.Fatalf("handleReactions: %v", err)
	}

	// Then each reactor's permission was decided once, not once per pass over
	// the list
	if got := permissionCalls.Load(); got != 2 {
		t.Fatalf("permission requests = %d, want 2 (one per reactor)", got)
	}
}
