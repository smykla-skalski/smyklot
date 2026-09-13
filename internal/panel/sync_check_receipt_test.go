package panel

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func postReceiptCheck(t *testing.T, h *panelHarness, session *http.Cookie, reason string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{"action": "check", "request_key": "check-request-1", "reason": reason})
	if err != nil {
		t.Fatal(err)
	}
	return h.request(t, http.MethodPost, "/panel/api/v1/targets/"+panelSyncTarget+"/sync/run-now", strings.NewReader(string(body)), session)
}

func TestSyncCheckRecoversAcceptanceBeforeNewerWork(t *testing.T) {
	h := newPanelHarness(t, "owner")
	session := h.signIn(t)
	first := postReceiptCheck(t, h, session, "Check saved settings")
	requireResponse(t, first, "first acceptance", http.StatusAccepted, `"status":"check_accepted"`)
	var accepted syncRunNowResponse
	if err := json.Unmarshal(first.Body.Bytes(), &accepted); err != nil {
		t.Fatal(err)
	}
	if accepted.CheckID == "" || accepted.Queue != nil {
		t.Fatalf("acceptance must identify a check without claiming live queue state: %s", first.Body.String())
	}
	targetID := panelSyncTarget
	leased, found, err := h.store.ClaimRecurringWork(t.Context(), workqueue.RecurringClaim{Kind: workqueue.KindSyncScan, TargetID: &targetID, Title: "Check", Now: h.now, LeaseDuration: time.Minute})
	if err != nil || !found {
		t.Fatalf("lease check: %t %v", found, err)
	}
	if _, err := h.store.FinishRecurringWork(t.Context(), leased.ID, workqueue.RecurringCompletion{Attempt: leased.Attempt}, h.now); err != nil {
		t.Fatal(err)
	}
	createPanelSyncPlan(t, h, "newer-work", h.now.Add(time.Hour))
	repeated := postReceiptCheck(t, h, session, "Check saved settings")
	requireResponse(t, repeated, "recovered acceptance", http.StatusOK, `"status":"check_accepted"`, `"repeated":true`)
	var recovered syncRunNowResponse
	if err := json.Unmarshal(repeated.Body.Bytes(), &recovered); err != nil {
		t.Fatal(err)
	}
	if recovered.CheckID != accepted.CheckID || recovered.Plan != nil || recovered.Queue != nil {
		t.Fatalf("recovery lost original identity: %s", repeated.Body.String())
	}
	changed := postReceiptCheck(t, h, session, "Different request")
	requireResponse(t, changed, "conflicting key", http.StatusConflict)
}

func TestSyncCheckReceiptRequiresCurrentSession(t *testing.T) {
	h := newPanelHarness(t, "owner")
	session := h.signIn(t)
	requireResponse(t, postReceiptCheck(t, h, session, "Check saved settings"), "accepted", http.StatusAccepted)
	if err := h.store.DeleteSession(t.Context(), tokenHash(session.Value), storage.ElevationRevoked, h.now); err != nil {
		t.Fatal(err)
	}
	response := postReceiptCheck(t, h, session, "Check saved settings")
	requireResponse(t, response, "revoked receipt read", http.StatusUnauthorized)
	if strings.Contains(response.Body.String(), "check_id") {
		t.Fatal("revoked session exposed check identity")
	}
}

func TestSyncCheckRetiresExpiredWaitingPlan(t *testing.T) {
	h := newPanelHarness(t, "owner")
	session := h.signIn(t)
	createPanelSyncPlan(t, h, "expired-blocker", h.now.Add(time.Minute))
	*h.clock = h.now.Add(time.Minute)
	response := postReceiptCheck(t, h, session, "Check current settings")
	requireResponse(t, response, "expired plan recovery", http.StatusAccepted, `"status":"check_accepted"`)
	plan, _, err := h.store.GetSyncPlan(t.Context(), panelSyncTarget, "expired-blocker")
	if err != nil || plan.State != "expired" {
		t.Fatalf("old plan was not retired: %#v %v", plan, err)
	}
}
