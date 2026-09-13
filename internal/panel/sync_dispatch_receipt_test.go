package panel

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

func postDispatchReceipt(t *testing.T, h *panelHarness, session *http.Cookie, input syncRunNowInput) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return h.request(t, http.MethodPost, "/panel/api/v1/targets/"+panelSyncTarget+"/sync/run-now", bytes.NewReader(body), session)
}

func acceptedDispatchFixture(t *testing.T) (*panelHarness, *http.Cookie, syncRunNowInput, syncRunNowResponse) {
	t.Helper()
	h := newPanelHarness(t, "owner")
	session := h.signIn(t)
	createPanelSyncPlan(t, h, "selected", h.now.Add(time.Hour))
	_, err := h.store.ApproveSyncPlan(t.Context(), orgsync.PlanApproval{TargetID: panelSyncTarget, PlanID: "selected", Digest: "sha256:selected", ActorID: "github:test:user:1", Now: h.now})
	if err != nil {
		t.Fatal(err)
	}
	item, err := h.store.GetQueueItem(t.Context(), "sync-plan:selected")
	if err != nil {
		t.Fatal(err)
	}
	input := syncRunNowInput{Action: "dispatch", RequestKey: "dispatch-request", PlanID: "selected", ExpectedRevision: item.Revision, Reason: "Run selected changes"}
	response := postDispatchReceipt(t, h, session, input)
	requireResponse(t, response, "accept dispatch", http.StatusAccepted, `"status":"dispatch_accepted"`)
	var accepted syncRunNowResponse
	if err := json.Unmarshal(response.Body.Bytes(), &accepted); err != nil {
		t.Fatal(err)
	}
	if accepted.PlanID != "selected" || accepted.QueueID != item.ID || accepted.Plan != nil || accepted.Queue != nil {
		t.Fatalf("acceptance must identify original work: %s", response.Body.String())
	}
	current, err := h.store.GetQueueItem(t.Context(), item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !current.Immediate || current.Revision != item.Revision+1 {
		t.Fatalf("dispatch was not applied once: %#v", current)
	}
	return h, session, input, accepted
}

func TestSyncDispatchRecoversBeforeNewerWork(t *testing.T) {
	h, session, input, accepted := acceptedDispatchFixture(t)
	_, err := h.store.DiscardSyncPlan(t.Context(), orgsync.PlanDiscard{TargetID: panelSyncTarget, PlanID: input.PlanID, ActorID: "github:test:user:1", Now: h.now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.PruneWorkQueue(t.Context(), h.now.AddDate(2, 0, 0)); err != nil {
		t.Fatal(err)
	}
	createPanelSyncPlan(t, h, "newer", h.now.Add(time.Hour))
	response := postDispatchReceipt(t, h, session, input)
	requireResponse(t, response, "recover dispatch", http.StatusOK, `"repeated":true`)
	var recovered syncRunNowResponse
	if err := json.Unmarshal(response.Body.Bytes(), &recovered); err != nil {
		t.Fatal(err)
	}
	if recovered.PlanID != accepted.PlanID || recovered.QueueID != accepted.QueueID || recovered.Plan != nil || recovered.Queue != nil {
		t.Fatalf("recovery rebound work: %s", response.Body.String())
	}
	assertSyncPlanNotDispatched(t, h, "newer")
	input.Reason = "Changed command"
	requireResponse(t, postDispatchReceipt(t, h, session, input), "conflicting dispatch", http.StatusConflict)
}

func TestSyncDispatchReceiptRequiresCurrentSession(t *testing.T) {
	h, session, input, _ := acceptedDispatchFixture(t)
	if err := h.store.DeleteSession(t.Context(), tokenHash(session.Value), storage.ElevationRevoked, h.now); err != nil {
		t.Fatal(err)
	}
	response := postDispatchReceipt(t, h, session, input)
	requireResponse(t, response, "revoked dispatch recovery", http.StatusUnauthorized)
	if strings.Contains(response.Body.String(), "queue_id") {
		t.Fatal("revoked session exposed accepted identity")
	}
}
