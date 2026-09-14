package panel

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestDeliveryRecoveryRechecksTimeAfterEligibility(t *testing.T) {
	h, session, source, input := recoveryAPIFixture(t)
	h.server.recovery = &recoveryAPICheck{reason: RecoveryAvailable, before: func() { *h.clock = h.now.Add(2 * time.Hour) }}
	path := fmt.Sprintf("/panel/api/v1/targets/github:installation:10/deliveries/%d/recovery", source)
	response := h.request(t, http.MethodPost, path, recoveryRequestBody(t, input), session)
	requireResponse(t, response, "authority expired during eligibility", http.StatusForbidden, `"code":"access_revoked"`)
	operation, err := h.store.GetDeliveryOperation(t.Context(), "github:installation:10", source)
	if err != nil || operation.Current.ID != source {
		t.Fatalf("expired recovery created work: %+v %v", operation, err)
	}
}
