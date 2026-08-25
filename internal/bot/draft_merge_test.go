package bot

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
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
	assertRequestOrder(t, (*requests)[:8], []string{
		"GET /repos/acme/web/pulls/7",
		"GET /repos/acme/web/pulls/7/reviews",
		"GET /repos/acme/web/issues/7/events",
		"GET /repos/acme/web/pulls/7",
		"POST /graphql",
		"GET /repos/acme/web/pulls/7",
		"GET /repos/acme/web/pulls/7/reviews",
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
			})
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/web/pulls/7/reviews":
			_, _ = w.Write([]byte(`[]`))
		case request.Method == http.MethodDelete &&
			request.URL.Path == "/repos/acme/web/issues/7/labels/smyklot:pending:ci:merge":
			w.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodGet &&
			request.URL.Path == "/repos/acme/web/issues/comments/99/reactions":
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"id": 41, "content": "eyes", "user": map[string]any{"login": "smyklot[bot]"},
			}})
		case request.Method == http.MethodDelete &&
			request.URL.Path == "/repos/acme/web/issues/comments/99/reactions/41":
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
		t.Context(), client, draftRuntime(), 7, 99,
		LabelPendingCIMerge, "2026-08-25T08:00:00Z",
	)
	if !errors.Is(err, pendingci.ErrStaleSourceRevision) {
		t.Fatalf("error = %v, want stale source", err)
	}
	if !slices.Contains(requests, "DELETE /repos/acme/web/issues/comments/99/reactions/41") {
		t.Fatalf("pending reaction was not rolled back: %v", requests)
	}
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
