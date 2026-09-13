package panel

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

func readCheckCapability(t *testing.T, h *panelHarness, session *http.Cookie, path string) syncCheckCapability {
	t.Helper()
	response := h.request(t, http.MethodGet, "/panel/api/v1/targets/"+panelSyncTarget+"/sync/"+path, nil, session)
	requireResponse(t, response, "check capability", http.StatusOK)
	var body struct {
		Check syncCheckCapability `json:"check"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Check.Action != "check" || body.Check.TargetID != panelSyncTarget || body.Check.Effect != "request_repository_check" {
		t.Fatalf("incomplete capability: %#v", body.Check)
	}
	return body.Check
}

func TestCheckCapabilityTracksWorkspaceInsteadOfHistoricalPlan(t *testing.T) {
	h := newPanelHarness(t, "owner")
	session := h.signIn(t)
	if got := readCheckCapability(t, h, session, "plan"); !got.Available {
		t.Fatalf("empty workspace: %#v", got)
	}
	createPanelSyncPlan(t, h, "older", h.now.Add(time.Minute))
	if got := readCheckCapability(t, h, session, "plans/older"); got.Available || got.BlockingPlanID != "older" {
		t.Fatalf("current blocker: %#v", got)
	}
	*h.clock = h.now.Add(time.Minute)
	if got := readCheckCapability(t, h, session, "plans/older"); !got.Available {
		t.Fatalf("expired work: %#v", got)
	}
	requireResponse(t, postReceiptCheck(t, h, session, "Check saved settings"), "check accepted", http.StatusAccepted)
	createPanelSyncPlan(t, h, "newer", h.now.Add(time.Hour))
	if got := readCheckCapability(t, h, session, "plans/older"); got.Available || got.BlockingPlanID != "newer" {
		t.Fatalf("historical read lost current blocker: %#v", got)
	}
}

func TestCheckCapabilityRequiresPrivilegedRole(t *testing.T) {
	h := newPanelHarness(t, "owner")
	for _, role := range []storage.InstallationRole{storage.InstallationRoleViewer, storage.InstallationRoleEditor, storage.InstallationRoleNone} {
		got, err := h.server.currentSyncCheckCapability(t.Context(), panelSyncTarget, role)
		if err != nil || got.Available || got.Reason != "admin_or_owner_required" || got.BlockingPlanID != "" || got.RunningCheckID != "" {
			t.Fatalf("role %s: %#v %v", role, got, err)
		}
	}
}
