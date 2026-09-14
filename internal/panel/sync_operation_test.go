package panel

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func readOperation(t *testing.T, h *panelHarness, session *http.Cookie, action, key string) syncOperationResponse {
	t.Helper()
	path := syncRequestsPath + "/" + action + "/" + url.PathEscape(key)
	response := h.request(t, http.MethodGet, path, nil, session)
	requireResponse(t, response, "operation", http.StatusOK)
	var body syncOperationResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.TargetID != panelSyncTarget || body.Acceptance.RequestKey != key || body.ObservationStartedAt.IsZero() || body.ObservedAt.Before(body.ObservationStartedAt) {
		t.Fatalf("lost subject or observation interval: %#v", body)
	}
	var wire any
	if err := json.Unmarshal(response.Body.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	assertWireNames(t, path, wire)
	return body
}

func TestSyncOperationRetainsIntentAndComparisonAfterExecutionFailure(t *testing.T) {
	h := newPanelHarness(t, "owner")
	session := h.signIn(t)
	requireResponse(t, postReceiptCheck(t, h, session, "Original reason"), "accept", http.StatusAccepted)
	pending := readOperation(t, h, session, "check", "check-request-1")
	if pending.Comparison != nil || pending.Execution == nil || pending.Plan != nil || pending.Dispatch != nil {
		t.Fatalf("pending acceptance fabricated result: %#v", pending)
	}
	id := createPanelCheckEvidence(t, h, panelSyncTarget, 2, true)
	compared := readOperation(t, h, session, "check", "check-request-1")
	if compared.Acceptance.CheckID != id || compared.Comparison == nil || compared.Comparison.Outcome == nil || compared.Execution == nil || compared.Execution.State != workqueue.StateRunning {
		t.Fatalf("lost comparison or worker facts: %#v", compared)
	}
	_, err := h.store.FinishRecurringWork(t.Context(), id, workqueue.RecurringCompletion{Attempt: compared.Execution.Attempt, Failure: "Completion failed", Retryable: false}, h.now)
	if err != nil {
		t.Fatal(err)
	}
	failed := readOperation(t, h, session, "check", "check-request-1")
	if failed.Execution == nil || failed.Execution.State != workqueue.StateFailed || !reflect.DeepEqual(failed.Comparison, compared.Comparison) {
		t.Fatalf("failure conflated with comparison: %#v", failed)
	}
	if _, err := h.store.PruneWorkQueue(t.Context(), h.now.AddDate(2, 0, 0)); err != nil {
		t.Fatal(err)
	}
	createPanelSyncPlan(t, h, "newer-operation", h.now.Add(time.Hour))
	retained := readOperation(t, h, session, "check", "check-request-1")
	if retained.Execution != nil || retained.Acceptance != pending.Acceptance || !reflect.DeepEqual(retained.Comparison, compared.Comparison) || retained.Check.BlockingPlanID != "newer-operation" {
		t.Fatalf("original result or current blocker lost: %#v", retained)
	}
}

func TestSyncOperationDispatchKeepsOriginalPlanAfterPruning(t *testing.T) {
	h, session, input, _ := acceptedDispatchFixture(t)
	before := readOperation(t, h, session, "dispatch", input.RequestKey)
	if before.Plan == nil || before.Plan.ID != input.PlanID || before.Execution == nil || before.Dispatch == nil || before.Comparison != nil {
		t.Fatalf("incomplete dispatch: %#v", before)
	}
	_, err := h.store.DiscardSyncPlan(t.Context(), orgsync.PlanDiscard{TargetID: panelSyncTarget, PlanID: input.PlanID, ActorID: "github:test:user:1", Now: h.now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.PruneWorkQueue(t.Context(), h.now.AddDate(2, 0, 0)); err != nil {
		t.Fatal(err)
	}
	createPanelSyncPlan(t, h, "newer-operation", h.now.Add(time.Hour))
	after := readOperation(t, h, session, "dispatch", input.RequestKey)
	if after.Acceptance != before.Acceptance || after.Plan == nil || after.Plan.ID != input.PlanID || after.Execution != nil || after.Dispatch == nil || after.Dispatch.Available {
		t.Fatalf("newer work replaced dispatch: %#v", after)
	}
}

type operationComparisonStore struct {
	storage.Store
	read func(context.Context, string, string) (orgsync.CheckDetails, error)
}

func (s operationComparisonStore) GetSyncCheckResult(ctx context.Context, target, id string) (orgsync.CheckDetails, error) {
	return s.read(ctx, target, id)
}

func TestSyncOperationWithholdsPartialResponseOnReadFailureOrRevocation(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		t.Run(map[bool]string{false: "read failure", true: "revocation during projection"}[revoke], func(t *testing.T) {
			h := newPanelHarness(t, "owner")
			session := h.signIn(t)
			requireResponse(t, postReceiptCheck(t, h, session, "Private original reason"), "accept", http.StatusAccepted)
			h.server.store = operationComparisonStore{Store: h.store, read: func(context.Context, string, string) (orgsync.CheckDetails, error) {
				if !revoke {
					return orgsync.CheckDetails{}, errors.New("read failed")
				}
				if err := h.store.DeleteSession(t.Context(), tokenHash(session.Value), storage.ElevationRevoked, h.now); err != nil {
					t.Fatal(err)
				}
				return orgsync.CheckDetails{}, storage.ErrNotFound
			}}
			status := http.StatusInternalServerError
			if revoke {
				status = http.StatusForbidden
			}
			response := h.request(t, http.MethodGet, syncRequestsPath+"/check/check-request-1", nil, session)
			requireResponse(t, response, "withheld operation", status)
			if strings.Contains(response.Body.String(), "Private original reason") || strings.Contains(response.Body.String(), "acceptance") {
				t.Fatal("partial operation leaked")
			}
		})
	}
}

func TestSyncOperationKeepsAcceptanceWhenNoResultWasRecorded(t *testing.T) {
	h := newPanelHarness(t, "owner")
	session := h.signIn(t)
	requireResponse(t, postReceiptCheck(t, h, session, "Unrecorded outcome"), "accept", http.StatusAccepted)
	before := readOperation(t, h, session, "check", "check-request-1")
	id := createPanelCheckEvidence(t, h, panelSyncTarget, 0, false)
	item, err := h.store.GetQueueItem(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.FinishRecurringWork(t.Context(), id, workqueue.RecurringCompletion{Attempt: item.Attempt}, h.now); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.PruneWorkQueue(t.Context(), h.now.AddDate(2, 0, 0)); err != nil {
		t.Fatal(err)
	}
	after := readOperation(t, h, session, "check", "check-request-1")
	if after.Acceptance != before.Acceptance || after.Comparison != nil || after.Execution != nil {
		t.Fatalf("invented unavailable outcome: %#v", after)
	}
}
