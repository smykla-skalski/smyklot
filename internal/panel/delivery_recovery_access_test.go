package panel

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

func TestDeliveryRecoveryRootRequiresLiveElevation(t *testing.T) {
	h, session, _, _ := recoveryAPIFixture(t)
	_, target := seedNonOwnedWorkspace(t, h)
	claim, err := h.store.ClaimDelivery(t.Context(), storage.DeliveryClaim{ClaimKey: "elevated-recovery", DeliveryID: "elevated", TargetID: target.TargetID, Event: "issue_comment", Payload: []byte(`{}`), ClaimedAt: h.now})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.store.FailDelivery(t.Context(), storage.DeliveryFailureChange{ClaimID: claim.ID, Reason: "failure", FailedAt: h.now}); err != nil {
		t.Fatal(err)
	}
	operation, err := h.store.GetDeliveryOperation(t.Context(), target.TargetID, claim.ID)
	if err != nil {
		t.Fatal(err)
	}
	input := deliveryRecoveryInput{ExpectedRunID: claim.ID, ExpectedRevision: operation.Revision, RequestKey: "elevated-once"}
	checker := &recoveryAPICheck{reason: RecoveryAvailable}
	h.server.recovery = checker
	base := "/panel/api/v1/root/workspaces/" + target.TargetID
	path := fmt.Sprintf("%s/deliveries/%d/recovery", base, claim.ID)
	preview := h.request(t, http.MethodGet, path, nil, session)
	requireResponse(t, preview, "visit required", http.StatusOK, `"reason":"access_required"`)
	denied := h.request(t, http.MethodPost, path, recoveryRequestBody(t, input), session)
	requireResponse(t, denied, "without visit", http.StatusForbidden)
	if checker.calls != 0 {
		t.Fatal("unauthorized preview checked the workload")
	}
	started := h.request(t, http.MethodPost, base+"/elevation", strings.NewReader(`{"acknowledged":true,"reason":"recover failed processing"}`), session)
	requireResponse(t, started, "start visit", http.StatusCreated)
	accepted := h.request(t, http.MethodPost, path, recoveryRequestBody(t, input), session)
	requireResponse(t, accepted, "elevated recovery", http.StatusAccepted)
	var elevation elevationResponse
	if err := json.Unmarshal(started.Body.Bytes(), &elevation); err != nil {
		t.Fatal(err)
	}
	ended := h.request(t, http.MethodDelete, "/panel/api/v1/root/elevations/"+elevation.ID, nil, session)
	requireResponse(t, ended, "end visit", http.StatusOK)
	repeated := h.request(t, http.MethodPost, path, recoveryRequestBody(t, input), session)
	requireResponse(t, repeated, "revoked visit receipt", http.StatusGone)
}

func TestDeliveryRecoveryStaleInputSkipsWorkloadCheck(t *testing.T) {
	h, session, source, input := recoveryAPIFixture(t)
	checker := &recoveryAPICheck{reason: RecoveryAvailable}
	h.server.recovery = checker
	input.ExpectedRevision++
	path := fmt.Sprintf("/panel/api/v1/targets/github:installation:10/deliveries/%d/recovery", source)
	response := h.request(t, http.MethodPost, path, recoveryRequestBody(t, input), session)
	requireResponse(t, response, "stale recovery", http.StatusConflict, `"reason":"state_changed"`)
	if checker.calls != 0 {
		t.Fatal("stale request queried workload eligibility")
	}
}

func TestDeliveryRecoveryRechecksAccessAfterEligibility(t *testing.T) {
	h, session, source, input := recoveryAPIFixture(t)
	h.server.recovery = &recoveryAPICheck{reason: RecoveryAvailable, before: func() {
		if err := h.store.DeleteSession(t.Context(), tokenHash(session.Value), storage.ElevationRevoked, h.now); err != nil {
			t.Fatal(err)
		}
	}}
	path := fmt.Sprintf("/panel/api/v1/targets/github:installation:10/deliveries/%d/recovery", source)
	response := h.request(t, http.MethodPost, path, recoveryRequestBody(t, input), session)
	requireResponse(t, response, "access revoked during check", http.StatusForbidden, `"code":"access_revoked"`)
	operation, err := h.store.GetDeliveryOperation(t.Context(), "github:installation:10", source)
	if err != nil || operation.Current.ID != source {
		t.Fatalf("revoked recovery created work: %+v %v", operation, err)
	}
}
