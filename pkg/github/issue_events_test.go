package github_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

func TestPullRequestDraftedAfterLabelFindsLaterDraftTransition(t *testing.T) {
	t.Parallel()
	client, closeServer := issueEventClient(t, []map[string]any{
		issueEvent(10, "labeled", "2026-08-25T08:00:00Z", "smyklot:pending:ci"),
		issueEvent(11, "convert_to_draft", "2026-08-25T08:01:00Z", ""),
		issueEvent(12, "ready_for_review", "2026-08-25T08:02:00Z", ""),
	})
	defer closeServer()

	drafted, err := client.PullRequestDraftedAfterLabel(
		t.Context(), "acme", "web", 7, "smyklot:pending:ci",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !drafted {
		t.Fatal("draft transition after authorization was not found")
	}
}

func TestPullRequestDraftedAfterLabelUsesCurrentLabelOccurrence(t *testing.T) {
	t.Parallel()
	client, closeServer := issueEventClient(t, []map[string]any{
		issueEvent(10, "labeled", "2026-08-25T08:00:00Z", "smyklot:pending:ci"),
		issueEvent(11, "convert_to_draft", "2026-08-25T08:01:00Z", ""),
		issueEvent(12, "unlabeled", "2026-08-25T08:02:00Z", "smyklot:pending:ci"),
		issueEvent(13, "labeled", "2026-08-25T08:03:00Z", "smyklot:pending:ci"),
	})
	defer closeServer()

	drafted, err := client.PullRequestDraftedAfterLabel(
		t.Context(), "acme", "web", 7, "smyklot:pending:ci",
	)
	if err != nil {
		t.Fatal(err)
	}
	if drafted {
		t.Fatal("draft transition from an earlier authorization leaked into the current request")
	}
}

func TestPullRequestDraftedAfterLabelFailsClosedWithoutAuthorizationEvent(t *testing.T) {
	t.Parallel()
	client, closeServer := issueEventClient(t, []map[string]any{
		issueEvent(10, "convert_to_draft", "2026-08-25T08:01:00Z", ""),
	})
	defer closeServer()

	_, err := client.PullRequestDraftedAfterLabel(
		t.Context(), "acme", "web", 7, "smyklot:pending:ci",
	)
	if err == nil {
		t.Fatal("missing authorization event was accepted")
	}
}

func TestPullRequestDraftedAfterLabelFailsClosedOnMalformedDraftEvent(t *testing.T) {
	t.Parallel()
	client, closeServer := issueEventClient(t, []map[string]any{
		issueEvent(10, "labeled", "2026-08-25T08:00:00Z", "smyklot:pending:ci"),
		{"id": 11, "event": "convert_to_draft"},
	})
	defer closeServer()

	_, err := client.PullRequestDraftedAfterLabel(
		t.Context(), "acme", "web", 7, "smyklot:pending:ci",
	)
	if err == nil {
		t.Fatal("malformed draft transition was accepted")
	}
}

func issueEventClient(
	t *testing.T,
	events []map[string]any,
) (*github.Client, func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/web/issues/7/events" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		if err := json.NewEncoder(w).Encode(events); err != nil {
			t.Error(err)
		}
	}))
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		server.Close()
		t.Fatal(err)
	}

	return client, server.Close
}

func issueEvent(id int64, event, createdAt, label string) map[string]any {
	value := map[string]any{"id": id, "event": event, "created_at": createdAt}
	if label != "" {
		value["label"] = map[string]any{"name": label}
	}

	return value
}
