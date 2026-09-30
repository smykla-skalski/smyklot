package catalog_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/orgsync/catalog"
)

const commit = "0123456789abcdef0123456789abcdef01234567"

type reader map[string]string

func (files reader) GetFileContent(
	_ context.Context, owner, repo, filePath, ref string, limit int,
) ([]byte, error) {
	if owner != "smykla-skalski" || repo != ".github" || ref != commit || limit <= 0 {
		return nil, fmt.Errorf("unexpected source: %s/%s@%s", owner, repo, ref)
	}
	if value, ok := files[filePath]; ok {
		return []byte(value), nil
	}
	return nil, nil
}

func source() catalog.Source {
	return catalog.Source{
		Owner: "smykla-skalski", Repo: ".github", Commit: commit,
		Path: "sync/catalog.json", Profiles: []string{"base", "typescript"},
		Paths: []string{"README.md", ".oxlintrc.json"},
	}
}

func TestResolveProfiles(t *testing.T) {
	files := reader{
		"sync/catalog.json": `{"version":1,"profiles":{"base":{"files":[{"source":"sync/base.md","path":"README.md"}]},"typescript":{"files":[{"source":"sync/ts.json","path":".oxlintrc.json"}]}}}`,
		"sync/base.md":      "Shared documentation\n",
		"sync/ts.json":      "{}\n",
	}
	got, err := catalog.Resolve(context.Background(), files, source())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Files) != 2 || got.Files[0].Path != "README.md" ||
		got.Files[1].Content != "{}\n" {
		t.Fatalf("unexpected resolved files: %#v", got.Files)
	}
}

func TestResolveRequiredPathNeedsRequiredProfile(t *testing.T) {
	selected := source()
	selected.RequiredProfiles = []string{"base"}
	selected.RequiredPaths = []string{".oxlintrc.json"}
	files := reader{
		"sync/catalog.json": `{"version":1,"profiles":{"base":{"files":[{"source":"sync/base.md","path":"README.md"}]},"typescript":{"files":[{"source":"sync/ts.json","path":".oxlintrc.json"}]}}}`,
		"sync/base.md":      "Shared documentation\n",
		"sync/ts.json":      "{}\n",
	}
	if _, err := catalog.Resolve(context.Background(), files, selected); err == nil ||
		!strings.Contains(err.Error(), "not in a required profile") {
		t.Fatalf("error = %v", err)
	}
	selected.RequiredPaths = []string{"README.md"}
	if _, err := catalog.Resolve(context.Background(), files, selected); err != nil {
		t.Fatalf("required path was rejected: %v", err)
	}
}

func TestResolveRefusesMissingAndConflictingFiles(t *testing.T) {
	cases := []struct {
		name    string
		files   reader
		wantErr string
	}{
		{
			name:    "missing source",
			files:   reader{"sync/catalog.json": `{"version":1,"profiles":{"base":{"files":[{"source":"sync/missing.md","path":"README.md"}]},"typescript":{"files":[]}}}`},
			wantErr: "is missing",
		},
		{
			name: "duplicate target in one profile",
			files: reader{
				"sync/catalog.json": `{"version":1,"profiles":{"base":{"files":[{"source":"sync/one.md","path":"README.md"},{"source":"sync/two.md","path":"README.md"}]},"typescript":{"files":[]}}}`,
				"sync/one.md":       "one", "sync/two.md": "two",
			},
			wantErr: "configured twice",
		},
		{
			name:    "path traversal",
			files:   reader{"sync/catalog.json": `{"version":1,"profiles":{"base":{"files":[{"source":"../secret","path":"README.md"}]},"typescript":{"files":[]}}}`},
			wantErr: "invalid path",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := catalog.Resolve(context.Background(), tc.files, source())
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, wanted %q", err, tc.wantErr)
			}
		})
	}
}

func TestResolveAllowsSharedTargetInSeparateProfiles(t *testing.T) {
	selected := source()
	selected.Paths = []string{"mise.toml"}
	files := reader{
		"sync/catalog.json": `{"version":1,"profiles":{"base":{"files":[{"source":"sync/go.toml","path":"mise.toml"}]},"typescript":{"files":[{"source":"sync/ts.toml","path":"mise.toml"}]}}}`,
		"sync/go.toml":      "[tools]\ngo = \"1.27.1\"\n",
		"sync/ts.toml":      "[tools]\nnode = \"24.21.0\"\n",
	}
	resolved, err := catalog.Resolve(context.Background(), files, selected)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Files) != 2 {
		t.Fatalf("resolved files = %#v", resolved.Files)
	}
}

func TestRequiredProfileRefusesSharedTarget(t *testing.T) {
	selected := source()
	selected.Paths = []string{"mise.toml"}
	selected.RequiredProfiles = []string{"base"}
	selected.RequiredPaths = []string{"mise.toml"}
	files := reader{
		"sync/catalog.json": `{"version":1,"profiles":{"base":{"files":[{"source":"sync/base.toml","path":"mise.toml"}]},"typescript":{"files":[{"source":"sync/ts.toml","path":"mise.toml"}]}}}`,
		"sync/base.toml":    "[tools]\ngo = \"1.27.1\"\n",
		"sync/ts.toml":      "[tools]\nnode = \"24.21.0\"\n",
	}
	if _, err := catalog.Resolve(context.Background(), files, selected); err == nil ||
		!strings.Contains(err.Error(), "overlaps another profile") {
		t.Fatalf("error = %v", err)
	}
}

func TestSourceNeedsImmutableRef(t *testing.T) {
	value := source()
	value.Commit = "main"
	if err := value.Validate(); err == nil {
		t.Fatal("mutable catalog ref was accepted")
	}
}

func TestSourceRequiresProfileForRequiredPaths(t *testing.T) {
	value := source()
	value.RequiredPaths = []string{"README.md"}
	if err := value.Validate(); err == nil {
		t.Fatal("required path without required profile was accepted")
	}
	value.RequiredProfiles = []string{"base"}
	value.RequiredPaths = []string{"missing.md"}
	if err := value.Validate(); err == nil {
		t.Fatal("required path outside catalog paths was accepted")
	}
}
