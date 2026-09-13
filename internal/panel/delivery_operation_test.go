package panel

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func TestQueueInspectorConnectsFailureToCurrentDelivery(t *testing.T) {
	h := newPanelHarness(t, "owner")
	session := h.signIn(t)
	claim := storage.DeliveryClaim{ClaimKey: "current-execution", DeliveryID: "current-execution", TargetID: "github:installation:10", Event: "issue_comment", Payload: []byte(`{"secret":"must-not-leak"}`), ClaimedAt: h.now}
	original, err := h.store.ClaimDelivery(t.Context(), claim)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.store.FailDelivery(t.Context(), storage.DeliveryFailureChange{ClaimID: original.ID, Reason: "original failure", Retryable: true, FailedAt: h.now}); err != nil {
		t.Fatal(err)
	}
	successor, err := h.store.ClaimDelivery(t.Context(), claim)
	if err != nil {
		t.Fatal(err)
	}
	itemID := fmt.Sprintf("delivery:%d", original.ID)
	for _, path := range []string{"/panel/api/v1/root/queue/", "/panel/api/v1/targets/github:installation:10/queue/"} {
		t.Run(path, func(t *testing.T) {
			response := h.request(t, http.MethodGet, path+itemID, nil, session)
			requireResponse(t, response, "current execution", http.StatusOK)
			var detail queueDetailResponse
			if err := json.Unmarshal(response.Body.Bytes(), &detail); err != nil {
				t.Fatal(err)
			}
			if detail.Item.State != workqueue.StateFailed || detail.Delivery == nil || !detail.Delivery.Retained || detail.Delivery.Current == nil {
				t.Fatalf("lost original failure or operation: %s", response.Body.String())
			}
			current := detail.Delivery.Current
			if current.ID != successor.ID || current.Queue == nil || current.Queue.State != workqueue.StateScheduled || !current.PayloadAvailable {
				t.Fatalf("incorrect current run: %#v", current)
			}
			if strings.Contains(response.Body.String(), "must-not-leak") {
				t.Fatal("exposed replay payload")
			}
		})
	}
	denied := h.request(t, http.MethodGet, "/panel/api/v1/targets/another-target/queue/"+itemID, nil, session)
	requireResponse(t, denied, "cross-target queue", http.StatusNotFound)
	assertPrunedDeliveryInspector(t, h, session, itemID)
}

func assertPrunedDeliveryInspector(t *testing.T, h *panelHarness, session *http.Cookie, itemID string) {
	t.Helper()

	if err := h.store.PruneDeliveries(t.Context(), h.now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	response := h.request(t, http.MethodGet, "/panel/api/v1/root/queue/"+itemID, nil, session)
	requireResponse(t, response, "pruned delivery", http.StatusOK)
	var detail queueDetailResponse
	if err := json.Unmarshal(response.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Delivery == nil || detail.Delivery.Retained || detail.Delivery.Current != nil {
		t.Fatalf("invented pruned delivery: %#v", detail.Delivery)
	}
}
