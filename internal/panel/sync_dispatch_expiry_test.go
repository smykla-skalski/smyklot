package panel

import (
	"net/http"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
)

func TestSyncDispatchRejectsExpiryAfterHTTPChecks(t *testing.T) {
	h := newPanelHarness(t, "owner")
	session := h.signIn(t)
	createPanelSyncPlan(t, h, "selected", h.now.Add(time.Minute))
	_, err := h.store.ApproveSyncPlan(t.Context(), orgsync.PlanApproval{TargetID: panelSyncTarget, PlanID: "selected", Digest: "sha256:selected", ActorID: "github:test:user:1", Now: h.now})
	if err != nil {
		t.Fatal(err)
	}
	item, err := h.store.GetQueueItem(t.Context(), "sync-plan:selected")
	if err != nil {
		t.Fatal(err)
	}
	input := syncRunNowInput{Action: "dispatch", RequestKey: "expired-before-acceptance", PlanID: "selected", ExpectedRevision: item.Revision, Reason: "Apply reviewed changes"}
	h.server.store = revokingDispatchStore{Store: h.store, before: func() {
		// The command has already sampled time. Simulate time passing before the
		// storage decision without sleeps or changing the selected plan itself.
		*h.clock = h.now.Add(2 * time.Minute)
	}}
	response := postDispatchReceipt(t, h, session, input)
	requireResponse(t, response, "expired after HTTP checks", http.StatusConflict)
	after, err := h.store.GetQueueItem(t.Context(), item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != item.Revision {
		t.Fatal("expired dispatch changed queue state")
	}
}
