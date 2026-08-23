package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/smykla-skalski/smyklot/internal/panelassets"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/storage/open"
	"github.com/smykla-skalski/smyklot/internal/storage/storagetest"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

// The route table the frontend build generated, so the requests below are answered
// against the addresses the panel really has rather than a fixture's account of
// them - which is the copy that drifted and cost the queue its reloads.
//
// Panics rather than skips: `mise run test` builds the bundle before the suite.
var testPanelRouteManifest = sync.OnceValue(func() []byte {
	assets, err := panelassets.Open()
	if err != nil {
		panic("open the panel bundle (mise run panel:assets:generate): " + err.Error())
	}
	document, err := fs.ReadFile(assets, routeManifestAsset)
	if err != nil {
		panic("the built bundle carries no " + routeManifestAsset + ": " + err.Error())
	}

	return document
})

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

type fakeRuntimeController struct {
	values RuntimeValues
}

type fakePendingCIController struct {
	store storage.Store
	wakes int
}

func (controller *fakePendingCIController) Wake() {
	controller.wakes++
}

func (controller *fakePendingCIController) Exclusive(
	_ context.Context,
	_ []string,
	operation func() error,
) error {
	return operation()
}

func (controller *fakePendingCIController) ExclusiveCatalog(
	_ context.Context,
	operation func() error,
) error {
	return operation()
}

func (controller *fakePendingCIController) CheckNow(
	ctx context.Context,
	change pendingci.CheckNowRequest,
) (pendingci.Request, error) {
	request, err := controller.store.CheckNow(ctx, change)
	if err == nil {
		controller.wakes++
	}

	return request, err
}

func (controller *fakePendingCIController) Cancel(
	ctx context.Context,
	change pendingci.FinishRequest,
) (pendingci.Request, error) {
	request, err := controller.store.Finish(ctx, change)
	if err == nil {
		controller.wakes++
	}

	return request, err
}

func (f *fakeRuntimeController) ApplyRuntimeSettings(values RuntimeValues) {
	f.values = values
}

func (f fakeUserResolver) ResolveUser(
	context.Context,
	string,
	string,
) (storage.Account, error) {
	return f.account, nil
}

func (f fakeUserResolver) ResolveRootUser(context.Context, string) (storage.Account, error) {
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
	// clock is what the server reads, so a test can move time forward. `now`
	// stays the moment the fixtures were built and is what they are dated with.
	clock     *time.Time
	runtime   *fakeRuntimeController
	pendingCI *fakePendingCIController
}

func newPanelHarness(t *testing.T, login string) *panelHarness {
	return newPanelHarnessForSubject(t, login, "1")
}

func newPanelHarnessForSubject(t *testing.T, login, subjectID string) *panelHarness {
	t.Helper()
	now := time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)
	clock := &now
	store, err := open.Store(t.Context(), storagetest.Connection(t, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	viewer := storage.Account{
		ID:          "github:test:user:" + subjectID,
		Provider:    "github:test",
		SubjectID:   subjectID,
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
		Ownership: storage.OwnershipSnapshot{
			Source:   storage.OwnershipSourceOrganizationAdmin,
			Status:   storage.OwnershipStatusFresh,
			Owners:   []storage.Account{viewer},
			SyncedAt: now,
		},
		SyncedAt: now,
	}
	assets := fstest.MapFS{
		"index.html":        &fstest.MapFile{Data: []byte(`<!doctype html><meta name="smyklot-panel-base" content="/__smyklot_panel_base__"><meta name="smyklot-panel-version" content="__smyklot_panel_version__"><meta name="smyklot-panel-service" content="__smyklot_panel_service__"><meta name="smyklot-panel-error" content="__smyklot_panel_error__"><link rel="icon" href="/__smyklot_panel_base__/smyklot-avatar.png?v=__smyklot_panel_version__"><noscript>__smyklot_panel_noscript__</noscript>`)},
		"_app/app.js":       &fstest.MapFile{Data: []byte("const base='__smyklot_panel_base__';")},
		"service-worker.js": &fstest.MapFile{Data: []byte(`const version='__smyklot_panel_version__';`)},
		"theme-boot.js":     &fstest.MapFile{Data: []byte(`document.documentElement.dataset.theme = "dark";`)},
		routeManifestAsset:  &fstest.MapFile{Data: testPanelRouteManifest()},
	}
	randomBytes := make([]byte, 0, tokenBytes*32)
	for index := range 32 {
		randomBytes = append(randomBytes, bytes.Repeat([]byte{byte(index + 1)}, tokenBytes)...)
	}
	random := bytes.NewReader(randomBytes)
	runtime := &fakeRuntimeController{}
	pendingCIController := &fakePendingCIController{store: store}
	server, err := New(Config{
		BasePath:                 "/panel",
		PublicOrigin:             "https://smyklot.example",
		SuperRootID:              1,
		ClientID:                 "client-id",
		ClientSecret:             "client-secret",
		AuthorizeURL:             "https://github.example/authorize",
		TokenURL:                 "https://github.example/token",
		APIURL:                   "https://api.github.example",
		Version:                  "1.0.0",
		ServiceHost:              "smyklot.example",
		ListenAddress:            ":8080",
		AdminAddress:             ":9090",
		WebhookPath:              "/webhook",
		LogLevel:                 slog.LevelInfo,
		PollInterval:             5 * time.Minute,
		PendingCIQuietPeriod:     30 * time.Second,
		SessionTTL:               12 * time.Hour,
		ProcessConfig:            config.Default(),
		WebhookCredentialPresent: true,
		AppCredentialPresent:     true,
		OAuthCredentialPresent:   true,
		Assets:                   assets,
	}, Dependencies{
		Store:     store,
		Catalog:   fakeCatalog{store: store, snapshot: snapshot},
		Users:     fakeUserResolver{account: viewer},
		SignIn:    fakeSignIn{account: viewer},
		Random:    random,
		Now:       func() time.Time { return *clock },
		Runtime:   runtime,
		PendingCI: pendingCIController,
	})
	if err != nil {
		t.Fatal(err)
	}

	return &panelHarness{
		server: server, store: store, handler: server.Handler(), now: now,
		clock: clock, runtime: runtime, pendingCI: pendingCIController,
	}
}

// advance moves the clock the server reads, leaving the moment the fixtures were
// dated with alone.
func (h *panelHarness) advance(d time.Duration) {
	*h.clock = h.clock.Add(d)
}

func seedNonOwnedInstallation(
	t *testing.T,
	harness *panelHarness,
) (storage.Account, storage.InstallationSnapshot) {
	t.Helper()
	owner := storage.Account{
		ID: "github:test:user:owner-2", Provider: "github:test", SubjectID: "owner-2",
		Login: "installation-owner", DisplayName: "Installation Owner", UpdatedAt: harness.now,
	}
	targetAccount := storage.Account{
		ID: "github:test:account:20", Provider: "github:test", SubjectID: "20",
		Login: "other-installation", DisplayName: "Other Installation", UpdatedAt: harness.now,
	}
	target := storage.InstallationSnapshot{
		TargetID: "github:installation:20", InstallationID: "20",
		Kind: storage.TargetOrganization, Account: targetAccount,
		Repositories: []storage.RepositorySnapshot{{
			ID: "repository-30", Name: "other", FullName: "other-installation/other",
			DefaultBranch: "main",
		}},
		Ownership: storage.OwnershipSnapshot{
			Source: storage.OwnershipSourceOrganizationAdmin, Status: storage.OwnershipStatusFresh,
			Owners: []storage.Account{owner}, SyncedAt: harness.now,
		},
		SyncedAt: harness.now,
	}
	if err := harness.store.ReconcileInstallation(t.Context(), target); err != nil {
		t.Fatal(err)
	}

	return owner, target
}

func activateOwnerSession(
	t *testing.T,
	harness *panelHarness,
	owner storage.Account,
) *http.Cookie {
	t.Helper()
	active, err := harness.store.ActivateDerivedOwner(t.Context(), owner.ID, harness.now)
	if err != nil || !active {
		t.Fatalf("activate Owner = %v, error %v", active, err)
	}
	const token = "installation-owner-session"
	if err := harness.store.CreateSession(t.Context(), storage.Session{
		TokenHash: tokenHash(token), AccountID: owner.ID,
		CreatedAt: harness.now, ExpiresAt: harness.now.Add(time.Hour),
	}, 1); err != nil {
		t.Fatal(err)
	}

	return &http.Cookie{Name: sessionCookieName, Value: token}
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

func (h *panelHarness) acceptInvitation(
	t *testing.T,
	account storage.Account,
	token string,
) *http.Cookie {
	t.Helper()
	h.server.signIn = fakeSignIn{account: account}
	start := httptest.NewRecorder()
	h.handler.ServeHTTP(
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
	h.handler.ServeHTTP(finished, callback)
	if finished.Code != http.StatusFound {
		t.Fatalf("accept invitation = %d %s", finished.Code, finished.Body.String())
	}

	return responseCookie(t, finished, sessionCookieName)
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
	if viewer.Code != http.StatusOK ||
		!strings.Contains(viewer.Body.String(), `"target_count":1`) ||
		!strings.Contains(viewer.Body.String(), `"system_role":"super_root"`) {
		t.Fatalf("viewer response = %d %s", viewer.Code, viewer.Body.String())
	}

	input := `{"target":{
		"repository_default_enabled":true,
		"pending_ci_mode_default":"checks",
		"pending_ci_branch_patterns_default":{"include":["~DEFAULT_BRANCH","refs/heads/release/*"],"exclude":[]},
		"pending_ci_quiet_period_seconds_override":0,
		"path_index_interval_seconds_override":null,
		"config_patch":{"quiet_success":true},
		"expected_revision":1
	}}`
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
	var answer installationSettingsBatchResponse
	if err := json.Unmarshal(updated.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	target := answer.Target
	if target == nil {
		t.Fatalf("target update answer = %#v", answer)
	}
	if !target.RepositoryDefaultEnabled ||
		target.ConfigPatch.QuietSuccess == nil || !*target.ConfigPatch.QuietSuccess ||
		target.PendingCIModeDefault != storage.PendingCIModeChecks ||
		target.PendingCIQuietPeriodSecondsOverride == nil ||
		*target.PendingCIQuietPeriodSecondsOverride != 0 ||
		len(target.PendingCIBranchPatternsDefault.Include) != 2 ||
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
	if audit.Code != http.StatusOK ||
		!strings.Contains(audit.Body.String(), "installation.settings.saved") {
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

	ready := readPanelReady(t, connection)
	if ready.Version != panelEventVersion ||
		ready.Prefs.Rev != 0 || string(ready.Prefs.Values) != "{}" {
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
	runtimeUpdate := harness.request(
		t, http.MethodPut, "/panel/api/v1/root/runtime/settings",
		strings.NewReader(rootRuntimeSettingsBody("debug", 0)), session,
	)
	requireResponse(t, runtimeUpdate, "runtime WebSocket update", http.StatusOK, `"revision":1`)
	var resync panelEvent
	if err := wsjson.Read(t.Context(), connection, &resync); err != nil {
		t.Fatal(err)
	}
	if resync.Type != panelEventResync {
		t.Fatalf("runtime resync event = %#v", resync)
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

func TestPanelBroadcastsRootSecurityChanges(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	_, elevatedTarget := seedNonOwnedInstallation(t, harness)
	subscriber, unsubscribe := harness.server.events.subscribe("", "root-live-test")
	t.Cleanup(unsubscribe)

	started := harness.request(
		t,
		http.MethodPost,
		"/panel/api/v1/root/installations/"+elevatedTarget.TargetID+"/elevation",
		strings.NewReader(`{"acknowledged":true,"reason":"live event proof"}`),
		session,
	)
	requireResponse(t, started, "live elevation start", http.StatusCreated)
	requirePanelEvent(t, subscriber.events, panelEventResync)

	var elevation elevationResponse
	if err := json.Unmarshal(started.Body.Bytes(), &elevation); err != nil {
		t.Fatal(err)
	}
	ended := harness.request(
		t,
		http.MethodDelete,
		"/panel/api/v1/root/elevations/"+elevation.ID,
		nil,
		session,
	)
	requireResponse(t, ended, "live elevation end", http.StatusOK)
	requirePanelEvent(t, subscriber.events, panelEventResync)
}

func requirePanelEvent(t *testing.T, events <-chan panelEvent, eventType string) {
	t.Helper()
	select {
	case event := <-events:
		if event.Type != eventType {
			t.Fatalf("panel event = %#v, want type %q", event, eventType)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for panel event %q", eventType)
	}
}

func dialPanelStream(
	t *testing.T,
	streamURL string,
	session *http.Cookie,
) *websocket.Conn {
	t.Helper()
	headers := http.Header{}
	headers.Set("Cookie", session.String())
	headers.Set("Origin", "https://smyklot.example")
	connection, response, err := websocket.Dial(t.Context(), streamURL, &websocket.DialOptions{
		HTTPHeader: headers,
	})
	if err != nil {
		t.Fatalf("dial WebSocket: response %#v, error %v", response, err)
	}
	t.Cleanup(func() { _ = connection.CloseNow() })

	return connection
}

func readPanelReady(t *testing.T, connection *websocket.Conn) panelEvent {
	t.Helper()
	var ready panelEvent
	if err := wsjson.Read(t.Context(), connection, &ready); err != nil {
		t.Fatal(err)
	}
	if ready.Type != panelEventReady || ready.Prefs == nil {
		t.Fatalf("ready event = %#v", ready)
	}

	return ready
}

func TestPanelWebSocketPreferences(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	firstSession := harness.signIn(t)
	secondSession := harness.signIn(t)
	endpoint := httptest.NewServer(harness.handler)
	t.Cleanup(endpoint.Close)
	streamURL := "ws" + strings.TrimPrefix(endpoint.URL, "http") + "/panel/api/v1/events"
	emptySum := prefsChecksum(map[string]json.RawMessage{})

	// A dial without handshake parameters gets the full (empty) snapshot.
	observer := dialPanelStream(t, streamURL, firstSession)
	observerReady := readPanelReady(t, observer)
	if observerReady.Prefs.Rev != 0 ||
		observerReady.Prefs.Sum != emptySum ||
		string(observerReady.Prefs.Values) != "{}" {
		t.Fatalf("observer ready prefs = %#v", observerReady.Prefs)
	}

	// A dial with matching revision and checksum gets no snapshot.
	editor := dialPanelStream(
		t,
		streamURL+"?prefs_rev=0&prefs_sum="+emptySum,
		secondSession,
	)
	editorReady := readPanelReady(t, editor)
	if editorReady.Prefs.Rev != 0 || len(editorReady.Prefs.Values) != 0 {
		t.Fatalf("editor ready prefs = %#v", editorReady.Prefs)
	}

	// A patch mixing valid and invalid keys applies the valid ones, fans the
	// change out to every connection of the account, and reports the rejected
	// keys only to the originator.
	patch := map[string]any{
		"version": panelEventVersion,
		"type":    panelInboundPrefsPatch,
		"changes": map[string]any{
			prefKeyTheme:      "dark",
			prefKeyUsersRoles: []string{"viewer", "admin"},
			"bogus":           "x",
		},
	}
	if err := wsjson.Write(t.Context(), editor, patch); err != nil {
		t.Fatal(err)
	}

	observed := readPrefsChanged(t, observer, 1)
	if string(observed.Changes["theme"]) != `"dark"` ||
		string(observed.Changes["table.users.roles"]) != `["admin","viewer"]` {
		t.Fatalf("observed changes = %#v", observed.Changes)
	}
	if _, present := observed.Changes["bogus"]; present {
		t.Fatalf("rejected key leaked into fan-out: %#v", observed.Changes)
	}

	readPrefsChanged(t, editor, 1)
	var rejected panelEvent
	if err := wsjson.Read(t.Context(), editor, &rejected); err != nil {
		t.Fatal(err)
	}
	if rejected.Type != panelEventPrefsRejected ||
		len(rejected.Keys) != 1 || rejected.Keys[0] != "bogus" {
		t.Fatalf("rejected event = %#v", rejected)
	}

	stored, err := harness.store.GetPreferences(t.Context(), "github:test:user:1")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Revision != 1 || len(stored.Values) != 2 {
		t.Fatalf("stored preferences = %#v", stored)
	}

	// A re-dial with the updated revision and checksum matches: no snapshot.
	rejoined := dialPanelStream(
		t,
		streamURL+"?prefs_rev=1&prefs_sum="+prefsChecksum(stored.Values),
		firstSession,
	)
	rejoinedReady := readPanelReady(t, rejoined)
	if rejoinedReady.Prefs.Rev != 1 || len(rejoinedReady.Prefs.Values) != 0 {
		t.Fatalf("rejoined ready prefs = %#v", rejoinedReady.Prefs)
	}

	// A deletion patch drops the key and bumps the revision for everyone.
	deletion := map[string]any{
		"version": panelEventVersion,
		"type":    panelInboundPrefsPatch,
		"changes": map[string]any{prefKeyTheme: nil},
	}
	if err := wsjson.Write(t.Context(), editor, deletion); err != nil {
		t.Fatal(err)
	}
	deleted := readPrefsChanged(t, observer, 2)
	if string(deleted.Changes["theme"]) != "null" {
		t.Fatalf("deleted event = %#v", deleted)
	}
}

func readPrefsChanged(t *testing.T, connection *websocket.Conn, rev int64) panelEvent {
	t.Helper()
	var event panelEvent
	if err := wsjson.Read(t.Context(), connection, &event); err != nil {
		t.Fatal(err)
	}
	if event.Type != panelEventPrefsChanged || event.Rev != rev {
		t.Fatalf("prefs.changed event = %#v", event)
	}

	return event
}

func TestPanelWebSocketPreferencesInboundLimits(t *testing.T) {
	t.Run("oversized frame closes the connection", func(t *testing.T) {
		harness := newPanelHarness(t, "owner")
		session := harness.signIn(t)
		endpoint := httptest.NewServer(harness.handler)
		t.Cleanup(endpoint.Close)
		streamURL := "ws" + strings.TrimPrefix(endpoint.URL, "http") + "/panel/api/v1/events"

		connection := dialPanelStream(t, streamURL, session)
		readPanelReady(t, connection)

		huge := bytes.Repeat([]byte("a"), panelInboundReadLimit+1)
		if err := connection.Write(t.Context(), websocket.MessageText, huge); err != nil {
			t.Fatal(err)
		}
		var next panelEvent
		if err := wsjson.Read(t.Context(), connection, &next); err == nil {
			t.Fatalf("expected the connection to close, read %#v", next)
		}
	})

	t.Run("rapid frames trip the rate limit", func(t *testing.T) {
		harness := newPanelHarness(t, "owner")
		session := harness.signIn(t)
		endpoint := httptest.NewServer(harness.handler)
		t.Cleanup(endpoint.Close)
		streamURL := "ws" + strings.TrimPrefix(endpoint.URL, "http") + "/panel/api/v1/events"

		connection := dialPanelStream(t, streamURL, session)
		readPanelReady(t, connection)

		// The harness clock is frozen, so the bucket never refills: the frame
		// after the burst allowance must close the connection.
		noise := map[string]any{"version": panelEventVersion, "type": "noop"}
		for range panelInboundBurst + 1 {
			if err := wsjson.Write(t.Context(), connection, noise); err != nil {
				break
			}
		}
		var next panelEvent
		err := wsjson.Read(t.Context(), connection, &next)
		if err == nil {
			t.Fatalf("expected the connection to close, read %#v", next)
		}
		if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
			t.Fatalf("close status = %v, want policy violation", err)
		}
	})
}

func TestEventHubAnnounceAccount(t *testing.T) {
	hub := newEventHub()
	mine, unsubscribeMine := hub.subscribe("account-1", "session-a")
	defer unsubscribeMine()
	sibling, unsubscribeSibling := hub.subscribe("account-1", "session-b")
	defer unsubscribeSibling()
	other, unsubscribeOther := hub.subscribe("account-2", "session-c")
	defer unsubscribeOther()

	hub.announceAccount("account-1", panelEvent{Type: panelEventPrefsChanged, Rev: 7})

	for _, subscriber := range []*eventSubscriber{mine, sibling} {
		select {
		case event := <-subscriber.events:
			if event.Type != panelEventPrefsChanged || event.Rev != 7 ||
				event.Version != panelEventVersion {
				t.Fatalf("account event = %#v", event)
			}
		default:
			t.Fatal("expected an event for the account's subscriber")
		}
	}
	select {
	case event := <-other.events:
		t.Fatalf("unexpected event for another account: %#v", event)
	default:
	}

	hub.deliver(mine, panelEvent{Type: panelEventPrefsRejected, Keys: []string{"bogus"}})
	select {
	case event := <-mine.events:
		if event.Type != panelEventPrefsRejected || len(event.Keys) != 1 {
			t.Fatalf("delivered event = %#v", event)
		}
	default:
		t.Fatal("expected a delivered event")
	}
	select {
	case event := <-sibling.events:
		t.Fatalf("deliver leaked to a sibling connection: %#v", event)
	default:
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
	if targets.Code != http.StatusOK || targets.Body.String() != `{"targets":[]}`+"\n" {
		t.Fatalf("unassigned viewer targets = %d %s", targets.Code, targets.Body.String())
	}
	viewerRole := storage.InstallationRoleViewer
	viewerOverride, err := harness.store.SetTargetAccess(t.Context(), storage.TargetAccessChange{
		TargetID:         "github:installation:10",
		SubjectAccountID: viewer.ID,
		ActorAccountID:   "github:test:user:1",
		Role:             &viewerRole,
		ExpectedRevision: 0,
		ChangedAt:        harness.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	targets = harness.request(t, http.MethodGet, "/panel/api/v1/targets", nil, viewerSession)
	if targets.Code != http.StatusOK ||
		!strings.Contains(targets.Body.String(), `"effective_role":"viewer"`) ||
		!strings.Contains(targets.Body.String(), `"write":false`) {
		t.Fatalf("viewer targets = %d %s", targets.Code, targets.Body.String())
	}
	target, err := harness.store.GetTarget(t.Context(), "github:installation:10")
	if err != nil {
		t.Fatal(err)
	}
	input := targetInstallationSettingsBatchBody(t, target, true)
	denied := harness.request(
		t,
		http.MethodPut,
		"/panel/api/v1/targets/github:installation:10/settings",
		bytes.NewReader(input),
		viewerSession,
	)
	if denied.Code != http.StatusNotFound {
		t.Fatalf("viewer write = %d %s", denied.Code, denied.Body.String())
	}

	editor := storage.InstallationRoleEditor
	override, err := harness.store.SetTargetAccess(t.Context(), storage.TargetAccessChange{
		TargetID:         "github:installation:10",
		SubjectAccountID: viewer.ID,
		ActorAccountID:   "github:test:user:1",
		Role:             &editor,
		ExpectedRevision: viewerOverride.Revision,
		ChangedAt:        harness.now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	updated := harness.request(
		t,
		http.MethodPut,
		"/panel/api/v1/targets/github:installation:10/settings",
		bytes.NewReader(input),
		viewerSession,
	)
	if updated.Code != http.StatusOK {
		t.Fatalf("editor write = %d %s", updated.Code, updated.Body.String())
	}

	noAccess := storage.InstallationRoleNone
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
		ActorAccountID: "github:test:user:1",
		ChangedAt:      harness.now,
	}); err != nil {
		t.Fatal(err)
	}
	role := storage.InstallationRoleViewer
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

func TestPanelActivatesFreshDerivedOwnerOnSignIn(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	_ = harness.signIn(t)
	derived := storage.Account{
		ID: "github:test:user:99", Provider: "github:test", SubjectID: "99",
		Login: "derived-owner", DisplayName: "Derived Owner", UpdatedAt: harness.now,
	}
	if err := harness.store.UpsertAccount(t.Context(), derived); err != nil {
		t.Fatal(err)
	}
	target, err := harness.store.GetTarget(t.Context(), "github:installation:10")
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.store.ReconcileInstallation(t.Context(), storage.InstallationSnapshot{
		TargetID: target.ID, InstallationID: target.InstallationID,
		Kind: target.Kind, Account: target.Account,
		Ownership: storage.OwnershipSnapshot{
			Source:   storage.OwnershipSourceOrganizationAdmin,
			Status:   storage.OwnershipStatusFresh,
			Owners:   []storage.Account{derived},
			SyncedAt: harness.now,
		},
		SyncedAt: harness.now,
	}); err != nil {
		t.Fatal(err)
	}
	authorized, err := harness.server.authorizeAccount(
		httptest.NewRequest(http.MethodGet, "/panel/auth/callback", nil), derived,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !authorized {
		t.Fatal("fresh GitHub-derived Owner was not activated")
	}
	user, err := harness.store.GetPanelUser(t.Context(), derived.ID)
	if err != nil {
		t.Fatal(err)
	}
	if user.SystemRole != storage.SystemRoleNone {
		t.Fatalf("derived Owner policy = %#v", user)
	}
}

func TestPanelManagesInstallationUsers(t *testing.T) {
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
		"/panel/api/v1/targets/github:installation:10/users",
		strings.NewReader(`{"login":"managed","role":"editor"}`),
		ownerSession,
	)
	if added.Code != http.StatusCreated ||
		!strings.Contains(added.Body.String(), `"effective_role":"editor"`) {
		t.Fatalf("add user = %d %s", added.Code, added.Body.String())
	}
	listed := harness.request(
		t, http.MethodGet, "/panel/api/v1/targets/github:installation:10/users", nil, ownerSession,
	)
	requireResponse(
		t, listed, "list users", http.StatusOK,
		`"items"`, `"login":"managed"`, `"total":2`,
	)
	filtered := harness.request(
		t,
		http.MethodGet,
		"/panel/api/v1/targets/github:installation:10/users?q=managed&role=editor&status=active&sort=updated_newest&limit=1",
		nil,
		ownerSession,
	)
	requireResponse(
		t, filtered, "filter users", http.StatusOK, `"login":"managed"`, `"total":1`,
	)
	invalidPage := harness.request(
		t, http.MethodGet, "/panel/api/v1/targets/github:installation:10/users?limit=0", nil,
		ownerSession,
	)
	requireResponse(t, invalidPage, "invalid user page", http.StatusBadRequest)
	suspended := harness.request(
		t,
		http.MethodPut,
		"/panel/api/v1/targets/github:installation:10/users/"+url.PathEscape(managed.ID),
		strings.NewReader(`{"role":"editor","suspended":true,"suspension_reason":"incident review","expected_revision":1}`),
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

func TestPanelSeparatesRootAndInstallationAccessRoutes(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	rootSession := harness.signIn(t)
	for _, path := range []string{"/panel/api/v1/users", "/panel/api/v1/invitations"} {
		response := harness.request(t, http.MethodGet, path, nil, rootSession)
		if response.Code != http.StatusNotFound {
			t.Fatalf("legacy route %s = %d %s", path, response.Code, response.Body.String())
		}
	}

	synced := harness.request(
		t, http.MethodPost, "/panel/api/v1/root/installations/sync", nil, rootSession,
	)
	requireResponse(
		t, synced, "Root installation sync", http.StatusOK,
		`"target_ids":["github:installation:10"]`,
	)

	ordinary := storage.Account{
		ID: "github:test:user:ordinary", Provider: "github:test", SubjectID: "ordinary",
		Login: "ordinary", DisplayName: "Ordinary User", UpdatedAt: harness.now,
	}
	if err := harness.store.UpsertAccount(t.Context(), ordinary); err != nil {
		t.Fatal(err)
	}
	if _, err := harness.store.CreatePanelUser(t.Context(), storage.PanelUserCreate{
		AccountID:      ordinary.ID,
		ActorAccountID: "github:test:user:1", ChangedAt: harness.now,
	}); err != nil {
		t.Fatal(err)
	}
	const ordinaryToken = "ordinary-session"
	if err := harness.store.CreateSession(t.Context(), storage.Session{
		TokenHash: tokenHash(ordinaryToken), AccountID: ordinary.ID, CreatedAt: harness.now,
		ExpiresAt: harness.now.Add(time.Hour),
	}, 1); err != nil {
		t.Fatal(err)
	}
	blocked := harness.request(
		t, http.MethodPost, "/panel/api/v1/root/installations/sync", nil,
		&http.Cookie{Name: sessionCookieName, Value: ordinaryToken},
	)
	requireResponse(t, blocked, "ordinary installation sync", http.StatusForbidden)
	blockedElevation := harness.request(
		t, http.MethodPost,
		"/panel/api/v1/root/installations/github:installation:10/elevation",
		strings.NewReader(`{"acknowledged":true}`),
		&http.Cookie{Name: sessionCookieName, Value: ordinaryToken},
	)
	requireResponse(t, blockedElevation, "ordinary Root elevation", http.StatusForbidden)
}

func TestPanelRootOverview(t *testing.T) {
	harness := newPanelHarness(t, "root")
	rootSession := harness.signIn(t)
	seedFailure(t, harness, "overview-failure", "GitHub provider timeout", true)
	target, err := harness.store.GetTarget(t.Context(), "github:installation:10")
	if err != nil {
		t.Fatal(err)
	}
	updated := harness.request(
		t, http.MethodPut, "/panel/api/v1/targets/github:installation:10/settings",
		bytes.NewReader(targetInstallationSettingsBatchBody(t, target, true)),
		rootSession,
	)
	requireResponse(t, updated, "seed Root audit", http.StatusOK, `"revision":2`)

	// Whatever the store says its engine is, not a name written here: the suite
	// runs against either engine, so naming one would pass by describing the
	// run rather than by proving the panel reports what the port reported.
	engine := harness.store.Status(t.Context()).Engine
	if engine == "" {
		t.Fatal("the store did not name its engine, leaving the assertion below vacuous")
	}

	overview := harness.request(
		t, http.MethodGet, "/panel/api/v1/root/overview", nil, rootSession,
	)
	requireResponse(
		t, overview, "Root overview", http.StatusOK,
		`"status":"healthy"`, `"version":"1.0.0"`,
		`"installations":1`, `"repositories":1`,
		`"fresh":1`, `"delivery_id":"overview-failure"`,
		`"storage":"healthy"`,
		`"database":{"state":"healthy","engine":"`+engine+`","version":"`,
	)

	audit := harness.request(
		t, http.MethodGet,
		"/panel/api/v1/root/history/audit?category=configuration&sort=newest&limit=10",
		nil, rootSession,
	)
	requireResponse(
		t, audit, "Root audit", http.StatusOK,
		`"category":"configuration"`, `"action":"installation.settings.saved"`,
		`"installation":{"id":"github:test:account:2"`,
	)
	failures := harness.request(
		t, http.MethodGet,
		"/panel/api/v1/root/history/failures?kind=retryable&q=provider&sort=newest&limit=10",
		nil, rootSession,
	)
	requireResponse(
		t, failures, "Root failures", http.StatusOK,
		`"delivery_id":"overview-failure"`, `"login":"smykla-skalski"`,
	)
	invalid := harness.request(
		t, http.MethodGet, "/panel/api/v1/root/history/audit?category=unknown", nil, rootSession,
	)
	requireResponse(t, invalid, "invalid Root audit category", http.StatusBadRequest)

	rootUsers := harness.request(
		t, http.MethodGet,
		"/panel/api/v1/root/access/users?system_role=super_root&status=active&sort=role_desc&limit=10",
		nil, rootSession,
	)
	requireResponse(
		t, rootUsers, "Root users", http.StatusOK,
		`"system_role":"super_root"`, `"owned_installations":1`,
		`"assigned_installations":0`, `"can_manage_system_role":false`,
	)
	invalidUsers := harness.request(
		t, http.MethodGet, "/panel/api/v1/root/access/users?system_role=owner", nil, rootSession,
	)
	requireResponse(t, invalidUsers, "invalid Root user role", http.StatusBadRequest)
}

func TestPanelManagesPendingCIQueue(t *testing.T) {
	harness := newPanelHarness(t, "root")
	rootSession := harness.signIn(t)
	result, err := harness.store.Arm(context.Background(), pendingci.ArmRequest{
		TargetID: "github:installation:10", InstallationID: 10,
		RepositoryID: "github:repository:20", RepositoryFullName: "smykla-skalski/smyklot",
		PullRequest: 42, HeadSHA: "abc123", BaseBranch: "main",
		MergeMethod: pendingci.MergeMethodSquash, Requester: "operator",
		SourceCommentID: 99, SourceRevision: harness.now.Format(time.RFC3339Nano),
		SourceSequence: 1, SourceOrder: 1,
		Label: "smyklot:pending:ci:squash", RequestedAt: harness.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	woke, err := harness.store.Wake(t.Context(), pendingci.WakeRequest{
		RepositoryID: result.Request.RepositoryID,
		PullRequest:  result.Request.PullRequest,
		EventName:    "check_suite",
		EventKey:     "check_suite:abc123:completed:success",
		DeliveryID:   "delivery-123",
		OccurredAt:   harness.now.Add(time.Second),
	})
	if err != nil || !woke {
		t.Fatalf("webhook wake = %t, %v", woke, err)
	}

	overview := harness.request(
		t, http.MethodGet, "/panel/api/v1/root/overview", nil, rootSession,
	)
	requireResponse(
		t, overview, "pending CI overview", http.StatusOK,
		`"pending_ci":{"active":[`, `"repository_full_name":"smykla-skalski/smyklot"`,
		`"merge_method":"squash"`, `"next_check_trigger":"webhook"`, `"recent":[]`,
	)
	detail := harness.request(
		t, http.MethodGet,
		"/panel/api/v1/root/pending-ci/"+strconv.FormatInt(result.Request.ID, 10),
		nil, rootSession,
	)
	requireResponse(
		t, detail, "pending CI audit", http.StatusOK,
		`"kind":"armed"`, `"kind":"wake_received"`,
		`"trigger":"webhook"`, `"event_name":"check_suite"`,
		`"delivery_id":"delivery-123"`,
	)

	check := harness.request(
		t, http.MethodPost,
		"/panel/api/v1/root/pending-ci/"+strconv.FormatInt(result.Request.ID, 10)+"/check",
		strings.NewReader(`{"expected_revision":2}`), rootSession,
	)
	requireResponse(t, check, "check pending CI now", http.StatusOK, `"schedule":"active"`, `"revision":3`)

	stale := harness.request(
		t, http.MethodDelete,
		"/panel/api/v1/root/pending-ci/"+strconv.FormatInt(result.Request.ID, 10),
		strings.NewReader(`{"expected_revision":2}`), rootSession,
	)
	requireResponse(t, stale, "stale pending CI cancellation", http.StatusConflict)

	cancel := harness.request(
		t, http.MethodDelete,
		"/panel/api/v1/root/pending-ci/"+strconv.FormatInt(result.Request.ID, 10),
		strings.NewReader(`{"expected_revision":3}`), rootSession,
	)
	requireResponse(t, cancel, "cancel pending CI", http.StatusOK, `"lifecycle":"cancelled"`)

	finished := harness.request(
		t, http.MethodGet, "/panel/api/v1/root/overview", nil, rootSession,
	)
	requireResponse(
		t, finished, "finished pending CI overview", http.StatusOK,
		`"active":[]`, `"recent":[`, `"lifecycle":"cancelled"`,
		`"reason":"cancelled by panel user @root"`,
	)
	if harness.pendingCI.wakes != 2 {
		t.Fatalf("scheduler wakes = %d, want 2", harness.pendingCI.wakes)
	}
}

func TestPanelRetiredSettingsAndSyncRoutesAreRemoved(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	target := "/panel/api/v1/targets/github:installation:10"
	rootTarget := "/panel/api/v1/root/installations/github:installation:10"

	for _, probe := range []struct {
		method string
		path   string
		status int
	}{
		{http.MethodPut, target + "/sync/config/labels", http.StatusMethodNotAllowed},
		{http.MethodPut, target + "/sync/config", http.StatusMethodNotAllowed},
		{http.MethodGet, target + "/sync/config/checkpoints/1", http.StatusNotFound},
		{http.MethodPost, target + "/sync/config/checkpoints/1/restore", http.StatusMethodNotAllowed},
		{http.MethodGet, target + "/sync/overrides/files", http.StatusNotFound},
		{http.MethodPut, target + "/repositories/repository-20/sync/files", http.StatusMethodNotAllowed},
		{http.MethodPut, target + "/settings/batch", http.StatusMethodNotAllowed},
		{http.MethodPut, target + "/repositories/repository-20/settings", http.StatusMethodNotAllowed},
		{http.MethodPut, rootTarget + "/settings/batch", http.StatusMethodNotAllowed},
		{http.MethodPut, rootTarget + "/repositories/repository-20/settings", http.StatusMethodNotAllowed},
		{http.MethodGet, rootTarget + "/sync/config/checkpoints/1", http.StatusNotFound},
		{http.MethodPost, rootTarget + "/sync/config/checkpoints/1/restore", http.StatusMethodNotAllowed},
	} {
		response := harness.request(
			t, probe.method, probe.path, strings.NewReader(`{}`), session,
		)
		if response.Code != probe.status {
			t.Errorf("%s %s = %d, want %d", probe.method, probe.path, response.Code, probe.status)
		}
	}
}

func TestPanelRootRuntimeSettings(t *testing.T) {
	harness := newPanelHarness(t, "root")
	rootSession := harness.signIn(t)

	current := harness.request(
		t, http.MethodGet, "/panel/api/v1/root/runtime/settings", nil, rootSession,
	)
	requireResponse(
		t, current, "Root runtime settings", http.StatusOK,
		`"effective_seconds":43200`,
		`"deployment":"info"`, `"public":":8080"`,
		`"reaction_poll_interval":{"deployment_seconds":300`,
		`"merge_after_ci_quiet_period":{"deployment_seconds":30`,
		`"webhook":true`, `"revision":0`,
	)

	behavior := config.Default()
	behavior.QuietSuccess = true
	content, err := json.Marshal(map[string]any{
		"bot_config":                          behavior,
		"log_level":                           "debug",
		"reaction_poll_interval_seconds":      90,
		"merge_after_ci_quiet_period_seconds": 45,
		"path_index_interval_seconds":         nil,
		"session_ttl_seconds":                 3600,
		"expected_revision":                   0,
	})
	if err != nil {
		t.Fatal(err)
	}
	updated := harness.request(
		t, http.MethodPut, "/panel/api/v1/root/runtime/settings", bytes.NewReader(content), rootSession,
	)
	requireResponse(
		t, updated, "update Root runtime settings", http.StatusOK,
		`"quiet_success":true`, `"override":"debug"`,
		`"reaction_poll_interval":{"deployment_seconds":300,"override_seconds":90,"effective_seconds":90}`,
		`"merge_after_ci_quiet_period":{"deployment_seconds":30,"override_seconds":45,"effective_seconds":45}`,
		`"override_seconds":3600`, `"revision":1`,
	)
	if !harness.runtime.values.BotConfig.QuietSuccess ||
		harness.runtime.values.LogLevel != slog.LevelDebug ||
		harness.runtime.values.PollInterval != 90*time.Second ||
		harness.runtime.values.PendingCIQuietPeriod != 45*time.Second ||
		harness.runtime.values.SessionTTL != time.Hour {
		t.Fatalf("applied runtime values = %#v", harness.runtime.values)
	}
	subscriber, unsubscribe := harness.server.events.subscribe("", "runtime-noop-test")
	t.Cleanup(unsubscribe)
	noOpContent, err := json.Marshal(map[string]any{
		"bot_config":                          behavior,
		"log_level":                           "debug",
		"reaction_poll_interval_seconds":      90,
		"merge_after_ci_quiet_period_seconds": 45,
		"path_index_interval_seconds":         nil,
		"session_ttl_seconds":                 3600,
		"expected_revision":                   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	noOp := harness.request(
		t, http.MethodPut, "/panel/api/v1/root/runtime/settings",
		bytes.NewReader(noOpContent), rootSession,
	)
	requireResponse(t, noOp, "no-op Root runtime settings", http.StatusOK, `"revision":1`)
	select {
	case event := <-subscriber.events:
		t.Fatalf("no-op runtime settings announced %#v", event)
	case <-time.After(25 * time.Millisecond):
	}
	shortened, err := harness.store.GetSession(
		t.Context(), tokenHash(rootSession.Value), harness.now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if want := harness.now.Add(time.Hour); !shortened.ExpiresAt.Equal(want) {
		t.Fatalf("session expiry = %s, want %s", shortened.ExpiresAt, want)
	}
	incompleteContent, err := json.Marshal(map[string]any{
		"bot_config":                     behavior,
		"log_level":                      "debug",
		"reaction_poll_interval_seconds": 90,
		"session_ttl_seconds":            3600,
		"expected_revision":              1,
	})
	if err != nil {
		t.Fatal(err)
	}
	incomplete := harness.request(
		t, http.MethodPut, "/panel/api/v1/root/runtime/settings",
		bytes.NewReader(incompleteContent), rootSession,
	)
	requireResponse(
		t, incomplete, "reject incomplete Root runtime settings", http.StatusBadRequest,
		`"code":"invalid_runtime_settings"`,
		`"message":"every runtime setting and expected revision is required"`,
	)
	disabledContent, err := json.Marshal(map[string]any{
		"bot_config":                          behavior,
		"log_level":                           "debug",
		"reaction_poll_interval_seconds":      0,
		"merge_after_ci_quiet_period_seconds": 45,
		"path_index_interval_seconds":         nil,
		"session_ttl_seconds":                 3600,
		"expected_revision":                   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	disabled := harness.request(
		t, http.MethodPut, "/panel/api/v1/root/runtime/settings",
		bytes.NewReader(disabledContent), rootSession,
	)
	requireResponse(
		t, disabled, "disable reaction sweep", http.StatusOK,
		`"reaction_poll_interval":{"deployment_seconds":300,"override_seconds":0,"effective_seconds":0}`,
		`"revision":2`,
	)
	if harness.runtime.values.PollInterval != 0 {
		t.Fatalf("disabled reaction sweep = %s", harness.runtime.values.PollInterval)
	}

	reset := harness.request(
		t, http.MethodPut, "/panel/api/v1/root/runtime/settings",
		strings.NewReader(`{
            "bot_config":null,
            "log_level":null,
			"reaction_poll_interval_seconds":null,
			"merge_after_ci_quiet_period_seconds":null,
			"path_index_interval_seconds":null,
            "session_ttl_seconds":null,
			"expected_revision":2
        }`),
		rootSession,
	)
	requireResponse(
		t, reset, "reset Root runtime settings", http.StatusOK,
		`"override":null`, `"override_seconds":null`, `"revision":3`,
	)
	if harness.runtime.values.BotConfig.QuietSuccess ||
		harness.runtime.values.LogLevel != slog.LevelInfo ||
		harness.runtime.values.PollInterval != 5*time.Minute ||
		harness.runtime.values.PendingCIQuietPeriod != 30*time.Second ||
		harness.runtime.values.SessionTTL != 12*time.Hour {
		t.Fatalf("reset runtime values = %#v", harness.runtime.values)
	}
	unchanged, err := harness.store.GetSession(
		t.Context(), tokenHash(rootSession.Value), harness.now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !unchanged.ExpiresAt.Equal(shortened.ExpiresAt) {
		t.Fatalf("session was extended from %s to %s", shortened.ExpiresAt, unchanged.ExpiresAt)
	}

	zeroQuiet := harness.request(
		t, http.MethodPut, "/panel/api/v1/root/runtime/settings",
		strings.NewReader(`{
            "bot_config":null,
            "log_level":null,
			"reaction_poll_interval_seconds":null,
			"merge_after_ci_quiet_period_seconds":0,
			"path_index_interval_seconds":null,
            "session_ttl_seconds":null,
			"expected_revision":3
        }`),
		rootSession,
	)
	requireResponse(
		t, zeroQuiet, "accept zero pending-CI quiet period", http.StatusOK,
		`"merge_after_ci_quiet_period":{"deployment_seconds":30,"override_seconds":0,"effective_seconds":0}`,
		`"revision":4`,
	)
	if harness.runtime.values.PendingCIQuietPeriod != 0 {
		t.Fatalf("zero pending-CI quiet period = %s", harness.runtime.values.PendingCIQuietPeriod)
	}
	overMaximum := harness.request(
		t, http.MethodPut, "/panel/api/v1/root/runtime/settings",
		strings.NewReader(`{
            "bot_config":null,
            "log_level":null,
			"reaction_poll_interval_seconds":null,
			"merge_after_ci_quiet_period_seconds":86401,
			"path_index_interval_seconds":null,
            "session_ttl_seconds":null,
			"expected_revision":4
        }`),
		rootSession,
	)
	requireResponse(
		t, overMaximum, "reject overlong pending-CI quiet period", http.StatusBadRequest,
		`"code":"invalid_runtime_settings"`,
		`"message":"merge-after-CI quiet period is outside the supported range"`,
	)

	audit := harness.request(
		t, http.MethodGet, "/panel/api/v1/root/history/audit?category=runtime", nil, rootSession,
	)
	requireResponse(
		t, audit, "Root runtime audit", http.StatusOK,
		`"category":"runtime"`, `"action":"runtime.settings.saved"`,
		`"settings_checkpoint_id":`,
	)
}

func TestPanelManagesRootUsers(t *testing.T) {
	harness := newPanelHarness(t, "root")
	rootSession := harness.signIn(t)
	ordinary := storage.Account{
		ID: "github:test:user:managed", Provider: "github:test", SubjectID: "managed",
		Login: "managed", DisplayName: "Managed User", UpdatedAt: harness.now,
	}
	if err := harness.store.UpsertAccount(t.Context(), ordinary); err != nil {
		t.Fatal(err)
	}
	created, err := harness.store.CreatePanelUser(t.Context(), storage.PanelUserCreate{
		AccountID:      ordinary.ID,
		ActorAccountID: "github:test:user:1", ChangedAt: harness.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	const ordinaryToken = "managed-session"
	if err := harness.store.CreateSession(t.Context(), storage.Session{
		TokenHash: tokenHash(ordinaryToken), AccountID: ordinary.ID,
		CreatedAt: harness.now, ExpiresAt: harness.now.Add(time.Hour),
	}, 1); err != nil {
		t.Fatal(err)
	}
	subscriber, unsubscribe := harness.server.events.subscribe(
		ordinary.ID, tokenHash(ordinaryToken),
	)
	t.Cleanup(unsubscribe)

	promoted := harness.request(
		t, http.MethodPut, "/panel/api/v1/root/access/users/"+ordinary.ID,
		strings.NewReader(fmt.Sprintf(
			`{"system_role":"root","expected_revision":%d}`, created.Revision,
		)), rootSession,
	)
	requireResponse(t, promoted, "promote Root user", http.StatusNoContent)
	demoted := harness.request(
		t, http.MethodPut, "/panel/api/v1/root/access/users/"+ordinary.ID,
		strings.NewReader(fmt.Sprintf(
			`{"system_role":"none","expected_revision":%d}`, created.Revision+1,
		)), rootSession,
	)
	requireResponse(t, demoted, "demote Root user", http.StatusNoContent)
	banned := harness.request(
		t, http.MethodPut, "/panel/api/v1/root/access/users/"+ordinary.ID,
		strings.NewReader(fmt.Sprintf(
			`{"status":"banned","reason":"security incident","expected_revision":%d}`,
			created.Revision+2,
		)), rootSession,
	)
	requireResponse(t, banned, "ban Root user", http.StatusNoContent)
	if _, err := harness.store.GetSession(
		t.Context(), tokenHash(ordinaryToken), harness.now,
	); !errors.Is(err, storage.ErrRevoked) {
		t.Fatalf("managed session error = %v", err)
	}
	select {
	case event := <-subscriber.terminal:
		if event.Type != panelEventSessionRevoked || event.Code != "account_banned" {
			t.Fatalf("managed session event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for managed session revocation")
	}

	selfChange := harness.request(
		t, http.MethodPut, "/panel/api/v1/root/access/users/github:test:user:1",
		strings.NewReader(`{"system_role":"root","expected_revision":1}`), rootSession,
	)
	requireResponse(t, selfChange, "change Super Root", http.StatusForbidden)
}

func TestPanelManagesRootInvitations(t *testing.T) {
	harness := newPanelHarness(t, "root")
	rootSession := harness.signIn(t)
	invitee := storage.Account{
		ID: "github:test:user:invited-root", Provider: "github:test", SubjectID: "invited-root",
		Login: "invited-root", DisplayName: "Invited Root", UpdatedAt: harness.now,
	}
	harness.server.users = fakeUserResolver{account: invitee}
	created := harness.request(
		t, http.MethodPost, "/panel/api/v1/root/access/invitations",
		strings.NewReader(`{"login":"invited-root","expires_in_days":7}`), rootSession,
	)
	requireResponse(
		t, created, "create Root invitation", http.StatusCreated,
		`"system_role":"root"`, `"login":"invited-root"`, `"invite_url":`,
	)
	var invitation invitationResponse
	if err := json.Unmarshal(created.Body.Bytes(), &invitation); err != nil {
		t.Fatal(err)
	}
	listed := harness.request(
		t, http.MethodGet, "/panel/api/v1/root/access/invitations?status=pending&limit=10",
		nil, rootSession,
	)
	requireResponse(t, listed, "list Root invitations", http.StatusOK, invitation.ID)
	reissued := harness.request(
		t, http.MethodPost,
		"/panel/api/v1/root/access/invitations/"+invitation.ID+"/reissue",
		strings.NewReader(`{"expires_in_days":7}`), rootSession,
	)
	requireResponse(t, reissued, "reissue Root invitation", http.StatusOK, `"invite_url":`)
	revoked := harness.request(
		t, http.MethodDelete, "/panel/api/v1/root/access/invitations/"+invitation.ID,
		nil, rootSession,
	)
	requireResponse(t, revoked, "revoke Root invitation", http.StatusOK, `"status":"revoked"`)
}

func TestPanelRootElevationAndOwnerNotifications(t *testing.T) {
	harness := newPanelHarness(t, "root")
	rootSession := harness.signIn(t)
	owner, target := seedNonOwnedInstallation(t, harness)

	installations := harness.request(
		t, http.MethodGet, "/panel/api/v1/root/installations", nil, rootSession,
	)
	requireResponse(
		t, installations, "Root installations", http.StatusOK,
		`"id":"github:installation:20"`, `"owner_count":1`,
		`"delivery_health":{"failed":0}`,
	)
	rootSettingsPath := "/panel/api/v1/root/installations/" + target.TargetID + "/settings"
	settings := harness.request(t, http.MethodGet, rootSettingsPath, nil, rootSession)
	requireResponse(
		t, settings, "Root installation settings", http.StatusOK,
		`"access_source":"root"`, `"write":false`,
	)
	targetSettings, err := harness.store.GetTarget(t.Context(), target.TargetID)
	if err != nil {
		t.Fatal(err)
	}
	settingsInput := targetInstallationSettingsBatchBody(t, targetSettings, true)
	blockedWrite := harness.request(
		t, http.MethodPut, rootSettingsPath, bytes.NewReader(settingsInput), rootSession,
	)
	requireResponse(
		t, blockedWrite, "Root write without elevation", http.StatusForbidden,
		`"code":"elevation_required"`,
	)
	blockedAccessWrite := harness.request(
		t, http.MethodPost,
		"/panel/api/v1/root/installations/"+target.TargetID+"/users",
		strings.NewReader(`{"login":"support-user","role":"viewer"}`), rootSession,
	)
	requireResponse(
		t, blockedAccessWrite, "Root access write without elevation", http.StatusForbidden,
		`"code":"elevation_required"`,
	)
	missingAcknowledgment := harness.request(
		t, http.MethodPost,
		"/panel/api/v1/root/installations/"+target.TargetID+"/elevation",
		strings.NewReader(`{"acknowledged":false}`), rootSession,
	)
	requireResponse(
		t, missingAcknowledgment, "Root elevation acknowledgment",
		http.StatusBadRequest, `"code":"acknowledgment_required"`,
	)

	reason := "investigate an installation incident"
	started := harness.request(
		t, http.MethodPost,
		"/panel/api/v1/root/installations/"+target.TargetID+"/elevation",
		strings.NewReader(`{"acknowledged":true,"reason":"`+reason+`"}`), rootSession,
	)
	requireResponse(t, started, "start Root elevation", http.StatusCreated, `"reason":"`+reason+`"`)
	var elevation elevationResponse
	if err := json.Unmarshal(started.Body.Bytes(), &elevation); err != nil {
		t.Fatal(err)
	}
	current := harness.request(
		t, http.MethodGet,
		"/panel/api/v1/root/installations/"+target.TargetID+"/elevation", nil, rootSession,
	)
	requireResponse(t, current, "current Root elevation", http.StatusOK, elevation.ID)

	rootHash := tokenHash(rootSession.Value)
	regularWrite := harness.request(
		t, http.MethodPut, "/panel/api/v1/targets/"+target.TargetID+"/settings",
		bytes.NewReader(settingsInput), rootSession,
	)
	requireResponse(t, regularWrite, "elevated regular-route write", http.StatusNotFound)
	elevatedWrite := harness.request(
		t, http.MethodPut, rootSettingsPath, bytes.NewReader(settingsInput), rootSession,
	)
	requireResponse(
		t, elevatedWrite, "elevated Root write", http.StatusOK,
		`"target_id":"`+target.TargetID+`"`, `"revision":2`, `"checkpoint_id":`,
	)
	repositoryWrite := harness.request(
		t, http.MethodPut, rootSettingsPath,
		strings.NewReader(`{"repositories":[{"repository_id":"repository-30",
			"enabled_override":true,"pending_ci_mode_override":null,
			"pending_ci_branch_patterns_override":null,
			"pending_ci_quiet_period_seconds_override":null,
			"path_index_interval_seconds_override":null,"config_patch":{},
			"ignore_repository_file":false,"expected_revision":1}]}`),
		rootSession,
	)
	requireResponse(
		t, repositoryWrite, "elevated Root repository write", http.StatusOK,
		`"repository_id":"repository-30"`, `"revision":2`,
	)
	proposal := 42
	if err := harness.store.SetRepositoryConfigMigration(
		t.Context(),
		storage.RepositoryConfigMigration{
			TargetID: target.TargetID, RepositoryID: "repository-30",
			State: storage.ConfigMigrationDeclined, PullRequest: &proposal,
		},
	); err != nil {
		t.Fatalf("seed declined configuration migration: %v", err)
	}
	migrationReset := harness.request(
		t, http.MethodPost,
		"/panel/api/v1/root/installations/"+target.TargetID+
			"/repositories/repository-30/config-migration",
		strings.NewReader(`{}`), rootSession,
	)
	requireResponse(
		t, migrationReset, "elevated Root configuration migration reset", http.StatusOK,
		`"config_migration":"none"`,
	)

	subject := storage.Account{
		ID: "github:test:user:support", Provider: "github:test", SubjectID: "support",
		Login: "support-user", DisplayName: "Support User", UpdatedAt: harness.now,
	}
	harness.server.users = fakeUserResolver{account: subject}
	rootAccessBase := "/panel/api/v1/root/installations/" + target.TargetID
	added := harness.request(
		t, http.MethodPost, rootAccessBase+"/users",
		strings.NewReader(`{"login":"support-user","role":"viewer"}`), rootSession,
	)
	requireResponse(
		t, added, "elevated Root user add", http.StatusCreated,
		`"login":"support-user"`, `"effective_role":"viewer"`,
	)
	listedUsers := harness.request(
		t, http.MethodGet, rootAccessBase+"/users?role=viewer&limit=10", nil, rootSession,
	)
	requireResponse(t, listedUsers, "Root installation users", http.StatusOK, `"login":"support-user"`)
	updatedUser := harness.request(
		t, http.MethodPut, rootAccessBase+"/users/"+subject.ID,
		strings.NewReader(`{"role":"editor","suspended":false,"expected_revision":1}`),
		rootSession,
	)
	requireResponse(
		t, updatedUser, "elevated Root user update", http.StatusOK,
		`"effective_role":"editor"`, `"revision":2`,
	)
	decisions := harness.request(
		t, http.MethodGet, rootAccessBase+"/users/"+subject.ID+"/decisions", nil, rootSession,
	)
	requireResponse(t, decisions, "Root installation decisions", http.StatusOK, `"target.access.updated"`)

	// support-user is an editor here by now, so an invitation would offer what they hold.
	refusedInvitation := harness.request(
		t, http.MethodPost, rootAccessBase+"/invitations",
		strings.NewReader(`{"login":"support-user","role":"viewer","expires_in_days":7}`),
		rootSession,
	)
	requireResponse(
		t, refusedInvitation, "elevated Root invitation for a member", http.StatusConflict,
		`"code":"already_has_access"`,
	)

	invited := storage.Account{
		ID: "github:test:user:newcomer", Provider: "github:test", SubjectID: "newcomer",
		Login: "newcomer", DisplayName: "Newcomer", UpdatedAt: harness.now,
	}
	harness.server.users = fakeUserResolver{account: invited}
	createdInvitation := harness.request(
		t, http.MethodPost, rootAccessBase+"/invitations",
		strings.NewReader(`{"login":"newcomer","role":"viewer","expires_in_days":7}`),
		rootSession,
	)
	requireResponse(
		t, createdInvitation, "elevated Root invitation", http.StatusCreated,
		`"login":"newcomer"`, `"role":"viewer"`,
	)
	var invitation invitationResponse
	if err := json.Unmarshal(createdInvitation.Body.Bytes(), &invitation); err != nil {
		t.Fatal(err)
	}
	listedInvitations := harness.request(
		t, http.MethodGet, rootAccessBase+"/invitations?status=pending&limit=10", nil, rootSession,
	)
	requireResponse(
		t, listedInvitations, "Root installation invitations", http.StatusOK, invitation.ID,
	)
	reissuedInvitation := harness.request(
		t, http.MethodPost, rootAccessBase+"/invitations/"+invitation.ID+"/reissue",
		strings.NewReader(`{"expires_in_days":7}`), rootSession,
	)
	requireResponse(t, reissuedInvitation, "Root invitation reissue", http.StatusOK, `"invite_url":`)
	revokedInvitation := harness.request(
		t, http.MethodDelete, rootAccessBase+"/invitations/"+invitation.ID, nil, rootSession,
	)
	requireResponse(
		t, revokedInvitation, "Root invitation revoke", http.StatusOK, `"status":"revoked"`,
	)
	installationAudit := harness.request(
		t, http.MethodGet, rootAccessBase+"/audit?sort=oldest&limit=20", nil, rootSession,
	)
	requireResponse(
		t, installationAudit, "Root installation audit", http.StatusOK,
		`"target.access.updated"`, `"invitation.created"`,
		`"repository.config_migration.reset"`,
	)
	notificationAudit := harness.request(
		t, http.MethodGet,
		"/panel/api/v1/root/history/audit?category=notification&limit=20", nil, rootSession,
	)
	requireResponse(
		t, notificationAudit, "Root notification audit", http.StatusOK,
		`"category":"notification"`, `"action":"owner.notification.created"`,
		`"elevation_id":"`+elevation.ID+`"`,
	)
	installationFailures := harness.request(
		t, http.MethodGet, rootAccessBase+"/failures?limit=20", nil, rootSession,
	)
	requireResponse(t, installationFailures, "Root installation failures", http.StatusOK, `"total":0`)

	ownerSession := activateOwnerSession(t, harness, owner)
	notifications := harness.request(
		t, http.MethodGet, "/panel/api/v1/notifications", nil, ownerSession,
	)
	requireResponse(
		t, notifications, "Owner notifications", http.StatusOK,
		`"unread":8`, `"elevation_id":"`+elevation.ID+`"`,
		`"action":"installation.settings.saved"`,
		`"action":"target.access.updated"`, `"action":"invitation.created"`,
		`"action":"repository.config_migration.reset"`,
	)
	var page notificationPageResponse
	if err := json.Unmarshal(notifications.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	read := harness.request(
		t, http.MethodPut, "/panel/api/v1/notifications/"+page.Items[0].ID+"/read", nil, ownerSession,
	)
	requireResponse(t, read, "read Owner notification", http.StatusOK, `"read_at":`)

	signedOut := harness.request(t, http.MethodPost, "/panel/api/v1/sign-out", nil, rootSession)
	if signedOut.Code != http.StatusNoContent {
		t.Fatalf("Root sign out = %d %s", signedOut.Code, signedOut.Body.String())
	}
	ended, err := harness.store.EndElevation(
		t.Context(), elevation.ID, rootHash, storage.ElevationEnded, harness.now,
	)
	if err != nil || ended.EndReason == nil || *ended.EndReason != storage.ElevationRevoked {
		t.Fatalf("ended elevation = %#v, error %v", ended, err)
	}
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
		"/panel/api/v1/targets/github:installation:10/invitations",
		strings.NewReader(`{"login":"invited","role":"editor","expires_in_days":7}`),
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
		"/panel/api/v1/targets/github:installation:10/invitations?q=invited&role=editor&status=pending&sort=name_desc&limit=1",
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
	// The invitation page names the scope to someone who has not signed in and may
	// never have heard of it, so the display name alone is not enough: the login is
	// what identifies the account on GitHub, and the kind says whether accepting
	// joins an organisation or one person's installation.
	requireResponse(
		t, review, "review invitation scope", http.StatusOK,
		`"target_name":"Smykla Skalski"`,
		`"target_login":"smykla-skalski"`,
		`"target_kind":"Organization"`,
	)

	invitedSession := harness.acceptInvitation(t, invitee, token)
	viewer := harness.request(t, http.MethodGet, "/panel/api/v1/session", nil, invitedSession)
	if viewer.Code != http.StatusOK || !strings.Contains(viewer.Body.String(), `"target_count":1`) {
		t.Fatalf("invited viewer = %d %s", viewer.Code, viewer.Body.String())
	}
	invitedTargets := harness.request(t, http.MethodGet, "/panel/api/v1/targets", nil, invitedSession)
	if invitedTargets.Code != http.StatusOK ||
		!strings.Contains(invitedTargets.Body.String(), `"effective_role":"editor"`) {
		t.Fatalf("invited targets = %d %s", invitedTargets.Code, invitedTargets.Body.String())
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
		"/panel/api/v1/targets/github:installation:10/invitations/"+
			targetInvitation.ID+"/reissue",
		strings.NewReader(`{"expires_in_days":7}`),
		ownerSession,
	)
	if reissued.Code != http.StatusOK || strings.Contains(reissued.Body.String(), targetInvitation.InviteURL) {
		t.Fatalf("reissue invitation = %d %s", reissued.Code, reissued.Body.String())
	}
	revoked := harness.request(
		t,
		http.MethodDelete,
		"/panel/api/v1/targets/github:installation:10/invitations/"+targetInvitation.ID,
		nil,
		ownerSession,
	)
	if revoked.Code != http.StatusOK || !strings.Contains(revoked.Body.String(), `"status":"revoked"`) {
		t.Fatalf("revoke invitation = %d %s", revoked.Code, revoked.Body.String())
	}
}

// TestPanelRefusesInvitationsThatCannotBeUsed covers the standings a manager can name, and the
// codes the panel reads back to decide between saying no and asking again. The generic conflict
// would tell them settings changed in another session, which is neither true nor actionable.
func TestPanelRefusesInvitationsThatCannotBeUsed(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	ownerSession := harness.signIn(t)
	const invitations = "/panel/api/v1/targets/github:installation:10/invitations"

	invite := func(login, body string) *httptest.ResponseRecorder {
		t.Helper()

		return harness.request(t, http.MethodPost, invitations, strings.NewReader(body), ownerSession)
	}

	owner, err := harness.store.GetAccount(t.Context(), "github:test:user:1")
	if err != nil {
		t.Fatal(err)
	}
	harness.server.users = fakeUserResolver{account: owner}
	requireResponse(
		t, invite("owner", `{"login":"owner","role":"viewer","expires_in_days":7}`),
		"self invitation", http.StatusForbidden, `"code":"self_invitation"`,
	)

	invitee := storage.Account{
		ID: "github:test:user:invitee", Provider: "github:test", SubjectID: "invitee",
		Login: "invitee", DisplayName: "Invitee", UpdatedAt: harness.now,
	}
	harness.server.users = fakeUserResolver{account: invitee}
	const offer = `{"login":"invitee","role":"viewer","expires_in_days":7}`

	first := invite("invitee", offer)
	requireResponse(t, first, "first invitation", http.StatusCreated, `"status":"pending"`)
	requireResponse(t, invite("invitee", offer), "unanswered invitation", http.StatusCreated)

	// Accepting grants the role, and an offer of what somebody holds is refused.
	var pending invitationResponse
	if err := json.Unmarshal(invite("invitee", offer).Body.Bytes(), &pending); err != nil {
		t.Fatal(err)
	}
	pendingURL, err := url.Parse(pending.InviteURL)
	if err != nil {
		t.Fatal(err)
	}
	accepted := strings.TrimPrefix(pendingURL.Path, "/panel/invite/")
	if _, err := harness.store.RespondToInvitation(t.Context(), storage.InvitationResponse{
		TokenHash: tokenHash(accepted), AccountID: invitee.ID, Accept: true, At: harness.now,
	}); err != nil {
		t.Fatal(err)
	}
	requireResponse(
		t, invite("invitee", offer), "invitation for a member", http.StatusConflict,
		`"code":"already_has_access"`,
	)

	// Once the access is gone the identity is invitable again, and a decline gates the next offer
	// behind a second, deliberate press rather than refusing it outright.
	if _, err := harness.store.SetTargetAccess(t.Context(), storage.TargetAccessChange{
		TargetID: "github:installation:10", SubjectAccountID: invitee.ID, ActorAccountID: owner.ID,
		Role: rolePointer(storage.InstallationRoleNone), ExpectedRevision: 1, ChangedAt: harness.now,
	}); err != nil {
		t.Fatal(err)
	}
	var reoffered invitationResponse
	if err := json.Unmarshal(invite("invitee", offer).Body.Bytes(), &reoffered); err != nil {
		t.Fatal(err)
	}
	reofferedURL, err := url.Parse(reoffered.InviteURL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := harness.store.RespondToInvitation(t.Context(), storage.InvitationResponse{
		TokenHash: tokenHash(strings.TrimPrefix(reofferedURL.Path, "/panel/invite/")),
		AccountID: invitee.ID, Accept: false, At: harness.now,
	}); err != nil {
		t.Fatal(err)
	}
	requireResponse(
		t, invite("invitee", offer), "invitation after a decline", http.StatusConflict,
		`"code":"invitation_declined"`,
	)
	requireResponse(
		t,
		invite("invitee", `{"login":"invitee","role":"viewer","expires_in_days":7,"acknowledge_declined":true}`),
		"acknowledged invitation after a decline", http.StatusCreated, `"status":"pending"`,
	)

	// The offer just made is outstanding. Granting the access directly makes it meaningless, and
	// renewing it has to be refused for the same reason making a new one would be - the reissue
	// button is one press away in the same table.
	var outstanding invitationResponse
	listed := harness.request(
		t, http.MethodGet, invitations+"?status=pending&limit=1", nil, ownerSession,
	)
	requireResponse(t, listed, "pending invitations", http.StatusOK, `"login":"invitee"`)
	var page struct {
		Items []invitationResponse `json:"items"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &page); err != nil || len(page.Items) == 0 {
		t.Fatalf("pending invitation page = %v %s", err, listed.Body.String())
	}
	outstanding = page.Items[0]
	if _, err := harness.store.SetTargetAccess(t.Context(), storage.TargetAccessChange{
		TargetID: "github:installation:10", SubjectAccountID: invitee.ID, ActorAccountID: owner.ID,
		Role: rolePointer(storage.InstallationRoleEditor), ExpectedRevision: 2, ChangedAt: harness.now,
	}); err != nil {
		t.Fatal(err)
	}
	reissued := harness.request(
		t, http.MethodPost, invitations+"/"+outstanding.ID+"/reissue",
		strings.NewReader(`{"expires_in_days":7}`), ownerSession,
	)
	requireResponse(
		t, reissued, "reissue for a member", http.StatusConflict, `"code":"already_has_access"`,
	)
}

func rolePointer(role storage.InstallationRole) *storage.InstallationRole {
	return &role
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
	settingsPath := "/panel/api/v1/targets/github:installation:10/settings"
	armed, err := harness.store.Arm(t.Context(), pendingci.ArmRequest{
		TargetID: "github:installation:10", InstallationID: 10,
		RepositoryID: "repository-20", RepositoryFullName: "smykla-skalski/smyklot",
		PullRequest: 99, HeadSHA: "quiet-head", BaseBranch: "main",
		MergeMethod: pendingci.MergeMethodSquash, RequiredChecksOnly: true,
		Requester: "owner", SourceCommentID: 99,
		SourceRevision: harness.now.Format(time.RFC3339Nano), SourceSequence: 1, SourceOrder: 99,
		Label: "smyklot:pending:ci:squash:required", RequestedAt: harness.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	lease, err := harness.store.LeaseDue(t.Context(), harness.now, harness.now.Add(time.Minute))
	if err != nil || lease.Request == nil {
		t.Fatalf("lease quiet-period request = %#v, %v", lease, err)
	}
	_, err = harness.store.Reschedule(t.Context(), pendingci.RescheduleRequest{
		ID: lease.Request.ID, ExpectedRevision: lease.Request.Revision,
		Schedule: pendingci.ScheduleActive, HeadSHA: armed.Request.HeadSHA,
		NextCheckAt: harness.now.Add(24 * time.Hour), NextCheckTrigger: pendingci.TriggerQuietPeriod,
		LastProgressAt: harness.now, LastObservedState: string(pendingci.ObservedPassing),
		LastFingerprint: "passing:1:1", CheckedAt: harness.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	wakesBefore := harness.pendingCI.wakes

	explicitOff := harness.request(
		t,
		http.MethodPut,
		settingsPath,
		strings.NewReader(`{"repositories":[{"repository_id":"repository-20",
			"enabled_override":false,
			"pending_ci_mode_override":"labels",
			"pending_ci_branch_patterns_override":{"include":["refs/heads/release/*"],"exclude":[]},
			"pending_ci_quiet_period_seconds_override":0,
			"path_index_interval_seconds_override":null,
			"config_patch":{},
			"ignore_repository_file":false,
			"expected_revision":1
		}]}`),
		session,
	)
	if explicitOff.Code != http.StatusOK {
		t.Fatalf("explicit Off = %d %s", explicitOff.Code, explicitOff.Body.String())
	}
	if harness.pendingCI.wakes != wakesBefore+1 {
		t.Fatalf("quiet-period update wakes = %d, want %d", harness.pendingCI.wakes, wakesBefore+1)
	}
	lease, err = harness.store.LeaseDue(t.Context(), harness.now, harness.now.Add(time.Minute))
	if err != nil || lease.Request == nil || lease.Request.ID != armed.Request.ID {
		t.Fatalf("retuned quiet-period request = %#v, %v", lease, err)
	}

	omitted := harness.request(
		t,
		http.MethodPut,
		settingsPath,
		strings.NewReader(`{"repositories":[{"repository_id":"repository-20",
			"config_patch":{},"ignore_repository_file":false,"expected_revision":2}]}`),
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
	if detail.PendingCIModeOverride == nil ||
		*detail.PendingCIModeOverride != storage.PendingCIModeLabels ||
		detail.PendingCIBranchPatternsOverride == nil ||
		detail.PendingCIQuietPeriodSecondsOverride == nil ||
		*detail.PendingCIQuietPeriodSecondsOverride != 0 || detail.PendingCIGate == nil ||
		detail.PendingCIGate.DesiredMode != storage.PendingCIModeLabels {
		t.Fatalf("pending CI repository overrides = %#v", detail)
	}

	inherited := harness.request(
		t,
		http.MethodPut,
		settingsPath,
		strings.NewReader(`{"repositories":[{"repository_id":"repository-20",
			"enabled_override":null,
			"pending_ci_mode_override":null,
			"pending_ci_branch_patterns_override":null,
			"pending_ci_quiet_period_seconds_override":null,
			"path_index_interval_seconds_override":null,
			"config_patch":{},
			"ignore_repository_file":false,
			"expected_revision":2
		}]}`),
		session,
	)
	if inherited.Code != http.StatusOK {
		t.Fatalf("explicit Default = %d %s", inherited.Code, inherited.Body.String())
	}
	current := harness.request(
		t, http.MethodGet,
		"/panel/api/v1/targets/github:installation:10/repositories/repository-20",
		nil, session,
	)
	if current.Code != http.StatusOK {
		t.Fatalf("repository after explicit Default = %d %s", current.Code, current.Body.String())
	}
	if err := json.Unmarshal(current.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Repository.EnabledOverride != nil || detail.PendingCIModeOverride != nil ||
		detail.PendingCIBranchPatternsOverride != nil ||
		detail.PendingCIQuietPeriodSecondsOverride != nil || detail.Revision != 3 {
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
			`{"target":{"repository_default_enabled":true,"pending_ci_mode_default":"labels",
			"pending_ci_branch_patterns_default":{"include":["~DEFAULT_BRANCH"],"exclude":[]},
			"pending_ci_quiet_period_seconds_override":null,
			"path_index_interval_seconds_override":null,
			"config_patch":{"quiet_success":%t},"expected_revision":%d}}`,
			revision == 1,
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
		targetPath+"/audit?scope=account&q=installation+settings&sort=oldest&limit=1",
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
		targetPath+"/audit?scope=account&q=installation+settings&sort=oldest&limit=1&cursor="+
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
		Ownership: storage.OwnershipSnapshot{
			Source: storage.OwnershipSourceOrganizationAdmin,
			Status: storage.OwnershipStatusFresh,
			Owners: []storage.Account{{
				ID: "github:test:user:1", Provider: "github:test", SubjectID: "1",
				Login: "owner", DisplayName: "Panel Owner", UpdatedAt: harness.now.Add(time.Minute),
			}},
			SyncedAt: harness.now.Add(time.Minute),
		},
		SyncedAt: harness.now.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	enabled := true
	if _, err := harness.store.SaveInstallationSettings(t.Context(), storage.SaveInstallationSettingsRequest{
		TargetID: targetID, ActorAccountID: target.Account.ID,
		ChangedAt: harness.now.Add(2 * time.Minute),
		Repositories: []storage.InstallationRepositorySettingsChange{{
			RepositoryID: "repository-22", EnabledOverride: &enabled,
			ConfigPatch:          config.Patch{QuietSuccess: &enabled},
			IgnoreRepositoryFile: false, ExpectedRevision: 1,
		}},
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
	nonOwner := newPanelHarnessForSubject(t, "someone-else", "99")
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
		"/panel/inbox",
		"/panel/invite/abcdefghijklmnopqrstuvwxyzABCDEFGH_01234567",
		"/panel/i/smykla-skalski/repositories",
		"/panel/i/smykla-skalski/access/users",
		"/panel/i/smykla-skalski/access/invitations",
		"/panel/i/smykla-skalski/history/audit",
		"/panel/i/smykla-skalski/history/failures",
		"/panel/i/auth/defaults",
		"/panel/root",
		"/panel/root/installations",
		"/panel/root/access",
		"/panel/root/history",
		// The three the queue shipped with, and that a reload answered 404 for.
		"/panel/root/queue",
		"/panel/root/queue/recent",
		"/panel/root/queue/request/pending-ci-1",
		"/panel/root/installations/smykla-skalski/repositories",
		"/panel/root/installations/smykla-skalski/history/audit",
		"/panel/root/installations/smykla-skalski/history/failures",
		"/panel/root/access/users",
		"/panel/root/access/invitations",
		"/panel/root/history/audit",
		"/panel/root/history/failures",
		"/panel/root/runtime",
		"/panel/root/runtime/service",
		"/panel/root/runtime/database",
		"/panel/root/runtime/settings",
		// Every dialog the panel gives an address to. A link to one, and a reload
		// of one, has to answer with the shell rather than the not-found page.
		"/panel/i/smykla-skalski/repositories/api-gateway",
		"/panel/i/smykla-skalski/repositories/api-gateway/behavior",
		"/panel/i/smykla-skalski/access/users/add",
		"/panel/i/smykla-skalski/access/users/octocat/history",
		"/panel/i/smykla-skalski/access/users/octocat/remove-access",
		"/panel/i/smykla-skalski/access/invitations/inv-1/revoke",
		"/panel/root/access/users/octocat/ban",
		"/panel/root/access/invitations/new",
		"/panel/root/access/invitations/inv-1/reissue",
		"/panel/root/installations/smykla-skalski/repositories/api-gateway/file",
		"/panel/root/installations/smykla-skalski/access/users/octocat/history",
		// A trailing slash is not part of the address; the panel's router reads
		// `/inbox/` as `/inbox`, and the server has to agree.
		"/panel/inbox/",
		"/panel/i/smykla-skalski/history/audit/",
		"/panel/root/access/users/",
	} {
		response := harness.request(t, http.MethodGet, path, nil, nil)
		body := response.Body.String()
		if response.Code != http.StatusOK ||
			response.Header().Get("Cache-Control") != "private, no-cache" ||
			response.Header().Get("Content-Security-Policy") != "frame-ancestors 'none'" ||
			response.Header().Get("ETag") == "" ||
			!strings.Contains(body, `content="/panel"`) ||
			!strings.Contains(body, `href="/panel/smyklot-avatar.png?v=1.0.0"`) ||
			strings.Contains(body, basePathSentinel) {
			t.Fatalf("index %s = %d %s", path, response.Code, response.Body.String())
		}
	}
	index := harness.request(t, http.MethodGet, "/panel/", nil, nil)
	conditionalRequest := httptest.NewRequest(http.MethodGet, "/panel/", nil)
	conditionalRequest.Header.Set("If-None-Match", index.Header().Get("ETag"))
	conditional := httptest.NewRecorder()
	harness.handler.ServeHTTP(conditional, conditionalRequest)
	if conditional.Code != http.StatusNotModified || conditional.Body.Len() != 0 {
		t.Fatalf("conditional index = %d %s", conditional.Code, conditional.Body.String())
	}
	asset := harness.request(t, http.MethodGet, "/panel/_app/app.js", nil, nil)
	if asset.Code != http.StatusOK || asset.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" ||
		strings.Contains(asset.Body.String(), basePathSentinel) {
		t.Fatalf("asset response = %d %#v %s", asset.Code, asset.Header(), asset.Body.String())
	}
	themeBoot := harness.request(t, http.MethodGet, "/panel/theme-boot.js", nil, nil)
	if themeBoot.Code != http.StatusOK || themeBoot.Header().Get("Cache-Control") != "public, max-age=3600" ||
		!strings.Contains(themeBoot.Body.String(), "document.documentElement.dataset.theme") ||
		strings.Contains(themeBoot.Body.String(), basePathSentinel) {
		t.Fatalf("theme boot response = %d %#v %s", themeBoot.Code, themeBoot.Header(), themeBoot.Body.String())
	}
	worker := harness.request(t, http.MethodGet, "/panel/service-worker.js", nil, nil)
	if worker.Code != http.StatusOK || worker.Header().Get("Cache-Control") != "no-cache" ||
		strings.Contains(worker.Body.String(), versionSentinel) {
		t.Fatalf("service worker response = %d %#v %s", worker.Code, worker.Header(), worker.Body.String())
	}
	for _, path := range []string{
		"/panel/users",
		"/panel/invitations",
		"/panel/help",
		"/panel/inbox/security",
		"/panel/i/smykla-skalski/inbox",
		"/panel/i/smykla-skalski/settings",
		"/panel/i/smykla-skalski/users",
		"/panel/i/smykla-skalski/invitations",
		// A view still has to be a view, and a dialog is one segment or two.
		"/panel/root/installations/smykla-skalski",
		"/panel/i/smykla-skalski/repositories/api-gateway/file/extra",
		"/panel/root/access/users/octocat/ban/extra",
		"/panel/smykla-skalski/repositories",
		// Nothing follows a view that hosts no dialog, and history takes one of
		// two sections. Both are the route tree's to say, and it says them: there
		// is no route these resolve to, so the refusal is on the wire rather than
		// drawn by the panel after a 200.
		"/panel/i/smykla-skalski/settings/anything",
		"/panel/i/smykla-skalski/sync/anything",
		"/panel/i/smykla-skalski/history/unknown",
		"/panel/root/installations/smykla-skalski/settings/anything",
		"/panel/root/installations/smykla-skalski/history/unknown",
		"/panel/auth/settings",
		"/panel/webhook/history",
		"/panel/i/smykla-skalski/help",
		"/panel/i/smykla-skalski/unknown",
		"/panel/root/unknown",
		"/panel/root/settings",
		"/panel/root/access/owners",
		"/panel/root/history/unknown",
		"/panel/root/runtime/unknown",
		"/panel/root/settings/database",
		"/panel/root/installations/smykla-skalski/settings",
		"/panel/root/installations/smykla-skalski/users/octocat/history",
		"/panel/root/installations/smykla-skalski/unknown",
		"/panel/@smykla-skalski/repositories",
		"/panel/invite/too-short",
		"/panel/invite/abcdefghijklmnopqrstuvwxyzABCDEFGH.01234567",
		"/panel/invite/abcdefghijklmnopqrstuvwxyzABCDEFGH_01234567/extra",
		"/panel/_app/missing.js",
	} {
		response := harness.request(t, http.MethodGet, path, nil, nil)
		if response.Code != http.StatusNotFound {
			t.Fatalf("unknown panel route %s = %d %s", path, response.Code, response.Body.String())
		}
	}
}

var _ fs.FS = fstest.MapFS{}

// A refusal is durable and never expires, so this endpoint is the only way back
// from one. If it does not work, "declined" is a state only a database edit can
// leave.
func TestConfigMigrationResetPutsItBackOnTheTable(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)

	const (
		target     = "github:installation:10"
		repository = "repository-20"
		path       = "/panel/api/v1/targets/" + target +
			"/repositories/" + repository + "/config-migration"
	)

	proposal := 12
	if err := harness.store.SetRepositoryConfigMigration(
		t.Context(),
		storage.RepositoryConfigMigration{
			TargetID:     target,
			RepositoryID: repository,
			State:        storage.ConfigMigrationDeclined,
			PullRequest:  &proposal,
		},
	); err != nil {
		t.Fatalf("seed a refusal: %v", err)
	}

	response := harness.request(t, http.MethodPost, path, strings.NewReader(`{}`), session)
	if response.Code != http.StatusOK {
		t.Fatalf("reset = %d %s", response.Code, response.Body.String())
	}

	var detail repositoryDetailResponse
	if err := json.Unmarshal(response.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.ConfigMigration != storage.ConfigMigrationNone {
		t.Errorf("after reset the state is %q", detail.ConfigMigration)
	}
	if detail.ConfigMigrationPR != nil {
		t.Errorf("after reset the proposal is still #%d", *detail.ConfigMigrationPR)
	}

	// Somebody decided this, so it is written down where decisions are
	audit, err := harness.store.ListAudit(t.Context(), target, storage.AuditPageRequest{
		HistoryPageRequest: storage.HistoryPageRequest{Limit: 50},
		Scope:              storage.AuditAll,
	})
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}

	var recorded bool
	for _, entry := range audit.Items {
		if strings.Contains(entry.Summary, "TOML migration") {
			recorded = true
		}
	}
	if !recorded {
		t.Error("the reset left no audit entry")
	}
}
