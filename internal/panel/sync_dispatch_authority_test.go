package panel

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

type revokingDispatchStore struct {
	storage.Store
	before func()
}

func (s revokingDispatchStore) DispatchSyncPlan(ctx context.Context, request orgsync.PlanDispatch, now func() time.Time) (orgsync.PlanDispatchReceipt, error) {
	s.before()
	return s.Store.DispatchSyncPlan(ctx, request, now)
}

func TestSyncDispatchRechecksSessionAtAcceptance(t *testing.T) {
	h, session, input, _ := acceptedDispatchFixture(t)
	// A new explicit request passes HTTP access and capability reads first.
	item, err := h.store.GetQueueItem(t.Context(), "sync-plan:selected")
	if err != nil {
		t.Fatal(err)
	}
	input.RequestKey, input.ExpectedRevision = "after-access-check", item.Revision
	h.server.store = revokingDispatchStore{Store: h.store, before: func() {
		if err := h.store.DeleteSession(t.Context(), tokenHash(session.Value), storage.ElevationRevoked, h.now); err != nil {
			t.Fatal(err)
		}
	}}
	response := postDispatchReceipt(t, h, session, input)
	requireResponse(t, response, "revoked during dispatch", http.StatusForbidden, `"code":"access_revoked"`)
	after, err := h.store.GetQueueItem(t.Context(), item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != item.Revision {
		t.Fatal("revoked command changed the queue")
	}
}
