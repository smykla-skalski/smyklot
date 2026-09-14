package panel

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func TestScheduleTimezoneRequiresSession(t *testing.T) {
	harness := newPanelHarness(t, "root")
	response := harness.request(t, http.MethodGet, "/panel/api/v1/schedule-timezone?timezone=UTC&at=2026-01-01T00:00:00Z", nil, nil)
	requireResponse(t, response, "anonymous timezone preview", http.StatusUnauthorized)
}

func TestScheduleTimezoneUsesAuthoritativeInstant(t *testing.T) {
	harness := newPanelHarness(t, "root")
	session := harness.signIn(t)
	response := harness.request(t, http.MethodGet, "/panel/api/v1/schedule-timezone?timezone=Europe/Warsaw&at=2026-03-29T01:00:00Z", nil, session)
	requireResponse(t, response, "timezone preview", http.StatusOK)
	var preview workqueue.TimezonePreview
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.OffsetSeconds != 7200 || preview.LocalTime != "2026-03-29T03:00:00+02:00" {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	for _, query := range []string{
		"timezone=Mars/Olympus&at=2026-01-01T00:00:00Z",
		"timezone=UTC&at=2026-01-01T00:00:00",
		"timezone=UTC&at=2026-02-30T00:00:00Z",
	} {
		response = harness.request(t, http.MethodGet, "/panel/api/v1/schedule-timezone?"+query, nil, session)
		requireResponse(t, response, "invalid timezone preview", http.StatusBadRequest)
	}
}
