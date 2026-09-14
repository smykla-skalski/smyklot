package panel

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestRuntimeValidationIdentifiesRequestField(t *testing.T) {
	harness := newPanelHarness(t, "root")
	session := harness.signIn(t)
	for _, sample := range []struct{ field, value string }{
		{"session_ttl_seconds", "1"},
		{"reaction_poll_interval_seconds", "-1"},
		{"merge_after_ci_quiet_period_seconds", "-1"},
		{"path_index_interval_seconds", "-1"},
		{"bot_config", `{"version":1,"overrides":{"quiet_success":null}}`},
		{"bot_config", `{"version":1,"overrides":{"quiet_success":false,"quiet_success":true}}`},
	} {
		t.Run(sample.field+sample.value, func(t *testing.T) {
			body := strings.Replace(rootRuntimeSettingsBody("info", 0), `"`+sample.field+`":null`, `"`+sample.field+`":`+sample.value, 1)
			response := harness.request(t, http.MethodPut, "/panel/api/v1/root/runtime/settings", strings.NewReader(body), session)
			requireResponse(t, response, "reject invalid runtime field", http.StatusBadRequest)
			var payload struct {
				Error struct{ Code, Field, Message string }
			}
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Error.Field != sample.field || payload.Error.Code != "invalid_runtime_settings" || payload.Error.Message == "" {
				t.Fatalf("invalid field response: %s", response.Body.String())
			}
			read := harness.request(t, http.MethodGet, "/panel/api/v1/root/runtime/settings", nil, session)
			var settings runtimeSettingsResponse
			if err := json.Unmarshal(read.Body.Bytes(), &settings); err != nil {
				t.Fatal(err)
			}
			if settings.Revision != 0 || settings.BehaviorDefaults.Intent != nil {
				t.Fatal("invalid request mutated settings")
			}
		})
	}
}
