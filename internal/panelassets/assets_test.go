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
