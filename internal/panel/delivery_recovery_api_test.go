package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

type recoveryAPICheck struct {
	reason DeliveryRecoveryReason
	calls  int
	before func()
}

func (c *recoveryAPICheck) CheckDeliveryRecovery(context.Context, storage.DeliveryRecoveryInput) (DeliveryRecoveryCheck, error) {
	c.calls++
	if c.before != nil {
		c.before()
	}
	return DeliveryRecoveryCheck{Reason: c.reason, Effect: "Process the original comment."}, nil
}

type recoveryAPIQueue struct{ wakes int }

func (q *recoveryAPIQueue) WakeQueue(lane workqueue.Lane) {
	if lane == workqueue.LaneWebhook {
		q.wakes++
	}
}

func recoveryAPIFixture(t *testing.T) (*panelHarness, *http.Cookie, int64, deliveryRecoveryInput) {
	t.Helper()
	h := newPanelHarness(t, "owner")
	session := h.signIn(t)
	claim, err := h.store.ClaimDelivery(t.Context(), storage.DeliveryClaim{ClaimKey: "api-recovery", DeliveryID: "api-recovery", TargetID: "github:installation:10", Event: "issue_comment", Payload: []byte(`{"secret":"private"}`), ClaimedAt: h.now})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.store.FailDelivery(t.Context(), storage.DeliveryFailureChange{ClaimID: claim.ID, Reason: "configuration failure", FailedAt: h.now}); err != nil {
		t.Fatal(err)
	}
	operation, err := h.store.GetDeliveryOperation(t.Context(), "github:installation:10", claim.ID)
	if err != nil {
		t.Fatal(err)
	}
	return h, session, claim.ID, deliveryRecoveryInput{ExpectedRunID: claim.ID, ExpectedRevision: operation.Revision, RequestKey: "request-once"}
}

func TestDeliveryRecoveryAPIKeepsAcceptedRequestAfterSuccess(t *testing.T) {
	for _, root := range []bool{false, true} {
		t.Run(fmt.Sprint(root), func(t *testing.T) {
			assertDeliveryRecoveryAccepted(t, root)
		})
	}
}

func TestDeliveryRecoveryAPIRejectsUnavailableAndWrongScope(t *testing.T) {
	h, session, source, input := recoveryAPIFixture(t)
	h.server.recovery = &recoveryAPICheck{reason: RecoveryConfigurationInvalid}
	path := fmt.Sprintf("/panel/api/v1/targets/github:installation:10/deliveries/%d/recovery", source)
	response := h.request(t, http.MethodPost, path, recoveryRequestBody(t, input), session)
	requireResponse(t, response, "configuration not fixed", http.StatusConflict)
	operation, err := h.store.GetDeliveryOperation(t.Context(), "github:installation:10", source)
	if err != nil || operation.Current.ID != source {
		t.Fatalf("unavailable recovery created work: %+v %v", operation, err)
	}
	wrong := fmt.Sprintf("/panel/api/v1/targets/another-target/deliveries/%d/recovery", source)
	response = h.request(t, http.MethodPost, wrong, recoveryRequestBody(t, input), session)
	requireResponse(t, response, "wrong target", http.StatusNotFound)
	input.RequestKey = ""
	response = h.request(t, http.MethodPost, path, recoveryRequestBody(t, input), session)
	requireResponse(t, response, "missing key", http.StatusBadRequest)
}

func recoveryRequestBody(t *testing.T, input deliveryRecoveryInput) *bytes.Reader {
	t.Helper()
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(data)
}

func assertDeliveryRecoveryAccepted(t *testing.T, root bool) {
	t.Helper()
	h, session, source, input := recoveryAPIFixture(t)
	checker := &recoveryAPICheck{reason: RecoveryAvailable}
	h.server.recovery = checker
	queue := &recoveryAPIQueue{}
	h.server.queue = queue
	prefix := "/panel/api/v1/targets/"
	if root {
		prefix = "/panel/api/v1/root/workspaces/"
	}
	path := fmt.Sprintf("%sgithub:installation:10/deliveries/%d/recovery", prefix, source)
	response := h.request(t, http.MethodGet, path, nil, session)
	requireResponse(t, response, "recovery preview", http.StatusOK)
	var preview deliveryRecoveryResponse
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if !preview.Available || preview.CurrentRunID != source || queue.wakes != 0 {
		t.Fatalf("incorrect preview: %+v", preview)
	}
	response = h.request(t, http.MethodPost, path, recoveryRequestBody(t, input), session)
	requireResponse(t, response, "recover", http.StatusAccepted)
	var accepted struct {
		RunID    int64 `json:"run_id"`
		Repeated bool  `json:"repeated"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &accepted); err != nil {
		t.Fatal(err)
	}
	if accepted.RunID <= source || accepted.Repeated || queue.wakes != 1 {
		t.Fatalf("incorrect accepted run: %+v", accepted)
	}
	lease, err := h.store.LeaseDelivery(t.Context(), h.now, h.now.Add(time.Minute))
	if err != nil || lease.Work == nil || lease.Work.ID != accepted.RunID || lease.Work.SourceOrder != source {
		t.Fatalf("recovery not dispatched: %+v, %v", lease, err)
	}
	if err := h.store.CompleteDelivery(t.Context(), accepted.RunID, h.now); err != nil {
		t.Fatal(err)
	}
	calls := checker.calls
	checker.reason = RecoveryPermissionChanged
	response = h.request(t, http.MethodPost, path, recoveryRequestBody(t, input), session)
	requireResponse(t, response, "repeat after success", http.StatusOK)
	var repeated struct {
		RunID    int64 `json:"run_id"`
		Repeated bool  `json:"repeated"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &repeated); err != nil {
		t.Fatal(err)
	}
	if !repeated.Repeated || repeated.RunID != accepted.RunID || checker.calls != calls {
		t.Fatalf("receipt lost after success: %+v", repeated)
	}
}
