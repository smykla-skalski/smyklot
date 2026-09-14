package panel

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

const syncRequestsPath = "/panel/api/v1/targets/" + panelSyncTarget + "/sync/requests"

func TestSyncRequestsExposeAcceptedIdentityAndScopedContinuation(t *testing.T) {
	h, session, dispatch, accepted := acceptedDispatchFixture(t)
	_, err := h.store.DiscardSyncPlan(t.Context(), orgsync.PlanDiscard{TargetID: panelSyncTarget, PlanID: dispatch.PlanID, ActorID: "github:test:user:1", Now: h.now})
	if err != nil {
		t.Fatal(err)
	}
	requireResponse(t, postReceiptCheck(t, h, session, "Check saved settings"), "check accepted", http.StatusAccepted)
	response := h.request(t, http.MethodGet, syncRequestsPath+"?limit=1", nil, session)
	requireResponse(t, response, "accepted history", http.StatusOK)
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("history must not be cached")
	}
	var first syncRequestHistoryResponse
	if err := json.Unmarshal(response.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 1 || first.Items[0].Action != "dispatch" || first.Items[0].PlanID != accepted.PlanID || first.Items[0].QueueID != accepted.QueueID || first.Items[0].Reason != dispatch.Reason || !first.Items[0].AcceptedAt.Equal(h.now) || first.NextCursor == nil {
		t.Fatalf("incorrect acceptance: %s", response.Body.String())
	}
	var wire any
	if err := json.Unmarshal(response.Body.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	assertWireNames(t, syncRequestsPath, wire)
	if strings.Contains(response.Body.String(), `"total"`) || strings.Contains(response.Body.String(), `"state"`) || strings.Contains(response.Body.String(), `"check_id"`) {
		t.Fatal("dispatch history invented totals, progress or a check subject")
	}
	response = h.request(t, http.MethodGet, syncRequestsPath+"?cursor="+url.QueryEscape(*first.NextCursor), nil, session)
	requireResponse(t, response, "next page", http.StatusOK, `"action":"check"`, `"reason":"Check saved settings"`)
	var last syncRequestHistoryResponse
	if err := json.Unmarshal(response.Body.Bytes(), &last); err != nil {
		t.Fatal(err)
	}
	if len(last.Items) != 1 || last.Items[0].CheckID == "" || last.Items[0].PlanID != "" || last.Items[0].QueueID != "" || last.NextCursor != nil {
		t.Fatalf("incorrect check page: %s", response.Body.String())
	}
	_, other := seedNonOwnedWorkspace(t, h)
	requireResponse(t, h.request(t, http.MethodGet, "/panel/api/v1/targets/"+other.TargetID+"/sync/requests", nil, session), "inaccessible history", http.StatusNotFound)
}

func TestSyncRequestsRemainReadableAfterCommandPermissionIsLost(t *testing.T) {
	h := newPanelHarness(t, "owner")
	h.signIn(t)
	actor := storage.Account{ID: "request-reader", Provider: "github", SubjectID: "request-reader", Login: "reader"}
	if err := h.store.UpsertAccount(t.Context(), actor); err != nil {
		t.Fatal(err)
	}
	_, err := h.store.CreatePanelUser(t.Context(), storage.PanelUserCreate{AccountID: actor.ID, ActorAccountID: "github:test:user:1", ChangedAt: h.now})
	if err != nil {
		t.Fatal(err)
	}
	role := storage.InstallationRoleAdmin
	_, err = h.store.SetTargetAccess(t.Context(), storage.TargetAccessChange{TargetID: panelSyncTarget, SubjectAccountID: actor.ID, ActorAccountID: "github:test:user:1", Role: &role, ChangedAt: h.now})
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: sessionCookieName, Value: "history-reader-session"}
	if err := h.store.CreateSession(t.Context(), storage.Session{TokenHash: tokenHash(cookie.Value), AccountID: actor.ID, CreatedAt: h.now, ExpiresAt: h.now.Add(time.Hour)}, 2); err != nil {
		t.Fatal(err)
	}
	requireResponse(t, postReceiptCheck(t, h, cookie, "My accepted check"), "admin acceptance", http.StatusAccepted)
	role = storage.InstallationRoleViewer
	_, err = h.store.SetTargetAccess(t.Context(), storage.TargetAccessChange{TargetID: panelSyncTarget, SubjectAccountID: actor.ID, ActorAccountID: "github:test:user:1", Role: &role, ExpectedRevision: 1, ChangedAt: h.now})
	if err != nil {
		t.Fatal(err)
	}
	requireResponse(t, h.request(t, http.MethodGet, syncRequestsPath, nil, cookie), "viewer history", http.StatusOK, `"reason":"My accepted check"`)
	requireResponse(t, postReceiptCheck(t, h, cookie, "My accepted check"), "viewer command", http.StatusForbidden)
}

type revokingSyncHistoryStore struct {
	storage.Store
	before func()
}

func (s revokingSyncHistoryStore) ListSyncRequests(ctx context.Context, query orgsync.RequestHistoryQuery, now func() time.Time) (orgsync.RequestHistoryPage, error) {
	s.before()
	return s.Store.ListSyncRequests(ctx, query, now)
}

func TestSyncRequestsRecheckAccessAfterHTTPAuthorization(t *testing.T) {
	h := newPanelHarness(t, "owner")
	session := h.signIn(t)
	requireResponse(t, postReceiptCheck(t, h, session, "Accepted before revocation"), "acceptance", http.StatusAccepted)
	h.server.store = revokingSyncHistoryStore{Store: h.store, before: func() {
		if err := h.store.DeleteSession(t.Context(), tokenHash(session.Value), storage.ElevationRevoked, h.now); err != nil {
			t.Fatal(err)
		}
	}}
	response := h.request(t, http.MethodGet, syncRequestsPath, nil, session)
	requireResponse(t, response, "revoked history", http.StatusForbidden, `"code":"access_revoked"`)
	if strings.Contains(response.Body.String(), "check_id") {
		t.Fatal("revoked session exposed accepted identity")
	}
}

func TestSyncRequestHistoryQueryContract(t *testing.T) {
	cursor := syncRequestCursor{Version: 1, ActorID: "actor", TargetID: "target", AcceptedAt: time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC), Action: "check", RequestKey: "request"}
	encode := func(value any) string {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return base64.RawURLEncoding.EncodeToString(encoded)
	}
	good := encode(cursor)
	query, err := parseSyncRequestHistory(url.Values{"limit": {"100"}, "cursor": {good}}, "actor", "target")
	if err != nil || query.Limit != 100 || query.After == nil || query.After.RequestKey != "request" {
		t.Fatalf("valid cursor refused: %+v %v", query, err)
	}
	for _, change := range []func(*syncRequestCursor){
		func(c *syncRequestCursor) { c.ActorID = "someone-else" }, func(c *syncRequestCursor) { c.TargetID = "elsewhere" },
		func(c *syncRequestCursor) { c.Version = 2 }, func(c *syncRequestCursor) { c.Action = "retry" },
		func(c *syncRequestCursor) { c.AcceptedAt = time.Time{} }, func(c *syncRequestCursor) { c.RequestKey = "" },
		func(c *syncRequestCursor) { c.RequestKey = " padded" }, func(c *syncRequestCursor) { c.RequestKey = strings.Repeat("ą", 101) },
	} {
		bad := cursor
		change(&bad)
		if _, err := parseSyncRequestHistory(url.Values{"cursor": {encode(bad)}}, "actor", "target"); err == nil {
			t.Fatalf("invalid continuation accepted: %+v", bad)
		}
	}
	for _, values := range []url.Values{
		{"limit": {"0"}},
		{"limit": {"101"}},
		{"limit": {"1.5"}},
		{"limit": {"1", "2"}},
		{"cursor": {good, good}},
		{"actor": {"someone"}},
		{"cursor": {"!"}},
		{"cursor": {good + "\n"}},
		{"cursor": {strings.Repeat("x", 4097)}},
		{"cursor": {base64.RawURLEncoding.EncodeToString([]byte(`{} {}`))}},
		{"cursor": {encode(map[string]any{"version": 1, "extra": true})}},
	} {
		if _, err := parseSyncRequestHistory(values, "actor", "target"); err == nil {
			t.Fatalf("invalid query accepted: %v", values)
		}
	}
}
