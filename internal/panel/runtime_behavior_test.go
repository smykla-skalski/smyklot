package panel

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

func TestRuntimeBehaviorIntentSaveResetRestore(t *testing.T) {
	harness := newPanelHarness(t, "root")
	session := harness.signIn(t)
	write := func(behavior string, revision int64) runtimeSettingsResponse {
		t.Helper()
		body := strings.Replace(rootRuntimeSettingsBody("info", revision), `"bot_config":null`, `"bot_config":`+behavior, 1)
		response := harness.request(t, http.MethodPut, "/panel/api/v1/root/runtime/settings", strings.NewReader(body), session)
		requireResponse(t, response, "save explicit runtime behavior", http.StatusOK)
		var saved runtimeSettingsResponse
		if err := json.Unmarshal(response.Body.Bytes(), &saved); err != nil {
			t.Fatal(err)
		}
		return saved
	}
	first := write(`{"version":1,"overrides":{"quiet_success":false}}`, 0)
	if first.CheckpointID == nil || first.BehaviorDefaults.Intent == nil {
		t.Fatal("equal-value intent was discarded")
	}
	patch := first.BehaviorDefaults.Intent.Patch()
	if patch.QuietSuccess == nil || *patch.QuietSuccess || patch.QuietPending != nil {
		t.Fatal("wrong explicit fields")
	}
	same := write(`{"version":1,"overrides":{"quiet_success":false}}`, first.Revision)
	if same.CheckpointID != nil || same.Revision != first.Revision {
		t.Fatal("unchanged intent created a checkpoint")
	}
	reset := write(`{"version":1,"overrides":{}}`, same.Revision)
	if reset.BehaviorDefaults.Intent != nil {
		t.Fatal("empty override did not reset inheritance")
	}
	restorePath := "/panel/api/v1/root/runtime/settings/checkpoints/" + *first.CheckpointID + "/restore"
	restore := harness.request(t, http.MethodPost, restorePath, strings.NewReader(`{"state":"after","selections":[{"kind":"runtime","expected_revision":2}]}`), session)
	requireResponse(t, restore, "restore sparse intent", http.StatusOK)
	read := harness.request(t, http.MethodGet, "/panel/api/v1/root/runtime/settings", nil, session)
	var restored runtimeSettingsResponse
	if err := json.Unmarshal(read.Body.Bytes(), &restored); err != nil {
		t.Fatal(err)
	}
	if restored.BehaviorDefaults.Intent == nil || restored.BehaviorDefaults.Intent.Patch().QuietPending != nil {
		t.Fatal("restore materialized inherited settings")
	}
	deployment := config.Default()
	deployment.QuietSuccess = true
	deployment.QuietPending = true
	resolved := restored.BehaviorDefaults.Intent.Resolve(deployment)
	if resolved.QuietSuccess || !resolved.QuietPending {
		t.Fatal("restored ownership does not follow new deployment correctly")
	}
}

func TestRuntimeBehaviorInvalidIntentNamesField(t *testing.T) {
	harness := newPanelHarness(t, "root")
	session := harness.signIn(t)
	body := strings.Replace(rootRuntimeSettingsBody("info", 0), `"bot_config":null`, `"bot_config":{"version":1,"overrides":{"quiet_success":null}}`, 1)
	response := harness.request(t, http.MethodPut, "/panel/api/v1/root/runtime/settings", strings.NewReader(body), session)
	requireResponse(t, response, "reject invalid explicit setting", http.StatusBadRequest)
	if !strings.Contains(response.Body.String(), "quiet_success") {
		t.Fatalf("missing field context: %s", response.Body.String())
	}
}
