// Package panelassets exposes the generated panel bundle as an fs.FS.
package panelassets

import (
	"archive/zip"
	"bytes"
	"embed"
	"fmt"
	"io/fs"
)

const bundlePath = "generated/bundle.zip"

// Embedding the directory rather than the generated file lets package tools
// inspect a clean checkout. Workflows that open the panel depend on the asset
// generation task and embed the ignored archive alongside the directory's
// tracked README.
//
//go:embed generated
var generated embed.FS

// Open returns the immutable frontend bundle generated from Vite's output.
func Open() (fs.FS, error) {
	bundle, err := generated.ReadFile(bundlePath)
	if err != nil {
		return nil, fmt.Errorf("read embedded panel assets: %w", err)
	}

	archive, err := zip.NewReader(bytes.NewReader(bundle), int64(len(bundle)))
	if err != nil {
		return nil, fmt.Errorf("open embedded panel assets: %w", err)
	}

	return archive, nil
}
