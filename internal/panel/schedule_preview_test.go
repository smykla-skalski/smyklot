package panel

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func TestSchedulePreviewIsAuthenticatedAndReadOnly(t *testing.T) {
	harness := newPanelHarness(t, "root")
	session := harness.signIn(t)
	const path = "/panel/api/v1/schedule-preview"
	const body = `{"date":"2026-10-25","profile":{"name":"Unsaved","timezone":"Europe/Warsaw","windows":[],"exceptions":[{"date":"2026-10-25","start_minute":135,"end_minute":165}]}}`
	response := harness.request(t, http.MethodPost, path, strings.NewReader(body), nil)
	requireResponse(t, response, "anonymous preview", http.StatusUnauthorized)
	before := harness.request(t, http.MethodGet, "/panel/api/v1/root/schedule-profiles", nil, session)
	requireResponse(t, before, "profiles before", http.StatusOK)
	response = harness.request(t, http.MethodPost, path, strings.NewReader(body), session)
	requireResponse(t, response, "date preview", http.StatusOK)
	var preview workqueue.DatePreview
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if len(preview.Windows) != 1 || preview.Windows[0].OpensAt != "2026-10-25T02:15:00+02:00" || preview.Windows[0].ClosesAt != "2026-10-25T02:45:00+01:00" {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	after := harness.request(t, http.MethodGet, "/panel/api/v1/root/schedule-profiles", nil, session)
	if before.Body.String() != after.Body.String() {
		t.Fatal("preview persisted a profile")
	}
	for _, bad := range []string{`{}`, strings.ReplaceAll(body, "Europe/Warsaw", "Mars/Olympus"), strings.ReplaceAll(body, "2026-10-25", "2026-02-30")} {
		response = harness.request(t, http.MethodPost, path, strings.NewReader(bad), session)
		requireResponse(t, response, "invalid preview", http.StatusBadRequest)
	}
}
