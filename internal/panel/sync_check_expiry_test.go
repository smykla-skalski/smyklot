package panel

import (
	"net/http"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
)

func TestSyncCheckRefreshesTimeAtStorageBoundary(t *testing.T) {
	for _, expireAuthority := range []bool{false, true} {
		name := "approval expired"
		if expireAuthority {
			name = "authority expired"
		}
		t.Run(name, func(t *testing.T) {
			h := newPanelHarness(t, "owner")
			session := h.signIn(t)
			createPanelSyncPlan(t, h, "waiting-clock", h.now.Add(time.Minute))
			h.server.store = revokingCheckStore{Store: h.store, before: func() {
				*h.clock = h.now.Add(2 * time.Minute)
				if expireAuthority {
					*h.clock = h.now.Add(2 * time.Hour)
				}
			}}
			response := postReceiptCheck(t, h, session, "Check current settings")
			expected, state := http.StatusAccepted, orgsync.PlanExpired
			if expireAuthority {
				expected, state = http.StatusForbidden, orgsync.PlanComputed
			}
			requireResponse(t, response, "time changed before acceptance", expected)
			plan, _, err := h.store.GetSyncPlan(t.Context(), panelSyncTarget, "waiting-clock")
			if err != nil || plan.State != state {
				t.Fatalf("wrong retirement outcome: %+v %v", plan, err)
			}
		})
	}
}
