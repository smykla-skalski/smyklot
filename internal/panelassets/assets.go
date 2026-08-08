// Package panelassets exposes the generated panel bundle as an fs.FS.
package panelassets

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"fmt"
	"io/fs"
)

//go:embed bundle.zip
var bundle []byte

// Open returns the immutable frontend bundle generated from Vite's output.
func Open() (fs.FS, error) {
	archive, err := zip.NewReader(bytes.NewReader(bundle), int64(len(bundle)))
	if err != nil {
		return nil, fmt.Errorf("open embedded panel assets: %w", err)
	}

	return archive, nil
}
