// Command generate writes the frontend sync-file render contract.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/smykla-skalski/smyklot/internal/panelcontract"
)

var errNoRoot = errors.New("no go.mod found in any parent directory")

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "panelcontract:", err)
		os.Exit(1)
	}
}

func run() error {
	root, err := moduleRoot()
	if err != nil {
		return err
	}
	content, err := panelcontract.RenderTypeScript()
	if err != nil {
		return err
	}
	path := filepath.Join(root, panelcontract.FrontendFile)
	//nolint:gosec // Generated source is checked in and read by every frontend build.
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	fmt.Println("panelcontract: wrote", panelcontract.FrontendFile)
	return nil
}

func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errNoRoot
		}
		dir = parent
	}
}
