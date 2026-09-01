package panel

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

func targetWorkspaceSettingsBatchBody(
	t *testing.T,
	target storage.Target,
	enabled bool,
) []byte {
	t.Helper()
	body := map[string]any{"target": map[string]any{
		"repository_default_enabled":               enabled,
		"pending_ci_mode_default":                  target.PendingCIModeDefault,
		"pending_ci_branch_patterns_default":       target.PendingCIBranchPatternsDefault,
		"pending_ci_quiet_period_seconds_override": nil,
		"path_index_interval_seconds_override":     nil,
		"config_patch":                             map[string]any{}, "expected_revision": target.Revision,
	}}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}

	return encoded
}

func TestWorkspaceSettingsBatchAuthorizationAndSameOrigin(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	target, err := harness.store.GetTarget(t.Context(), "github:installation:10")
	if err != nil {
		t.Fatal(err)
	}
	body := targetWorkspaceSettingsBatchBody(t, target, true)

	unauthenticated := harness.request(
		t, http.MethodPut, workspaceSettingsBatchPath, bytes.NewReader(body), nil,
	)
	requireResponse(t, unauthenticated, "unauthenticated batch", http.StatusUnauthorized)
	ordinary := createOrdinarySession(t, harness)
	unauthorized := harness.request(
		t, http.MethodPut, workspaceSettingsBatchPath, bytes.NewReader(body), ordinary,
	)
	requireResponse(t, unauthorized, "unowned batch", http.StatusNotFound)
	request := httptest.NewRequest(http.MethodPut, workspaceSettingsBatchPath, bytes.NewReader(body))
	request.AddCookie(session)
	request.Header.Set("Origin", "https://untrusted.example")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	harness.handler.ServeHTTP(response, request)
	requireResponse(t, response, "cross-origin batch", http.StatusForbidden)
}

func TestWorkspaceSettingsRejectsLegacyFlatDocuments(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	const flatDocument = `{"repository_default_enabled":true,
		"pending_ci_mode_default":"labels",
		"pending_ci_branch_patterns_default":{"include":["~DEFAULT_BRANCH"],"exclude":[]},
		"pending_ci_quiet_period_seconds_override":null,
		"path_index_interval_seconds_override":null,"config_patch":{},"expected_revision":1}`

	ordinary := harness.request(
		t, http.MethodPut, workspaceSettingsBatchPath, strings.NewReader(flatDocument), session,
	)
	requireResponse(t, ordinary, "flat workspace settings", http.StatusBadRequest,
		`"code":"invalid_request"`)

	_, snapshot := seedNonOwnedWorkspace(t, harness)
	started := harness.request(
		t, http.MethodPost,
		"/panel/api/v1/root/workspaces/"+snapshot.TargetID+"/elevation",
		strings.NewReader(`{"acknowledged":true,"reason":"verify canonical settings input"}`),
		session,
	)
	requireResponse(t, started, "start Root settings elevation", http.StatusCreated)
	root := harness.request(
		t, http.MethodPut,
		"/panel/api/v1/root/workspaces/"+snapshot.TargetID+"/settings",
		strings.NewReader(flatDocument), session,
	)
	requireResponse(t, root, "flat Root workspace settings", http.StatusBadRequest,
		`"code":"invalid_request"`)
}

func TestRootWorkspaceSettingsBatchRequiresAndRecordsElevation(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	rootSession := harness.signIn(t)
	_, snapshot := seedNonOwnedWorkspace(t, harness)
	target, err := harness.store.GetTarget(t.Context(), snapshot.TargetID)
	if err != nil {
		t.Fatal(err)
	}
	path := "/panel/api/v1/root/workspaces/" + target.ID + "/settings"
	body := targetWorkspaceSettingsBatchBody(t, target, true)

	blocked := harness.request(t, http.MethodPut, path, bytes.NewReader(body), rootSession)
	requireResponse(t, blocked, "Root batch without elevation", http.StatusForbidden,
		`"code":"elevation_required"`)
	started := harness.request(t, http.MethodPost,
		"/panel/api/v1/root/workspaces/"+target.ID+"/elevation",
		strings.NewReader(`{"acknowledged":true,"reason":"edit workspace settings"}`),
		rootSession)
	requireResponse(t, started, "start batch elevation", http.StatusCreated)
	var elevation elevationResponse
	if err := json.Unmarshal(started.Body.Bytes(), &elevation); err != nil {
		t.Fatal(err)
	}
	saved := harness.request(t, http.MethodPut, path, bytes.NewReader(body), rootSession)
	requireResponse(t, saved, "elevated Root batch", http.StatusOK,
		`"target_id":"`+target.ID+`"`, `"checkpoint_id":`)
	history := harness.request(t, http.MethodGet,
		"/panel/api/v1/root/history/audit?category=configuration&limit=100", nil, rootSession)
	requireResponse(t, history, "Root batch history", http.StatusOK,
		`"elevation_id":"`+elevation.ID+`"`, `"action":"installation.settings.saved"`)
}

func TestWorkspaceSettingsBatchRejectsMalformedResourcesBeforeWrite(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	tests := []struct {
		name, body string
		status     int
		fragment   string
	}{
		{"empty", `{}`, http.StatusBadRequest, `"code":"invalid_request"`},
		{"unknown field", `{"surprise":true}`, http.StatusBadRequest, `"code":"invalid_request"`},
		{"incomplete target", `{"target":{"repository_default_enabled":true}}`, http.StatusBadRequest, `"code":"invalid_request"`},
		{"negative revision", `{"sync_configs":[{"kind":"labels","enabled":true,"labels":[],"allow_removal":false,"excludes":[],"expected_revision":-1}]}`, http.StatusBadRequest, `"code":"invalid_request"`},
		{"unknown kind", `{"sync_configs":[{"kind":"future","enabled":true,"document":{},"expected_revision":0}]}`, http.StatusBadRequest, `"code":"invalid_request"`},
		{"duplicate kind", `{"sync_configs":[{"kind":"labels","enabled":true,"labels":[],"allow_removal":false,"excludes":[],"expected_revision":0},{"kind":"labels","enabled":false,"labels":[],"allow_removal":false,"excludes":[],"expected_revision":0}]}`, http.StatusBadRequest, `"code":"invalid_request"`},
		{"duplicate repository", `{"repositories":[{"repository_id":"repository-20","enabled_override":null,"pending_ci_mode_override":null,"pending_ci_branch_patterns_override":null,"pending_ci_quiet_period_seconds_override":null,"path_index_interval_seconds_override":null,"config_patch":{},"ignore_repository_file":false,"expected_revision":1},{"repository_id":"repository-20","enabled_override":true,"pending_ci_mode_override":null,"pending_ci_branch_patterns_override":null,"pending_ci_quiet_period_seconds_override":null,"path_index_interval_seconds_override":null,"config_patch":{},"ignore_repository_file":false,"expected_revision":1}]}`, http.StatusBadRequest, `"code":"invalid_request"`},
		{"duplicate override", `{"sync_overrides":[{"repository_id":"repository-20","kind":"labels","enabled":null,"document":{},"expected_revision":0},{"repository_id":"repository-20","kind":"labels","enabled":false,"document":{},"expected_revision":0}]}`, http.StatusBadRequest, `"code":"invalid_request"`},
		{"null override document", `{"sync_overrides":[{"repository_id":"repository-20","kind":"labels","enabled":null,"document":null,"expected_revision":0}]}`, http.StatusBadRequest, `"code":"invalid_request"`},
		{"wrong repository", `{"sync_overrides":[{"repository_id":"repository-elsewhere","kind":"labels","enabled":null,"document":{},"expected_revision":0}]}`, http.StatusNotFound, `"code":"not_found"`},
		{"nonempty label override", `{"sync_overrides":[{"repository_id":"repository-20","kind":"labels","enabled":null,"document":{"labels":[]},"expected_revision":0}]}`, http.StatusBadRequest, `"code":"invalid_sync_override"`},
		{"unknown nullable field", `{"repositories":[{"repository_id":"repository-20","enabled_override":null,"pending_ci_mode_override":null,"pending_ci_branch_patterns_override":{"include":["~DEFAULT_BRANCH"],"exclude":[],"typo":true},"pending_ci_quiet_period_seconds_override":null,"path_index_interval_seconds_override":null,"config_patch":{},"ignore_repository_file":false,"expected_revision":1}]}`, http.StatusBadRequest, `"code":"invalid_request"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := harness.request(
				t, http.MethodPut, workspaceSettingsBatchPath,
				strings.NewReader(test.body), session,
			)
			requireResponse(t, response, test.name, test.status, test.fragment)
		})
	}
	target, err := harness.store.GetTarget(t.Context(), "github:installation:10")
	if err != nil || target.Revision != 1 {
		t.Fatalf("target after rejected batches = revision %d, error %v", target.Revision, err)
	}
}

func TestWorkspaceSettingsBatchBoundsTheWholeRequest(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	body := `{"sync_configs":[{"kind":"files","enabled":true,"document":{"files":[{
		"path":"README.md","content":"` + strings.Repeat("x", maxWorkspaceSettingsBatchBody) +
		`"}]},"expected_revision":0}]}`
	response := harness.request(
		t, http.MethodPut, workspaceSettingsBatchPath, strings.NewReader(body), session,
	)
	requireResponse(t, response, "oversized settings batch", http.StatusRequestEntityTooLarge,
		`"code":"request_too_large"`)
}

func TestWorkspaceSettingsBatchRejectsUnavailableRepository(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	target, err := harness.store.GetTarget(t.Context(), "github:installation:10")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := harness.store.GetAccount(t.Context(), "github:test:user:1")
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.store.ReconcileInstallation(t.Context(), storage.InstallationSnapshot{
		TargetID: target.ID, InstallationID: target.InstallationID, Kind: target.Kind,
		Account: target.Account, SyncedAt: harness.now,
		Ownership: storage.OwnershipSnapshot{
			Source: target.Ownership.Source, Status: storage.OwnershipStatusFresh,
			Owners: []storage.Account{owner}, SyncedAt: harness.now,
		},
		Permissions: target.Permissions,
	}); err != nil {
		t.Fatal(err)
	}
	body := `{"repositories":[{"repository_id":"repository-20","enabled_override":null,
		"pending_ci_mode_override":null,"pending_ci_branch_patterns_override":null,
		"pending_ci_quiet_period_seconds_override":null,
		"path_index_interval_seconds_override":null,"config_patch":{},
		"ignore_repository_file":false,"expected_revision":1}]}`
	response := harness.request(
		t, http.MethodPut, workspaceSettingsBatchPath, strings.NewReader(body), session,
	)
	requireResponse(t, response, "unavailable repository", http.StatusNotFound,
		`"code":"not_found"`)
}
