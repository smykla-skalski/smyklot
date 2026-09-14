package panel

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func TestScheduleLocalTimeResolution(t *testing.T) {
	harness := newPanelHarness(t, "root")
	path := "/panel/api/v1/schedule-local-time?timezone=Europe/Warsaw&local_time=2026-10-25T02:30"
	requireResponse(t, harness.request(t, http.MethodGet, path, nil, nil), "anonymous local time", http.StatusUnauthorized)
	session := harness.signIn(t)
	response := harness.request(t, http.MethodGet, path, nil, session)
	requireResponse(t, response, "repeated local time", http.StatusOK)
	var result workqueue.LocalTimeResolution
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Options) != 2 || result.Options[0].OffsetSeconds != 7200 || result.Options[1].OffsetSeconds != 3600 {
		t.Fatalf("unexpected choices: %+v", result)
	}
	for _, query := range []string{"timezone=Local&local_time=2026-10-25T02:30", "timezone=UTC&local_time=2026-02-30T02:30"} {
		requireResponse(t, harness.request(t, http.MethodGet, "/panel/api/v1/schedule-local-time?"+query, nil, session), "invalid local time", http.StatusBadRequest)
	}
}
