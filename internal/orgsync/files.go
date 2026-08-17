package orgsync

import (
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/smykla-skalski/smyklot/internal/orgsync/filemerge"
)

// File is one file every repository is expected to carry, and what it should
// say.
//
// The content is here rather than read from a repository somewhere. The tool
// this replaces kept the templates in one repository and fetched each of them
// per repository per run, which is a request that can fail - and when it did,
// the file was skipped with a warning and the run still reported success.
type File struct {
	// Path is where the file sits, relative to the repository root.
	Path string `json:"path"`

	// Content is the template, before a repository's own adjustments.
	Content string `json:"content"`
}

// FileConfig is the files an installation expects its repositories to carry.
type FileConfig struct {
	Files []File `json:"files"`

	// Retired are paths this organization used to install and now removes.
	//
	// This is the whole of deletion for files, and it is a named list rather
	// than a switch. The tool this replaces published an allow_removal that
	// promised to delete "files in this repo that are NOT in the central sync
	// config" - which is every file in the repository - documented it as
	// dangerous, and never implemented it. Naming a path is the only way to
	// have it removed, and naming it is the consent.
	Retired []string `json:"retired,omitempty"`

	// Excludes are the paths to leave alone entirely, neither written nor
	// removed. They travel with the files because they only mean anything
	// beside them.
	Excludes []string `json:"excludes,omitempty"`
}

// Exclusions is what the planner matches against.
func (c FileConfig) Exclusions() Excludes { return Excludes{Patterns: c.Excludes} }

// FileOverride is one repository's answer about the files: what it adjusts, and
// what it wants left alone.
//
// A repository rather than the organization, because that is where the
// knowledge is. One repository ignores a directory the others do not, and the
// template cannot know that. The tool this replaces read these from a file in
// each repository, which meant a repository could grant itself anything; here
// they are the panel's, against the repository's own row, so a rename cannot
// orphan them.
type FileOverride struct {
	Merges []FileMerge `json:"merges,omitempty"`

	// Excludes narrow further than the installation's, never wider.
	Excludes []string `json:"excludes,omitempty"`
}

// Exclusions is what the planner matches against.
func (o FileOverride) Exclusions() Excludes { return Excludes{Patterns: o.Excludes} }

// MergeFor answers how one path is composed for this repository, if at all.
func (o FileOverride) MergeFor(filePath string) filemerge.Spec {
	for _, merge := range o.Merges {
		if merge.Path == filePath {
			return merge.Spec
		}
	}

	return filemerge.Spec{}
}

// FileMerge is how one repository adjusts one file.
type FileMerge struct {
	Path string `json:"path"`

	filemerge.Spec
}

// The limits a file configuration is held to.
//
// The size is what the tool this replaces silently dropped a file for, counting
// it in no statistic and reporting the run as a success. It is a template
// somebody typed into a panel, so a megabyte is far past anything real and the
// refusal arrives beside the field.
const (
	longestFilePath    = 255
	largestFileContent = 1 << 20
)

// placeholder finds what a template asks to have filled in.
//
// Every one is checked against what this knows how to fill. A template naming
// something nobody implements would otherwise be written into a repository with
// the braces still in it, which is how the tool this replaces would have
// handled a typo in the one placeholder it had.
var placeholder = regexp.MustCompile(`\{\{[^{}]*\}\}`)

// placeholders are what a template may ask for.
var placeholders = map[string]struct{}{
	"{{DEFAULT_BRANCH}}": {},
}

// Render fills a template in for one repository.
//
// Line endings are settled here too, and for every file rather than only the
// ones a merge touches: Smyklot writes LF, so a template somebody pasted from
// an editor that writes CRLF becomes one file changed once rather than a file
// whose every line reads as changed each time something else about it moves.
func Render(content, defaultBranch string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")

	if defaultBranch == "" {
		return content
	}

	return strings.ReplaceAll(content, "{{DEFAULT_BRANCH}}", defaultBranch)
}

// Validate reports configuration nobody should be able to save.
//
// Every rule here is something the tool this replaces would have discovered
// while writing to somebody's repository, or would never have discovered at
// all: it validated the file list not at all - no empty path, no duplicate, no
// rejection of "..", no size limit.
func (c FileConfig) Validate() error {
	if err := c.Exclusions().Validate(); err != nil {
		return err
	}

	seen := foldedNames{}

	for index, file := range c.Files {
		if err := file.validate(index, seen); err != nil {
			return err
		}
	}

	return c.validateRetired(slices.Collect(maps.Keys(seen)))
}

func (c FileConfig) validateRetired(files []string) error {
	retired := foldedNames{}

	for index, retiredPath := range c.Retired {
		if err := validateFilePath("retired path", index, retiredPath); err != nil {
			return err
		}

		if earlier, clashed := retired.clash(retiredPath); clashed {
			return invalid("retired path %q is listed twice", earlier)
		}

		// A path cannot be both written and removed. Which one won would depend
		// on the order the two lists happened to be walked in, and the answer
		// somebody meant is not knowable from the document.
		if slices.Contains(files, strings.ToLower(retiredPath)) {
			return invalid(
				"%q is configured as a file and as a retired path, so it would be "+
					"written and removed by the same change", retiredPath)
		}
	}

	return nil
}

func (f File) validate(index int, seen foldedNames) error {
	if err := validateFilePath("file", index, f.Path); err != nil {
		return err
	}

	// Folded, because git will hold two paths that differ only in case and no
	// checkout on a case-insensitive filesystem will.
	if earlier, clashed := seen.clash(f.Path); clashed {
		if earlier == f.Path {
			return invalid("file %q is configured twice", f.Path)
		}

		return invalid("files %q and %q differ only in case", earlier, f.Path)
	}

	if f.Content == "" {
		return invalid("file %q has no content", f.Path)
	}

	if len(f.Content) > largestFileContent {
		return invalid("file %q is larger than %d bytes", f.Path, largestFileContent)
	}

	// A document is JSON on the way in and on the way out, and JSON carries
	// text. Bytes that are not text survive neither trip: they come back as
	// replacement characters, which is a file that says something nobody wrote.
	if !utf8.ValidString(f.Content) {
		return invalid("file %q is not text", f.Path)
	}

	return validatePlaceholders(f)
}

func validatePlaceholders(file File) error {
	for _, found := range placeholder.FindAllString(file.Content, -1) {
		if _, known := placeholders[found]; !known {
			return invalid("file %q asks for %s, which nothing fills in", file.Path, found)
		}
	}

	return nil
}

// validateFilePath refuses a path that would not land where it reads as landing.
//
// The first three questions - is there anything there, is it wrapped in
// whitespace, is it too long - are the ones every named thing here is asked,
// and they are asked in one place so a path and a label are refused in the same
// words.
func validateFilePath(noun string, index int, filePath string) error {
	if err := validateName(noun, "path", index, filePath, longestFilePath); err != nil {
		return err
	}

	cleaned := path.Clean(filePath)

	switch {
	case strings.HasPrefix(filePath, "/"):
		return invalid("%s %q starts at the root; paths are relative to the repository",
			noun, filePath)

	case strings.Contains(filePath, `\`):
		return invalid("%s %q has a backslash in it; git separates paths with /",
			noun, filePath)

	case cleaned == ".." || strings.HasPrefix(cleaned, "../"):
		// Asked of the cleaned path, so a traversal spelled the long way round
		// is answered as a traversal rather than as untidy punctuation.
		return invalid("%s %q climbs out of the repository", noun, filePath)

	case cleaned == ".":
		return invalid("%s %q names the repository rather than a file in it",
			noun, filePath)

	case cleaned != filePath:
		// A trailing separator, a doubled one, or a "./" - each is a way of
		// writing a path that is not the way git writes it.
		return invalid("%s %q is not a plain path; %q is the same place spelled once",
			noun, filePath, cleaned)
	}

	return nil
}

// Paths returns every configured file path, in configuration order.
func (c FileConfig) Paths() []string {
	paths := make([]string, 0, len(c.Files))
	for _, file := range c.Files {
		paths = append(paths, file.Path)
	}

	return paths
}

// Validate reports a repository's own adjustments that could never be applied.
//
// Checked against the installation's files, because an adjustment to a file
// nobody syncs is the same silence as a mistyped path: it reads as configured
// and does nothing.
func (o FileOverride) Validate(config FileConfig) error {
	if err := o.Exclusions().Validate(); err != nil {
		return err
	}

	paths := config.Paths()
	seen := foldedNames{}

	for index, merge := range o.Merges {
		if err := validateFilePath("adjustment", index, merge.Path); err != nil {
			return err
		}

		if earlier, clashed := seen.clash(merge.Path); clashed {
			return invalid("%q is adjusted twice", earlier)
		}

		if !slices.Contains(paths, merge.Path) {
			return invalid("%q is adjusted here and is not one of the files synchronized",
				merge.Path)
		}

		if err := merge.Validate(merge.Path); err != nil {
			return invalid("adjusting %q: %s", merge.Path, err)
		}
	}

	return nil
}
