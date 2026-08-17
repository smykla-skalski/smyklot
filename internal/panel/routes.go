package panel

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// The route table the panel's own router is built from, generated into the bundle
// by the frontend build and read here.
//
// The server has to answer an address before the application has loaded: a reload
// of a panel page is a request for a document, and the server decides then whether
// it is a page of the panel's or nothing at all. It used to decide that from a
// grammar written out by hand in Go, beside the one in `src/routes`. Two copies of
// a route tree is one too many, and the second one drifted - the queue shipped three
// addresses the panel linked to and rendered, and that a reload answered 404,
// because a route is added in one place and was needed in two.
//
// So the router's own table crosses over. `build/route-manifest.ts` writes it from
// `builder.routes`, which is what SvelteKit builds the client router from, and the
// patterns are the ones it matches URLs with. Adding a route is adding a directory,
// as it should be, and nothing here needs to know it happened.
const routeManifestAsset = "routes.json"

type routeTable struct {
	routes []panelRoute
}

type panelRoute struct {
	id      string
	pattern *regexp.Regexp
	// One entry per capturing group, positionally - the pairing the manifest proves
	// at build time. A nil entry is a parameter that takes any single segment.
	matchers []*regexp.Regexp
}

type routeManifestDocument struct {
	Version int `json:"version"`
	Routes  []struct {
		ID      string `json:"id"`
		Pattern string `json:"pattern"`
		Params  []struct {
			Name    string `json:"name"`
			Matcher string `json:"matcher"`
		} `json:"params"`
	} `json:"routes"`
}

// The manifest version this server knows how to read. A bundle written by a
// different shape of generator is refused rather than half-understood.
const routeManifestVersion = 1

func loadRouteTable(document []byte) (*routeTable, error) {
	var parsed routeManifestDocument
	if err := json.Unmarshal(document, &parsed); err != nil {
		return nil, fmt.Errorf("parse panel route manifest: %w", err)
	}
	if parsed.Version != routeManifestVersion {
		return nil, fmt.Errorf(
			"panel route manifest is version %d, want %d",
			parsed.Version,
			routeManifestVersion,
		)
	}
	if len(parsed.Routes) == 0 {
		return nil, fmt.Errorf("panel route manifest names no routes")
	}

	table := &routeTable{routes: make([]panelRoute, 0, len(parsed.Routes))}
	for _, entry := range parsed.Routes {
		pattern, err := regexp.Compile(entry.Pattern)
		if err != nil {
			return nil, fmt.Errorf("compile panel route %s: %w", entry.ID, err)
		}
		// The manifest pairs parameters to capturing groups by position and proves
		// the arity when it is written. Prove it again on the way in: a mismatch here
		// would check a matcher against the wrong segment, which reads as a route
		// quietly refusing addresses it should serve.
		if groups := pattern.NumSubexp(); groups != len(entry.Params) {
			return nil, fmt.Errorf(
				"panel route %s has %d parameters and %d capturing groups",
				entry.ID, len(entry.Params), groups,
			)
		}
		matchers := make([]*regexp.Regexp, len(entry.Params))
		for index, param := range entry.Params {
			if param.Matcher == "" {
				continue
			}
			matcher, err := regexp.Compile(param.Matcher)
			if err != nil {
				return nil, fmt.Errorf(
					"compile matcher for %s in panel route %s: %w", param.Name, entry.ID, err,
				)
			}
			matchers[index] = matcher
		}
		table.routes = append(
			table.routes,
			panelRoute{id: entry.ID, pattern: pattern, matchers: matchers},
		)
	}

	return table, nil
}

// matches reports whether the panel's router has a page at this path.
//
// The path is absolute and base-relative, the form the generated patterns are
// written against.
func (t *routeTable) matches(path string) bool {
	for _, route := range t.routes {
		if route.accepts(path) {
			return true
		}
	}

	return false
}

func (r panelRoute) accepts(path string) bool {
	found := r.pattern.FindStringSubmatchIndex(path)
	if found == nil {
		return false
	}
	for index, matcher := range r.matchers {
		if matcher == nil {
			continue
		}
		start, end := found[2*(index+1)], found[2*(index+1)+1]
		// A group that did not participate is an optional or rest parameter the
		// address left out, which is the parameter being absent rather than empty.
		// Its matcher has nothing to judge - and would reject it, since a matcher
		// describes a value that is there. `/root/history` is the everyday case.
		if start == -1 {
			continue
		}
		if !matcher.MatchString(path[start:end]) {
			return false
		}
	}

	return true
}
