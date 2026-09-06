package panel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/configsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

type configFileControllerProbe struct {
	previewCalls []string
	requests     []ConfigFileResolutionRequest
	err          error
}

func (probe *configFileControllerProbe) PreviewConfigurationFile(_ context.Context, target, repo string) (configsync.ConnectionPreview, error) {
	probe.previewCalls = append(probe.previewCalls, target+"/"+repo)
	return configsync.ConnectionPreview{
		Status: configsync.StatusBlocked, ReviewToken: strings.Repeat("a", 64),
		Choices: []configsync.PreviewChoice{{Side: "panel", Available: true, Document: json.RawMessage(`{"command_prefix":"/panel "}`)}},
	}, probe.err
}

func (probe *configFileControllerProbe) ResolveConfigurationFile(_ context.Context, request ConfigFileResolutionRequest) error {
	probe.requests = append(probe.requests, request)
	return probe.err
}

func configFileChoiceBody() string {
	return `{"review_token":"` + strings.Repeat("a", 64) + `","side":"panel"}`
}

func TestConfigFileReviewRoutesAuthorizeAndReturnOnlyPreparedResults(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	probe := &configFileControllerProbe{}
	harness.server.configFiles = probe
	for _, prefix := range []string{"/panel/api/v1/targets/", "/panel/api/v1/root/workspaces/"} {
		for _, repo := range []string{"", "/repositories/repository-20"} {
			path := prefix + "github:installation:10" + repo + "/config-file"
			requireResponse(t, harness.request(t, http.MethodGet, path+"/preview", nil, nil), "anonymous preview", http.StatusUnauthorized)
			requireResponse(t, harness.request(t, http.MethodPost, path+"/resolution", strings.NewReader(configFileChoiceBody()), nil), "anonymous choice", http.StatusUnauthorized)
			response := harness.request(t, http.MethodGet, path+"/preview", nil, session)
			requireResponse(t, response, "fresh preview", http.StatusOK, `"choices":`, `"document":`, `"review_token":`)
			var wire map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &wire); err != nil {
				t.Fatal(err)
			}
			assertWireNames(t, "configuration file preview", wire)
			response = harness.request(t, http.MethodPost, path+"/resolution", strings.NewReader(configFileChoiceBody()), session)
			requireResponse(t, response, "accepted choice", http.StatusAccepted, `"status":"pending"`)
			request := probe.requests[len(probe.requests)-1]
			if request.ActorAccountID != "github:test:user:1" || request.TargetID != "github:installation:10" ||
				request.RepositoryID != strings.TrimPrefix(repo, "/repositories/") || request.Side != "panel" || request.ReviewToken != strings.Repeat("a", 64) {
				t.Fatalf("resolution identity = %+v", request)
			}
		}
	}
	if len(probe.requests) != 4 || len(probe.previewCalls) != 4 {
		t.Fatalf("unauthorized requests reached the controller: %+v", probe)
	}
}

func TestConfigFileReviewRejectsForeignScopeAndCrossOrigin(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	probe := &configFileControllerProbe{}
	harness.server.configFiles = probe
	for _, prefix := range []string{"/panel/api/v1/targets/", "/panel/api/v1/root/workspaces/"} {
		for _, scope := range []string{"unknown", "github:installation:10/repositories/foreign"} {
			path := prefix + scope + "/config-file"
			requireResponse(t, harness.request(t, http.MethodGet, path+"/preview", nil, session), "foreign preview", http.StatusNotFound)
			requireResponse(t, harness.request(t, http.MethodPost, path+"/resolution", strings.NewReader(configFileChoiceBody()), session), "foreign choice", http.StatusNotFound)
		}
		path := prefix + "github:installation:10/config-file/resolution"
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(configFileChoiceBody()))
		req.AddCookie(session)
		req.Header.Set("Origin", "https://untrusted.example")
		response := httptest.NewRecorder()
		harness.handler.ServeHTTP(response, req)
		requireResponse(t, response, "cross-origin choice", http.StatusForbidden)
	}
	if len(probe.requests)+len(probe.previewCalls) != 0 {
		t.Fatalf("rejected scopes reached controller: %+v", probe)
	}
}

func TestConfigFileReviewViewersCannotResolve(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	_ = harness.signIn(t)
	session := createOrdinarySession(t, harness)
	role := storage.InstallationRoleViewer
	_, err := harness.store.SetTargetAccess(t.Context(), storage.TargetAccessChange{
		TargetID: "github:installation:10", SubjectAccountID: "github:test:user:ordinary",
		ActorAccountID: "github:test:user:1", Role: &role, ChangedAt: harness.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	probe := &configFileControllerProbe{}
	harness.server.configFiles = probe
	const path = "/panel/api/v1/targets/github:installation:10/config-file"
	requireResponse(t, harness.request(t, http.MethodGet, path+"/preview", nil, session), "viewer preview", http.StatusOK)
	requireResponse(t, harness.request(t, http.MethodPost, path+"/resolution", strings.NewReader(configFileChoiceBody()), session), "viewer choice", http.StatusNotFound)
	if len(probe.requests) != 0 {
		t.Fatal("viewer could resolve a conflict")
	}
}

func TestConfigFileReviewBoundsInputsAndExplainsStaleChoices(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	probe := &configFileControllerProbe{}
	harness.server.configFiles = probe
	const path = "/panel/api/v1/targets/github:installation:10/config-file/resolution"
	for _, body := range []string{
		`null`, `{}`, `{"side":"file","review_token":"old"}`, strings.ReplaceAll(configFileChoiceBody(), "panel", "other"),
		strings.TrimSuffix(configFileChoiceBody(), "}") + `,"document":{"allowed_commands":[]}}`,
		strings.TrimSuffix(configFileChoiceBody(), "}") + `,"actor_account_id":"another-user"}`,
	} {
		requireResponse(t, harness.request(t, http.MethodPost, path, strings.NewReader(body), session), "invalid choice", http.StatusBadRequest)
	}
	requireResponse(t, harness.request(t, http.MethodPost, path, strings.NewReader(`{"review_token":"`+strings.Repeat("a", 5000)+`"}`), session), "oversized choice", http.StatusRequestEntityTooLarge)
	if len(probe.requests) != 0 {
		t.Fatal("invalid input reached the controller")
	}
	probe.err = storage.ErrConflict
	requireResponse(t, harness.request(t, http.MethodPost, path, strings.NewReader(configFileChoiceBody()), session), "stale choice", http.StatusConflict, "config_file_changed")
	probe.err = &configsync.BlockedError{Code: "file_removed", Message: "The configuration file was removed"}
	requireResponse(t, harness.request(t, http.MethodPost, path, strings.NewReader(configFileChoiceBody()), session), "blocked choice", http.StatusUnprocessableEntity, "file_removed")
	harness.server.configFiles = nil
	requireResponse(t, harness.request(t, http.MethodPost, path, strings.NewReader(configFileChoiceBody()), session), "unavailable controller", http.StatusServiceUnavailable, "config_file_unavailable")
}

func TestConfigFileRootResolutionCarriesElevationAndHonorsExpiredGrant(t *testing.T) {
	harness := newPanelHarness(t, "root")
	session := harness.signIn(t)
	_, snapshot := seedNonOwnedWorkspace(t, harness)
	probe := &configFileControllerProbe{}
	harness.server.configFiles = probe
	path := "/panel/api/v1/root/workspaces/" + snapshot.TargetID
	before := harness.request(t, http.MethodPost, path+"/config-file/resolution", strings.NewReader(configFileChoiceBody()), session)
	if before.Code < 400 || len(probe.requests) != 0 {
		t.Fatalf("non-owned resolution without elevation = %d", before.Code)
	}
	elevated := harness.request(t, http.MethodPost, path+"/elevation", strings.NewReader(`{"acknowledged":true,"reason":"resolve configuration conflict"}`), session)
	requireResponse(t, elevated, "begin elevation", http.StatusCreated)
	requireResponse(t, harness.request(t, http.MethodPost, path+"/config-file/resolution", strings.NewReader(configFileChoiceBody()), session), "elevated resolution", http.StatusAccepted)
	if len(probe.requests) != 1 || probe.requests[0].ElevationID == nil || probe.requests[0].SessionTokenHash == "" {
		t.Fatalf("elevation was not bound to the prepared write: %+v", probe.requests)
	}
	probe.err = storage.ErrExpired
	requireResponse(t, harness.request(t, http.MethodPost, path+"/config-file/resolution", strings.NewReader(configFileChoiceBody()), session), "grant expired during read", http.StatusGone, "elevation_expired")
}
