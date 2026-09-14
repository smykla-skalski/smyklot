package panel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

func TestSyncRequestReadReturnsExactAcceptance(t *testing.T) {
	h, session, dispatch, accepted := acceptedDispatchFixture(t)
	path := syncRequestsPath + "/dispatch/" + url.PathEscape(dispatch.RequestKey)
	response := h.request(t, http.MethodGet, path, nil, session)
	requireResponse(t, response, "exact dispatch", http.StatusOK)
	var wire any
	if err := json.Unmarshal(response.Body.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	assertWireNames(t, path, wire)
	var body syncOperationResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	got := body.Acceptance
	if got.PlanID != accepted.PlanID || got.QueueID != accepted.QueueID || got.ExpectedRevision != dispatch.ExpectedRevision || got.Reason != dispatch.Reason || !got.AcceptedAt.Equal(h.now) {
		t.Fatalf("lost acceptance: %#v", got)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("cacheable receipt")
	}
	requireResponse(t, h.request(t, http.MethodGet, syncRequestsPath+"/check/"+url.PathEscape(dispatch.RequestKey), nil, session), "wrong action", http.StatusNotFound)
	requireResponse(t, h.request(t, http.MethodGet, syncRequestsPath+"/check/not-accepted", nil, session), "missing acceptance", http.StatusNotFound)
	for _, suffix := range []string{"/retry/key", "/check/%20padded", "/check/" + strings.Repeat("a", 201), "/check/key?actor=another"} {
		requireResponse(t, h.request(t, http.MethodGet, syncRequestsPath+suffix, nil, session), "invalid lookup", http.StatusBadRequest)
	}
}

type revokingSyncLookupStore struct {
	storage.Store
	before func()
}

func (s revokingSyncLookupStore) GetSyncRequest(ctx context.Context, query orgsync.RequestLookup, now func() time.Time) (orgsync.RequestAcceptance, error) {
	s.before()
	return s.Store.GetSyncRequest(ctx, query, now)
}

func TestSyncRequestReadRechecksAuthority(t *testing.T) {
	h := newPanelHarness(t, "owner")
	session := h.signIn(t)
	requireResponse(t, postReceiptCheck(t, h, session, "Original check"), "accepted", http.StatusAccepted)
	path := syncRequestsPath + "/check/check-request-1"
	requireResponse(t, h.request(t, http.MethodGet, path, nil, session), "exact check", http.StatusOK, `"reason":"Original check"`, `"check_id":`)
	h.server.store = revokingSyncLookupStore{Store: h.store, before: func() {
		if err := h.store.DeleteSession(t.Context(), tokenHash(session.Value), storage.ElevationRevoked, h.now); err != nil {
			t.Fatal(err)
		}
	}}
	response := h.request(t, http.MethodGet, path, nil, session)
	requireResponse(t, response, "revoked lookup", http.StatusForbidden, `"code":"access_revoked"`)
	if strings.Contains(response.Body.String(), "check_id") {
		t.Fatal("receipt leaked after revocation")
	}
}
