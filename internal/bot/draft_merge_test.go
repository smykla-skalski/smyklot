package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

func TestDraftMergeAuthorizationOrdersAgainstGitHubDraftHistory(t *testing.T) {
	t.Parallel()
	draftedAt := time.Date(2026, 8, 25, 8, 1, 0, 0, time.UTC)
	for _, test := range []struct {
		name     string
		revision string
		wantErr  error
	}{
		{name: "older command", revision: "2026-08-25T08:00:59Z", wantErr: pendingci.ErrStaleSourceRevision},
		{name: "same second", revision: "2026-08-25T08:01:00Z", wantErr: pendingci.ErrAmbiguousSourceRevision},
		{name: "fresh command", revision: "2026-08-25T08:01:01Z"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateDraftMergeAuthorization(
				t.Context(), draftHistoryStub{draftedAt: draftedAt, found: true},
				"acme", "web", 7, test.revision,
			)
			if !errors.Is(err, test.wantErr) || test.wantErr == nil && err != nil {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func TestDraftMergeCommandRevisionRequiresExactDeliveredComment(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name         string
		eventBody    string
		eventAuthor  string
		eventVersion string
		liveBody     string
		liveAuthor   string
		liveVersion  string
		wantErr      error
	}{
		{
			name: "exact delivery", eventBody: "/merge", eventAuthor: "operator",
			eventVersion: "2026-08-25T08:00:00Z", liveBody: "/merge",
			liveAuthor: "operator", liveVersion: "2026-08-25T08:00:00Z",
		},
		{
			name: "command edited away", eventBody: "/merge", eventAuthor: "operator",
			eventVersion: "2026-08-25T08:00:00Z", liveBody: "not merging",
			liveAuthor: "operator", liveVersion: "2026-08-25T08:02:00Z",
			wantErr: pendingci.ErrStaleSourceRevision,
		},
		{
			name: "newer identical body", eventBody: "/merge", eventAuthor: "operator",
			eventVersion: "2026-08-25T08:00:00Z", liveBody: "/merge",
			liveAuthor: "operator", liveVersion: "2026-08-25T08:02:00Z",
			wantErr: pendingci.ErrStaleSourceRevision,
		},
		{
			name: "different author", eventBody: "/merge", eventAuthor: "operator",
			eventVersion: "2026-08-25T08:00:00Z", liveBody: "/merge",
			liveAuthor: "attacker", liveVersion: "2026-08-25T08:00:00Z",
			wantErr: pendingci.ErrStaleSourceRevision,
		},
		{
			name: "missing event revision", eventBody: "/merge", eventAuthor: "operator",
			liveBody: "/merge", liveAuthor: "operator", liveVersion: "2026-08-25T08:02:00Z",
			wantErr: pendingci.ErrInvalidRequest,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/repos/acme/web/issues/comments/99" {
					http.NotFound(w, request)

					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id": 99, "body": test.liveBody, "updated_at": test.liveVersion,
					"user": map[string]any{"login": test.liveAuthor},
				})
			}))
			defer server.Close()
			client, err := github.NewClient("test-token", server.URL)
			if err != nil {
				t.Fatal(err)
			}
			runtime := draftRuntime()
			runtime.CommentBody = test.eventBody
			runtime.CommentAuthor = test.eventAuthor
			runtime.CommentRevision = test.eventVersion

			revision, err := draftMergeCommandRevision(
				t.Context(), client, runtime, 99, CommandEnvironment{},
			)
			if !errors.Is(err, test.wantErr) || test.wantErr == nil && err != nil {
				t.Fatalf("revision=%q error=%v, want error %v", revision, err, test.wantErr)
			}
			if test.wantErr == nil && revision != test.eventVersion {
				t.Fatalf("revision=%q, want %q", revision, test.eventVersion)
			}
		})
	}
}

func TestCoordinatedMergePreservesExactSourceThroughFinalEffect(t *testing.T) {
	t.Parallel()
	merged := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/repos/acme/web/pulls/7":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"number": 7, "state": "open", "draft": false, "mergeable": true,
				"mergeable_state": "clean", "user": map[string]any{"login": "author"},
			})
		case "/repos/acme/web/pulls/7/reviews":
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"state": "APPROVED", "user": map[string]any{"login": "operator"},
			}})
		case "/repos/acme/web/issues/comments/99":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 99, "body": "command removed", "updated_at": "2026-08-25T08:02:00Z",
				"user": map[string]any{"login": "operator"},
			})
		case "/repos/acme/web/pulls/7/merge":
			merged = true
			_ = json.NewEncoder(w).Encode(map[string]any{"merged": true})
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	runtime := draftRuntime()
	runtime.CommentRevision = "2026-08-25T08:00:00Z"
	environment := CommandEnvironment{
		DraftMergeRevision: runtime.CommentRevision,
		PendingCI: &PendingCICommand{
			Store: pendingCICommandStoreStub{
				finishResult: &pendingci.Request{ID: 1, ArtifactKind: pendingci.ArtifactLabel},
			},
			Coordinator: NewCoordinator(), RepositoryID: "repository:7",
			SourceCommentID: 99, SourceRevision: runtime.CommentRevision,
			SourceSequence: 1, SourceOrder: 1,
			Now:  func() time.Time { return time.Date(2026, 8, 25, 8, 3, 0, 0, time.UTC) },
			Wake: func() {},
		},
	}

	result, err := executeCoordinatedMerge(
		t.Context(), client, runtime, draftMergeConfig(), 7, 99,
		github.MergeMethodMerge, false, environment,
	)
	if err != nil {
		t.Fatal(err)
	}
	if merged {
		t.Fatal("coordinated merge executed a command edited away after source acceptance")
	}
	if result == nil || !strings.Contains(result.Message, "comment changed") {
		t.Fatalf("feedback = %#v, want stale-comment failure", result)
	}
}

func TestActionPendingCIMarkerBindsTheExactCommentRevision(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name          string
		fenceAt       string
		wantCancelled bool
	}{
		{name: "fence predates edited comment", fenceAt: "2026-08-25T08:02:00Z", wantCancelled: true},
		{name: "same second is ambiguous", fenceAt: "2026-08-25T08:04:00Z", wantCancelled: true},
		{name: "fence follows exact revision", fenceAt: "2026-08-25T08:04:01Z"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/repos/acme/web/issues/7/comments":
					_ = json.NewEncoder(w).Encode([]map[string]any{{
						"id": 99, "body": "/merge after ci", "updated_at": "2026-08-25T08:04:00Z",
					}})
				case "/repos/acme/web/issues/comments/99/reactions":
					_ = json.NewEncoder(w).Encode([]map[string]any{
						{
							"id": 41, "content": "eyes", "created_at": test.fenceAt,
							"user": map[string]any{"login": "smyklot[bot]"},
						},
						{
							"id": 42, "content": "hooray", "created_at": "2026-08-25T08:04:02Z",
							"user": map[string]any{"login": "smyklot[bot]"},
						},
					})
				case "/repos/acme/web/issues/7/events":
					_ = json.NewEncoder(w).Encode([]map[string]any{{
						"id": 3, "event": "convert_to_draft", "created_at": "2026-08-25T08:03:00Z",
					}})
				default:
					http.NotFound(w, request)
				}
			}))
			defer server.Close()
			client, err := github.NewClient("test-token", server.URL)
			if err != nil {
				t.Fatal(err)
			}
			cancelled, err := actionPendingCIDraftCancelled(
				t.Context(), client, draftMergeConfig(), "acme", "web", 7,
				github.MergeMethodMerge, false, LabelPendingCIMerge, "smyklot[bot]", false,
			)
			if err != nil {
				t.Fatal(err)
			}
			if cancelled != test.wantCancelled {
				t.Fatalf("cancelled = %t, want %t", cancelled, test.wantCancelled)
			}
		})
	}
}

func TestActionPendingCILegacyRequestSurvivesConfigEnablement(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/repos/acme/web/issues/7/comments":
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"id": 99, "body": "/merge after ci", "updated_at": "2026-08-25T08:02:00Z",
			}})
		case "/repos/acme/web/issues/comments/99/reactions":
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"id": 42, "content": "eyes", "user": map[string]any{"login": "smyklot[bot]"},
			}})
		case "/repos/acme/web/issues/7/events":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id": 1, "event": "convert_to_draft", "created_at": "2026-08-25T08:00:00Z",
				},
				{
					"id": 2, "event": "labeled", "created_at": "2026-08-25T08:01:00Z",
					"label": map[string]any{"name": LabelPendingCIMerge},
					"actor": map[string]any{"login": "smyklot[bot]"},
				},
			})
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := actionPendingCIDraftCancelled(
		t.Context(), client, draftMergeConfig(), "acme", "web", 7,
		github.MergeMethodMerge, false, LabelPendingCIMerge, "smyklot[bot]", false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled {
		t.Fatal("legacy eyes-and-label request was cancelled when the setting became enabled")
	}
}

func TestActionPendingCIFastPathRevalidatesBeforeMerge(t *testing.T) {
	t.Parallel()
	merged := false
	commentReads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/repos/acme/web/issues/comments/99":
			commentReads++
			body := "/merge after ci"
			revision := "2026-08-25T08:02:00Z"
			if commentReads >= 3 {
				body = "command removed"
				revision = "2026-08-25T08:04:00Z"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 99, "body": body, "updated_at": revision,
				"user": map[string]any{"login": "operator"},
			})
		case "/repos/acme/web/issues/7/events":
			_, _ = w.Write([]byte(`[]`))
		case "/repos/acme/web/pulls/7/merge":
			merged = true
			_ = json.NewEncoder(w).Encode(map[string]any{"merged": true})
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	botConfig := config.Default()
	botConfig.AllowDraftMerges = true
	runtime := draftRuntime()
	runtime.CommentBody = "/merge after ci"
	authorize := commandMergeAuthorizer(
		t.Context(), client, runtime, 7, 99, runtime.CommentRevision,
	)
	result, err := executeImmediateMerge(
		t.Context(), client, runtime, botConfig, 7, github.MergeMethodMerge,
		&github.PRInfo{Author: "author", ApprovedBy: []string{"operator"}},
		"head", authorize,
	)
	if err != nil {
		t.Fatal(err)
	}
	if merged {
		t.Fatal("stale Action command reached the merge effect")
	}
	if result == nil || !strings.Contains(result.Message, "reissue the command") {
		t.Fatalf("feedback = %#v, want reissue guidance", result)
	}
}

func TestActionPendingCIFastPathRevalidatesBeforeFallback(t *testing.T) {
	t.Parallel()
	commentReads := 0
	mergeRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/repos/acme/web/issues/comments/99":
			commentReads++
			body := "/merge after ci"
			revision := "2026-08-25T08:02:00Z"
			if commentReads >= 4 {
				body = "command removed"
				revision = "2026-08-25T08:04:00Z"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 99, "body": body, "updated_at": revision,
				"user": map[string]any{"login": "operator"},
			})
		case "/repos/acme/web/issues/7/events":
			_, _ = w.Write([]byte(`[]`))
		case "/repos/acme/web/pulls/7/merge":
			mergeRequests++
			w.WriteHeader(http.StatusMethodNotAllowed)
			_, _ = w.Write([]byte(`{"message":"Merge commits are not allowed"}`))
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	runtime := draftRuntime()
	runtime.CommentBody = "/merge after ci"
	authorize := commandMergeAuthorizer(
		t.Context(), client, runtime, 7, 99, runtime.CommentRevision,
	)
	result, err := executeImmediateMerge(
		t.Context(), client, runtime, draftMergeConfig(), 7, github.MergeMethodMerge,
		&github.PRInfo{Author: "author", ApprovedBy: []string{"operator"}},
		"head", authorize,
	)
	if err != nil {
		t.Fatal(err)
	}
	if mergeRequests != 1 {
		t.Fatalf("merge requests = %d, want only the initial merge attempt", mergeRequests)
	}
	if result == nil || !strings.Contains(result.Message, "reissue the command") {
		t.Fatalf("feedback = %#v, want reissue guidance", result)
	}
}

func TestActionPendingCIPollRevalidatesAtFinalMergeBoundary(t *testing.T) {
	t.Parallel()
	merged := false
	historyReads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/web/pulls/7":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"number": 7, "state": "open", "draft": false,
				"head":   map[string]any{"sha": "head"},
				"labels": []map[string]any{{"name": LabelPendingCIMerge}},
			})
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/web/issues/7/reactions":
			_, _ = w.Write([]byte(`[]`))
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/web/issues/7/comments":
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"id": 99, "body": "/merge after ci", "updated_at": "2026-08-25T08:02:00Z",
			}})
		case request.Method == http.MethodGet &&
			request.URL.Path == "/repos/acme/web/issues/comments/99":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 99, "body": "/merge after ci", "updated_at": "2026-08-25T08:02:00Z",
			})
		case request.Method == http.MethodGet &&
			request.URL.Path == "/repos/acme/web/issues/comments/99/reactions":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id": 41, "content": "eyes", "created_at": "2026-08-25T08:02:01Z",
					"user": map[string]any{"login": "smyklot[bot]"},
				},
				{
					"id": 42, "content": "hooray", "created_at": "2026-08-25T08:02:02Z",
					"user": map[string]any{"login": "smyklot[bot]"},
				},
			})
		case request.Method == http.MethodDelete &&
			request.URL.Path == "/repos/acme/web/issues/7/labels/smyklot:pending:ci":
			w.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/web/issues/7/events":
			historyReads++
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"id": 2, "event": "convert_to_draft", "created_at": "2026-08-25T08:03:00Z",
			}})
		case request.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":1}`))
		case request.URL.Path == "/repos/acme/web/pulls/7/merge":
			merged = true
			_ = json.NewEncoder(w).Encode(map[string]any{"merged": true})
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	botConfig := config.Default()
	botConfig.AllowDraftMerges = true
	err = handlePendingCIPassed(
		t.Context(), client, botConfig, "acme", "web", 7,
		PendingCIPR{Method: github.MergeMethodMerge, Label: LabelPendingCIMerge},
		"smyklot[bot]", "head",
	)
	if err != nil {
		t.Fatal(err)
	}
	if merged {
		t.Fatal("stale Action pending-CI command reached the merge effect")
	}
	if historyReads < 2 {
		t.Fatalf("draft history reads = %d, want final-boundary revalidation", historyReads)
	}
}

func TestPendingCIMergeFallbackReauthorizesEveryMethod(t *testing.T) {
	t.Parallel()
	mergeAttempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPut || request.URL.Path != "/repos/acme/web/pulls/7/merge" {
			http.NotFound(w, request)

			return
		}
		mergeAttempts++
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte(`{"message":"Merge commits are not allowed"}`))
	}))
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	authorizations := 0
	authorize := func() error {
		authorizations++
		if authorizations > 1 {
			return pendingci.ErrStaleSourceRevision
		}

		return nil
	}

	err = MergePendingPRAtHead(
		t.Context(), client, "acme", "web", 7,
		github.MergeMethodMerge, "head", authorize,
	)
	if !errors.Is(err, pendingci.ErrStaleSourceRevision) {
		t.Fatalf("error = %v, want stale source", err)
	}
	if authorizations != 2 || mergeAttempts != 1 {
		t.Fatalf(
			"authorizations=%d mergeAttempts=%d, want 2 and 1",
			authorizations, mergeAttempts,
		)
	}
}

func TestReactionMergeRevalidatesEveryFinalEffect(t *testing.T) {
	for _, mergeable := range []bool{true, false} {
		t.Run(map[bool]string{true: "merge", false: "auto merge"}[mergeable], func(t *testing.T) {
			server, state := reactionDraftRaceServer(t, mergeable)
			defer server.Close()
			client, err := github.NewClient("test-token", server.URL)
			if err != nil {
				t.Fatal(err)
			}
			botConfig := config.Default()
			botConfig.AllowDraftMerges = true
			err = handleReactionMerge(
				t.Context(), client, draftRuntime(), botConfig, 7, 99, "operator",
				time.Date(2026, 8, 25, 8, 2, 0, 0, time.UTC),
			)
			if !errors.Is(err, pendingci.ErrStaleSourceRevision) {
				t.Fatalf("error = %v, want stale source", err)
			}
			if state.effectCalled {
				t.Fatal("stale reaction reached a merge effect")
			}
			if state.historyReads != 2 {
				t.Fatalf("draft history reads = %d, want 2", state.historyReads)
			}
		})
	}
}

type reactionDraftRaceState struct {
	effectCalled bool
	historyReads int
}

func reactionDraftRaceServer(
	t *testing.T,
	mergeable bool,
) (*httptest.Server, *reactionDraftRaceState) {
	t.Helper()
	state := &reactionDraftRaceState{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/repos/acme/web/pulls/7":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"number": 7, "node_id": "PR_node", "state": "open", "draft": false,
				"mergeable":       mergeable,
				"mergeable_state": map[bool]string{true: "clean", false: "blocked"}[mergeable],
				"user":            map[string]any{"login": "author"},
			})
		case request.URL.Path == "/repos/acme/web/pulls/7/reviews":
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"state": "APPROVED", "user": map[string]any{"login": "operator"},
			}})
		case request.URL.Path == "/repos/acme/web/issues/7/events":
			state.historyReads++
			draftedAt := "2026-08-25T08:00:00Z"
			if state.historyReads > 1 {
				draftedAt = "2026-08-25T08:03:00Z"
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"id": state.historyReads, "event": "convert_to_draft", "created_at": draftedAt,
			}})
		case request.URL.Path == "/repos/acme/web/pulls/7/merge" || request.URL.Path == "/graphql":
			state.effectCalled = true
			_ = json.NewEncoder(w).Encode(map[string]any{"merged": true})
		case request.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			http.NotFound(w, request)
		}
	}))

	return server, state
}

func TestDraftMergePreparesEveryTextMergeMethod(t *testing.T) {
	for _, method := range []github.MergeMethod{
		github.MergeMethodMerge,
		github.MergeMethodSquash,
		github.MergeMethodRebase,
	} {
		t.Run(string(method), func(t *testing.T) {
			server, requests := draftMergeServer(t, method)
			defer server.Close()
			client, err := github.NewClient("test-token", server.URL)
			if err != nil {
				t.Fatal(err)
			}
			botConfig := config.Default()
			botConfig.AllowDraftMerges = true
			result, err := executeMerge(
				t.Context(), client, draftRuntime(), botConfig, 7, 99,
				method, false, false, CommandEnvironment{},
			)
			if err != nil {
				t.Fatal(err)
			}
			if result == nil || result.Message == "" {
				t.Fatalf("merge feedback = %#v", result)
			}
			assertRequestOrder(t, *requests, []string{
				"GET /repos/acme/web/pulls/7",
				"GET /repos/acme/web/pulls/7/reviews",
				"GET /repos/acme/web/issues/comments/99",
				"GET /repos/acme/web/issues/7/events",
				"GET /repos/acme/web/issues/comments/99",
				"GET /repos/acme/web/issues/7/events",
				"GET /repos/acme/web/pulls/7",
				"POST /graphql",
				"GET /repos/acme/web/pulls/7",
				"GET /repos/acme/web/pulls/7/reviews",
				"GET /repos/acme/web/issues/comments/99",
				"GET /repos/acme/web/issues/7/events",
				"GET /repos/acme/web/issues/comments/99",
				"GET /repos/acme/web/issues/7/events",
				"PUT /repos/acme/web/pulls/7/merge",
			})
		})
	}
}

func TestTextMergeFallbackRechecksExactComment(t *testing.T) {
	t.Parallel()
	commentReads := 0
	mergeAttempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/repos/acme/web/issues/comments/99":
			commentReads++
			body := "/merge"
			updatedAt := "2026-08-25T08:02:00Z"
			if commentReads > 1 {
				body = "command removed"
				updatedAt = "2026-08-25T08:04:00Z"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 99, "body": body, "updated_at": updatedAt,
				"user": map[string]any{"login": "operator"},
			})
		case "/repos/acme/web/issues/7/events":
			_, _ = w.Write([]byte(`[]`))
		case "/repos/acme/web/pulls/7/merge":
			mergeAttempts++
			w.WriteHeader(http.StatusMethodNotAllowed)
			_, _ = w.Write([]byte(`{"message":"Merge commits are not allowed"}`))
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	runtime := draftRuntime()
	authorize := commandMergeAuthorizer(
		t.Context(), client, runtime, 7, 99, runtime.CommentRevision,
	)
	result, err := mergeCommandPR(
		t.Context(), client, runtime, draftMergeConfig(), 7,
		github.MergeMethodMerge, authorize,
	)
	if err != nil {
		t.Fatal(err)
	}
	if mergeAttempts != 1 {
		t.Fatalf("merge attempts = %d, want 1", mergeAttempts)
	}
	if result == nil || !strings.Contains(result.Message, "reissue the command") {
		t.Fatalf("feedback = %#v, want reissue guidance", result)
	}
}

func TestDraftMergeDisabledFailsBeforeMutation(t *testing.T) {
	client := &draftMergeStub{responses: []*github.PRInfo{{Draft: true}}}
	_, err := prepareDraftMerge(t.Context(), client, "acme", "web", 7, false, client.responses[0])
	if err == nil || err.Error() != errDraftMergeDisabled.Error() {
		t.Fatalf("error = %v, want %v", err, errDraftMergeDisabled)
	}
	if client.readyCalls != 0 || client.infoCalls != 0 {
		t.Fatalf("unexpected effects: ready=%d info=%d", client.readyCalls, client.infoCalls)
	}
}

func TestDraftMergeRefetchesAndRejectsStaleDraftState(t *testing.T) {
	client := &draftMergeStub{responses: []*github.PRInfo{{Draft: true}}}
	_, err := prepareDraftMerge(
		t.Context(), client, "acme", "web", 7, true, &github.PRInfo{Draft: true},
	)
	if err == nil || err.Error() != "pull request is still a draft after marking it ready for review" {
		t.Fatalf("error = %v", err)
	}
	if client.readyCalls != 1 || client.infoCalls != 1 {
		t.Fatalf("effects: ready=%d info=%d", client.readyCalls, client.infoCalls)
	}
}

func TestReactionMergePreparesDraftBeforeMerging(t *testing.T) {
	server, requests := draftMergeServer(t, github.MergeMethodMerge)
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	botConfig := config.Default()
	botConfig.AllowDraftMerges = true
	if err := handleReactionMerge(
		t.Context(), client, draftRuntime(), botConfig, 7, 99, "operator",
		time.Date(2026, 8, 25, 8, 2, 0, 0, time.UTC),
	); err != nil {
		t.Fatal(err)
	}
	assertRequestOrder(t, (*requests)[:9], []string{
		"GET /repos/acme/web/pulls/7",
		"GET /repos/acme/web/pulls/7/reviews",
		"GET /repos/acme/web/issues/7/events",
		"GET /repos/acme/web/pulls/7",
		"POST /graphql",
		"GET /repos/acme/web/pulls/7",
		"GET /repos/acme/web/pulls/7/reviews",
		"GET /repos/acme/web/issues/7/events",
		"PUT /repos/acme/web/pulls/7/merge",
	})
}

func TestDraftMergeApprovesBeforeBlockedAutoMerge(t *testing.T) {
	server, requests := draftBlockedServer(t)
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	botConfig := config.Default()
	botConfig.AllowDraftMerges = true
	result, err := executeMerge(
		t.Context(), client, draftRuntime(), botConfig, 7, 99,
		github.MergeMethodMerge, false, false, CommandEnvironment{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatalf("auto-merge feedback = %#v", result)
	}
	assertApprovalBeforeAutoMerge(t, *requests)
}

func TestDraftMergeRefusesSelfApprovalBeforePublishing(t *testing.T) {
	server, requests := draftBlockedServer(t)
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	botConfig := config.Default()
	botConfig.AllowDraftMerges = true
	runtime := draftRuntime()
	runtime.CommentAuthor = "author"
	result, err := executeMerge(
		t.Context(), client, runtime, botConfig, 7, 99,
		github.MergeMethodMerge, false, false, CommandEnvironment{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Message == "" {
		t.Fatalf("self-approval feedback = %#v", result)
	}
	for _, request := range *requests {
		if request == "POST /graphql" || request == "POST /repos/acme/web/pulls/7/reviews" {
			t.Fatalf("self-approval mutated pull request: %v", *requests)
		}
	}
}

func TestDraftReactionMergeApprovesBeforeBlockedAutoMerge(t *testing.T) {
	server, requests := draftBlockedServer(t)
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	botConfig := config.Default()
	botConfig.AllowDraftMerges = true
	if err := handleReactionMerge(
		t.Context(), client, draftRuntime(), botConfig, 7, 99, "operator",
		time.Date(2026, 8, 25, 8, 2, 0, 0, time.UTC),
	); err != nil {
		t.Fatal(err)
	}
	assertApprovalBeforeAutoMerge(t, *requests)
}

func TestActionPendingCICommandTreatsMissingLabelAsDisarmed(t *testing.T) {
	labels := &actionPendingCILabelStub{removeErr: github.NewAPIError(
		github.ErrAPIRequest, http.StatusNotFound, http.MethodDelete, "/labels/pending", nil,
	)}
	if err := removeActionPendingCILabel(
		t.Context(), labels, "acme", "web", 7, LabelPendingCIMerge,
	); err != nil {
		t.Fatal(err)
	}
	if len(labels.calls) != 1 || labels.calls[0] != "remove" {
		t.Fatalf("label operations = %v, want [remove]", labels.calls)
	}
}

func TestActionPendingCIPublishesActivationAfterExactRevalidation(t *testing.T) {
	t.Parallel()
	steps := make([]string, 0, 9)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodDelete &&
			request.URL.Path == "/repos/acme/web/issues/7/labels/smyklot:pending:ci":
			steps = append(steps, "disarm label")
			w.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodGet &&
			request.URL.Path == "/repos/acme/web/issues/comments/99/reactions":
			steps = append(steps, "scan reactions")
			_, _ = w.Write([]byte(`[]`))
		case request.Method == http.MethodPost &&
			request.URL.Path == "/repos/acme/web/issues/comments/99/reactions":
			var body map[string]string
			_ = json.NewDecoder(request.Body).Decode(&body)
			steps = append(steps, "add "+body["content"])
			reactions := map[string]map[string]any{
				"laugh": {
					"id": 40, "content": "laugh", "created_at": "2026-08-25T08:02:01Z",
				},
				"eyes": {
					"id": 41, "content": "eyes", "created_at": "2026-08-25T08:02:02Z",
				},
				"hooray": {
					"id": 42, "content": "hooray", "created_at": "2026-08-25T08:02:03Z",
				},
			}
			_ = json.NewEncoder(w).Encode(reactions[body["content"]])
		case request.Method == http.MethodDelete &&
			request.URL.Path == "/repos/acme/web/issues/comments/99/reactions/40":
			steps = append(steps, "open gate")
			w.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodPost &&
			request.URL.Path == "/repos/acme/web/issues/7/labels":
			steps = append(steps, "arm label")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`[]`))
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	authorize := func() error {
		steps = append(steps, "authorize exact command")

		return nil
	}
	err = publishActionPendingCI(
		t.Context(), client, draftRuntime(), draftMergeConfig(), 7, 99,
		github.MergeMethodMerge, false, LabelPendingCIMerge,
		"2026-08-25T08:02:00Z", authorize,
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"disarm label",
		"scan reactions", "scan reactions", "scan reactions",
		"add laugh", "add eyes", "authorize exact command", "add hooray",
		"open gate", "arm label",
	}
	if !slices.Equal(steps, want) {
		t.Fatalf("steps = %v, want %v", steps, want)
	}
}

func TestActionPendingCIRejectedBeforeActivationCannotBeRevived(t *testing.T) {
	t.Parallel()
	state := &actionPendingCIRejectionState{publishing: true}
	server := httptest.NewServer(actionPendingCIRejectionHandler(state))
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	err = publishActionPendingCI(
		t.Context(), client, draftRuntime(), draftMergeConfig(), 7, 99,
		github.MergeMethodMerge, false, LabelPendingCIMerge,
		"2026-08-25T08:02:00Z",
		func() error {
			return fmt.Errorf("%w: command changed", pendingci.ErrStaleSourceRevision)
		},
	)
	if !errors.Is(err, pendingci.ErrStaleSourceRevision) {
		t.Fatalf("error = %v, want stale source", err)
	}
	if state.activationAdded || state.labelAdded {
		t.Fatalf(
			"rejected request activated=%t labeled=%t",
			state.activationAdded, state.labelAdded,
		)
	}

	state.publishing = false
	if err := client.RemoveReactionByUser(
		t.Context(), "acme", "web", 99, ReactionError, "smyklot[bot]",
	); err != nil {
		t.Fatal(err)
	}
	if !state.rejectionPresent {
		t.Fatal("ordinary retry cleanup removed the pending CI rejection gate")
	}
	state.feedbackCleanup = true
	if err := PostFeedback(
		t.Context(), client, draftRuntime(), 7, 99, "", ReactionError,
	); err == nil {
		t.Fatal("feedback cleanup unexpectedly succeeded")
	}
	state.feedbackCleanup = false
	cancelled, err := actionPendingCIDraftCancelled(
		t.Context(), client, draftMergeConfig(), "acme", "web", 7,
		github.MergeMethodMerge, false, LabelPendingCIMerge, "smyklot[bot]", false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !cancelled {
		t.Fatal("a restored label revived a request rejected before activation")
	}
}

func TestActionPendingCILostActivationResponseStaysClosed(t *testing.T) {
	t.Parallel()
	state := &actionPendingCIRejectionState{
		publishing: true, activationResponseLost: true,
	}
	server := httptest.NewServer(actionPendingCIRejectionHandler(state))
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	err = publishActionPendingCI(
		t.Context(), client, draftRuntime(), draftMergeConfig(), 7, 99,
		github.MergeMethodMerge, false, LabelPendingCIMerge,
		"2026-08-25T08:02:00Z", func() error { return nil },
	)
	if err == nil {
		t.Fatal("lost activation response unexpectedly succeeded")
	}
	if !state.activationAdded || state.labelAdded {
		t.Fatalf(
			"ambiguous activation persisted=%t labeled=%t",
			state.activationAdded, state.labelAdded,
		)
	}

	state.publishing = false
	cancelled, err := actionPendingCIDraftCancelled(
		t.Context(), client, draftMergeConfig(), "acme", "web", 7,
		github.MergeMethodMerge, false, LabelPendingCIMerge, "smyklot[bot]", false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !cancelled {
		t.Fatal("persisted activation opened a generation with an intact rejection gate")
	}
}

type actionPendingCIRejectionState struct {
	publishing             bool
	feedbackCleanup        bool
	activationResponseLost bool
	rejectionPresent       bool
	activationAdded        bool
	labelAdded             bool
}

func actionPendingCIRejectionHandler(
	state *actionPendingCIRejectionState,
) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		switch request.Method + " " + request.URL.Path {
		case "DELETE /repos/acme/web/issues/7/labels/smyklot:pending:ci":
			w.WriteHeader(http.StatusNoContent)
		case "GET /repos/acme/web/issues/comments/99/reactions":
			writeActionPendingCIRejectionMarkers(w, state)
		case "POST /repos/acme/web/issues/comments/99/reactions":
			writeActionPendingCIRejectionMarker(w, request, state)
		case "POST /repos/acme/web/issues/7/labels":
			state.labelAdded = true
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`[]`))
		case "DELETE /repos/acme/web/issues/comments/99/reactions/40":
			state.rejectionPresent = false
			w.WriteHeader(http.StatusNoContent)
		case "DELETE /repos/acme/web/issues/comments/99/reactions/41":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"reaction cleanup failed"}`))
		case "GET /repos/acme/web/issues/7/comments":
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"id": 99, "body": "/merge after ci", "updated_at": "2026-08-25T08:03:00Z",
			}})
		case "GET /repos/acme/web/issues/7/events":
			_, _ = w.Write([]byte(`[]`))
		default:
			http.NotFound(w, request)
		}
	}
}

func writeActionPendingCIRejectionMarkers(
	w http.ResponseWriter,
	state *actionPendingCIRejectionState,
) {
	if state.publishing {
		_, _ = w.Write([]byte(`[]`))

		return
	}
	reactions := []map[string]any{{
		"id": 41, "content": "eyes", "created_at": "2026-08-25T08:02:02Z",
		"user": map[string]any{"login": "smyklot[bot]"},
	}}
	if state.rejectionPresent {
		reactions = append(reactions, map[string]any{
			"id": 40, "content": string(ReactionPendingCIRejected),
			"created_at": "2026-08-25T08:02:01Z",
			"user":       map[string]any{"login": "smyklot[bot]"},
		})
	}
	if state.activationAdded {
		reactions = append(reactions, map[string]any{
			"id": 42, "content": "hooray", "created_at": "2026-08-25T08:02:03Z",
			"user": map[string]any{"login": "smyklot[bot]"},
		})
	}
	_ = json.NewEncoder(w).Encode(reactions)
}

func writeActionPendingCIRejectionMarker(
	w http.ResponseWriter,
	request *http.Request,
	state *actionPendingCIRejectionState,
) {
	if state.feedbackCleanup {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"reaction cleanup failed"}`))

		return
	}
	var body map[string]string
	_ = json.NewDecoder(request.Body).Decode(&body)
	createdAt := "2026-08-25T08:02:01Z"
	id := 40
	switch body["content"] {
	case string(ReactionPendingCIRejected):
		state.rejectionPresent = true
	case "eyes":
		createdAt = "2026-08-25T08:02:02Z"
		id = 41
	case "hooray":
		state.activationAdded = true
		if state.activationResponseLost {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"response lost"}`))

			return
		}
		createdAt = "2026-08-25T08:02:03Z"
		id = 42
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": id, "content": body["content"], "created_at": createdAt,
	})
}

func TestActionPendingCIFailureRestoresAnotherAuthorizedRequest(t *testing.T) {
	t.Parallel()
	state := &actionPendingCIPublishFailureState{}
	server := httptest.NewServer(actionPendingCIPublishFailureHandler(state))
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	err = publishActionPendingCI(
		t.Context(), client, draftRuntime(), draftMergeConfig(), 7, 99,
		github.MergeMethodMerge, false, LabelPendingCIMerge,
		"2026-08-25T08:05:00Z", func() error { return nil },
	)
	if err == nil {
		t.Fatal("failed publication unexpectedly succeeded")
	}
	if !state.labelRestored {
		t.Fatal("failed publication stranded another authorized pending request")
	}
}

func TestActionPendingCIRepairFailureLeavesDurableRetry(t *testing.T) {
	t.Parallel()
	state := &actionPendingCIPublishFailureState{
		scanFailures: 3, nextRepairSignalID: 70,
	}
	server := httptest.NewServer(actionPendingCIPublishFailureHandler(state))
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	err = publishActionPendingCI(
		t.Context(), client, draftRuntime(), draftMergeConfig(), 7, 99,
		github.MergeMethodMerge, false, LabelPendingCIMerge,
		"2026-08-25T08:05:00Z", func() error { return nil },
	)
	if err == nil {
		t.Fatal("failed publication unexpectedly succeeded")
	}
	if state.labelRestored || state.repairReadyID == 0 || state.repairClaimID != 0 {
		t.Fatalf(
			"labelRestored=%t ready=%d claim=%d, want only a non-authorizing retry signal",
			state.labelRestored, state.repairReadyID, state.repairClaimID,
		)
	}
	request, found, err := recoverActionPendingCIRepair(
		t.Context(), client, draftMergeConfig(), "acme", "web",
		map[string]interface{}{"number": float64(7)}, "smyklot[bot]",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !found || request.Label != LabelPendingCIMerge || !state.labelRestored {
		t.Fatalf(
			"found=%t label=%q restored=%t, want recovered merge request",
			found, request.Label, state.labelRestored,
		)
	}
	if state.repairReadyID != 0 || state.repairClaimID != 0 {
		t.Fatalf(
			"successful repair left signals ready=%d claim=%d",
			state.repairReadyID, state.repairClaimID,
		)
	}
}

func TestActionPendingCIRepairLabelFailureLeavesDurableRetry(t *testing.T) {
	t.Parallel()
	state := &actionPendingCIPublishFailureState{
		nextRepairSignalID: 70, labelRestoreStatus: http.StatusInternalServerError,
	}
	server := httptest.NewServer(actionPendingCIPublishFailureHandler(state))
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	err = repairActionPendingCILabel(
		t.Context(), client, draftMergeConfig(), "acme", "web", 7,
		github.MergeMethodMerge, false, "smyklot[bot]", LabelPendingCIMerge,
		actionPendingCIArtifactExclusion{}, errors.New("publication failed"),
	)
	if err == nil {
		t.Fatal("label restoration failure was hidden")
	}
	assertActionPendingCIReadyOnly(t, state)
}

func TestActionPendingCIRepairAuthorizationFailureLeavesDurableRetry(t *testing.T) {
	t.Parallel()
	state := &actionPendingCIPublishFailureState{
		nextRepairSignalID: 70, eventFailures: 3,
	}
	server := httptest.NewServer(actionPendingCIPublishFailureHandler(state))
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	err = repairActionPendingCILabel(
		t.Context(), client, draftMergeConfig(), "acme", "web", 7,
		github.MergeMethodMerge, false, "smyklot[bot]", LabelPendingCIMerge,
		actionPendingCIArtifactExclusion{}, errors.New("publication failed"),
	)
	if err == nil {
		t.Fatal("draft-history failure was hidden")
	}
	assertActionPendingCIReadyOnly(t, state)
}

func TestRestorePendingCILabelFailureLeavesDurableRetry(t *testing.T) {
	t.Parallel()
	state := &actionPendingCIPublishFailureState{
		nextRepairSignalID: 70, labelRestoreStatus: http.StatusInternalServerError,
	}
	server := httptest.NewServer(actionPendingCIPublishFailureHandler(state))
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = restorePendingCILabel(
		t.Context(), client, "acme", "web", 7, LabelPendingCIMerge,
		errors.New("concurrent command survived"),
	)
	if err == nil {
		t.Fatal("concurrent label restoration failure was hidden")
	}
	assertActionPendingCIReadyOnly(t, state)
}

func assertActionPendingCIReadyOnly(
	t *testing.T,
	state *actionPendingCIPublishFailureState,
) {
	t.Helper()
	if state.labelRestored || state.repairReadyID == 0 || state.repairClaimID != 0 {
		t.Fatalf(
			"labelRestored=%t ready=%d claim=%d, want only a durable ready signal",
			state.labelRestored, state.repairReadyID, state.repairClaimID,
		)
	}
}

func TestActionPendingCIRecoveryDoesNotClearConcurrentRepairSignal(t *testing.T) {
	t.Parallel()
	state := &actionPendingCIPublishFailureState{
		repairReadyID: 70, nextRepairSignalID: 71, signalOnLabel: true,
	}
	server := httptest.NewServer(actionPendingCIPublishFailureHandler(state))
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, found, err := recoverActionPendingCIRepair(
		t.Context(), client, draftMergeConfig(), "acme", "web",
		map[string]interface{}{"number": float64(7)}, "smyklot[bot]",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !found || state.repairReadyID != 72 || state.repairClaimID != 0 {
		t.Fatalf(
			"found=%t ready=%d claim=%d, want concurrent ready signal 72 preserved",
			found, state.repairReadyID, state.repairClaimID,
		)
	}
}

func TestActionPendingCIRecoveryKeepsReadySignalWhenClaimCreationFails(t *testing.T) {
	t.Parallel()
	state := &actionPendingCIPublishFailureState{
		repairReadyID: 70, nextRepairSignalID: 71,
		claimCreationStatus: http.StatusForbidden,
	}
	server := httptest.NewServer(actionPendingCIPublishFailureHandler(state))
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, found, err := recoverActionPendingCIRepair(
		t.Context(), client, draftMergeConfig(), "acme", "web",
		map[string]interface{}{"number": float64(7)}, "smyklot[bot]",
	)
	if err == nil || found {
		t.Fatalf("found=%t error=%v, want claim creation failure", found, err)
	}
	if state.repairReadyID != 70 || state.repairClaimID != 0 {
		t.Fatalf(
			"ready=%d claim=%d, want original ready signal preserved",
			state.repairReadyID, state.repairClaimID,
		)
	}
}

func TestActionPendingCIRecoveryFinishesInterruptedClaimHandoff(t *testing.T) {
	t.Parallel()
	state := &actionPendingCIPublishFailureState{
		repairReadyID: 70, repairClaimID: 71, nextRepairSignalID: 72,
	}
	server := httptest.NewServer(actionPendingCIPublishFailureHandler(state))
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, found, err := recoverActionPendingCIRepair(
		t.Context(), client, draftMergeConfig(), "acme", "web",
		map[string]interface{}{"number": float64(7)}, "smyklot[bot]",
	)
	if err != nil || !found {
		t.Fatalf("found=%t error=%v, want interrupted handoff recovered", found, err)
	}
	if state.repairReadyID != 0 || state.repairClaimID != 0 {
		t.Fatalf(
			"ready=%d claim=%d, want completed handoff cleared",
			state.repairReadyID, state.repairClaimID,
		)
	}
}

func TestActionPendingCIRecoveryFailureKeepsClaimForRetry(t *testing.T) {
	t.Parallel()
	state := &actionPendingCIPublishFailureState{
		repairReadyID: 70, nextRepairSignalID: 71, scanFailures: 3,
	}
	server := httptest.NewServer(actionPendingCIPublishFailureHandler(state))
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	pr := map[string]interface{}{"number": float64(7)}
	_, found, err := recoverActionPendingCIRepair(
		t.Context(), client, draftMergeConfig(), "acme", "web", pr, "smyklot[bot]",
	)
	if err == nil || found {
		t.Fatalf("found=%t error=%v, want transient recovery failure", found, err)
	}
	if state.repairReadyID != 0 || state.repairClaimID != 71 {
		t.Fatalf(
			"ready=%d claim=%d, want durable claim 71",
			state.repairReadyID, state.repairClaimID,
		)
	}
	_, found, err = recoverActionPendingCIRepair(
		t.Context(), client, draftMergeConfig(), "acme", "web", pr, "smyklot[bot]",
	)
	if err != nil || !found {
		t.Fatalf("found=%t error=%v, want retry to recover", found, err)
	}
	if state.repairReadyID != 0 || state.repairClaimID != 0 {
		t.Fatalf(
			"ready=%d claim=%d, want successful retry to clear claim",
			state.repairReadyID, state.repairClaimID,
		)
	}
}

type actionPendingCIPublishFailureState struct {
	labelRestored       bool
	signalOnLabel       bool
	repairReadyID       int64
	repairClaimID       int64
	nextRepairSignalID  int64
	claimCreationStatus int
	labelRestoreStatus  int
	scanFailures        int
	eventFailures       int
}

func actionPendingCIPublishFailureHandler(
	state *actionPendingCIPublishFailureState,
) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		if strings.HasPrefix(request.URL.Path, "/repos/acme/web/issues/7/reactions") {
			handleActionPendingCIRepairRequest(w, request, state)

			return
		}
		switch {
		case request.Method == http.MethodDelete &&
			request.URL.Path == "/repos/acme/web/issues/7/labels/smyklot:pending:ci":
			w.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodGet &&
			request.URL.Path == "/repos/acme/web/issues/comments/99/reactions":
			_, _ = w.Write([]byte(`[]`))
		case request.Method == http.MethodPost &&
			request.URL.Path == "/repos/acme/web/issues/comments/99/reactions":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"reaction failed"}`))
		case request.Method == http.MethodGet &&
			request.URL.Path == "/repos/acme/web/issues/7/comments":
			if state.scanFailures > 0 {
				state.scanFailures--
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"message":"try again"}`))

				return
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"id": 88, "body": "/merge after ci", "updated_at": "2026-08-25T08:04:00Z",
			}})
		case request.Method == http.MethodGet &&
			request.URL.Path == "/repos/acme/web/issues/comments/88/reactions":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id": 51, "content": "eyes", "created_at": "2026-08-25T08:04:01Z",
					"user": map[string]any{"login": "smyklot[bot]"},
				},
				{
					"id": 52, "content": "hooray", "created_at": "2026-08-25T08:04:02Z",
					"user": map[string]any{"login": "smyklot[bot]"},
				},
			})
		case request.Method == http.MethodGet &&
			request.URL.Path == "/repos/acme/web/issues/7/events":
			if state.eventFailures > 0 {
				state.eventFailures--
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"message":"try again"}`))

				return
			}
			_, _ = w.Write([]byte(`[]`))
		case request.Method == http.MethodPost &&
			request.URL.Path == "/repos/acme/web/issues/7/labels":
			recordActionPendingCILabelRestore(w, state)
		case request.Method == http.MethodGet &&
			request.URL.Path == "/repos/acme/web/pulls/7":
			writeActionPendingCIPublishPullState(w, state)
		default:
			http.NotFound(w, request)
		}
	}
}

func recordActionPendingCILabelRestore(
	w http.ResponseWriter,
	state *actionPendingCIPublishFailureState,
) {
	if state.labelRestoreStatus != 0 {
		w.WriteHeader(state.labelRestoreStatus)
		_, _ = w.Write([]byte(`{"message":"label rejected"}`))

		return
	}
	state.labelRestored = true
	if state.signalOnLabel && state.repairReadyID == 0 {
		state.repairReadyID = state.nextRepairSignalID
		state.nextRepairSignalID++
	}
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`[]`))
}

func writeActionPendingCIPublishPullState(
	w http.ResponseWriter,
	state *actionPendingCIPublishFailureState,
) {
	labels := []map[string]any{}
	if state.labelRestored {
		labels = append(labels, map[string]any{"name": LabelPendingCIMerge})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"number": 7, "state": "open", "draft": false,
		"head": map[string]any{"sha": "head"}, "labels": labels,
	})
}

func handleActionPendingCIRepairRequest(
	w http.ResponseWriter,
	request *http.Request,
	state *actionPendingCIPublishFailureState,
) {
	switch request.Method {
	case http.MethodPost:
		var body map[string]string
		_ = json.NewDecoder(request.Body).Decode(&body)
		reactionType := github.ReactionType(body["content"])
		if reactionType == ReactionPendingCIRepairClaim &&
			state.claimCreationStatus != 0 {
			w.WriteHeader(state.claimCreationStatus)
			_, _ = w.Write([]byte(`{"message":"claim rejected"}`))

			return
		}
		reactionID := actionPendingCIRepairReactionID(state, reactionType)
		w.WriteHeader(http.StatusCreated)
		writeActionPendingCIRepairCreation(w, reactionID, reactionType)
	case http.MethodGet:
		writeActionPendingCIRepairSignals(
			w, state.repairReadyID, state.repairClaimID,
		)
	case http.MethodDelete:
		deleteActionPendingCIRepairSignal(w, request, state)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func actionPendingCIRepairReactionID(
	state *actionPendingCIPublishFailureState,
	reactionType github.ReactionType,
) int64 {
	current := &state.repairReadyID
	if reactionType == ReactionPendingCIRepairClaim {
		current = &state.repairClaimID
	}
	if *current == 0 {
		*current = state.nextRepairSignalID
		state.nextRepairSignalID++
	}

	return *current
}

func writeActionPendingCIRepairSignals(w http.ResponseWriter, readyID, claimID int64) {
	reactions := make([]map[string]any, 0, 2)
	if readyID != 0 {
		reactions = append(reactions, actionPendingCIRepairReaction(
			readyID, ReactionPendingCIRepair,
		))
	}
	if claimID != 0 {
		reactions = append(reactions, actionPendingCIRepairReaction(
			claimID, ReactionPendingCIRepairClaim,
		))
	}
	_ = json.NewEncoder(w).Encode(reactions)
}

func actionPendingCIRepairReaction(
	reactionID int64,
	reactionType github.ReactionType,
) map[string]any {
	return map[string]any{
		"id": reactionID, "content": string(reactionType),
		"created_at": "2026-08-25T08:05:01Z",
		"user":       map[string]any{"login": "smyklot[bot]"},
	}
}

func writeActionPendingCIRepairCreation(
	w http.ResponseWriter,
	reactionID int64,
	reactionType github.ReactionType,
) {
	_ = json.NewEncoder(w).Encode(actionPendingCIRepairReaction(
		reactionID, reactionType,
	))
}

func deleteActionPendingCIRepairSignal(
	w http.ResponseWriter,
	request *http.Request,
	state *actionPendingCIPublishFailureState,
) {
	reactionID, _ := strconv.ParseInt(path.Base(request.URL.Path), 10, 64)
	switch reactionID {
	case state.repairReadyID:
		state.repairReadyID = 0
	case state.repairClaimID:
		state.repairClaimID = 0
	default:
		http.NotFound(w, request)

		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func TestActionPendingCIRepairPreservesNewerGenerationOnSameComment(t *testing.T) {
	t.Parallel()
	labelRestored := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodDelete &&
			request.URL.Path == "/repos/acme/web/issues/7/labels/smyklot:pending:ci":
			w.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodGet &&
			request.URL.Path == "/repos/acme/web/issues/7/comments":
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"id": 99, "body": "/merge after ci", "updated_at": "2026-08-25T08:04:00Z",
			}})
		case request.Method == http.MethodGet &&
			request.URL.Path == "/repos/acme/web/issues/comments/99/reactions":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id": 61, "content": "eyes", "created_at": "2026-08-25T08:04:01Z",
					"user": map[string]any{"login": "smyklot[bot]"},
				},
				{
					"id": 62, "content": "hooray", "created_at": "2026-08-25T08:04:02Z",
					"user": map[string]any{"login": "smyklot[bot]"},
				},
			})
		case request.Method == http.MethodGet &&
			request.URL.Path == "/repos/acme/web/issues/7/events":
			_, _ = w.Write([]byte(`[]`))
		case request.Method == http.MethodPost &&
			request.URL.Path == "/repos/acme/web/issues/7/labels":
			labelRestored = true
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`[]`))
		case request.Method == http.MethodGet &&
			request.URL.Path == "/repos/acme/web/issues/7/reactions":
			_, _ = w.Write([]byte(`[]`))
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	err = repairActionPendingCILabel(
		t.Context(), client, draftMergeConfig(), "acme", "web", 7,
		github.MergeMethodMerge, false, "smyklot[bot]", LabelPendingCIMerge,
		actionPendingCIArtifactExclusion{commentID: 99, fenceID: 41},
		errors.New("older publication failed"),
	)
	if err == nil {
		t.Fatal("repair hid the original publication failure")
	}
	if !labelRestored {
		t.Fatal("older publication stranded a newer generation on the same comment")
	}
}

func TestActionPendingCILegacyCancellationNeverMintsEditedRevisionMarker(t *testing.T) {
	t.Parallel()
	for _, allowDraftMerges := range []bool{false, true} {
		t.Run(fmt.Sprintf("allow_draft_merges=%t", allowDraftMerges), func(t *testing.T) {
			t.Parallel()
			assertActionPendingCILegacyCancellation(t, allowDraftMerges)
		})
	}
}

type legacyActionPendingCITestState struct {
	pendingPresent bool
	markerCreated  bool
	labelDeleted   bool
}

func assertActionPendingCILegacyCancellation(t *testing.T, allowDraftMerges bool) {
	t.Helper()
	state := &legacyActionPendingCITestState{pendingPresent: true}
	server := httptest.NewServer(legacyActionPendingCIHandler(state))
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	botConfig := config.Default()
	botConfig.AllowDraftMerges = allowDraftMerges

	cancelled, err := reconcileDraftPendingCI(
		t.Context(), client, botConfig, "acme", "web", 7,
		PendingCIPR{Method: github.MergeMethodMerge, Label: LabelPendingCIMerge},
		"smyklot[bot]", false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !cancelled || !state.labelDeleted || state.pendingPresent {
		t.Fatalf(
			"cancelled=%t labelDeleted=%t pendingPresent=%t",
			cancelled, state.labelDeleted, state.pendingPresent,
		)
	}
	if state.markerCreated {
		t.Fatal("legacy cancellation minted an authorization marker from an edited comment")
	}
}

func legacyActionPendingCIHandler(state *legacyActionPendingCITestState) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/web/pulls/7":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"number": 7, "state": "open", "draft": false,
				"head": map[string]any{"sha": "head"},
			})
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/web/issues/7/comments":
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"id": 99, "body": "/merge after ci", "updated_at": "2026-08-25T08:02:00Z",
			}})
		case request.Method == http.MethodGet &&
			request.URL.Path == "/repos/acme/web/issues/comments/99/reactions":
			writeLegacyActionPendingCIReactions(w, state.pendingPresent)
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/web/issues/7/events":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id": 1, "event": "labeled", "created_at": "2026-08-25T08:00:00Z",
					"label": map[string]any{"name": LabelPendingCIMerge},
					"actor": map[string]any{"login": "smyklot[bot]"},
				},
				{"id": 2, "event": "convert_to_draft", "created_at": "2026-08-25T08:01:00Z"},
			})
		case request.Method == http.MethodDelete &&
			request.URL.Path == "/repos/acme/web/issues/7/labels/smyklot:pending:ci":
			state.labelDeleted = true
			w.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodDelete &&
			request.URL.Path == "/repos/acme/web/issues/comments/99/reactions/42":
			state.pendingPresent = false
			w.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodPost &&
			request.URL.Path == "/repos/acme/web/issues/comments/99/reactions":
			writeLegacyActionPendingCIReaction(w, request, state)
		default:
			http.NotFound(w, request)
		}
	}
}

func writeLegacyActionPendingCIReactions(w http.ResponseWriter, pending bool) {
	if !pending {
		_, _ = w.Write([]byte(`[]`))

		return
	}
	_ = json.NewEncoder(w).Encode([]map[string]any{{
		"id": 42, "content": "eyes", "created_at": "2026-08-25T08:00:01Z",
		"user": map[string]any{"login": "smyklot[bot]"},
	}})
}

func writeLegacyActionPendingCIReaction(
	w http.ResponseWriter,
	request *http.Request,
	state *legacyActionPendingCITestState,
) {
	var body map[string]string
	_ = json.NewDecoder(request.Body).Decode(&body)
	state.markerCreated = state.markerCreated || body["content"] == "hooray"
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": 43, "content": body["content"], "created_at": "2026-08-25T08:03:00Z",
	})
}

func TestActionPendingCICancellationRestoresConcurrentNewerCommand(t *testing.T) {
	t.Parallel()
	commentReads := 0
	labelRestored := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodDelete &&
			request.URL.Path == "/repos/acme/web/issues/7/labels/smyklot:pending:ci":
			w.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/web/issues/7/comments":
			commentReads++
			if commentReads <= 2 {
				_, _ = w.Write([]byte(`[]`))

				return
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"id": 202, "body": "/merge after ci", "updated_at": "2026-08-25T08:04:00Z",
			}})
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/web/pulls/7":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"number": 7, "state": "open", "draft": false,
				"head": map[string]any{"sha": "head"},
			})
		case request.Method == http.MethodGet &&
			request.URL.Path == "/repos/acme/web/issues/comments/202/reactions":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id": 51, "content": "eyes", "created_at": "2026-08-25T08:04:01Z",
					"user": map[string]any{"login": "smyklot[bot]"},
				},
				{
					"id": 52, "content": "hooray", "created_at": "2026-08-25T08:04:02Z",
					"user": map[string]any{"login": "smyklot[bot]"},
				},
			})
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/web/issues/7/events":
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"id": 2, "event": "convert_to_draft", "created_at": "2026-08-25T08:03:00Z",
			}})
		case request.Method == http.MethodPost && request.URL.Path == "/repos/acme/web/issues/7/labels":
			labelRestored = true
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`[]`))
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	botConfig := config.Default()
	botConfig.AllowDraftMerges = true
	cancelled, err := cancelDraftPendingCI(
		t.Context(), client, botConfig, "acme", "web", 7,
		PendingCIPR{Method: github.MergeMethodMerge, Label: LabelPendingCIMerge},
		"smyklot[bot]",
	)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled {
		t.Fatal("concurrent newer command was cancelled")
	}
	if !labelRestored {
		t.Fatal("concurrent newer command label was not restored")
	}
}

func TestActionPendingCICancellationRestoresLabelWhenFinalReadFails(t *testing.T) {
	t.Parallel()
	pullReads := 0
	labelRestored := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodDelete &&
			request.URL.Path == "/repos/acme/web/issues/7/labels/smyklot:pending:ci":
			w.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/web/issues/7/comments":
			_, _ = w.Write([]byte(`[]`))
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/web/pulls/7":
			pullReads++
			if pullReads == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"number": 7, "state": "open", "draft": false,
					"head": map[string]any{"sha": "head"},
					"base": map[string]any{"ref": "main"},
				})

				return
			}
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"message":"try again"}`))
		case request.Method == http.MethodPost && request.URL.Path == "/repos/acme/web/issues/7/labels":
			labelRestored = true
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`[]`))
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := cancelDraftPendingCI(
		t.Context(), client, draftMergeConfig(), "acme", "web", 7,
		PendingCIPR{Method: github.MergeMethodMerge, Label: LabelPendingCIMerge},
		"smyklot[bot]",
	)
	if err == nil {
		t.Fatal("final state read failure was hidden")
	}
	if cancelled {
		t.Fatal("uncertain request was reported as cancelled")
	}
	if labelRestored {
		t.Fatal("unproven label was restored as new authorization after an uncertain final read")
	}
}

func TestActionPendingCILegacyCancellationErrorDoesNotReauthorizeLabel(t *testing.T) {
	t.Parallel()
	for _, allowDraftMerges := range []bool{false, true} {
		t.Run(fmt.Sprintf("allow_draft_merges=%t", allowDraftMerges), func(t *testing.T) {
			t.Parallel()
			assertLegacyCancellationErrorDisarmsLabel(t, allowDraftMerges)
		})
	}
}

type legacyCancellationErrorTestState struct {
	pullReads     int
	scanFailures  int
	failFinalRead bool
	labelPresent  bool
	labelRestored bool
	repairReadyID int64
	repairClaimID int64
	merged        bool
}

func assertLegacyCancellationErrorDisarmsLabel(t *testing.T, allowDraftMerges bool) {
	t.Helper()
	state := &legacyCancellationErrorTestState{
		failFinalRead: true, labelPresent: true, scanFailures: 3,
	}
	server := httptest.NewServer(legacyCancellationErrorHandler(state))
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	botConfig := config.Default()
	botConfig.AllowDraftMerges = allowDraftMerges
	pr := PendingCIPR{Method: github.MergeMethodMerge, Label: LabelPendingCIMerge}

	cancelled, err := cancelDraftPendingCI(
		t.Context(), client, botConfig, "acme", "web", 7, pr, "smyklot[bot]",
	)
	if err == nil || cancelled {
		t.Fatalf("cancelled=%t error=%v, want uncertain failure", cancelled, err)
	}
	if state.labelPresent || state.labelRestored || state.repairReadyID == 0 ||
		state.repairClaimID != 0 {
		t.Fatalf(
			"present=%t restored=%t ready=%d claim=%d, want only a repair signal",
			state.labelPresent, state.labelRestored,
			state.repairReadyID, state.repairClaimID,
		)
	}
	state.failFinalRead = false
	_, found, err := recoverActionPendingCIRepair(
		t.Context(), client, botConfig, "acme", "web",
		map[string]interface{}{"number": float64(7)}, "smyklot[bot]",
	)
	if err != nil {
		t.Fatal(err)
	}
	if found || state.labelPresent || state.labelRestored ||
		state.repairReadyID != 0 || state.repairClaimID != 0 {
		t.Fatalf(
			"found=%t present=%t restored=%t ready=%d claim=%d after legacy recovery",
			found, state.labelPresent, state.labelRestored,
			state.repairReadyID, state.repairClaimID,
		)
	}
	pr.PRData = map[string]any{"number": float64(7)}
	if err = processPendingCIPR(
		t.Context(), client, botConfig, "acme", "web", pr, "smyklot[bot]",
	); err != nil {
		t.Fatal(err)
	}
	if state.merged {
		t.Fatal("legacy request merged on the poll after cancellation uncertainty")
	}
}

func legacyCancellationErrorHandler(state *legacyCancellationErrorTestState) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/web/pulls/7":
			writeLegacyCancellationState(w, state)
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/web/issues/7/comments":
			writeLegacyCancellationComments(w, state)
		case request.Method == http.MethodGet &&
			request.URL.Path == "/repos/acme/web/issues/comments/99/reactions":
			writeLegacyActionPendingCIReactions(w, true)
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/web/issues/7/events":
			writeLegacyCancellationEvents(w, state.labelRestored)
		case request.Method == http.MethodDelete &&
			request.URL.Path == "/repos/acme/web/issues/7/labels/smyklot:pending:ci":
			state.labelPresent = false
			w.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodPost && request.URL.Path == "/repos/acme/web/issues/7/labels":
			state.labelPresent = true
			state.labelRestored = true
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`[]`))
		case request.Method == http.MethodPost &&
			request.URL.Path == "/repos/acme/web/issues/7/reactions":
			var body map[string]string
			_ = json.NewDecoder(request.Body).Decode(&body)
			reactionType := github.ReactionType(body["content"])
			reactionID := int64(70)
			if reactionType == ReactionPendingCIRepairClaim {
				reactionID = 71
				state.repairClaimID = reactionID
			} else {
				state.repairReadyID = reactionID
			}
			w.WriteHeader(http.StatusCreated)
			writeActionPendingCIRepairCreation(w, reactionID, reactionType)
		case request.Method == http.MethodGet &&
			request.URL.Path == "/repos/acme/web/issues/7/reactions":
			writeActionPendingCIRepairSignals(
				w, state.repairReadyID, state.repairClaimID,
			)
		case request.Method == http.MethodDelete &&
			request.URL.Path == "/repos/acme/web/issues/7/reactions/70":
			state.repairReadyID = 0
			w.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodDelete &&
			request.URL.Path == "/repos/acme/web/issues/7/reactions/71":
			state.repairClaimID = 0
			w.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodPut && request.URL.Path == "/repos/acme/web/pulls/7/merge":
			state.merged = true
			_ = json.NewEncoder(w).Encode(map[string]any{"merged": true})
		default:
			http.NotFound(w, request)
		}
	}
}

func writeLegacyCancellationState(
	w http.ResponseWriter,
	state *legacyCancellationErrorTestState,
) {
	state.pullReads++
	if state.pullReads > 1 && state.failFinalRead {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"message":"try again"}`))

		return
	}
	labels := []map[string]any{}
	if state.labelPresent {
		labels = append(labels, map[string]any{"name": LabelPendingCIMerge})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"number": 7, "state": "open", "draft": false,
		"head": map[string]any{"sha": "head"}, "labels": labels,
	})
}

func writeLegacyCancellationComments(
	w http.ResponseWriter,
	state *legacyCancellationErrorTestState,
) {
	if !state.labelPresent && state.scanFailures > 0 {
		state.scanFailures--
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"message":"try again"}`))

		return
	}
	_ = json.NewEncoder(w).Encode([]map[string]any{{
		"id": 99, "body": "/merge after ci", "updated_at": "2026-08-25T08:02:00Z",
	}})
}

func writeLegacyCancellationEvents(w http.ResponseWriter, restored bool) {
	events := []map[string]any{
		{
			"id": 1, "event": "labeled", "created_at": "2026-08-25T08:00:00Z",
			"label": map[string]any{"name": LabelPendingCIMerge},
			"actor": map[string]any{"login": "smyklot[bot]"},
		},
		{"id": 2, "event": "convert_to_draft", "created_at": "2026-08-25T08:01:00Z"},
	}
	if restored {
		events = append(events, map[string]any{
			"id": 3, "event": "labeled", "created_at": "2026-08-25T08:02:00Z",
			"label": map[string]any{"name": LabelPendingCIMerge},
			"actor": map[string]any{"login": "smyklot[bot]"},
		})
	}
	_ = json.NewEncoder(w).Encode(events)
}

type actionPendingCILabelStub struct {
	calls     []string
	removeErr error
}

func (stub *actionPendingCILabelStub) AddLabel(
	context.Context, string, string, int, string,
) error {
	stub.calls = append(stub.calls, "add")

	return nil
}

func (stub *actionPendingCILabelStub) RemoveLabel(
	context.Context, string, string, int, string,
) error {
	stub.calls = append(stub.calls, "remove")

	return stub.removeErr
}

type draftHistoryStub struct {
	draftedAt time.Time
	found     bool
	err       error
}

func (stub draftHistoryStub) LatestPullRequestDraftTransition(
	context.Context,
	string,
	string,
	int,
) (time.Time, bool, error) {
	return stub.draftedAt, stub.found, stub.err
}

type draftMergeStub struct {
	responses  []*github.PRInfo
	infoCalls  int
	readyCalls int
}

func (stub *draftMergeStub) GetPRInfo(
	context.Context, string, string, int,
) (*github.PRInfo, error) {
	response := stub.responses[stub.infoCalls]
	stub.infoCalls++

	return response, nil
}

func (stub *draftMergeStub) MarkPullRequestReadyForReview(
	context.Context, string, string, int,
) error {
	stub.readyCalls++

	return nil
}

func draftRuntime() *RuntimeConfig {
	return &RuntimeConfig{
		RepoOwner: "acme", RepoName: "web", CommentAuthor: "operator",
		CommentBody: "/merge", CommentRevision: "2026-08-25T08:02:00Z",
		BotUsername: "smyklot[bot]",
	}
}

func draftMergeConfig() *config.Config {
	botConfig := config.Default()
	botConfig.AllowDraftMerges = true

	return botConfig
}

func draftMergeServer(
	t *testing.T,
	expectedMethod github.MergeMethod,
) (*httptest.Server, *[]string) {
	t.Helper()
	var mu sync.Mutex
	requests := []string{}
	pullReads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		mu.Lock()
		requests = append(requests, request.Method+" "+request.URL.Path)
		mu.Unlock()

		switch {
		case request.URL.Path == "/repos/acme/web/pulls/7" && request.Method == http.MethodGet:
			pullReads++
			draft := pullReads < 3
			_ = json.NewEncoder(w).Encode(map[string]any{
				"number": 7, "node_id": "PR_node", "state": "open", "draft": draft,
				"mergeable": !draft, "mergeable_state": map[bool]string{true: "blocked", false: "clean"}[draft],
				"user": map[string]any{"login": "author"},
				"head": map[string]any{"sha": "head"},
			})
		case request.URL.Path == "/repos/acme/web/pulls/7/reviews":
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"state": "APPROVED", "user": map[string]any{"login": "operator"},
			}})
		case request.URL.Path == "/graphql":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"markPullRequestReadyForReview": map[string]any{"clientMutationId": nil},
			}})
		case request.URL.Path == "/repos/acme/web/pulls/7/merge":
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["merge_method"] != string(expectedMethod) {
				t.Errorf("merge method = %v, want %s", body["merge_method"], expectedMethod)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"merged": true})
		case request.URL.Path == "/repos/acme/web/issues/comments/99":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 99, "body": "/merge", "updated_at": "2026-08-25T08:02:00Z",
				"user": map[string]any{"login": "operator"},
			})
		case request.URL.Path == "/repos/acme/web/issues/7/events":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": 1, "event": "convert_to_draft", "created_at": "2026-08-25T08:00:00Z"},
			})
		case request.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 1})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	return server, &requests
}

func draftBlockedServer(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var mu sync.Mutex
	requests := []string{}
	pullReads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		mu.Lock()
		requests = append(requests, request.Method+" "+request.URL.Path)
		mu.Unlock()

		switch {
		case request.URL.Path == "/repos/acme/web/pulls/7" && request.Method == http.MethodGet:
			pullReads++
			draft := pullReads < 3
			_ = json.NewEncoder(w).Encode(map[string]any{
				"number": 7, "node_id": "PR_node", "state": "open", "draft": draft,
				"mergeable": false, "mergeable_state": "blocked",
				"user": map[string]any{"login": "author"},
				"head": map[string]any{"sha": "head"},
			})
		case request.URL.Path == "/repos/acme/web/pulls/7/reviews" &&
			request.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case request.URL.Path == "/repos/acme/web/pulls/7/reviews" &&
			request.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":1}`))
		case request.URL.Path == "/graphql":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"markPullRequestReadyForReview": map[string]any{"clientMutationId": nil},
				"enablePullRequestAutoMerge":    map[string]any{"clientMutationId": nil},
			}})
		case request.URL.Path == "/repos/acme/web/issues/7/comments" &&
			request.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":1}`))
		case request.URL.Path == "/repos/acme/web/issues/comments/99":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 99, "body": "/merge", "updated_at": "2026-08-25T08:02:00Z",
				"user": map[string]any{"login": "operator"},
			})
		case request.URL.Path == "/repos/acme/web/issues/7/events":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": 1, "event": "convert_to_draft", "created_at": "2026-08-25T08:00:00Z"},
			})
		case request.URL.Path == "/repos/acme/web/issues/comments/99/reactions" &&
			request.Method == http.MethodGet:
			_, _ = w.Write([]byte(`[]`))
		case request.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	return server, &requests
}

func assertApprovalBeforeAutoMerge(t *testing.T, requests []string) {
	t.Helper()
	approval := -1
	autoMerge := -1
	graphqlCalls := 0
	for index, request := range requests {
		switch request {
		case "POST /repos/acme/web/pulls/7/reviews":
			approval = index
		case "POST /graphql":
			graphqlCalls++
			if graphqlCalls == 2 {
				autoMerge = index
			}
		}
	}
	if approval < 0 || autoMerge < 0 || approval >= autoMerge {
		t.Fatalf("approval=%d auto-merge=%d requests=%v", approval, autoMerge, requests)
	}
}

func assertRequestOrder(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) < len(want) {
		t.Fatalf("requests = %v, want prefix %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("request %d = %q, want %q; all=%v", index, got[index], want[index], got)
		}
	}
}
