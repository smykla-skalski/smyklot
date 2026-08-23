package panel

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// panelViewsSource is where the browser's list of views lives. It is the one
// that decides what the panel renders; this side only decides what a reload of
// an address is answered with, which is why the two have to agree.
const panelViewsSource = "frontend/src/lib/routes.ts"

// TestEveryBrowserViewIsServedOnReload holds what the server answers to the
// frontend's list of views.
//
// There used to be three copies of that list - the browser's, the SvelteKit
// param matcher's, and a grammar written out in Go - and the third drifted: the
// sync view was built, routed and tested in the browser while a reload of its
// address answered with the not-found page. The Go copy is gone now, and the
// server matches the route table the build generates from `src/routes`, so this
// asks the question of that table.
//
// It is still worth asking. The table is only as right as the matchers behind
// it, and those are what carry the difference this checks.
func TestEveryBrowserViewIsServedOnReload(t *testing.T) {
	table := shippedRouteTable(t)
	installation := browserPanelViews(t, "PANEL_VIEWS")

	for _, view := range installation {
		if !table.matches(installationViewPath("/i/acme", view)) {
			t.Errorf("the browser has a %q view and a reload of it is refused", view)
		}
	}

	// The console's own subset, at its own addresses. It renders fewer views
	// than an installation has, and what it does not render must not be served
	// either - an address answered with a shell that says the view is
	// unavailable reads as a fault rather than a boundary.
	console := browserPanelViews(t, "ROOT_INSTALLATION_VIEWS")
	for _, view := range console {
		if !table.matches(installationViewPath("/root/installations/acme", view)) {
			t.Errorf("the console has a %q view and a reload of it is refused", view)
		}
	}

	for _, view := range installation {
		if slices.Contains(console, view) {
			continue
		}
		if table.matches(installationViewPath("/root/installations/acme", view)) {
			t.Errorf("the console renders no %q view and its address is served anyway", view)
		}
	}
}

func installationViewPath(base, view string) string {
	if view == "users" || view == "invitations" {
		return base + "/access/" + view
	}

	return base + "/" + view
}

// browserPanelViews reads a view list out of the frontend source.
//
// Read rather than duplicated, because a copy is the thing being guarded
// against. A parse that finds nothing fails the test rather than passing
// vacuously - a guard that stops seeing its subject is worse than none, because
// it is counted.
func browserPanelViews(t *testing.T, declared string) []string {
	t.Helper()

	source, err := os.ReadFile(panelViewsSource)
	if err != nil {
		t.Fatalf("read the browser's view list: %v", err)
	}

	declaration := regexp.MustCompile(`(?s)export const ` + declared + ` = \[(.*?)\]`).
		FindSubmatch(source)
	if declaration == nil {
		t.Fatalf("no %s declaration in %s", declared, panelViewsSource)
	}

	views := make([]string, 0, 6)
	for _, quoted := range regexp.MustCompile(`'([a-z-]+)'`).
		FindAllStringSubmatch(string(declaration[1]), -1) {
		views = append(views, quoted[1])
	}

	if len(views) == 0 {
		t.Fatalf("%s in %s parsed as empty", declared, panelViewsSource)
	}

	return views
}

// TestEveryBrowserViewHasSomethingToRender is the other half, and the one that
// would have caught a whole page nobody could reach.
//
// A component nobody renders passes every test written about it. The sync view
// had an API, a route, a Go authorization matrix entry and its own specs, and no
// page mounted it, so the panel simply had no such page - twice, because the
// console keeps its own list and its own dispatch and was missed when the first
// half was fixed.
func TestEveryBrowserViewHasSomethingToRender(t *testing.T) {
	for _, surface := range []struct {
		declared string
		page     string
	}{
		{
			"PANEL_VIEWS",
			// The routes are thin now - three of them, one per shape of address -
			// and the switch they all render lives here.
			"frontend/src/lib/components/InstallationView.svelte",
		},
		{
			"ROOT_INSTALLATION_VIEWS",
			"frontend/src/lib/components/RootInstallationView.svelte",
		},
	} {
		source, err := os.ReadFile(surface.page)
		if err != nil {
			t.Fatalf("read %s: %v", surface.page, err)
		}

		for _, view := range browserPanelViews(t, surface.declared) {
			if !rendersView(string(source), view) {
				t.Errorf("%s has a %q view and %s renders nothing for it",
					surface.declared, view, surface.page)
			}
		}
	}
}

// TestRendersViewReadsTheBranchAndItsContents pins the guard itself.
//
// A guard is only worth what it refuses, and this one has been wrong twice
// already: it read the whole file once, and then read a branch without reading
// what was in it.
func TestRendersViewReadsTheBranchAndItsContents(t *testing.T) {
	t.Parallel()

	for _, markup := range []struct {
		name    string
		source  string
		renders bool
	}{
		{"a branch with something in it", "{#if view === 'sync'}\n  <SyncView />\n{/if}\n", true},
		{"a later branch", "{#if view === 'settings'}\n  <A />\n" +
			"{:else if view === 'sync'}\n  <B />\n{/if}\n", true},
		{
			"a branch shared with another view",
			"{:else if view === 'users' || view === 'sync'}\n  <B />\n{/if}\n", true,
		},
		{"a branch with nothing in it", "{#if view === 'sync'}\n{/if}\n", false},
		{
			"a branch holding only a comment",
			"{#if view === 'sync'}\n  <!-- one day -->\n{/if}\n", false,
		},
		{
			"a branch holding a comment that runs on",
			"{#if view === 'sync'}\n  <!-- one\n  day -->\n{/if}\n", false,
		},
		{"the name in a nav row", "  class:active={view === 'sync'}\n", false},
		{"another view's branch", "{#if view === 'settings'}\n  <A />\n{/if}\n", false},
		{
			"a following branch's contents",
			"{#if view === 'sync'}\n{:else if view === 'settings'}\n  <A />\n{/if}\n", false,
		},
	} {
		if rendersView(markup.source, "sync") != markup.renders {
			t.Errorf("%s: renders = %t, wanted %t", markup.name, !markup.renders, markup.renders)
		}
	}
}

// rendersView reports a page whose markup branches on a view and puts something
// in that branch.
//
// The branch, not the name anywhere in the file: a nav row highlights itself by
// comparing the same two things, so a bare search found "invitations" in the
// console's tab styling and would have waved through the deletion of the branch
// that renders it. And the branch's contents, not merely its existence, because
// a branch emptied by a bad merge renders exactly as much as no branch at all.
//
// A condition split across lines fails this rather than passing it. That is the
// safe direction for a guard: a false alarm is read, and a guard that quietly
// stops seeing its subject is not.
func rendersView(source, view string) bool {
	var inside, commented bool

	for line := range strings.Lines(source) {
		markup := strings.TrimSpace(line)

		switch {
		case commented:
			commented = !strings.Contains(markup, "-->")

		case strings.HasPrefix(markup, "{#if "), strings.HasPrefix(markup, "{:else if "):
			inside = strings.Contains(markup, "view === '"+view+"'")

		// The branch ends where the next one starts.
		case strings.HasPrefix(markup, "{:else"), strings.HasPrefix(markup, "{/if}"):
			inside = false

		// A comment is not something a reader sees, however many lines it runs
		// to, so it is not what a branch renders.
		case strings.HasPrefix(markup, "<!--"):
			commented = !strings.Contains(markup, "-->")

		case inside && markup != "":
			return true
		}
	}

	return false
}
