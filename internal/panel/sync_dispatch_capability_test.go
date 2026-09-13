package panel

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func TestSyncDispatchCapabilityAuthorization(t *testing.T) {
	now := time.Now()
	p := orgsync.Plan{ID: "plan", TargetID: "target", State: orgsync.PlanApproved, ExpiresAt: now.Add(time.Hour)}
	i := workqueue.Item{ID: "queue", TargetID: &p.TargetID, SourceID: p.ID, SourceKind: "sync_plan", Kind: workqueue.KindSyncApply, State: workqueue.StateReady, Revision: 7}
	for _, role := range []storage.InstallationRole{storage.InstallationRoleOwner, storage.InstallationRoleAdmin, storage.InstallationRoleEditor, storage.InstallationRoleViewer, storage.InstallationRoleNone, ""} {
		got := currentSyncDispatchCapability(p, &i, role, now)
		allowed := role == storage.InstallationRoleOwner || role == storage.InstallationRoleAdmin
		if got.Available != allowed || got.PlanID != p.ID || got.Effect == "" {
			t.Fatalf("role %s: %#v", role, got)
		}
		if allowed && (got.QueueID != i.ID || got.ExpectedRevision != 7 || got.Reason != "available") {
			t.Fatalf("missing exact intent: %#v", got)
		}
		if !allowed && (got.Reason != "admin_or_owner_required" || got.ExpectedRevision != 0 || got.QueueID != "") {
			t.Fatalf("unauthorized command: %#v", got)
		}
	}
}

func TestSyncDispatchCapabilityReflectsCurrentExpiry(t *testing.T) {
	h := newPanelHarness(t, "owner")
	session := h.signIn(t)
	createPanelSyncPlan(t, h, "capability", h.now.Add(time.Minute))
	_, err := h.store.ApproveSyncPlan(t.Context(), orgsync.PlanApproval{TargetID: panelSyncTarget, PlanID: "capability", Digest: "sha256:capability", ActorID: "github:test:user:1", Now: h.now})
	if err != nil {
		t.Fatal(err)
	}
	for _, expired := range []bool{false, true} {
		if expired {
			*h.clock = h.now.Add(time.Minute)
		}
		response := h.request(t, http.MethodGet, "/panel/api/v1/targets/"+panelSyncTarget+"/sync/plans/capability", nil, session)
		if response.Code != http.StatusOK {
			t.Fatalf("read: %s", response.Body.String())
		}
		var body struct {
			Plan syncPlanDTO `json:"plan"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		got := body.Plan.Dispatch
		if expired {
			rejected := postDispatchReceipt(t, h, session, syncRunNowInput{Action: "dispatch", RequestKey: "expired-capability", PlanID: "capability", ExpectedRevision: 1, Reason: "Run reviewed changes"})
			requireResponse(t, rejected, "expired capability submit", http.StatusConflict, `"reason":"plan_expired"`)
		}
		if got.Available == expired {
			t.Fatalf("expired=%t capability=%#v", expired, got)
		}
		if expired && (got.Reason != "plan_expired" || got.ExpectedRevision != 0) {
			t.Fatalf("expired intent: %#v", got)
		}
	}
	plan, _, err := h.store.GetSyncPlan(t.Context(), panelSyncTarget, "capability")
	if err != nil || plan.State != orgsync.PlanApproved {
		t.Fatalf("capability read mutated plan: %#v %v", plan, err)
	}
}
