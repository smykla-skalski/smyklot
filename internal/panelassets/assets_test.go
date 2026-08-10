package panelassets_test

import (
	"bytes"
	"io/fs"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/panelassets"
)

// The Go asset handler rewrites the build-time base-path sentinel only in
// index.html, so a sentinel reference in any other bundle file would 404 at
// runtime on every deployment with a non-sentinel mount point.
func TestBundleKeepsBaseSentinelOutOfAssets(t *testing.T) {
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
		content, err := fs.ReadFile(assets, path)
		if err != nil {
			return err
		}
		if path == "index.html" {
			sawIndex = true
			if !bytes.Contains(content, sentinel) {
				t.Errorf("index.html lost the base sentinel the server rewrites")
			}
			return nil
		}
		if bytes.Contains(content, sentinel) {
			t.Errorf("bundle file %s bakes in the base sentinel", path)
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
