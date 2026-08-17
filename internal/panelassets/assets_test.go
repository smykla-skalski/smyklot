package panelassets_test

import (
	"bytes"
	"io/fs"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/panelassets"
)

// The SvelteKit build bakes the base-path sentinel into index.html and JS
// chunks. The Go server resolves them at startup, so the bundle the server
// embeds must carry the sentinel in index.html at minimum.
func TestBundleCarriesBaseSentinel(t *testing.T) {
	assets, err := panelassets.Open()
	if err != nil {
		t.Fatal(err)
	}

	sentinel := []byte("__smyklot_panel_base__")
	sawIndex := false
	err = fs.WalkDir(assets, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		if path == "index.html" {
			sawIndex = true
			content, err := fs.ReadFile(assets, path)
			if err != nil {
				return err
			}
			if !bytes.Contains(content, sentinel) {
				t.Errorf("index.html lost the base sentinel the server rewrites")
			}
			return nil
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sawIndex {
		t.Fatal("bundle has no index.html")
	}
}

// The service worker names its cache after the deployment version, and that is the
// only thing that makes it rotate: on a new release the name changes, `activate`
// finds the old keys and drops all but the previous one.
//
// So the built worker has to carry the version sentinel for the server to rewrite.
// It stopped once already and nothing said so. SvelteKit 3 builds the worker with
// `consumer: 'client'`, which resolves `$app/env` to its browser branch, and that
// reads a payload only the page bootstrap fills - so `version` was `undefined`, every
// release shared one cache name, and the store grew until the browser evicted the
// origin and the offline shell with it. A unit test mocked the module and passed
// throughout, because the mock supplied the version the bundle did not have.
func TestServiceWorkerCarriesVersionSentinel(t *testing.T) {
	assets, err := panelassets.Open()
	if err != nil {
		t.Fatal(err)
	}

	worker, err := fs.ReadFile(assets, "service-worker.js")
	if err != nil {
		t.Fatal(err)
	}

	sentinel := []byte("__smyklot_panel_version__")
	if !bytes.Contains(worker, sentinel) {
		t.Fatalf("service-worker.js carries no %q, so every release would share a cache name", sentinel)
	}
	if bytes.Contains(worker, []byte(":${undefined}")) || bytes.Contains(worker, []byte(":undefined`")) {
		t.Error("service-worker.js builds its cache name from an undefined version")
	}
}

func TestBundleCarriesExecutableStaticCSP(t *testing.T) {
	assets, err := panelassets.Open()
	if err != nil {
		t.Fatal(err)
	}

	index, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range [][]byte{
		[]byte(`<meta http-equiv="content-security-policy"`),
		[]byte(`script-src 'self' 'sha256-`),
		[]byte(`style-src-attr 'unsafe-inline'`),
		[]byte(`src="/__smyklot_panel_base__/theme-boot.js`),
	} {
		if !bytes.Contains(index, required) {
			t.Errorf("index.html does not contain %q", required)
		}
	}

	themeBoot, err := fs.ReadFile(assets, "theme-boot.js")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(themeBoot, []byte("document.documentElement.dataset.theme")) {
		t.Error("theme-boot.js does not set the document theme")
	}
}
