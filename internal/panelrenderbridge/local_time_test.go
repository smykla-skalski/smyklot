package panelrenderbridge_test

import (
	"testing"

	"github.com/smykla-skalski/smyklot/internal/panelrenderbridge"
)

func TestBridgeLocalTimePreservesClockChangeChoices(t *testing.T) {
	for _, tc := range []struct {
		wall  string
		count int
	}{{"2026-10-25T02:30", 2}, {"2026-03-29T02:30", 0}, {"2026-07-01T12:00", 1}} {
		response := panelrenderbridge.Render(panelrenderbridge.Request{Version: panelrenderbridge.ProtocolVersion, ID: "local-time", LocalTime: &panelrenderbridge.LocalTimeRequest{Timezone: "Europe/Warsaw", LocalTime: tc.wall}})
		if !response.Valid || response.LocalTime == nil || len(response.LocalTime.Options) != tc.count {
			t.Fatalf("unexpected response for %s: %+v", tc.wall, response)
		}
	}
	response := panelrenderbridge.Render(panelrenderbridge.Request{Version: panelrenderbridge.ProtocolVersion, ID: "invalid", LocalTime: &panelrenderbridge.LocalTimeRequest{Timezone: "UTC", LocalTime: "invalid"}})
	if response.Valid || len(response.Diagnostics) != 1 || response.Diagnostics[0].Code != "invalid_local_time" {
		t.Fatalf("unexpected invalid response: %+v", response)
	}
}
