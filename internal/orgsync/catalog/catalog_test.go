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
			name: "duplicate target",
			files: reader{
				"sync/catalog.json": `{"version":1,"profiles":{"base":{"files":[{"source":"sync/one.md","path":"README.md"}]},"typescript":{"files":[{"source":"sync/two.md","path":"README.md"}]}}}`,
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

func TestSourceNeedsImmutableRef(t *testing.T) {
	value := source()
	value.Commit = "main"
	if err := value.Validate(); err == nil {
		t.Fatal("mutable catalog ref was accepted")
	}
}
