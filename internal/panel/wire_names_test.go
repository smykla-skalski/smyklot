package panel

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

// Every name the panel API puts on the wire is lower_snake_case.
//
// It is a convention until something breaks it, and then it is a bug the tests
// cannot see. storage.RepositoryCounts went out as itself: nothing below the port
// needs struct tags, so it marshalled under its Go field names, and the panel read
// `enabled` and `disabled` off an object that spelled them `Enabled` and `Disabled`.
// Adding two undefined numbers is what wrote "of NaN enabled" across the Root
// console's installations table.
//
// Nothing caught it. The types compile either way, the Go tests asserted on the
// struct rather than the JSON, and the development fixture spells these fields the
// way the panel reads them - so the page was correct everywhere except against the
// service. What is checked here is the shape of the wire itself, which is the only
// place the two halves actually meet.
func TestPanelResponsesUseWireNames(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	seedPanelWireNameRows(t, harness)

	paths := panelWireNameProbePaths()
	read := 0
	for _, path := range paths {
		response := harness.request(t, http.MethodGet, path, nil, session)
		if response.Code != http.StatusOK {
			continue
		}
		read++
		var body any
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Errorf("%s answered something that is not JSON: %v", path, err)

			continue
		}
		assertWireNames(t, path, body)
	}

	// A list of addresses that all answered 403 would pass every assertion above
	// while reading nothing at all.
	if read < len(paths)/2 {
		t.Fatalf("only %d of %d addresses answered 200; this proved almost nothing", read, len(paths))
	}
}

// seedPanelWireNameRows gives the sync reads something to answer with.
//
// An empty list carries no field names, so a probe against one asserts that
// nothing was spelled wrong in nothing. `/sync/paths` and
// `/sync/overrides/{kind}` both answer `{"...": []}` on a bare harness, which
// is how four new names - `path`, `repositories`, `repository_id` and
// `repository_name` - sat in this list proving none of themselves.
func seedPanelWireNameRows(t *testing.T, harness *panelHarness) {
	t.Helper()

	const (
		target     = "github:installation:10"
		repository = "repository-30"
	)

	// The repository the probe list names, which `routePlaceholders` shares
	// with the authorization matrix. Without it in the catalog every address
	// below it answers 404 and is skipped, so six probes proved nothing.
	if err := harness.store.ReconcileInstallation(t.Context(), storage.InstallationSnapshot{
		TargetID:       target,
		InstallationID: "10",
		Kind:           storage.TargetOrganization,
		Account: storage.Account{
			ID:          "github:test:account:2",
			Provider:    "github:test",
			SubjectID:   "2",
			Login:       "smykla-skalski",
			DisplayName: "Smykla Skalski",
			UpdatedAt:   harness.now,
		},
		Repositories: []storage.RepositorySnapshot{
			{ID: "repository-20", Name: "smyklot", FullName: "smykla-skalski/smyklot"},
			{ID: repository, Name: "docs", FullName: "smykla-skalski/docs"},
		},
		SyncedAt: harness.now,
	}); err != nil {
		t.Fatal(err)
	}

	if err := harness.store.SetSyncRepositoryPaths(t.Context(), orgsync.RepositoryPaths{
		RepositoryID: repository,
		TargetID:     target,
		Paths:        []string{"renovate.json"},
		ObservedAt:   harness.now,
		HeadSHA:      "abc123",
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := harness.store.SetSyncRepositoryOverride(
		t.Context(), orgsync.RepositoryOverrideChange{
			RepositoryID: repository,
			Kind:         orgsync.KindLabels,
			Document:     []byte(`{}`),
			ActorID:      "github:test:user:1",
			Now:          harness.now,
		},
	); err != nil {
		t.Fatal(err)
	}
}

// The addresses the panel reads from, which is every GET the server registers.
// TestPanelWireNameProbesCoverEveryReadableRoute holds this to that.
func panelWireNameProbePaths() []string {
	const target = "github:installation:10"

	const (
		account    = "github:test:user:ordinary"
		repository = "repository-30"
		token      = "abcdefghijklmnopqrstuvwxyzABCDEFGH_01234567"
	)

	return []string{
		"/panel/api/v1/session",
		"/panel/api/v1/notifications",
		"/panel/api/v1/invites/" + token,
		"/panel/api/v1/targets",
		"/panel/api/v1/targets/" + target,
		"/panel/api/v1/targets/" + target + "/repositories",
		"/panel/api/v1/targets/" + target + "/repositories/" + repository,
		"/panel/api/v1/targets/" + target + "/repositories/" + repository + "/sync/labels",
		"/panel/api/v1/targets/" + target + "/users",
		"/panel/api/v1/targets/" + target + "/users/" + account + "/decisions",
		"/panel/api/v1/targets/" + target + "/user-suggestions",
		"/panel/api/v1/targets/" + target + "/invitations",
		"/panel/api/v1/targets/" + target + "/sync/config",
		"/panel/api/v1/targets/" + target + "/sync/config/labels",
		"/panel/api/v1/targets/" + target + "/sync/overrides/labels",
		"/panel/api/v1/targets/" + target + "/sync/paths",
		"/panel/api/v1/targets/" + target + "/sync/plan",
		"/panel/api/v1/targets/" + target + "/audit",
		"/panel/api/v1/targets/" + target + "/failures",
		"/panel/api/v1/root/overview",
		"/panel/api/v1/root/pending-ci/7",
		"/panel/api/v1/root/installations",
		"/panel/api/v1/root/access/users",
		"/panel/api/v1/root/access/invitations",
		"/panel/api/v1/root/history/audit",
		"/panel/api/v1/root/history/failures",
		"/panel/api/v1/root/settings",
		"/panel/api/v1/root/installations/" + target + "/elevation",
		"/panel/api/v1/root/installations/" + target + "/settings",
		"/panel/api/v1/root/installations/" + target + "/repositories",
		"/panel/api/v1/root/installations/" + target + "/repositories/" + repository,
		"/panel/api/v1/root/installations/" + target + "/users",
		"/panel/api/v1/root/installations/" + target + "/users/" + account + "/decisions",
		"/panel/api/v1/root/installations/" + target + "/user-suggestions",
		"/panel/api/v1/root/installations/" + target + "/invitations",
		"/panel/api/v1/root/installations/" + target + "/audit",
		"/panel/api/v1/root/installations/" + target + "/failures",
	}
}

// The event stream is not JSON and cannot be read by a request that expects a
// body to end, so it is named here rather than left to look like an oversight.
var notJSONRoutes = map[string]bool{"/api/v1/events": true}

// The wildcards the readable routes use that the authorization matrix has no
// reason to name, since it only walks the installation-scoped ones. A section
// wildcard stands for one of its sections; the probe list above asks for both.
var readableRoutePlaceholders = map[string]string{
	"{token}":   "abcdefghijklmnopqrstuvwxyzABCDEFGH_01234567",
	"{request}": "7",
	"{history}": "audit",
	"{access}":  "users",
}

var wireName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func assertWireNames(t *testing.T, where string, value any) {
	t.Helper()

	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if !wireName.MatchString(key) {
				t.Errorf(
					"%s: %q is not a wire name - a Go struct is going out untagged, and the "+
						"panel reads these in lower_snake_case",
					where, key,
				)
			}
			assertWireNames(t, where+"."+key, nested)
		}
	case []any:
		for index, nested := range typed {
			assertWireNames(t, fmt.Sprintf("%s[%d]", where, index), nested)
		}
	}
}

// The list above is hand-written, and a hand-written list is one you can forget to
// add to - the same trap the authorization matrix guards against, and for the same
// reason: the new endpoint works, its own specs pass, and nobody asks what its
// fields are called.
func TestPanelWireNameProbesCoverEveryReadableRoute(t *testing.T) {
	source, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	pattern := regexp.MustCompile(`"GET "\+base\+"(/api/v1/[^"]*)"`)

	probed := map[string]bool{}
	for _, path := range panelWireNameProbePaths() {
		probed[path] = true
	}

	for _, match := range pattern.FindAllStringSubmatch(string(source), -1) {
		if notJSONRoutes[match[1]] {
			continue
		}
		concrete := match[1]
		for _, placeholders := range []map[string]string{routePlaceholders, readableRoutePlaceholders} {
			for placeholder, value := range placeholders {
				concrete = strings.ReplaceAll(concrete, placeholder, value)
			}
		}
		concrete = strings.ReplaceAll(concrete, "{target}", "github:installation:10")
		if strings.Contains(concrete, "{") {
			t.Errorf("GET %s uses a wildcard routePlaceholders does not name", match[1])

			continue
		}
		if !probed["/panel"+concrete] {
			t.Errorf("GET %s is registered but nothing checks what its fields are called", match[1])
		}
	}
}
