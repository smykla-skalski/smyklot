package panel

import (
	"os"
	"regexp"
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
	for _, view := range browserPanelViews(t) {
		if !isPanelViewPath(view, nil) {
			t.Errorf("the browser has a %q view and a reload of it is refused", view)
		}
	}
}

// browserPanelViews reads PANEL_VIEWS out of the frontend source.
//
// Read rather than duplicated, because a copy is the thing being guarded
// against. A parse that finds nothing fails the test rather than passing
// vacuously - a guard that stops seeing its subject is worse than none, because
// it is counted.
func browserPanelViews(t *testing.T) []string {
	t.Helper()

	source, err := os.ReadFile(panelViewsSource)
	if err != nil {
		t.Fatalf("read the browser's view list: %v", err)
	}

	declaration := regexp.MustCompile(`(?s)export const PANEL_VIEWS = \[(.*?)\]`).
		FindSubmatch(source)
	if declaration == nil {
		t.Fatalf("no PANEL_VIEWS declaration in %s", panelViewsSource)
	}

	views := make([]string, 0, 6)
	for _, quoted := range regexp.MustCompile(`'([a-z-]+)'`).
		FindAllStringSubmatch(string(declaration[1]), -1) {
		views = append(views, quoted[1])
	}

	if len(views) == 0 {
		t.Fatalf("PANEL_VIEWS in %s parsed as empty", panelViewsSource)
	}

	return views
}

// TestEveryBrowserViewHasSomethingToRender is the other half, and the one that
// would have caught a whole page nobody could reach.
//
// A component nobody renders passes every test written about it. The sync view
// had an API, a route, a Go authorization matrix entry and its own specs, and no
// page mounted it, so the panel simply had no such page.
func TestEveryBrowserViewHasSomethingToRender(t *testing.T) {
	const page = "frontend/src/routes/i/[account]/[view=panelView]/[...rest]/+page.svelte"

	source, err := os.ReadFile(page)
	if err != nil {
		t.Fatalf("read the view page: %v", err)
	}

	for _, view := range browserPanelViews(t) {
		if !strings.Contains(string(source), "view === '"+view+"'") {
			t.Errorf("the browser has a %q view and no page renders it", view)
		}
	}
}
