package panel

import (
	"encoding/json"
	"html"
	"strings"
	"testing"
	"testing/fstest"
)

// A bundle whose build wrote no route table cannot be served: the server would
// have no way to tell a panel address from a typing mistake, and would have to
// guess the same answer for both. It refuses to start instead.
func TestPanelAssetBundleRequiresARouteManifest(t *testing.T) {
	_, err := newAssetBundle(Config{
		BasePath: "/panel",
		Assets: fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte(
				`<!doctype html><meta content="__smyklot_panel_error__">` +
					`<noscript>__smyklot_panel_noscript__</noscript>`,
			)},
		},
	})
	if err == nil {
		t.Fatal("bundle built without a route manifest")
	}
}

// The manifest is read out of the bundle, never served from it - it describes the
// panel's addresses and is not one of them.
func TestPanelDoesNotServeTheRouteManifest(t *testing.T) {
	bundle, err := newAssetBundle(Config{
		BasePath: "/panel",
		Assets: fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte(
				`<!doctype html><meta content="__smyklot_panel_error__">` +
					`<noscript>__smyklot_panel_noscript__</noscript>`,
			)},
			routeManifestAsset: &fstest.MapFile{Data: testRouteManifest()},
		},
	})
	if err != nil {
		t.Fatalf("newAssetBundle() error = %v", err)
	}
	if _, ok := bundle.files[routeManifestAsset]; ok {
		t.Errorf("%s is served to readers", routeManifestAsset)
	}
	if !bundle.routes.matches("/root") {
		t.Error("the bundle did not read its route manifest")
	}
}

func TestPanelAssetRewriteUsesTheDestinationSyntax(t *testing.T) {
	version := "release-`-${candidate}-\"<&"
	serviceHost := `host-"-<&`
	bundle, err := newAssetBundle(Config{
		BasePath:    "/panel",
		Version:     version,
		ServiceHost: serviceHost,
		Assets: fstest.MapFS{
			"index.html":        &fstest.MapFile{Data: []byte(`<!doctype html><meta content="/__smyklot_panel_base__"><meta content="__smyklot_panel_version__"><meta content="__smyklot_panel_service__"><meta content="__smyklot_panel_error__"><noscript>__smyklot_panel_noscript__</noscript>`)},
			"service-worker.js": &fstest.MapFile{Data: []byte("const version = `__smyklot_panel_version__`; const service = '__smyklot_panel_service__';")},
			"_app/version.json": &fstest.MapFile{Data: []byte(`{"version":"__smyklot_panel_version__"}`)},
			routeManifestAsset:  &fstest.MapFile{Data: testRouteManifest()},
		},
	})
	if err != nil {
		t.Fatalf("newAssetBundle() error = %v", err)
	}

	index := string(bundle.index)
	if !strings.Contains(index, html.EscapeString(version)) ||
		!strings.Contains(index, html.EscapeString(serviceHost)) {
		t.Fatalf("index does not contain HTML-escaped runtime values: %s", index)
	}

	worker := string(bundle.files[serviceWorkerAsset])
	var quotedVersion, quotedService string
	if encoded, marshalErr := json.Marshal(version); marshalErr == nil {
		quotedVersion = string(encoded)
	}
	if encoded, marshalErr := json.Marshal(serviceHost); marshalErr == nil {
		quotedService = string(encoded)
	}
	if !strings.Contains(worker, quotedVersion) || !strings.Contains(worker, quotedService) {
		t.Fatalf("worker does not contain JavaScript string literals: %s", worker)
	}

	var manifest struct {
		Version string `json:"version"`
	}
	if unmarshalErr := json.Unmarshal(bundle.files["_app/version.json"], &manifest); unmarshalErr != nil {
		t.Fatalf("version manifest is invalid JSON: %v", unmarshalErr)
	}
	if manifest.Version != version {
		t.Fatalf("version manifest = %q, want %q", manifest.Version, version)
	}
}

func TestPanelAssetRewriteRefreshesInlineScriptCSPHash(t *testing.T) {
	originalScript := `globalThis.base = "/__smyklot_panel_base__"`
	originalHash := quotedScriptHash(originalScript)
	index := `<!doctype html><meta http-equiv="content-security-policy" content="script-src 'self' ` +
		originalHash + `"><meta content="/__smyklot_panel_base__"><meta content="__smyklot_panel_version__">` +
		`<meta content="__smyklot_panel_service__"><meta content="__smyklot_panel_error__">` +
		`<script>` + originalScript + `</script><noscript>__smyklot_panel_noscript__</noscript>`
	bundle, err := newAssetBundle(Config{
		BasePath: "/panel",
		Version:  "test",
		Assets: fstest.MapFS{
			"index.html":       &fstest.MapFile{Data: []byte(index)},
			routeManifestAsset: &fstest.MapFile{Data: testRouteManifest()},
		},
		ServiceHost: "localhost",
	})
	if err != nil {
		t.Fatalf("newAssetBundle() error = %v", err)
	}

	served := string(bundle.index)
	rewrittenScript := `globalThis.base = "/panel"`
	if strings.Contains(served, originalHash) {
		t.Fatal("served index retained the build-time inline script hash")
	}
	if !strings.Contains(served, quotedScriptHash(rewrittenScript)) {
		t.Fatalf("served index has no hash for rewritten script: %s", served)
	}
}
