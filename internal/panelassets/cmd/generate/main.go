// Command generate packages Vite's output into the archive embedded by Go.
package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	sourceDirectory = "internal/panel/frontend/dist"
	outputPath      = "internal/panelassets/bundle.zip"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	root, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("read repository directory: %w", err)
	}
	source := filepath.Join(root, sourceDirectory)
	output := filepath.Join(root, outputPath)
	if !strings.HasPrefix(source, root+string(filepath.Separator)) ||
		!strings.HasPrefix(output, root+string(filepath.Separator)) {
		return errors.New("panel asset paths escaped the repository")
	}
	paths, err := sourceFiles(source)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return errors.New("panel frontend build produced no assets")
	}
	temporary, err := os.CreateTemp(filepath.Dir(output), ".bundle-*.zip")
	if err != nil {
		return fmt.Errorf("create panel asset archive: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := writeArchive(temporary, source, paths); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close panel asset archive: %w", err)
	}
	if err := os.Chmod(temporaryPath, 0o644); err != nil { //nolint:gosec // Generated bundle is a public build artifact.
		return fmt.Errorf("set panel asset archive permissions: %w", err)
	}
	if err := os.Rename(temporaryPath, output); err != nil {
		return fmt.Errorf("activate panel asset archive: %w", err)
	}

	return nil
}

func sourceFiles(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("panel asset must not be a symbolic link: %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("panel asset is not a regular file: %s", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(relative))

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk panel frontend build: %w", err)
	}
	sort.Strings(paths)

	return paths, nil
}

func writeArchive(target io.Writer, root string, paths []string) error {
	archive := zip.NewWriter(target)
	for _, path := range paths {
		content, err := os.ReadFile( //nolint:gosec // path came from WalkDir under root and passed fs.ValidPath.
			filepath.Join(root, filepath.FromSlash(path)),
		)
		if err != nil {
			_ = archive.Close()
			return fmt.Errorf("read panel asset %s: %w", path, err)
		}
		header := &zip.FileHeader{Name: path, Method: zip.Deflate}
		header.SetMode(0o644)
		header.Modified = time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)
		writer, err := archive.CreateHeader(header)
		if err != nil {
			_ = archive.Close()
			return fmt.Errorf("create panel asset %s: %w", path, err)
		}
		if _, err := writer.Write(content); err != nil {
			_ = archive.Close()
			return fmt.Errorf("write panel asset %s: %w", path, err)
		}
	}
	if err := archive.Close(); err != nil {
		return fmt.Errorf("finish panel asset archive: %w", err)
	}

	return nil
}
