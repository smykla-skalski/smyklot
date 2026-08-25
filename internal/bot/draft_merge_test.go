package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
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

func TestActionPendingCIMarkerBindsTheExactCommentRevision(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name          string
		markerAt      string
		wantCancelled bool
	}{
		{name: "marker predates edited comment", markerAt: "2026-08-25T08:02:00Z", wantCancelled: true},
		{name: "same second is ambiguous", markerAt: "2026-08-25T08:04:00Z", wantCancelled: true},
		{name: "marker follows exact revision", markerAt: "2026-08-25T08:04:01Z"},
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
							"id": 41, "content": "hooray", "created_at": test.markerAt,
							"user": map[string]any{"login": "smyklot[bot]"},
						},
						{"id": 42, "content": "eyes", "user": map[string]any{"login": "smyklot[bot]"}},
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/repos/acme/web/issues/7/events":
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"id": 2, "event": "convert_to_draft", "created_at": "2026-08-25T08:03:00Z",
			}})
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
	result, err := executeImmediateMerge(
		t.Context(), client, draftRuntime(), botConfig, 7, github.MergeMethodMerge,
		&github.PRInfo{Author: "author", ApprovedBy: []string{"operator"}},
		"head", "2026-08-25T08:02:00Z",
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
					"id": 41, "content": "hooray", "created_at": "2026-08-25T08:02:01Z",
					"user": map[string]any{"login": "smyklot[bot]"},
				},
				{"id": 42, "content": "eyes", "user": map[string]any{"login": "smyklot[bot]"}},
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
				"GET /repos/acme/web/pulls/7",
				"POST /graphql",
				"GET /repos/acme/web/pulls/7",
				"GET /repos/acme/web/pulls/7/reviews",
				"GET /repos/acme/web/issues/7/events",
				"GET /repos/acme/web/issues/7/events",
				"PUT /repos/acme/web/pulls/7/merge",
			})
		})
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

func TestActionPendingCICommandRotatesItsAuthorizationLabel(t *testing.T) {
	labels := &actionPendingCILabelStub{}
	err := rotateActionPendingCILabel(
		t.Context(), labels, "acme", "web", 7, LabelPendingCIMerge,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(labels.calls) != 2 || labels.calls[0] != "remove" || labels.calls[1] != "add" {
		t.Fatalf("label operations = %v, want [remove add]", labels.calls)
	}
}

func TestActionPendingCICommandAddsMissingAuthorizationLabel(t *testing.T) {
	labels := &actionPendingCILabelStub{removeErr: github.NewAPIError(
		github.ErrAPIRequest, http.StatusNotFound, http.MethodDelete, "/labels/pending", nil,
	)}
	err := rotateActionPendingCILabel(
		t.Context(), labels, "acme", "web", 7, LabelPendingCIMerge,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(labels.calls) != 2 || labels.calls[0] != "remove" || labels.calls[1] != "add" {
		t.Fatalf("label operations = %v, want [remove add]", labels.calls)
	}
}

func TestActionPendingCIRevalidationRollsBackWhenPullRequestReturnsToDraft(t *testing.T) {
	t.Parallel()
	requests := make([]string, 0, 5)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests = append(requests, request.Method+" "+request.URL.Path)
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/web/pulls/7":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"number": 7, "state": "open", "draft": true,
				"head": map[string]any{"sha": "head"},
				"base": map[string]any{"ref": "main"},
			})
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/web/pulls/7/reviews":
			_, _ = w.Write([]byte(`[]`))
		case request.Method == http.MethodGet &&
			request.URL.Path == "/repos/acme/web/issues/comments/99":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 99, "body": "/merge after ci", "updated_at": "2026-08-25T08:00:00Z",
			})
		case request.Method == http.MethodGet &&
			request.URL.Path == "/repos/acme/web/issues/comments/99/reactions":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id": 41, "content": "hooray", "created_at": "2026-08-25T08:02:01Z",
					"user": map[string]any{"login": "smyklot[bot]"},
				},
				{"id": 42, "content": "eyes", "user": map[string]any{"login": "smyklot[bot]"}},
			})
		case request.Method == http.MethodDelete &&
			request.URL.Path == "/repos/acme/web/issues/7/labels/smyklot:pending:ci":
			w.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/web/issues/7/comments":
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"id": 99, "body": "/merge after ci", "updated_at": "2026-08-25T08:00:00Z",
			}})
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/web/issues/7/events":
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"id": 7, "event": "convert_to_draft", "created_at": "2026-08-25T08:00:01Z",
			}})
		case request.Method == http.MethodDelete &&
			(request.URL.Path == "/repos/acme/web/issues/comments/99/reactions/41" ||
				request.URL.Path == "/repos/acme/web/issues/comments/99/reactions/42"):
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}

	err = revalidateActionPendingCI(
		t.Context(), client, draftRuntime(), draftMergeConfig(), 7, 99, 41,
		LabelPendingCIMerge, "2026-08-25T08:00:00Z",
	)
	if !errors.Is(err, pendingci.ErrStaleSourceRevision) {
		t.Fatalf("error = %v, want stale source", err)
	}
	if !slices.Contains(requests, "DELETE /repos/acme/web/issues/comments/99/reactions/41") {
		t.Fatalf("pending command marker was not rolled back: %v", requests)
	}
	if !slices.Contains(requests, "DELETE /repos/acme/web/issues/7/labels/smyklot:pending:ci") {
		t.Fatalf("stale pending label was not reconciled: %v", requests)
	}
}

func TestActionPendingCIStaleWorkflowPreservesNewerCommentArtifact(t *testing.T) {
	t.Parallel()
	deletedReaction := false
	labelDeleted := false
	labelRestored := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/web/pulls/7":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"number": 7, "state": "open", "draft": false,
				"head": map[string]any{"sha": "head"},
			})
		case request.Method == http.MethodGet &&
			request.URL.Path == "/repos/acme/web/issues/comments/99":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 99, "body": "/merge after ci", "updated_at": "2026-08-25T08:02:00Z",
			})
		case request.Method == http.MethodDelete &&
			request.URL.Path == "/repos/acme/web/issues/7/labels/smyklot:pending:ci":
			labelDeleted = true
			w.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/web/issues/7/comments":
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"id": 99, "body": "/merge after ci", "updated_at": "2026-08-25T08:02:00Z",
			}})
		case request.Method == http.MethodGet &&
			request.URL.Path == "/repos/acme/web/issues/comments/99/reactions":
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"id": 51, "content": "hooray", "created_at": "2026-08-25T08:02:01Z",
				"user": map[string]any{"login": "smyklot[bot]"},
			}})
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/web/issues/7/events":
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"id": 5, "event": "convert_to_draft", "created_at": "2026-08-25T08:01:00Z",
			}})
		case request.Method == http.MethodPost && request.URL.Path == "/repos/acme/web/issues/7/labels":
			labelRestored = true
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`[]`))
		case request.Method == http.MethodDelete &&
			request.URL.Path == "/repos/acme/web/issues/comments/99/reactions/41":
			w.WriteHeader(http.StatusNotFound)
		case request.Method == http.MethodDelete:
			deletedReaction = true
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	err = revalidateActionPendingCI(
		t.Context(), client, draftRuntime(), draftMergeConfig(), 7, 99, 41,
		LabelPendingCIMerge, "2026-08-25T08:00:00Z",
	)
	if !errors.Is(err, pendingci.ErrStaleSourceRevision) {
		t.Fatalf("error = %v, want stale source", err)
	}
	if deletedReaction {
		t.Fatal("stale workflow deleted a newer command artifact")
	}
	if !labelDeleted || !labelRestored {
		t.Fatalf("newer command label was not repaired: deleted=%t restored=%t", labelDeleted, labelRestored)
	}
}

func TestActionPendingCIStaleWorkflowRemovesItsOwnMarkerBeforeReconciliation(t *testing.T) {
	t.Parallel()
	markerPresent := true
	labelDeleted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet &&
			request.URL.Path == "/repos/acme/web/issues/comments/99":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 99, "body": "/merge after ci", "updated_at": "2026-08-25T08:02:00Z",
			})
		case request.Method == http.MethodDelete &&
			request.URL.Path == "/repos/acme/web/issues/comments/99/reactions/41":
			markerPresent = false
			w.WriteHeader(http.StatusNoContent)
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
			if !markerPresent {
				_, _ = w.Write([]byte(`[]`))

				return
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"id": 41, "content": "hooray", "created_at": "2026-08-25T08:03:00Z",
				"user": map[string]any{"login": "smyklot[bot]"},
			}})
		case request.Method == http.MethodDelete &&
			request.URL.Path == "/repos/acme/web/issues/7/labels/smyklot:pending:ci":
			labelDeleted = true
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}

	err = revalidateActionPendingCI(
		t.Context(), client, draftRuntime(), draftMergeConfig(), 7, 99, 41,
		LabelPendingCIMerge, "2026-08-25T08:00:00Z",
	)
	if !errors.Is(err, pendingci.ErrStaleSourceRevision) {
		t.Fatalf("error = %v, want stale source", err)
	}
	if markerPresent {
		t.Fatal("stale workflow marker survived exact cleanup")
	}
	if !labelDeleted {
		t.Fatal("stale workflow label survived reconciliation")
	}
}

func TestActionPendingCIRejectedWorkflowDisarmsUnownedLabel(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name             string
		allowDraftMerges bool
		deleteStatus     int
	}{
		{name: "disabled config after successful marker cleanup", deleteStatus: http.StatusNoContent},
		{
			name: "enabled config after failed marker cleanup", allowDraftMerges: true,
			deleteStatus: http.StatusInternalServerError,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertActionPendingCIRejectedWorkflowDisarms(
				t, test.allowDraftMerges, test.deleteStatus,
			)
		})
	}
}

type rejectedActionPendingCITestState struct {
	markerPresent bool
	labelPresent  bool
	labelRestored bool
	deleteStatus  int
}

func assertActionPendingCIRejectedWorkflowDisarms(
	t *testing.T,
	allowDraftMerges bool,
	deleteStatus int,
) {
	t.Helper()
	state := &rejectedActionPendingCITestState{
		markerPresent: true, labelPresent: true, deleteStatus: deleteStatus,
	}
	server := httptest.NewServer(rejectedActionPendingCIHandler(state))
	defer server.Close()
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	botConfig := config.Default()
	botConfig.AllowDraftMerges = allowDraftMerges

	err = revalidateActionPendingCI(
		t.Context(), client, draftRuntime(), botConfig, 7, 99, 41,
		LabelPendingCIMerge, "2026-08-25T08:00:00Z",
	)
	if !errors.Is(err, pendingci.ErrStaleSourceRevision) {
		t.Fatalf("error = %v, want stale source", err)
	}
	if state.labelPresent || state.labelRestored {
		t.Fatalf(
			"rejected workflow remained discoverable: labelPresent=%t restored=%t",
			state.labelPresent, state.labelRestored,
		)
	}
	if deleteStatus == http.StatusNoContent && state.markerPresent {
		t.Fatal("successful exact cleanup left the rejected marker")
	}
	if deleteStatus != http.StatusNoContent && !state.markerPresent {
		t.Fatal("failed exact cleanup unexpectedly removed the marker")
	}
}

func rejectedActionPendingCIHandler(state *rejectedActionPendingCITestState) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet &&
			request.URL.Path == "/repos/acme/web/issues/comments/99":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 99, "body": "/merge after ci", "updated_at": "2026-08-25T08:02:00Z",
			})
		case request.Method == http.MethodDelete &&
			request.URL.Path == "/repos/acme/web/issues/comments/99/reactions/41":
			w.WriteHeader(state.deleteStatus)
			if state.deleteStatus == http.StatusNoContent {
				state.markerPresent = false
			}
		case request.Method == http.MethodDelete &&
			request.URL.Path == "/repos/acme/web/issues/7/labels/smyklot:pending:ci":
			state.labelPresent = false
			w.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/web/issues/7/comments":
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"id": 99, "body": "/merge after ci", "updated_at": "2026-08-25T08:02:00Z",
			}})
		case request.Method == http.MethodGet &&
			request.URL.Path == "/repos/acme/web/issues/comments/99/reactions":
			writeRejectedActionPendingCIReactions(w, state.markerPresent)
		case request.Method == http.MethodPost && request.URL.Path == "/repos/acme/web/issues/7/labels":
			state.labelPresent = true
			state.labelRestored = true
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`[]`))
		default:
			http.NotFound(w, request)
		}
	}
}

func writeRejectedActionPendingCIReactions(w http.ResponseWriter, present bool) {
	if !present {
		_, _ = w.Write([]byte(`[]`))

		return
	}
	_ = json.NewEncoder(w).Encode([]map[string]any{{
		"id": 41, "content": "hooray", "created_at": "2026-08-25T08:03:00Z",
		"user": map[string]any{"login": "smyklot[bot]"},
	}})
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
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"id": 51, "content": "hooray", "created_at": "2026-08-25T08:04:01Z",
				"user": map[string]any{"login": "smyklot[bot]"},
			}})
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
	failFinalRead bool
	labelPresent  bool
	labelRestored bool
	merged        bool
}

func assertLegacyCancellationErrorDisarmsLabel(t *testing.T, allowDraftMerges bool) {
	t.Helper()
	state := &legacyCancellationErrorTestState{failFinalRead: true, labelPresent: true}
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
	if state.labelPresent || state.labelRestored {
		t.Fatal("legacy cancellation failure created a new authorization label")
	}
	state.failFinalRead = false
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
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/web/issues/7/comments":
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"id": 99, "body": "/merge after ci", "updated_at": "2026-08-25T08:02:00Z",
			}})
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
		case request.Method == http.MethodPut && request.URL.Path == "/repos/acme/web/pulls/7/merge":
			state.merged = true
			_ = json.NewEncoder(w).Encode(map[string]any{"merged": true})
		default:
			http.NotFound(w, request)
		}
	}
}

func writeLegacyCancellationEvents(w http.ResponseWriter, restored bool) {
	events := []map[string]any{
		{
			"id": 1, "event": "labeled", "created_at": "2026-08-25T08:00:00Z",
			"label": map[string]any{"name": LabelPendingCIMerge},
		},
		{"id": 2, "event": "convert_to_draft", "created_at": "2026-08-25T08:01:00Z"},
	}
	if restored {
		events = append(events, map[string]any{
			"id": 3, "event": "labeled", "created_at": "2026-08-25T08:02:00Z",
			"label": map[string]any{"name": LabelPendingCIMerge},
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
