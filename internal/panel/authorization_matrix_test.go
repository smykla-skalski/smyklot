package panel

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

type authorizationProbe struct {
	method string
	path   string
}

func TestPanelRootRouteAuthorizationMatrix(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	_ = harness.signIn(t)
	ordinarySession := createOrdinarySession(t, harness)
	const target = "github:installation:10"
	const account = "github:test:user:ordinary"
	const invitation = "invitation-id"

	probes := []authorizationProbe{
		{http.MethodGet, "/panel/api/v1/root/overview"},
		{http.MethodPost, "/panel/api/v1/root/pending-ci/7/check"},
		{http.MethodDelete, "/panel/api/v1/root/pending-ci/7"},
		{http.MethodGet, "/panel/api/v1/root/installations"},
		{http.MethodPost, "/panel/api/v1/root/installations/sync"},
		{http.MethodGet, "/panel/api/v1/root/access/users"},
		{http.MethodGet, "/panel/api/v1/root/access/invitations"},
		{http.MethodPut, "/panel/api/v1/root/access/users/" + account},
		{http.MethodPost, "/panel/api/v1/root/access/invitations"},
		{http.MethodPost, "/panel/api/v1/root/access/invitations/" + invitation + "/reissue"},
		{http.MethodDelete, "/panel/api/v1/root/access/invitations/" + invitation},
		{http.MethodGet, "/panel/api/v1/root/history/audit"},
		{http.MethodGet, "/panel/api/v1/root/history/failures"},
		{http.MethodGet, "/panel/api/v1/root/settings"},
		{http.MethodPut, "/panel/api/v1/root/settings"},
		{http.MethodGet, "/panel/api/v1/root/installations/" + target + "/elevation"},
		{http.MethodPost, "/panel/api/v1/root/installations/" + target + "/elevation"},
		{http.MethodDelete, "/panel/api/v1/root/elevations/elevation-id"},
		{http.MethodGet, "/panel/api/v1/root/installations/" + target + "/settings"},
		{http.MethodPut, "/panel/api/v1/root/installations/" + target + "/settings"},
		{http.MethodGet, "/panel/api/v1/root/installations/" + target + "/repositories"},
		{http.MethodGet, "/panel/api/v1/root/installations/" + target + "/repositories/repository-20"},
		{http.MethodPut, "/panel/api/v1/root/installations/" + target + "/repositories/repository-20/settings"},
		{http.MethodGet, "/panel/api/v1/root/installations/" + target + "/users"},
		{http.MethodPost, "/panel/api/v1/root/installations/" + target + "/users"},
		{http.MethodPut, "/panel/api/v1/root/installations/" + target + "/users/" + account},
		{http.MethodGet, "/panel/api/v1/root/installations/" + target + "/users/" + account + "/decisions"},
		{http.MethodGet, "/panel/api/v1/root/installations/" + target + "/invitations"},
		{http.MethodPost, "/panel/api/v1/root/installations/" + target + "/invitations"},
		{http.MethodPost, "/panel/api/v1/root/installations/" + target + "/invitations/" + invitation + "/reissue"},
		{http.MethodDelete, "/panel/api/v1/root/installations/" + target + "/invitations/" + invitation},
		{http.MethodGet, "/panel/api/v1/root/installations/" + target + "/audit"},
		{http.MethodGet, "/panel/api/v1/root/installations/" + target + "/failures"},
	}

	for _, probe := range probes {
		probe := probe
		t.Run(probe.method+" "+probe.path, func(t *testing.T) {
			response := harness.request(
				t, probe.method, probe.path, strings.NewReader(`{}`), ordinarySession,
			)
			requireResponse(t, response, "ordinary Root route", http.StatusForbidden)
		})
	}
}

func TestPanelRegularRouteRejectsNonOwnedRootMatrix(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	rootSession := harness.signIn(t)
	_, installation := seedNonOwnedInstallation(t, harness)
	target := "/panel/api/v1/targets/" + installation.TargetID
	const account = "github:test:user:ordinary"
	const invitation = "invitation-id"

	probes := []authorizationProbe{
		{http.MethodPut, target + "/settings"},
		{http.MethodGet, target + "/users"},
		{http.MethodPost, target + "/users"},
		{http.MethodPut, target + "/users/" + account},
		{http.MethodGet, target + "/users/" + account + "/decisions"},
		{http.MethodGet, target + "/invitations"},
		{http.MethodPost, target + "/invitations"},
		{http.MethodPost, target + "/invitations/" + invitation + "/reissue"},
		{http.MethodDelete, target + "/invitations/" + invitation},
		{http.MethodGet, target + "/repositories"},
		{http.MethodGet, target + "/repositories/repository-30"},
		{http.MethodPut, target + "/repositories/repository-30/settings"},
		{http.MethodGet, target + "/audit"},
		{http.MethodGet, target + "/failures"},
	}

	for _, probe := range probes {
		probe := probe
		t.Run(probe.method+" "+probe.path, func(t *testing.T) {
			response := harness.request(
				t, probe.method, probe.path, strings.NewReader(`{}`), rootSession,
			)
			requireResponse(t, response, "non-owned regular route", http.StatusNotFound)
		})
	}
}

func createOrdinarySession(t *testing.T, harness *panelHarness) *http.Cookie {
	t.Helper()
	account := storage.Account{
		ID: "github:test:user:ordinary", Provider: "github:test", SubjectID: "ordinary",
		Login: "ordinary", DisplayName: "Ordinary User", UpdatedAt: harness.now,
	}
	if err := harness.store.UpsertAccount(t.Context(), account); err != nil {
		t.Fatal(err)
	}
	if _, err := harness.store.CreatePanelUser(t.Context(), storage.PanelUserCreate{
		AccountID: account.ID, ActorAccountID: "github:test:user:1", ChangedAt: harness.now,
	}); err != nil {
		t.Fatal(err)
	}
	const token = "ordinary-authorization-session"
	if err := harness.store.CreateSession(t.Context(), storage.Session{
		TokenHash: tokenHash(token), AccountID: account.ID,
		CreatedAt: harness.now, ExpiresAt: harness.now.Add(time.Hour),
	}, 1); err != nil {
		t.Fatal(err)
	}

	return &http.Cookie{Name: sessionCookieName, Value: token}
}
