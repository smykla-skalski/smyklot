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

// TestEveryBrowserViewIsServedOnReload holds this package's grammar to the
// frontend's list of views.
//
// Three copies of that list exist by necessity - the browser's, the SvelteKit
// param matcher's, and this one - and the matcher already derives from the
// first. This is the third, and it drifted: the sync view was built, routed and
// tested in the browser while a reload of its address answered with the
// not-found page, because nothing held the two lists together.
func TestEveryBrowserViewIsServedOnReload(t *testing.T) {
	installation := browserPanelViews(t, "PANEL_VIEWS")

	for _, view := range installation {
		if !isPanelViewPath(view, nil) {
			t.Errorf("the browser has a %q view and a reload of it is refused", view)
		}
	}

	// The console's own subset, at its own addresses. It renders fewer views
	// than an installation has, and what it does not render must not be served
	// either - an address answered with a shell that says the view is
	// unavailable reads as a fault rather than a boundary.
	console := browserPanelViews(t, "ROOT_INSTALLATION_VIEWS")
	for _, view := range console {
		if !isRootNavigationPath([]string{panelRootPath, panelInstallationsResource, "acme", view}) {
			t.Errorf("the console has a %q view and a reload of it is refused", view)
		}
	}

	for _, view := range installation {
		if slices.Contains(console, view) {
			continue
		}
		if isRootNavigationPath([]string{panelRootPath, panelInstallationsResource, "acme", view}) {
			t.Errorf("the console renders no %q view and its address is served anyway", view)
		}
	}
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
			"frontend/src/routes/i/[account]/[view=panelView]/[...rest]/+page.svelte",
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
			if !strings.Contains(string(source), "view === '"+view+"'") {
				t.Errorf("%s has a %q view and %s renders nothing for it",
					surface.declared, view, surface.page)
			}
		}
	}
}
