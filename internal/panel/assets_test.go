package panel

import (
	"encoding/json"
	"html"
	"strings"
	"testing"
	"testing/fstest"
)

// The grammar the shell is served for, checked at the function rather than
// through a request.
//
// TestPanelServesRewrittenAssetsAndSPAFallback covers what a browser asks for.
// Two cases cannot be reached that way: an address with an empty segment is
// refused by fs.ValidPath before this function is consulted, and the shape of
// the refusal is worth pinning anyway, because this function has to be right on
// its own rather than because of what the caller happens to check first.
func TestPanelNavigationGrammar(t *testing.T) {
	served := []string{
		"inbox",
		"root",
		"root/installations",
		"root/access",
		"root/access/users/octocat/ban",
		"root/access/invitations/new",
		"root/installations/acme/history/audit",
		"root/installations/acme/repositories/api-gateway/file",
		"i/acme/settings",
		"i/acme/history",
		"i/acme/history/failures",
		"i/acme/repositories/api-gateway",
		"i/acme/users/add",
		"i/acme/invitations/inv-1/revoke",
	}
	refused := []string{
		"",
		"inbox/security",
		"root/installations/acme",
		"root/installations//repositories",
		"root/access/owners",
		"root/access/users/octocat/ban/extra",
		"i/acme/settings/anything",
		"i/acme/history/everything",
		"i//repositories",
		"i/acme/repositories//file",
		"i/acme/repositories/api-gateway/file/extra",
		"i/acme/inbox",
	}
	for _, path := range served {
		if !isPanelNavigationPath(path) {
			t.Errorf("panel navigation path %q was refused", path)
		}
	}
	for _, path := range refused {
		if isPanelNavigationPath(path) {
			t.Errorf("panel navigation path %q was served", path)
		}
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
		BasePath:    "/panel",
		Version:     "test",
		Assets:      fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte(index)}},
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
