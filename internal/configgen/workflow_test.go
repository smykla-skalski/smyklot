package configgen_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// workflowFile is the Test workflow, relative to this package.
const workflowFile = "../../.github/workflows/test.yaml"

// TestEveryTestedPackageRunsInCI fails when a package with tests is in no
// matrix entry of the Test workflow.
//
// The matrix is a hand-written list of directories, and a hand-written list is
// one that comes to disagree with the tree beside it. It did: this package's
// own suite - the drift, completeness and defaults-agree tests that are the
// only thing holding pkg/config/zz_generated.go to patch.go - ran on nobody's
// machine but a developer's, and so did internal/storage/sqlstore's. A green
// pipeline said the generator was current when nothing had asked it.
func TestEveryTestedPackageRunsInCI(t *testing.T) {
	t.Parallel()

	covered := workflowDirectories(t)

	for _, pkg := range testedPackages(t) {
		if !isCovered(pkg, covered) {
			t.Errorf("%s has tests and is in no matrix entry of %s (covered: %s)",
				pkg, workflowFile, strings.Join(covered, " "))
		}
	}
}

// workflowDirectories reads the directories the go job's matrix runs.
func workflowDirectories(t *testing.T) []string {
	t.Helper()

	content, err := os.ReadFile(workflowFile)
	if err != nil {
		t.Fatalf("read %s: %v", workflowFile, err)
	}

	var workflow struct {
		Jobs struct {
			Go struct {
				Strategy struct {
					Matrix struct {
						Include []struct {
							Dirs string `yaml:"dirs"`
						} `yaml:"include"`
					} `yaml:"matrix"`
				} `yaml:"strategy"`
			} `yaml:"go"`
		} `yaml:"jobs"`
	}

	if err := yaml.Unmarshal(content, &workflow); err != nil {
		t.Fatalf("parse %s: %v", workflowFile, err)
	}

	var dirs []string
	for _, entry := range workflow.Jobs.Go.Strategy.Matrix.Include {
		dirs = append(dirs, strings.Fields(entry.Dirs)...)
	}

	if len(dirs) == 0 {
		t.Fatalf("%s names no directories, so this test proves nothing", workflowFile)
	}

	return dirs
}

// testedPackages walks the module for directories holding a Go test.
func testedPackages(t *testing.T) []string {
	t.Helper()

	const root = "../.."

	var packages []string

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			// Vendored trees and the frontend's dependencies are nobody's
			// packages, and the frontend has its own job.
			switch entry.Name() {
			case ".git", "node_modules", "generated", "bin":
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}

		relative, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}

		pkg := "./" + filepath.ToSlash(relative)
		if !contains(packages, pkg) {
			packages = append(packages, pkg)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk the module: %v", err)
	}

	if len(packages) == 0 {
		t.Fatal("found no package with tests, so this test proves nothing")
	}

	return packages
}

// isCovered reports a package some matrix entry recurses into, which is what
// `ginkgo -r` does with each directory it is given.
func isCovered(pkg string, dirs []string) bool {
	for _, dir := range dirs {
		if pkg == dir || strings.HasPrefix(pkg, strings.TrimSuffix(dir, "/")+"/") {
			return true
		}
	}

	return false
}
