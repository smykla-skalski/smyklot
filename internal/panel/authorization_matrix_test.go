package panel

import (
	"net/http"
	"os"
	"regexp"
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
		{http.MethodGet, "/panel/api/v1/root/pending-ci/7"},
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
		{
			http.MethodPost,
			"/panel/api/v1/root/installations/" + target +
				"/repositories/repository-20/config-migration",
		},
		{http.MethodGet, "/panel/api/v1/root/installations/" + target + "/users"},
		{http.MethodPost, "/panel/api/v1/root/installations/" + target + "/users"},
		{http.MethodGet, "/panel/api/v1/root/installations/" + target + "/user-suggestions"},
		{http.MethodPut, "/panel/api/v1/root/installations/" + target + "/users/" + account},
		{http.MethodGet, "/panel/api/v1/root/installations/" + target + "/users/" + account + "/decisions"},
		{http.MethodGet, "/panel/api/v1/root/installations/" + target + "/invitations"},
		{http.MethodPost, "/panel/api/v1/root/installations/" + target + "/invitations"},
		{http.MethodPost, "/panel/api/v1/root/installations/" + target + "/invitations/" + invitation + "/reissue"},
		{http.MethodDelete, "/panel/api/v1/root/installations/" + target + "/invitations/" + invitation},
		{http.MethodGet, "/panel/api/v1/root/installations/" + target + "/audit"},
		{http.MethodGet, "/panel/api/v1/root/installations/" + target + "/sync/config/checkpoints/1"},
		{http.MethodPost, "/panel/api/v1/root/installations/" + target + "/sync/config/checkpoints/1/restore"},
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
	probes := regularRouteProbes("/panel/api/v1/targets/" + installation.TargetID)

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

// routePlaceholders is what a registered pattern's wildcards stand for in a
// probe. A pattern using one that is not here fails the completeness check
// loudly, which is right: a new kind of wildcard is a new decision about who
// may reach it.
var routePlaceholders = map[string]string{
	"{account}":    "github:test:user:ordinary",
	"{invitation}": "invitation-id",
	"{repository}": "repository-30",
	"{plan}":       "sync-plan-1",
	"{kind}":       "labels",
	"{checkpoint}": "1",
}

// regularRouteProbes is every installation-scoped route, with the concrete
// values a request needs. Shared so the matrix and its completeness check
// cannot describe different route sets.
func regularRouteProbes(target string) []authorizationProbe {
	const (
		account    = "github:test:user:ordinary"
		invitation = "invitation-id"
	)

	return []authorizationProbe{
		{http.MethodPut, target + "/settings"},
		{http.MethodGet, target + "/users"},
		{http.MethodPost, target + "/users"},
		{http.MethodGet, target + "/user-suggestions"},
		{http.MethodPut, target + "/users/" + account},
		{http.MethodGet, target + "/users/" + account + "/decisions"},
		{http.MethodGet, target + "/invitations"},
		{http.MethodPost, target + "/invitations"},
		{http.MethodPost, target + "/invitations/" + invitation + "/reissue"},
		{http.MethodDelete, target + "/invitations/" + invitation},
		{http.MethodGet, target + "/repositories"},
		{http.MethodGet, target + "/repositories/repository-30"},
		{http.MethodPut, target + "/repositories/repository-30/settings"},
		{http.MethodPost, target + "/repositories/repository-30/config-migration"},
		{http.MethodGet, target + "/repositories/repository-30/sync/labels"},
		{http.MethodPut, target + "/repositories/repository-30/sync/labels"},
		{http.MethodGet, target + "/sync/paths"},
		{http.MethodGet, target + "/sync/overrides/labels"},
		{http.MethodGet, target + "/sync/config/labels"},
		{http.MethodPut, target + "/sync/config/labels"},
		{http.MethodPut, target + "/sync/config"},
		{http.MethodGet, target + "/sync/config/checkpoints/1"},
		{http.MethodPost, target + "/sync/config/checkpoints/1/restore"},
		{http.MethodGet, target + "/sync/plan"},
		{http.MethodGet, target + "/sync/status"},
		{http.MethodGet, target + "/sync/files/context"},
		{http.MethodPost, target + "/sync/plans/sync-plan-1/approval"},
		{http.MethodDelete, target + "/sync/plans/sync-plan-1"},
		{http.MethodGet, target + "/audit"},
		{http.MethodGet, target + "/failures"},
	}
}

// TestPanelRegularRouteMatrixCoversEveryRoute fails when a route is added and
// not probed.
//
// The matrix above is a hand-written list, and a hand-written list is a thing
// you can forget to add to. Nothing downstream notices: the new route works,
// its specs pass, and the one question nobody asked is whether somebody else's
// installation can reach it. So the list is checked against the routes the
// server actually registers.
func TestPanelRegularRouteMatrixCoversEveryRoute(t *testing.T) {
	registered := registeredTargetRoutes(t)
	if len(registered) == 0 {
		t.Fatal("read no target routes out of server.go")
	}

	const target = "/panel/api/v1/targets/the-installation"

	probed := map[string]bool{}
	for _, probe := range regularRouteProbes(target) {
		probed[probe.method+" "+probe.path] = true
	}

	for _, route := range registered {
		method, pattern, _ := strings.Cut(route, " ")

		concrete := strings.Replace(pattern, "{target}", "the-installation", 1)
		for placeholder, value := range routePlaceholders {
			concrete = strings.ReplaceAll(concrete, placeholder, value)
		}
		if strings.Contains(concrete, "{") {
			t.Errorf("%s uses a wildcard routePlaceholders does not name", route)

			continue
		}

		if !probed[method+" /panel"+concrete] {
			t.Errorf("%s is registered but nothing probes who may reach it", route)
		}
	}
}

// registeredTargetRoutes reads the installation-scoped routes out of the file
// that registers them.
//
// Reading the source rather than the mux, because http.ServeMux does not report
// its patterns and teaching the server to record them would change production
// code to suit a test.
func registeredTargetRoutes(t *testing.T) []string {
	t.Helper()

	source, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}

	pattern := regexp.MustCompile(`"(GET|PUT|POST|DELETE) "\+base\+"(/api/v1/targets/\{target\}[^"]*)"`)

	var routes []string
	for _, match := range pattern.FindAllStringSubmatch(string(source), -1) {
		routes = append(routes, match[1]+" "+match[2])
	}

	return routes
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
