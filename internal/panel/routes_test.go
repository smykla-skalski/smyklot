package panel

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/panelassets"
)

// The route table the frontend build generated, read out of the shipped bundle.
//
// Fails rather than skips when the frontend has not been built: `mise run test`
// builds it first, and a guard that quietly stops seeing its subject is worse
// than none, because it is counted.
func shippedRouteTable(t *testing.T) *routeTable {
	t.Helper()

	assets, err := panelassets.Open()
	if err != nil {
		t.Fatalf("open the panel bundle (mise run panel:assets:generate): %v", err)
	}
	document, err := fs.ReadFile(assets, routeManifestAsset)
	if err != nil {
		t.Fatalf("the built bundle carries no %s: %v", routeManifestAsset, err)
	}
	table, err := loadRouteTable(document)
	if err != nil {
		t.Fatalf("the shipped route manifest does not load: %v", err)
	}

	return table
}

// A manifest with the shape the generator writes, for the tests that build a
// bundle out of a handful of files and do not care which addresses it serves.
func testRouteManifest() []byte {
	return []byte(`{"version":1,"routes":[
		{"id":"/","pattern":"^\\/$","params":[]},
		{"id":"/root","pattern":"^\\/root\\/?$","params":[]}
	]}`)
}

// The engine, on a manifest written here rather than generated.
//
// These cases are about how a route table reads a path - an absent optional
// parameter, a matcher that refuses, a rest parameter that runs too deep - and
// each of them is a way to answer a real address with the not-found page. The
// routes are invented so that the behaviour under test is visible in the file
// testing it; whether the panel's own routes are right is the test below.
func TestRouteTableMatching(t *testing.T) {
	table, err := loadRouteTable([]byte(`{"version":1,"routes":[
		{"id":"/plain","pattern":"^\\/plain\\/?$","params":[]},
		{"id":"/opt/[[s=sec]]","pattern":"^\\/opt(?:\\/([^/]+))?\\/?$",
		 "params":[{"name":"s","matcher":"^(?:audit|failures)$"}]},
		{"id":"/free/[id]","pattern":"^\\/free\\/([^/]+?)\\/?$",
		 "params":[{"name":"id","matcher":""}]},
		{"id":"/rest/[v=view]/[...r=dialog]","pattern":"^\\/rest\\/([^/]+?)(?:\\/([\\s\\S]*))?\\/?$",
		 "params":[{"name":"v","matcher":"^(?:one|two)$"},
		           {"name":"r","matcher":"^(?:[^/]+(?:/[^/]+)?)?$"}]}
	]}`))
	if err != nil {
		t.Fatalf("loadRouteTable() error = %v", err)
	}

	// "/opt" leaves the optional parameter absent rather than empty, which is the
	// distinction a matcher would otherwise refuse. "/rest/one" does the same for a
	// rest parameter, and is what every view with no dialog open looks like.
	for _, path := range []string{
		"/plain",
		"/plain/",
		"/opt",
		"/opt/audit",
		"/free/anything",
		"/rest/one",
		"/rest/two/a",
		"/rest/two/a/b",
	} {
		if !table.matches(path) {
			t.Errorf("route table refused %q", path)
		}
	}

	// "/opt/nonsense" is the optional parameter present and not one the matcher
	// names; "/free" and "/free/a/b" are one segment too few and too many; and
	// "/rest/one/a/b/c" is a dialog deeper than a dialog goes.
	for _, path := range []string{
		"/plain/extra",
		"/opt/nonsense",
		"/free",
		"/free/a/b",
		"/rest/three",
		"/rest/one/a/b/c",
		"/nowhere",
	} {
		if table.matches(path) {
			t.Errorf("route table served %q", path)
		}
	}
}

// A manifest the server cannot trust is a server that cannot tell a panel address
// from a typing mistake, so each of these has to stop it starting rather than
// leave it answering both with a guess.
func TestRouteTableRefusesUnusableManifests(t *testing.T) {
	for _, testCase := range []struct{ name, document string }{
		{"not json", `{`},
		{"no routes", `{"version":1,"routes":[]}`},
		{
			"a version this server does not read",
			`{"version":2,"routes":[{"id":"/","pattern":"^/$","params":[]}]}`,
		},
		{
			"a pattern RE2 cannot compile",
			`{"version":1,"routes":[{"id":"/x","pattern":"^/x[^]$","params":[]}]}`,
		},
		{
			"a matcher RE2 cannot compile",
			`{"version":1,"routes":[{"id":"/x/[a]","pattern":"^/x/([^/]+?)$",` +
				`"params":[{"name":"a","matcher":"(?=x)"}]}]}`,
		},
		{
			"parameters that do not pair with the groups",
			`{"version":1,"routes":[{"id":"/x/[a]","pattern":"^/x/([^/]+?)/([^/]+?)$",` +
				`"params":[{"name":"a","matcher":""}]}]}`,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := loadRouteTable([]byte(testCase.document)); err == nil {
				t.Fatal("loadRouteTable() accepted a manifest it cannot route with")
			}
		})
	}
}

// The panel's real addresses, against the manifest the frontend build generated.
//
// This is the test the queue needed and did not have. Every address here is one
// the panel links to or writes into the address bar, and a reload of any of them
// used to depend on somebody having added the same route twice - once to
// `src/routes`, once to a grammar in Go. The second copy is gone; this proves the
// generated one answers for it.
//
// It reads the shipped bundle rather than a fixture, so it fails rather than
// skips when the frontend has not been built. `mise run test` builds it first.
func TestPanelRoutesServeEveryPanelAddress(t *testing.T) {
	table := shippedRouteTable(t)
	token := strings.Repeat("a", 43)
	for _, path := range []string{
		"/inbox",
		"/root",
		"/root/runtime",
		"/root/runtime/service",
		"/root/runtime/database",
		"/root/runtime/settings",
		"/root/queue",
		"/root/queue/request/pending-ci-1",
		"/root/history",
		"/root/history/audit",
		"/root/history/failures",
		"/root/workspaces",
		"/root/access",
		"/root/access/users",
		"/root/access/users/octocat/ban",
		"/root/access/invitations/new",
		"/root/workspaces/acme/history/audit",
		"/root/workspaces/acme/repositories/api-gateway",
		"/workspace/acme/settings",
		"/workspace/acme/history",
		"/workspace/acme/history/failures",
		"/workspace/acme/repositories/api-gateway",
		// Sync is six sections and two of them name one of their own. A file's
		// path keeps its separators, which is why that one is a rest parameter:
		// the server matches the decoded address, so a path travelling as
		// `%2F` would arrive here as a path this table cannot find.
		"/workspace/acme/sync",
		"/workspace/acme/sync/labels",
		"/workspace/acme/sync/plan",
		"/workspace/acme/sync/rulesets",
		"/workspace/acme/sync/rulesets/main-branch-protection",
		"/workspace/acme/sync/files",
		"/workspace/acme/sync/files/.github/workflows/test.yaml",
		"/workspace/acme/access/users/add",
		"/workspace/acme/access/invitations/inv-1/revoke",
		"/invite/" + token,
	} {
		if !table.matches(path) {
			t.Errorf("the panel serves %q and the route manifest refuses it", path)
		}
	}

	for _, path := range []string{
		"/inbox/security",
		"/root/queue/request",
		"/root/queue/nonsense",
		"/root/history/nonsense",
		"/root/runtime/unknown",
		"/root/settings",
		"/root/settings/database",
		"/root/workspaces/acme",
		"/root/workspaces//repositories",
		"/root/access/owners",
		"/root/access/users/octocat/ban/extra",
		"/workspace//repositories",
		// A repository is the whole address, and the whole page is on it. The
		// five pane addresses it used to have are gone rather than redirected,
		// so a shared link says so on the wire instead of opening the page and
		// pretending it meant what it says.
		"/workspace/acme/repositories/api-gateway/behavior",
		"/workspace/acme/repositories/api-gateway/sync",
		"/root/workspaces/acme/repositories/api-gateway/file",
		"/workspace/acme/repositories/api-gateway/file/extra",
		"/workspace/acme/inbox",
		"/workspace/acme/defaults",
		"/workspace/acme/users/add",
		"/workspace/acme/invitations/inv-1/revoke",
		// A view that hosts no dialog has no route with anything after it, and
		// history's section has to be one of the two there are.
		"/workspace/acme/settings/anything",
		"/workspace/acme/sync/anything",
		// The overview is the bare address, so writing it is an address that
		// says the same thing twice - and a ruleset is one name, never a path.
		"/workspace/acme/sync/overview",
		"/workspace/acme/sync/rulesets/main/extra",
		"/workspace/acme/history/everything",
		"/root/workspaces/acme/settings/anything",
		"/root/workspaces/acme/defaults",
		"/root/workspaces/acme/users/octocat/history",
		"/root/workspaces/acme/history/everything",
		"/invite/" + strings.Repeat("a", 42),
		"/invite/" + strings.Repeat("!", 43),
		"/nowhere",
	} {
		if table.matches(path) {
			t.Errorf("the route manifest serves %q, which is not a panel address", path)
		}
	}
}
