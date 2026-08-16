// Command generate writes everything derived from config.Patch.
//
// It is wired to `mise run generate` and, through a go:generate directive, to
// `go generate ./...`. Both produce the same bytes, because the generator reads
// only the source and writes only what it renders.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/smykla-skalski/smyklot/internal/configgen"
)

// errNoRoot reports a working directory outside the module.
var errNoRoot = errors.New("no go.mod found in any parent directory")

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "configgen:", err)
		os.Exit(1)
	}
}

func run() error {
	root, err := moduleRoot()
	if err != nil {
		return err
	}

	model, err := configgen.Parse(filepath.Join(root, configgen.PackageDir))
	if err != nil {
		return err
	}

	source, err := configgen.RenderGo(model)
	if err != nil {
		return err
	}

	schema, err := configgen.RenderSchema(model)
	if err != nil {
		return err
	}

	for path, content := range map[string][]byte{
		configgen.GoFile:     source,
		configgen.SchemaFile: schema,
	} {
		//nolint:gosec // Generated files are checked in and read by every build.
		if err := os.WriteFile(filepath.Join(root, path), content, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}

		fmt.Println("configgen: wrote", path)
	}

	return nil
}

// moduleRoot walks up from the working directory to the module it is in, so the
// command produces the same files whether it is run from the repository root,
// from a mise task, or from `go generate` in the package being generated.
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("working directory: %w", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errNoRoot
		}

		dir = parent
	}
}
