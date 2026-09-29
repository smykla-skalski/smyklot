// Package catalog resolves versioned shared-file profiles from a GitHub repository.
package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"slices"
	"strings"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
)

const maxDocumentBytes = 1 << 20

type Reader interface {
	GetFileContent(context.Context, string, string, string, string, int) ([]byte, error)
}

// Source identifies one immutable catalog and the profiles to render.
type Source = orgsync.CatalogSource

type entry struct {
	Source string `json:"source"`
	Path   string `json:"path"`
}

type profile struct {
	Files []entry `json:"files"`
}

type document struct {
	Version  int                `json:"version"`
	Profiles map[string]profile `json:"profiles"`
}

// Resolve reads only the named commit, then validates the complete file set.
func Resolve(ctx context.Context, reader Reader, source Source) (orgsync.FileConfig, error) {
	if err := source.Validate(); err != nil {
		return orgsync.FileConfig{}, err
	}
	body, err := read(ctx, reader, source, source.Path)
	if err != nil {
		return orgsync.FileConfig{}, fmt.Errorf("read catalog: %w", err)
	}
	var catalog document
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&catalog); err != nil {
		return orgsync.FileConfig{}, fmt.Errorf("decode catalog: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return orgsync.FileConfig{}, fmt.Errorf("catalog contains trailing data")
	}
	if catalog.Version != 1 {
		return orgsync.FileConfig{}, fmt.Errorf("unsupported catalog version %d", catalog.Version)
	}

	files := orgsync.FileConfig{}
	for _, name := range source.Profiles {
		selected, ok := catalog.Profiles[name]
		if !ok {
			return orgsync.FileConfig{}, fmt.Errorf("catalog has no profile %q", name)
		}
		imported, err := readProfile(ctx, reader, source, name, selected)
		if err != nil {
			return orgsync.FileConfig{}, err
		}
		files.Files = append(files.Files, imported...)
	}
	if err := files.Validate(); err != nil {
		return orgsync.FileConfig{}, err
	}
	actual := slices.Sorted(slices.Values(files.Paths()))
	expected := slices.Sorted(slices.Values(source.Paths))
	if !slices.Equal(actual, expected) {
		return orgsync.FileConfig{}, fmt.Errorf("catalog paths differ from configured paths")
	}
	return files, nil
}

func readProfile(
	ctx context.Context, reader Reader, source Source, name string, selected profile,
) ([]orgsync.File, error) {
	files := make([]orgsync.File, 0, len(selected.Files))
	for _, entry := range selected.Files {
		if err := validPath(entry.Source); err != nil {
			return nil, fmt.Errorf("profile %q source: %w", name, err)
		}
		if err := validPath(entry.Path); err != nil {
			return nil, fmt.Errorf("profile %q target: %w", name, err)
		}
		content, err := read(ctx, reader, source, entry.Source)
		if err != nil {
			return nil, fmt.Errorf("profile %q file %q: %w", name, entry.Source, err)
		}
		files = append(files, orgsync.File{Path: entry.Path, Content: string(content)})
	}
	return files, nil
}

func read(ctx context.Context, reader Reader, source Source, filePath string) ([]byte, error) {
	content, err := reader.GetFileContent(
		ctx, source.Owner, source.Repo, filePath, source.Commit, maxDocumentBytes,
	)
	if err != nil {
		return nil, err
	}
	if content == nil {
		return nil, fmt.Errorf("%s is missing at %s", filePath, source.Commit)
	}
	return content, nil
}

func validPath(value string) error {
	if value == "" || path.IsAbs(value) || path.Clean(value) != value ||
		strings.Contains(value, "\\") || value == ".git" ||
		strings.HasPrefix(value, ".git/") || strings.ContainsAny(value, "?#%") {
		return fmt.Errorf("invalid path %q", value)
	}
	if slices.Contains(strings.Split(value, "/"), "..") {
		return fmt.Errorf("invalid path %q", value)
	}
	return nil
}
