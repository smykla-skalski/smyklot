package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/smykla-skalski/smyklot/internal/storage"
	storagesqlite "github.com/smykla-skalski/smyklot/internal/storage/sqlite"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

type fakeSignIn struct {
	account storage.Account
}

func (f fakeSignIn) AuthorizeURL(state string) string {
	return "https://github.example/authorize?state=" + url.QueryEscape(state)
}

func (f fakeSignIn) ExchangeIdentity(context.Context, string) (storage.Account, error) {
	return f.account, nil
}

type fakeCatalog struct {
	store    storage.Store
	snapshot storage.InstallationSnapshot
}

type fakeUserResolver struct {
	account storage.Account
}

func (f fakeUserResolver) ResolveUser(
	context.Context,
	string,
	string,
) (storage.Account, error) {
	return f.account, nil
}

func (f fakeCatalog) SyncCatalog(ctx context.Context) ([]string, error) {
	if err := f.store.ReconcileInstallation(ctx, f.snapshot); err != nil {
		return nil, err
	}

	return []string{f.snapshot.TargetID}, nil
}

type panelHarness struct {
	server  *Server
	store   storage.Store
	handler http.Handler
	now     time.Time
}

func newPanelHarness(t *testing.T, login string) *panelHarness {
	t.Helper()
	now := time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)
	store, err := storagesqlite.Open(t.Context(), filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	viewer := storage.Account{
		ID:          "github:test:user:1",
		Provider:    "github:test",
		SubjectID:   "1",
		Login:       login,
		DisplayName: "Panel Owner",
	}
	targetAccount := storage.Account{
		ID:          "github:test:account:2",
		Provider:    "github:test",
		SubjectID:   "2",
		Login:       "smykla-skalski",
		DisplayName: "Smykla Skalski",
		UpdatedAt:   now,
	}
	snapshot := storage.InstallationSnapshot{
		TargetID:       "github:installation:10",
		InstallationID: "10",
		Kind:           storage.TargetOrganization,
		Account:        targetAccount,
		Repositories: []storage.RepositorySnapshot{{
			ID:       "repository-20",
			Name:     "smyklot",
			FullName: "smykla-skalski/smyklot",
		}},
		SyncedAt: now,
	}
	assets := fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte(`<!doctype html><meta name="smyklot-panel-base" content="/__smyklot_panel_base__"><meta name="smyklot-panel-version" content="__smyklot_panel_version__"><meta name="smyklot-panel-service" content="__smyklot_panel_service__"><link rel="icon" href="/__smyklot_panel_base__/smyklot-avatar.png?v=__smyklot_panel_version__">`)},
		"assets/app.js": &fstest.MapFile{Data: []byte("export {}")},
	}
	randomBytes := make([]byte, 0, tokenBytes*32)
	for index := range 32 {
		randomBytes = append(randomBytes, bytes.Repeat([]byte{byte(index + 1)}, tokenBytes)...)
	}
	random := bytes.NewReader(randomBytes)
	server, err := New(Config{
		BasePath:      "/panel",
		PublicOrigin:  "https://smyklot.example",
		OwnerLogin:    "owner",
		ClientID:      "client-id",
		ClientSecret:  "client-secret",
		AuthorizeURL:  "https://github.example/authorize",
		TokenURL:      "https://github.example/token",
		APIURL:        "https://api.github.example",
		Version:       "1.0.0",
		ServiceHost:   "smyklot.example",
		ProcessConfig: config.Default(),
		Assets:        assets,
	}, Dependencies{
		Store:   store,
		Catalog: fakeCatalog{store: store, snapshot: snapshot},
		Users:   fakeUserResolver{account: viewer},
		SignIn:  fakeSignIn{account: viewer},
		Random:  random,
		Now:     func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}

	return &panelHarness{server: server, store: store, handler: server.Handler(), now: now}
}

func (h *panelHarness) signIn(t *testing.T) *http.Cookie {
	t.Helper()
	start := httptest.NewRecorder()
	h.handler.ServeHTTP(start, httptest.NewRequest(http.MethodGet, "/panel/auth/github/start", nil))
	if start.Code != http.StatusFound {
		t.Fatalf("start status = %d, body = %s", start.Code, start.Body.String())
	}
	stateCookie := responseCookie(t, start, stateCookieName)
	location, err := url.Parse(start.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	callback := httptest.NewRequest(
		http.MethodGet,
		"/panel/auth/github/callback?code=code&state="+url.QueryEscape(location.Query().Get("state")),
		nil,
	)
	callback.AddCookie(stateCookie)
	finished := httptest.NewRecorder()
	h.handler.ServeHTTP(finished, callback)
	if finished.Code != http.StatusFound {
		t.Fatalf("callback status = %d, body = %s", finished.Code, finished.Body.String())
	}
	for _, cookie := range finished.Result().Cookies() {
		if cookie.Name == sessionCookieName {
			return cookie
		}
	}
	t.Fatal("callback did not issue a session cookie")

	return nil
}

func responseCookie(t *testing.T, response *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == name && cookie.MaxAge > 0 {
			return cookie
		}
	}
	t.Fatalf("response did not issue the %s cookie", name)

	return nil
}

func requireResponse(
	t *testing.T,
	response *httptest.ResponseRecorder,
	label string,
	status int,
	fragments ...string,
) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("%s = %d %s", label, response.Code, response.Body.String())
	}
	for _, fragment := range fragments {
		if !strings.Contains(response.Body.String(), fragment) {
			t.Fatalf("%s is missing %q: %s", label, fragment, response.Body.String())
		}
	}
}

func (h *panelHarness) request(
	t *testing.T,
	method, path string,
	body io.Reader,
	session *http.Cookie,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, body)
	if session != nil {
		request.AddCookie(session)
	}
	if method != http.MethodGet {
		request.Header.Set("Origin", "https://smyklot.example")
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	h.handler.ServeHTTP(response, request)

	return response
}

func TestPanelSignInAndSettings(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)

	viewer := harness.request(t, http.MethodGet, "/panel/api/v1/session", nil, session)
	if viewer.Code != http.StatusOK || !strings.Contains(viewer.Body.String(), `"target_count":1`) {
		t.Fatalf("viewer response = %d %s", viewer.Code, viewer.Body.String())
	}

	input := `{"repository_default_enabled":true,"config_patch":{"quiet_success":true},"expected_revision":1}`
	updated := harness.request(
		t,
		http.MethodPut,
		"/panel/api/v1/targets/github:installation:10/settings",
		strings.NewReader(input),
		session,
	)
	if updated.Code != http.StatusOK {
		t.Fatalf("target update = %d %s", updated.Code, updated.Body.String())
	}
	var target targetResponse
	if err := json.Unmarshal(updated.Body.Bytes(), &target); err != nil {
		t.Fatal(err)
	}
	if !target.RepositoryDefaultEnabled ||
		!target.EffectiveConfig.QuietSuccess ||
		target.InheritedConfig.QuietSuccess ||
		target.Revision != 2 {
		t.Fatalf("unexpected target: %#v", target)
	}

	audit := harness.request(
		t,
		http.MethodGet,
		"/panel/api/v1/targets/github:installation:10/audit",
		nil,
		session,
	)
	if audit.Code != http.StatusOK || !strings.Contains(audit.Body.String(), "target.settings.updated") {
		t.Fatalf("audit response = %d %s", audit.Code, audit.Body.String())
	}
}

func TestPanelWebSocketEvents(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	endpoint := httptest.NewServer(harness.handler)
	t.Cleanup(endpoint.Close)
	streamURL := "ws" + strings.TrimPrefix(endpoint.URL, "http") + "/panel/api/v1/events"

	unauthenticated, response, err := websocket.Dial(t.Context(), streamURL, nil)
	if unauthenticated != nil {
		_ = unauthenticated.CloseNow()
	}
	if err == nil || response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated WebSocket = response %#v, error %v", response, err)
	}

	headers := http.Header{}
	headers.Set("Cookie", session.String())
	headers.Set("Origin", "https://untrusted.example")
	untrusted, response, err := websocket.Dial(t.Context(), streamURL, &websocket.DialOptions{
		HTTPHeader: headers,
	})
	if untrusted != nil {
		_ = untrusted.CloseNow()
	}
	if err == nil || response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin WebSocket = response %#v, error %v", response, err)
	}

	headers.Set("Origin", "https://smyklot.example")
	connection, response, err := websocket.Dial(t.Context(), streamURL, &websocket.DialOptions{
		HTTPHeader: headers,
	})
	if err != nil {
		t.Fatalf("dial WebSocket: response %#v, error %v", response, err)
	}
	t.Cleanup(func() { _ = connection.CloseNow() })

	var ready panelEvent
	if err := wsjson.Read(t.Context(), connection, &ready); err != nil {
		t.Fatal(err)
	}
	if ready.Version != panelEventVersion || ready.Type != "ready" {
		t.Fatalf("ready event = %#v", ready)
	}

	harness.server.Announce("github:installation:10", "repository-20")
	var changed panelEvent
	if err := wsjson.Read(t.Context(), connection, &changed); err != nil {
		t.Fatal(err)
	}
	if changed.Type != "repository.changed" ||
		changed.TargetID != "github:installation:10" ||
		changed.RepositoryID != "repository-20" {
		t.Fatalf("changed event = %#v", changed)
	}

	signedOut := harness.request(
		t,
		http.MethodPost,
		"/panel/api/v1/sign-out",
		nil,
		session,
	)
	if signedOut.Code != http.StatusNoContent {
		t.Fatalf("sign out = %d %s", signedOut.Code, signedOut.Body.String())
	}
	var revoked panelEvent
	if err := wsjson.Read(t.Context(), connection, &revoked); err != nil {
		t.Fatal(err)
	}
	if revoked.Type != "session.revoked" || revoked.Code != "signed_out" {
		t.Fatalf("revoked event = %#v", revoked)
	}
}

func TestPanelEnforcesResolvedRoleCapabilities(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	_ = harness.signIn(t)
	viewer := storage.Account{
		ID:          "github:test:user:viewer",
		Provider:    "github:test",
		SubjectID:   "viewer",
		Login:       "viewer",
		DisplayName: "Panel Viewer",
		UpdatedAt:   harness.now,
	}
	if err := harness.store.UpsertAccount(t.Context(), viewer); err != nil {
		t.Fatal(err)
	}
	if _, err := harness.store.CreatePanelUser(t.Context(), storage.PanelUserCreate{
		AccountID:      viewer.ID,
		GlobalRole:     storage.PanelRoleViewer,
		ActorAccountID: "github:test:user:1",
		ChangedAt:      harness.now,
	}); err != nil {
		t.Fatal(err)
	}
	const viewerToken = "viewer-session"
	if err := harness.store.CreateSession(t.Context(), storage.Session{
		TokenHash: tokenHash(viewerToken),
		AccountID: viewer.ID,
		CreatedAt: harness.now,
		ExpiresAt: harness.now.Add(time.Hour),
	}, 1); err != nil {
		t.Fatal(err)
	}
	viewerSession := &http.Cookie{Name: sessionCookieName, Value: viewerToken}

	targets := harness.request(t, http.MethodGet, "/panel/api/v1/targets", nil, viewerSession)
	if targets.Code != http.StatusOK ||
		!strings.Contains(targets.Body.String(), `"effective_role":"viewer"`) ||
		!strings.Contains(targets.Body.String(), `"write":false`) {
		t.Fatalf("viewer targets = %d %s", targets.Code, targets.Body.String())
	}
	input := `{"repository_default_enabled":true,"config_patch":{},"expected_revision":1}`
	denied := harness.request(
		t,
		http.MethodPut,
		"/panel/api/v1/targets/github:installation:10/settings",
		strings.NewReader(input),
		viewerSession,
	)
	if denied.Code != http.StatusNotFound {
		t.Fatalf("viewer write = %d %s", denied.Code, denied.Body.String())
	}

	editor := storage.PanelRoleEditor
	override, err := harness.store.SetTargetAccess(t.Context(), storage.TargetAccessChange{
		TargetID:         "github:installation:10",
		SubjectAccountID: viewer.ID,
		ActorAccountID:   "github:test:user:1",
		Role:             &editor,
		ExpectedRevision: 0,
		ChangedAt:        harness.now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	updated := harness.request(
		t,
		http.MethodPut,
		"/panel/api/v1/targets/github:installation:10/settings",
		strings.NewReader(input),
		viewerSession,
	)
	if updated.Code != http.StatusOK {
		t.Fatalf("editor write = %d %s", updated.Code, updated.Body.String())
	}

	noAccess := storage.PanelRoleNone
	if _, err := harness.store.SetTargetAccess(t.Context(), storage.TargetAccessChange{
		TargetID:         "github:installation:10",
		SubjectAccountID: viewer.ID,
		ActorAccountID:   "github:test:user:1",
		Role:             &noAccess,
		ExpectedRevision: override.Revision,
		ChangedAt:        harness.now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	missing := harness.request(
		t,
		http.MethodGet,
		"/panel/api/v1/targets/github:installation:10/repositories",
		nil,
		viewerSession,
	)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("no-access read = %d %s", missing.Code, missing.Body.String())
	}
}

func TestPanelAuthorizesActiveUserWithOnlyTargetAccess(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	_ = harness.signIn(t)
	viewer := storage.Account{
		ID:          "github:test:user:target-only",
		Provider:    "github:test",
		SubjectID:   "target-only",
		Login:       "target-only",
		DisplayName: "Target Only",
		UpdatedAt:   harness.now,
	}
	if err := harness.store.UpsertAccount(t.Context(), viewer); err != nil {
		t.Fatal(err)
	}
	if _, err := harness.store.CreatePanelUser(t.Context(), storage.PanelUserCreate{
		AccountID:      viewer.ID,
		GlobalRole:     storage.PanelRoleNone,
		ActorAccountID: "github:test:user:1",
		ChangedAt:      harness.now,
	}); err != nil {
		t.Fatal(err)
	}
	role := storage.PanelRoleViewer
	if _, err := harness.store.SetTargetAccess(t.Context(), storage.TargetAccessChange{
		TargetID:         "github:installation:10",
		SubjectAccountID: viewer.ID,
		ActorAccountID:   "github:test:user:1",
		Role:             &role,
		ExpectedRevision: 0,
		ChangedAt:        harness.now,
	}); err != nil {
		t.Fatal(err)
	}

	authorized, err := harness.server.authorizeAccount(
		httptest.NewRequest(http.MethodGet, "/panel/auth/callback", nil),
		viewer,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !authorized {
		t.Fatal("active user with installation access was not authorized")
	}
}

func TestPanelManagesUsersAndRevokesBannedSessions(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	ownerSession := harness.signIn(t)
	managed := storage.Account{
		ID:          "github:test:user:managed",
		Provider:    "github:test",
		SubjectID:   "managed",
		Login:       "managed",
		DisplayName: "Managed User",
		UpdatedAt:   harness.now,
	}
	harness.server.users = fakeUserResolver{account: managed}
	added := harness.request(
		t,
		http.MethodPost,
		"/panel/api/v1/users",
		strings.NewReader(`{"login":"managed","role":"editor","target_id":"github:installation:10"}`),
		ownerSession,
	)
	if added.Code != http.StatusCreated ||
		!strings.Contains(added.Body.String(), `"global_role":"editor"`) {
		t.Fatalf("add user = %d %s", added.Code, added.Body.String())
	}
	listed := harness.request(t, http.MethodGet, "/panel/api/v1/users", nil, ownerSession)
	requireResponse(
		t, listed, "list users", http.StatusOK,
		`"items"`, `"login":"managed"`, `"total":2`,
	)
	filtered := harness.request(
		t,
		http.MethodGet,
		"/panel/api/v1/users?q=managed&role=editor&status=active&sort=updated_newest&limit=1",
		nil,
		ownerSession,
	)
	requireResponse(
		t, filtered, "filter users", http.StatusOK, `"login":"managed"`, `"total":1`,
	)
	invalidPage := harness.request(
		t, http.MethodGet, "/panel/api/v1/users?limit=0", nil, ownerSession,
	)
	requireResponse(t, invalidPage, "invalid user page", http.StatusBadRequest)

	const managedToken = "managed-session"
	if err := harness.store.CreateSession(t.Context(), storage.Session{
		TokenHash: tokenHash(managedToken), AccountID: managed.ID, CreatedAt: harness.now,
		ExpiresAt: harness.now.Add(time.Hour),
	}, 1); err != nil {
		t.Fatal(err)
	}
	ban := harness.request(
		t,
		http.MethodPut,
		"/panel/api/v1/users/"+url.PathEscape(managed.ID),
		strings.NewReader(`{"global_role":"editor","status":"banned","ban_reason":"security review","expected_revision":1}`),
		ownerSession,
	)
	if ban.Code != http.StatusOK || !strings.Contains(ban.Body.String(), `"status":"banned"`) {
		t.Fatalf("ban user = %d %s", ban.Code, ban.Body.String())
	}
	decisions := harness.request(
		t,
		http.MethodGet,
		"/panel/api/v1/users/"+url.PathEscape(managed.ID)+"/decisions",
		nil,
		ownerSession,
	)
	requireResponse(
		t, decisions, "list global access decisions", http.StatusOK,
		`"action":"user.banned"`, `"summary":"banned user: security review"`,
	)
	managedSession := &http.Cookie{Name: sessionCookieName, Value: managedToken}
	revoked := harness.request(t, http.MethodGet, "/panel/api/v1/session", nil, managedSession)
	if revoked.Code != http.StatusUnauthorized ||
		!strings.Contains(revoked.Body.String(), `"code":"session_revoked"`) ||
		!strings.Contains(revoked.Body.String(), "security review") {
		t.Fatalf("revoked session = %d %s", revoked.Code, revoked.Body.String())
	}
	blockedTargetChange := harness.request(
		t,
		http.MethodPut,
		"/panel/api/v1/targets/github:installation:10/users/"+url.PathEscape(managed.ID),
		strings.NewReader(`{"role":"viewer","suspended":true,"expected_revision":0}`),
		ownerSession,
	)
	if blockedTargetChange.Code != http.StatusForbidden {
		t.Fatalf(
			"target change for globally banned user = %d %s",
			blockedTargetChange.Code,
			blockedTargetChange.Body.String(),
		)
	}

	unbanned := harness.request(
		t,
		http.MethodPut,
		"/panel/api/v1/users/"+url.PathEscape(managed.ID),
		strings.NewReader(`{"global_role":"none","status":"active","expected_revision":2}`),
		ownerSession,
	)
	if unbanned.Code != http.StatusOK {
		t.Fatalf("unban user = %d %s", unbanned.Code, unbanned.Body.String())
	}
	assigned := harness.request(
		t,
		http.MethodPost,
		"/panel/api/v1/targets/github:installation:10/users",
		strings.NewReader(`{"login":"managed","role":"viewer"}`),
		ownerSession,
	)
	if assigned.Code != http.StatusCreated ||
		!strings.Contains(assigned.Body.String(), `"effective_role":"viewer"`) {
		t.Fatalf("assign target user = %d %s", assigned.Code, assigned.Body.String())
	}
	suspended := harness.request(
		t,
		http.MethodPut,
		"/panel/api/v1/targets/github:installation:10/users/"+url.PathEscape(managed.ID),
		strings.NewReader(`{"role":"viewer","suspended":true,"suspension_reason":"incident review","expected_revision":1}`),
		ownerSession,
	)
	if suspended.Code != http.StatusOK ||
		!strings.Contains(suspended.Body.String(), `"source":"suspended"`) {
		t.Fatalf("suspend target user = %d %s", suspended.Code, suspended.Body.String())
	}
	targetDecisions := harness.request(
		t,
		http.MethodGet,
		"/panel/api/v1/targets/github:installation:10/users/"+
			url.PathEscape(managed.ID)+"/decisions",
		nil,
		ownerSession,
	)
	requireResponse(
		t, targetDecisions, "list target access decisions", http.StatusOK,
		`"action":"target.access.suspended"`,
		`"summary":"suspended installation access: incident review"`,
	)
}

func TestPanelInvitesNamedGitHubUserThroughOAuth(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	ownerSession := harness.signIn(t)
	invitee := storage.Account{
		ID: "github:test:user:invited", Provider: "github:test", SubjectID: "invited",
		Login: "invited", DisplayName: "Invited User", UpdatedAt: harness.now,
	}
	harness.server.users = fakeUserResolver{account: invitee}
	created := harness.request(
		t,
		http.MethodPost,
		"/panel/api/v1/invitations",
		strings.NewReader(`{"login":"invited","role":"editor","target_id":"github:installation:10","expires_in_days":7}`),
		ownerSession,
	)
	if created.Code != http.StatusCreated {
		t.Fatalf("create invitation = %d %s", created.Code, created.Body.String())
	}
	var invitation struct {
		ID        string `json:"id"`
		InviteURL string `json:"invite_url"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &invitation); err != nil {
		t.Fatal(err)
	}
	listed := harness.request(
		t,
		http.MethodGet,
		"/panel/api/v1/invitations?q=invited&role=editor&status=pending&sort=name_desc&limit=1",
		nil,
		ownerSession,
	)
	requireResponse(
		t, listed, "list invitations", http.StatusOK, `"login":"invited"`, `"total":1`,
	)
	inviteURL, err := url.Parse(invitation.InviteURL)
	if err != nil {
		t.Fatal(err)
	}
	token := strings.TrimPrefix(inviteURL.Path, "/panel/invite/")
	review := harness.request(
		t,
		http.MethodGet,
		"/panel/api/v1/invites/"+token,
		nil,
		nil,
	)
	if review.Code != http.StatusOK ||
		!strings.Contains(review.Body.String(), `"login":"invited"`) ||
		!strings.Contains(review.Body.String(), `"status":"pending"`) {
		t.Fatalf("review invitation = %d %s", review.Code, review.Body.String())
	}

	harness.server.signIn = fakeSignIn{account: invitee}
	start := httptest.NewRecorder()
	harness.handler.ServeHTTP(
		start,
		httptest.NewRequest(
			http.MethodGet,
			"/panel/auth/github/start?invite="+url.QueryEscape(token)+"&action=accept",
			nil,
		),
	)
	if start.Code != http.StatusFound {
		t.Fatalf("start invited sign-in = %d %#v", start.Code, start.Result().Cookies())
	}
	location, err := url.Parse(start.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	callback := httptest.NewRequest(
		http.MethodGet,
		"/panel/auth/github/callback?code=code&state="+
			url.QueryEscape(location.Query().Get("state")),
		nil,
	)
	callback.AddCookie(responseCookie(t, start, stateCookieName))
	callback.AddCookie(responseCookie(t, start, inviteCookieName))
	finished := httptest.NewRecorder()
	harness.handler.ServeHTTP(finished, callback)
	if finished.Code != http.StatusFound {
		t.Fatalf("accept invitation = %d %s", finished.Code, finished.Body.String())
	}
	var invitedSession *http.Cookie
	for _, cookie := range finished.Result().Cookies() {
		if cookie.Name == sessionCookieName {
			invitedSession = cookie
		}
	}
	if invitedSession == nil {
		t.Fatal("accepted invitation did not issue a session")
	}
	viewer := harness.request(t, http.MethodGet, "/panel/api/v1/session", nil, invitedSession)
	if viewer.Code != http.StatusOK || !strings.Contains(viewer.Body.String(), `"global_role":"editor"`) {
		t.Fatalf("invited viewer = %d %s", viewer.Code, viewer.Body.String())
	}

	harness.server.signIn = fakeSignIn{account: storage.Account{
		ID: "github:test:user:target-invite", Provider: "github:test", SubjectID: "target-invite",
		Login: "target-invite", DisplayName: "Target Invite", UpdatedAt: harness.now,
	}}
	harness.server.users = fakeUserResolver{account: harness.server.signIn.(fakeSignIn).account}
	targetCreated := harness.request(
		t,
		http.MethodPost,
		"/panel/api/v1/targets/github:installation:10/invitations",
		strings.NewReader(`{"login":"target-invite","role":"viewer","expires_in_days":1}`),
		ownerSession,
	)
	if targetCreated.Code != http.StatusCreated {
		t.Fatalf("create target invitation = %d %s", targetCreated.Code, targetCreated.Body.String())
	}
	invalidTargetRole := harness.request(
		t,
		http.MethodGet,
		"/panel/api/v1/targets/github:installation:10/invitations?role=owner",
		nil,
		ownerSession,
	)
	requireResponse(
		t, invalidTargetRole, "invalid target invitation role", http.StatusBadRequest,
	)
	var targetInvitation struct {
		ID        string `json:"id"`
		InviteURL string `json:"invite_url"`
	}
	if err := json.Unmarshal(targetCreated.Body.Bytes(), &targetInvitation); err != nil {
		t.Fatal(err)
	}
	reissued := harness.request(
		t,
		http.MethodPost,
		"/panel/api/v1/invitations/"+targetInvitation.ID+"/reissue",
		strings.NewReader(`{"expires_in_days":7}`),
		ownerSession,
	)
	if reissued.Code != http.StatusOK || strings.Contains(reissued.Body.String(), targetInvitation.InviteURL) {
		t.Fatalf("reissue invitation = %d %s", reissued.Code, reissued.Body.String())
	}
	revoked := harness.request(
		t,
		http.MethodDelete,
		"/panel/api/v1/invitations/"+targetInvitation.ID,
		nil,
		ownerSession,
	)
	if revoked.Code != http.StatusOK || !strings.Contains(revoked.Body.String(), `"status":"revoked"`) {
		t.Fatalf("revoke invitation = %d %s", revoked.Code, revoked.Body.String())
	}
}

func TestPanelOAuthStateIsBrowserBound(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	const starts = 65
	randomBytes := make([]byte, 0, tokenBytes*(starts+1))
	for index := range starts + 1 {
		randomBytes = append(randomBytes, bytes.Repeat([]byte{byte(index + 1)}, tokenBytes)...)
	}
	harness.server.random = bytes.NewReader(randomBytes)

	type pendingSignIn struct {
		cookie *http.Cookie
		state  string
	}
	pending := make([]pendingSignIn, 0, starts)
	for index := range starts {
		response := harness.request(
			t,
			http.MethodGet,
			"/panel/auth/github/start",
			nil,
			nil,
		)
		if response.Code != http.StatusFound {
			t.Fatalf("OAuth start %d = %d %s", index, response.Code, response.Body.String())
		}
		location, err := url.Parse(response.Header().Get("Location"))
		if err != nil {
			t.Fatal(err)
		}
		pending = append(pending, pendingSignIn{
			cookie: responseCookie(t, response, stateCookieName),
			state:  location.Query().Get("state"),
		})
	}

	callback := func(state string, cookie *http.Cookie) *httptest.ResponseRecorder {
		request := httptest.NewRequest(
			http.MethodGet,
			"/panel/auth/github/callback?code=code&state="+url.QueryEscape(state),
			nil,
		)
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		harness.handler.ServeHTTP(response, request)

		return response
	}
	if response := callback(pending[0].state, pending[0].cookie); response.Code != http.StatusFound {
		t.Fatalf("unrelated starts invalidated first browser = %d %s", response.Code, response.Body.String())
	}
	if response := callback(pending[1].state, pending[2].cookie); response.Code != http.StatusUnauthorized {
		t.Fatalf("cross-browser OAuth state = %d %s", response.Code, response.Body.String())
	}
	replacement := byte('A')
	if pending[3].state[10] == replacement {
		replacement = 'B'
	}
	tamperedBytes := []byte(pending[3].state)
	tamperedBytes[10] = replacement
	tampered := string(tamperedBytes)
	tamperedCookie := *pending[3].cookie
	tamperedCookie.Value = tampered
	if response := callback(tampered, &tamperedCookie); response.Code != http.StatusUnauthorized {
		t.Fatalf("tampered OAuth state = %d %s", response.Code, response.Body.String())
	}
	harness.server.now = func() time.Time { return harness.now.Add(DefaultStateTTL) }
	if response := callback(pending[4].state, pending[4].cookie); response.Code != http.StatusUnauthorized {
		t.Fatalf("expired OAuth state = %d %s", response.Code, response.Body.String())
	}
}

func TestRepositoryEnablementDistinguishesOmittedFromNull(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	settingsPath := "/panel/api/v1/targets/github:installation:10/repositories/repository-20/settings"

	explicitOff := harness.request(
		t,
		http.MethodPut,
		settingsPath,
		strings.NewReader(`{"enabled_override":false,"config_patch":{},"ignore_repository_file":false,"expected_revision":1}`),
		session,
	)
	if explicitOff.Code != http.StatusOK {
		t.Fatalf("explicit Off = %d %s", explicitOff.Code, explicitOff.Body.String())
	}

	omitted := harness.request(
		t,
		http.MethodPut,
		settingsPath,
		strings.NewReader(`{"config_patch":{},"ignore_repository_file":false,"expected_revision":2}`),
		session,
	)
	if omitted.Code != http.StatusBadRequest {
		t.Fatalf("omitted enablement = %d %s", omitted.Code, omitted.Body.String())
	}

	unchanged := harness.request(
		t,
		http.MethodGet,
		"/panel/api/v1/targets/github:installation:10/repositories/repository-20",
		nil,
		session,
	)
	var detail repositoryDetailResponse
	if unchanged.Code != http.StatusOK {
		t.Fatalf("repository after omitted field = %d %s", unchanged.Code, unchanged.Body.String())
	}
	if err := json.Unmarshal(unchanged.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Repository.EnabledOverride == nil || *detail.Repository.EnabledOverride || detail.Revision != 2 {
		t.Fatalf("omitted field changed repository: %#v", detail)
	}

	inherited := harness.request(
		t,
		http.MethodPut,
		settingsPath,
		strings.NewReader(`{"enabled_override":null,"config_patch":{},"ignore_repository_file":false,"expected_revision":2}`),
		session,
	)
	if inherited.Code != http.StatusOK {
		t.Fatalf("explicit Default = %d %s", inherited.Code, inherited.Body.String())
	}
	if err := json.Unmarshal(inherited.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Repository.EnabledOverride != nil || detail.Revision != 3 {
		t.Fatalf("explicit null did not restore inheritance: %#v", detail)
	}
}

func TestPanelHistoryPaginationFilteringAndSorting(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	targetPath := "/panel/api/v1/targets/github:installation:10"
	seedPanelHistory(t, harness, targetPath, session)
	assertAuditPagination(t, harness, targetPath, session)
	assertAuditChangeFilter(t, harness, targetPath, session)
	assertFailureHistory(t, harness, targetPath, session)
	assertInvalidHistoryQueries(t, harness, targetPath, session)
}

func seedPanelHistory(t *testing.T, harness *panelHarness, targetPath string, session *http.Cookie) {
	t.Helper()
	for revision := int64(1); revision <= 2; revision++ {
		input := fmt.Sprintf(
			`{"repository_default_enabled":true,"config_patch":{"quiet_success":true},"expected_revision":%d}`,
			revision,
		)
		response := harness.request(
			t,
			http.MethodPut,
			targetPath+"/settings",
			strings.NewReader(input),
			session,
		)
		if response.Code != http.StatusOK {
			t.Fatalf("target update %d = %d %s", revision, response.Code, response.Body.String())
		}
	}
	seedFailure(t, harness, "delivery-permanent", "repository configuration is invalid", false)
	seedFailure(t, harness, "delivery-retryable", "GitHub request timed out", true)
}

func assertAuditPagination(t *testing.T, harness *panelHarness, targetPath string, session *http.Cookie) {
	t.Helper()
	first := harness.request(
		t,
		http.MethodGet,
		targetPath+"/audit?scope=account&q=defaults&sort=oldest&limit=1",
		nil,
		session,
	)
	if first.Code != http.StatusOK {
		t.Fatalf("first audit page = %d %s", first.Code, first.Body.String())
	}
	var firstPage pageResponse[auditResponse]
	if err := json.Unmarshal(first.Body.Bytes(), &firstPage); err != nil {
		t.Fatal(err)
	}
	if firstPage.Total != 2 || len(firstPage.Items) != 1 || firstPage.NextCursor == nil {
		t.Fatalf("unexpected first audit page: %#v", firstPage)
	}
	if *firstPage.NextCursor != "1" {
		t.Fatalf("first audit cursor = %q, want offset 1", *firstPage.NextCursor)
	}

	second := harness.request(
		t,
		http.MethodGet,
		targetPath+"/audit?scope=account&q=defaults&sort=oldest&limit=1&cursor="+
			url.QueryEscape(*firstPage.NextCursor),
		nil,
		session,
	)
	var secondPage pageResponse[auditResponse]
	if second.Code != http.StatusOK {
		t.Fatalf("second audit page = %d %s", second.Code, second.Body.String())
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondPage); err != nil {
		t.Fatal(err)
	}
	if secondPage.Total != 2 || len(secondPage.Items) != 1 || secondPage.NextCursor != nil ||
		secondPage.Items[0].ID <= firstPage.Items[0].ID {
		t.Fatalf("unexpected second audit page: %#v", secondPage)
	}
}

func assertAuditChangeFilter(t *testing.T, harness *panelHarness, targetPath string, session *http.Cookie) {
	t.Helper()
	accountChanges := harness.request(
		t,
		http.MethodGet,
		targetPath+"/audit?change=account&sort=change_asc&limit=10",
		nil,
		session,
	)
	var accountPage pageResponse[auditResponse]
	if accountChanges.Code != http.StatusOK {
		t.Fatalf("account-change audit = %d %s", accountChanges.Code, accountChanges.Body.String())
	}
	if err := json.Unmarshal(accountChanges.Body.Bytes(), &accountPage); err != nil {
		t.Fatal(err)
	}
	if accountPage.Total != 2 || len(accountPage.Items) != 2 {
		t.Fatalf("unexpected account-change audit: %#v", accountPage)
	}
}

func assertFailureHistory(t *testing.T, harness *panelHarness, targetPath string, session *http.Cookie) {
	t.Helper()
	failures := harness.request(
		t,
		http.MethodGet,
		targetPath+"/failures?kind=retryable&q=timed+out&sort=newest&limit=10",
		nil,
		session,
	)
	var failurePage pageResponse[failureResponse]
	if failures.Code != http.StatusOK {
		t.Fatalf("failure history = %d %s", failures.Code, failures.Body.String())
	}
	if err := json.Unmarshal(failures.Body.Bytes(), &failurePage); err != nil {
		t.Fatal(err)
	}
	if failurePage.Total != 1 || len(failurePage.Items) != 1 ||
		failurePage.Items[0].DeliveryID != "delivery-retryable" {
		t.Fatalf("unexpected failure page: %#v", failurePage)
	}

	statusAscending := harness.request(
		t,
		http.MethodGet,
		targetPath+"/failures?sort=status_asc&limit=10",
		nil,
		session,
	)
	if statusAscending.Code != http.StatusOK {
		t.Fatalf("status-sorted failures = %d %s", statusAscending.Code, statusAscending.Body.String())
	}
	if err := json.Unmarshal(statusAscending.Body.Bytes(), &failurePage); err != nil {
		t.Fatal(err)
	}
	if len(failurePage.Items) != 2 || failurePage.Items[0].Retryable {
		t.Fatalf("unexpected status-sorted failures: %#v", failurePage)
	}
}

func assertInvalidHistoryQueries(t *testing.T, harness *panelHarness, targetPath string, session *http.Cookie) {
	t.Helper()
	wrongAuditSort := harness.request(
		t,
		http.MethodGet,
		targetPath+"/audit?sort=status_asc",
		nil,
		session,
	)
	if wrongAuditSort.Code != http.StatusBadRequest {
		t.Fatalf("failure-only audit sort = %d %s", wrongAuditSort.Code, wrongAuditSort.Body.String())
	}
	invalidChange := harness.request(
		t,
		http.MethodGet,
		targetPath+"/audit?change=sideways",
		nil,
		session,
	)
	if invalidChange.Code != http.StatusBadRequest {
		t.Fatalf("invalid audit change = %d %s", invalidChange.Code, invalidChange.Body.String())
	}

	invalid := harness.request(
		t,
		http.MethodGet,
		targetPath+"/audit?sort=sideways",
		nil,
		session,
	)
	if invalid.Code != http.StatusBadRequest ||
		!strings.Contains(invalid.Body.String(), `"code":"invalid_history_query"`) {
		t.Fatalf("invalid history query = %d %s", invalid.Code, invalid.Body.String())
	}
}

func TestPanelRepositoryPaginationFilteringAndSorting(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	targetID := "github:installation:10"
	target, err := harness.store.GetTarget(t.Context(), targetID)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.store.ReconcileInstallation(t.Context(), storage.InstallationSnapshot{
		TargetID:       target.ID,
		InstallationID: target.InstallationID,
		Kind:           target.Kind,
		Account:        target.Account,
		Repositories: []storage.RepositorySnapshot{
			{ID: "repository-20", Name: "smyklot", FullName: "smykla-skalski/smyklot"},
			{ID: "repository-21", Name: "alpha", FullName: "smykla-skalski/alpha"},
			{ID: "repository-22", Name: "beta-service", FullName: "smykla-skalski/beta-service"},
		},
		SyncedAt: harness.now.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	enabled := true
	if _, err := harness.store.UpdateRepositorySettings(t.Context(), storage.RepositorySettingsChange{
		TargetID:             targetID,
		RepositoryID:         "repository-22",
		ActorAccountID:       target.Account.ID,
		EnabledOverride:      &enabled,
		ConfigPatch:          config.Patch{QuietSuccess: &enabled},
		IgnoreRepositoryFile: false,
		ExpectedRevision:     1,
		ChangedAt:            harness.now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := harness.store.UpdateRepositoryFileState(t.Context(), storage.RepositoryFileState{
		TargetID:     targetID,
		RepositoryID: "repository-22",
		Status:       storage.RepositoryFileInvalid,
		ObservedAt:   harness.now.Add(3 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	filteredPage := getRepositoryPage(
		t,
		harness,
		"/panel/api/v1/targets/"+targetID+
			"/repositories?q=service&sort=newest&state=enabled&file=invalid&setting=quiet_success&limit=1",
		session,
	)
	if filteredPage.Total != 1 || len(filteredPage.Items) != 1 ||
		filteredPage.Items[0].Name != "beta-service" || filteredPage.NextCursor != nil {
		t.Fatalf("unexpected filtered repositories: %#v", filteredPage)
	}

	multiplePage := getRepositoryPage(
		t,
		harness,
		"/panel/api/v1/targets/"+targetID+
			"/repositories?file=missing&file=invalid&setting=quiet_success&setting=command_prefix&limit=10",
		session,
	)
	if multiplePage.Total != 1 || len(multiplePage.Items) != 1 ||
		multiplePage.Items[0].Name != "beta-service" {
		t.Fatalf("unexpected multi-filtered repositories: %#v", multiplePage)
	}

	firstPage := getRepositoryPage(
		t,
		harness,
		"/panel/api/v1/targets/"+targetID+"/repositories?sort=name_desc&limit=1",
		session,
	)
	if firstPage.Total != 3 || len(firstPage.Items) != 1 || firstPage.NextCursor == nil ||
		firstPage.Items[0].Name != "smyklot" {
		t.Fatalf("unexpected first repository page: %#v", firstPage)
	}

	secondPage := getRepositoryPage(
		t,
		harness,
		"/panel/api/v1/targets/"+targetID+"/repositories?sort=name_desc&limit=1&cursor="+
			url.QueryEscape(*firstPage.NextCursor),
		session,
	)
	if secondPage.Total != 3 || len(secondPage.Items) != 1 ||
		secondPage.Items[0].Name != "beta-service" {
		t.Fatalf("unexpected second repository page: %#v", secondPage)
	}

	filePage := getRepositoryPage(
		t,
		harness,
		"/panel/api/v1/targets/"+targetID+"/repositories?sort=file_asc&limit=10",
		session,
	)
	if len(filePage.Items) != 3 || filePage.Items[0].Name != "beta-service" {
		t.Fatalf("unexpected file-sorted repositories: %#v", filePage)
	}

	overridePage := getRepositoryPage(
		t,
		harness,
		"/panel/api/v1/targets/"+targetID+"/repositories?sort=overrides_desc&limit=10",
		session,
	)
	if len(overridePage.Items) != 3 || overridePage.Items[0].Name != "beta-service" {
		t.Fatalf("unexpected override-sorted repositories: %#v", overridePage)
	}

	invalid := harness.request(
		t,
		http.MethodGet,
		"/panel/api/v1/targets/"+targetID+"/repositories?setting=runner",
		nil,
		session,
	)
	if invalid.Code != http.StatusBadRequest ||
		!strings.Contains(invalid.Body.String(), `"code":"invalid_repository_query"`) {
		t.Fatalf("invalid repository query = %d %s", invalid.Code, invalid.Body.String())
	}

	mixedPreset := harness.request(
		t,
		http.MethodGet,
		"/panel/api/v1/targets/"+targetID+
			"/repositories?setting=custom&setting=quiet_success",
		nil,
		session,
	)
	if mixedPreset.Code != http.StatusBadRequest ||
		!strings.Contains(mixedPreset.Body.String(), `"code":"invalid_repository_query"`) {
		t.Fatalf(
			"mixed repository setting preset = %d %s",
			mixedPreset.Code,
			mixedPreset.Body.String(),
		)
	}
}

func getRepositoryPage(
	t *testing.T,
	harness *panelHarness,
	path string,
	session *http.Cookie,
) pageResponse[repositorySummaryResponse] {
	t.Helper()
	response := harness.request(t, http.MethodGet, path, nil, session)
	if response.Code != http.StatusOK {
		t.Fatalf("repository page = %d %s", response.Code, response.Body.String())
	}

	var page pageResponse[repositorySummaryResponse]
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}

	return page
}

func seedFailure(
	t *testing.T,
	harness *panelHarness,
	deliveryID, reason string,
	retryable bool,
) {
	t.Helper()
	repositoryID := "repository-20"
	claim, err := harness.store.ClaimDelivery(t.Context(), storage.DeliveryClaim{
		ClaimKey:           "github:test:" + deliveryID,
		DeliveryID:         deliveryID,
		TargetID:           "github:installation:10",
		RepositoryID:       &repositoryID,
		RepositoryFullName: "smykla-skalski/smyklot",
		Event:              "issue_comment",
		ClaimedAt:          harness.now,
	})
	if err != nil || claim.Disposition != storage.DeliveryClaimAccepted {
		t.Fatalf("claim %q: disposition=%s error=%v", deliveryID, claim.Disposition, err)
	}
	if err := harness.store.FailDelivery(t.Context(), storage.DeliveryFailureChange{
		ClaimID:   claim.ID,
		Stage:     "github",
		Reason:    reason,
		Retryable: retryable,
		FailedAt:  harness.now,
	}); err != nil {
		t.Fatalf("fail %q: %v", deliveryID, err)
	}
}

func TestPanelRejectsAnotherOwnerAndCrossOriginWrites(t *testing.T) {
	nonOwner := newPanelHarness(t, "someone-else")
	start := httptest.NewRecorder()
	nonOwner.handler.ServeHTTP(start, httptest.NewRequest(http.MethodGet, "/panel/auth/github/start", nil))
	stateCookie := responseCookie(t, start, stateCookieName)
	location, _ := url.Parse(start.Header().Get("Location"))
	callback := httptest.NewRequest(http.MethodGet, "/panel/auth/github/callback?code=x&state="+url.QueryEscape(location.Query().Get("state")), nil)
	callback.AddCookie(stateCookie)
	finished := httptest.NewRecorder()
	nonOwner.handler.ServeHTTP(finished, callback)
	if finished.Code != http.StatusForbidden {
		t.Fatalf("non-owner callback = %d %s", finished.Code, finished.Body.String())
	}

	owner := newPanelHarness(t, "owner")
	session := owner.signIn(t)
	request := httptest.NewRequest(http.MethodPut, "/panel/api/v1/targets/github:installation:10/settings", strings.NewReader(`{}`))
	request.AddCookie(session)
	request.Header.Set("Origin", "https://attacker.example")
	response := httptest.NewRecorder()
	owner.handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("cross-origin update = %d %s", response.Code, response.Body.String())
	}
}

func TestPanelServesRewrittenAssetsAndSPAFallback(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	for _, path := range []string{
		"/panel/",
		"/panel/users",
		"/panel/invite/abcdefghijklmnopqrstuvwxyzABCDEFGH_01234567",
		"/panel/i/smykla-skalski/repositories",
		"/panel/i/smykla-skalski/users",
		"/panel/i/auth/settings",
		"/panel/help",
	} {
		response := harness.request(t, http.MethodGet, path, nil, nil)
		body := response.Body.String()
		if response.Code != http.StatusOK ||
			!strings.Contains(body, `content="/panel"`) ||
			!strings.Contains(body, `href="/panel/smyklot-avatar.png?v=1.0.0"`) ||
			strings.Contains(body, basePathSentinel) {
			t.Fatalf("index %s = %d %s", path, response.Code, response.Body.String())
		}
	}
	asset := harness.request(t, http.MethodGet, "/panel/assets/app.js", nil, nil)
	if asset.Code != http.StatusOK || asset.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("asset response = %d %#v", asset.Code, asset.Header())
	}
	for _, path := range []string{
		"/panel/smykla-skalski/repositories",
		"/panel/auth/settings",
		"/panel/webhook/history",
		"/panel/i/smykla-skalski/help",
		"/panel/i/smykla-skalski/unknown",
		"/panel/@smykla-skalski/repositories",
		"/panel/invite/too-short",
		"/panel/invite/abcdefghijklmnopqrstuvwxyzABCDEFGH.01234567",
		"/panel/invite/abcdefghijklmnopqrstuvwxyzABCDEFGH_01234567/extra",
		"/panel/assets/missing.js",
	} {
		response := harness.request(t, http.MethodGet, path, nil, nil)
		if response.Code != http.StatusNotFound {
			t.Fatalf("unknown panel route %s = %d %s", path, response.Code, response.Body.String())
		}
	}
}

var _ fs.FS = fstest.MapFS{}
