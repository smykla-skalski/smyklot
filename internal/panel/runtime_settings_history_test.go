package panel

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

func TestRootRuntimeSettingsCheckpointInspectionAndRestore(t *testing.T) {
	harness := newPanelHarness(t, "root")
	session := harness.signIn(t)

	firstResponse := harness.request(
		t,
		http.MethodPut,
		"/panel/api/v1/root/runtime/settings",
		strings.NewReader(rootRuntimeSettingsBody("debug", 0)),
		session,
	)
	requireResponse(
		t,
		firstResponse,
		"save Root runtime settings",
		http.StatusOK,
		`"checkpoint_id":`,
		`"revision":1`,
	)
	var first runtimeSettingsResponse
	if err := json.Unmarshal(firstResponse.Body.Bytes(), &first); err != nil ||
		first.CheckpointID == nil {
		t.Fatalf("first runtime save = %#v, %v", first, err)
	}

	secondResponse := harness.request(
		t,
		http.MethodPut,
		"/panel/api/v1/root/runtime/settings",
		strings.NewReader(rootRuntimeSettingsBody("error", 1)),
		session,
	)
	requireResponse(t, secondResponse, "change Root runtime settings", http.StatusOK, `"revision":2`)

	path := "/panel/api/v1/root/runtime/settings/checkpoints/" + *first.CheckpointID
	inspectionResponse := harness.request(t, http.MethodGet, path, nil, session)
	requireResponse(
		t,
		inspectionResponse,
		"inspect Root runtime checkpoint",
		http.StatusOK,
		`"action":"runtime.settings.saved"`,
		`"kind":"runtime"`,
		`"differs":true`,
		`"restorable":true`,
		`"revision":2`,
	)
	stale := harness.request(
		t,
		http.MethodPost,
		path+"/restore",
		strings.NewReader(`{"state":"after","selections":[{"kind":"runtime","expected_revision":1}]}`),
		session,
	)
	requireResponse(
		t,
		stale,
		"reject stale Root runtime restore",
		http.StatusConflict,
		`"code":"conflict"`,
	)

	restoredResponse := harness.request(
		t,
		http.MethodPost,
		path+"/restore",
		strings.NewReader(`{"state":"after","selections":[{"kind":"runtime","expected_revision":2}]}`),
		session,
	)
	requireResponse(
		t,
		restoredResponse,
		"restore Root runtime settings",
		http.StatusOK,
		`"checkpoint_id":`,
		`"override":"debug"`,
		`"revision":3`,
	)
	if harness.runtime.values.LogLevel != slog.LevelDebug {
		t.Fatalf("restored runtime log level = %s", harness.runtime.values.LogLevel)
	}
	var restored runtimeSettingsResponse
	if err := json.Unmarshal(restoredResponse.Body.Bytes(), &restored); err != nil ||
		restored.CheckpointID == nil {
		t.Fatalf("restored runtime settings = %#v, %v", restored, err)
	}

	audit := harness.request(
		t,
		http.MethodGet,
		"/panel/api/v1/root/history/audit?category=runtime",
		nil,
		session,
	)
	requireResponse(
		t,
		audit,
		"Root runtime restore audit",
		http.StatusOK,
		`"action":"runtime.settings.restored"`,
		`"settings_checkpoint_id":"`+*restored.CheckpointID+`"`,
	)

	noop := harness.request(
		t,
		http.MethodPost,
		path+"/restore",
		strings.NewReader(`{"state":"after","selections":[{"kind":"runtime","expected_revision":3}]}`),
		session,
	)
	requireResponse(
		t,
		noop,
		"reject Root runtime restore no-op",
		http.StatusConflict,
		`"code":"settings_restore_noop"`,
	)
}

func TestRootRuntimeSettingsCheckpointRejectsIncompatibleSnapshot(t *testing.T) {
	harness := newPanelHarness(t, "root")
	session := harness.signIn(t)
	checkpointID := saveRuntimeSettingsCheckpoint(t, harness, "debug", 0)
	rewriteSettingsCheckpointDocumentVersion(t, harness, checkpointID,
		storage.SettingsCheckpointItemIdentity{Kind: storage.SettingsCheckpointItemRuntime})
	path := "/panel/api/v1/root/runtime/settings/checkpoints/" +
		strconv.FormatInt(checkpointID, 10)
	inspection := harness.request(t, http.MethodGet, path, nil, session)
	requireResponse(
		t,
		inspection,
		"inspect incompatible Root runtime checkpoint",
		http.StatusOK,
		`"restorable":false`,
		`"code":"unsupported_document_version"`,
		`"reason":"This checkpoint uses a settings format this version cannot restore"`,
	)
	restore := harness.request(
		t,
		http.MethodPost,
		path+"/restore",
		strings.NewReader(`{"state":"after","selections":[{"kind":"runtime","expected_revision":1}]}`),
		session,
	)
	requireResponse(
		t,
		restore,
		"reject incompatible Root runtime restore",
		http.StatusConflict,
		`"code":"settings_restore_blocked"`,
	)
}

func TestRootRuntimeSettingsCheckpointRejectsUnsafeRestore(t *testing.T) {
	harness := newPanelHarness(t, "root")
	session := harness.signIn(t)
	invalid := harness.request(
		t,
		http.MethodPost,
		"/panel/api/v1/root/runtime/settings/checkpoints/1/restore",
		strings.NewReader(`{"state":"after","selections":[{"kind":"target","expected_revision":0}]}`),
		session,
	)
	requireResponse(t, invalid, "reject non-runtime Root restore", http.StatusBadRequest)

	request := httptest.NewRequest(
		http.MethodPost,
		"/panel/api/v1/root/runtime/settings/checkpoints/1/restore",
		strings.NewReader(`{"state":"after","selections":[{"kind":"runtime","expected_revision":0}]}`),
	)
	request.AddCookie(session)
	request.Header.Set("Origin", "https://untrusted.example")
	response := httptest.NewRecorder()
	harness.server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("cross-origin Root restore = %d %s", response.Code, response.Body.String())
	}
}

func rootRuntimeSettingsBody(level string, revision int64) string {
	return `{
        "bot_config":null,
        "log_level":"` + level + `",
        "reaction_poll_interval_seconds":null,
        "merge_after_ci_quiet_period_seconds":null,
        "path_index_interval_seconds":null,
        "session_ttl_seconds":null,
		"expected_revision":` + strconv.FormatInt(revision, 10) + `
    }`
}
