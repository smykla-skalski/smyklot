package panel

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

type revokingCheckStore struct {
	storage.Store
	before func()
	onRead bool
}

func (s revokingCheckStore) RequestRecurringWork(ctx context.Context, request workqueue.RecurringRequest) (workqueue.Item, error) {
	if !s.onRead {
		s.before()
	}
	return s.Store.RequestRecurringWork(ctx, request)
}

func (s revokingCheckStore) FindRecurringWorkRequest(ctx context.Context, request workqueue.RecurringRequest) (workqueue.Item, error) {
	if s.onRead {
		s.before()
	}
	return s.Store.FindRecurringWorkRequest(ctx, request)
}

func TestSyncCheckRechecksSessionAfterHTTPAccess(t *testing.T) {
	for _, onRead := range []bool{false, true} {
		name := "acceptance"
		if onRead {
			name = "receipt read"
		}
		t.Run(name, func(t *testing.T) {
			h := newPanelHarness(t, "owner")
			session := h.signIn(t)
			if onRead {
				requireResponse(t, postReceiptCheck(t, h, session, "Check settings"), "initial acceptance", http.StatusAccepted)
			}
			createPanelSyncPlan(t, h, "expired-authorization", h.now.Add(time.Minute))
			*h.clock = h.now.Add(time.Minute)
			h.server.store = revokingCheckStore{Store: h.store, onRead: onRead, before: func() {
				if err := h.store.DeleteSession(t.Context(), tokenHash(session.Value), storage.ElevationRevoked, *h.clock); err != nil {
					t.Fatal(err)
				}
			}}
			response := postReceiptCheck(t, h, session, "Check settings")
			requireResponse(t, response, "revoked inside command", http.StatusForbidden, `"code":"access_revoked"`)
			plan, _, err := h.store.GetSyncPlan(t.Context(), panelSyncTarget, "expired-authorization")
			if err != nil || plan.State != orgsync.PlanComputed {
				t.Fatalf("unauthorized check retired work: %+v %v", plan, err)
			}
		})
	}
}
