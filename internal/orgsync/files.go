package orgsync

import (
	"encoding/json"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode"
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
// The per-file size is what the tool this replaces silently dropped a file for,
// counting it in no statistic and reporting the run as a success. It is a
// template somebody typed into a panel, so a megabyte is far past anything
// real and the refusal arrives beside the field.
//
// The total is bounded for a different reason: a plan carries what it will
// write, once per repository it would write it to, so an installation of two
// hundred repositories multiplies this by two hundred. A megabyte all together
// is fifty times what the organization this replaces a tool for actually
// synchronizes, and it is what keeps that multiplication a number somebody
// could have predicted.
const (
	longestFilePath    = 255
	largestFileContent = 1 << 20
	largestFileTotal   = 1 << 20
)

// placeholder finds what a template asks to have filled in.
//
// Every one is checked against what this knows how to fill. A template naming
// something nobody implements would otherwise be written into a repository with
// the braces still in it, which is how the tool this replaces would have
// handled a typo in the one placeholder it had.
//
// Shaped like the placeholders this has rather than like every pair of braces,
// because doubled braces are ordinary text in the files an organization shares
// most: `${{ github.sha }}` in a workflow, `{{depName}}` in a Renovate
// configuration, `{{ .Values.image }}` in a chart. Read as placeholders those
// were refused, so the one kind of file this feature exists for could not be
// configured at all. Upper case and underscores, and never after a `$`, which
// is what tells Smyklot's own from somebody else's.
// The spacing is matched too, so `{{ DEFAULT_BRANCH }}` is refused rather than
// waved through: Render substitutes the exact spelling, so a spaced one passes
// validation and is then committed to every repository with its braces still
// on - which is the failure this check exists for, spelled the other way.
var placeholder = regexp.MustCompile(`(\$?)\{\{\s*([A-Z][A-Z0-9_]*)\s*\}\}`)

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
	total := 0

	for index, file := range c.Files {
		if err := file.validate(index, seen); err != nil {
			return err
		}

		total += len(file.Content)
		if total > largestFileTotal {
			return invalid("the files come to more than %d bytes together", largestFileTotal)
		}
	}

	if err := c.validateRetired(seen); err != nil {
		return err
	}

	return c.validateNesting(seen)
}

// validateNesting refuses a path that sits under another managed path.
//
// git records a path as a file or as the directory holding one, never both, so
// the two entries are one commit contradicting itself. Nothing else catches
// it: the duplicate rules compare a path to an identical path, and the
// conflict checks compare a path to what a repository already holds - and a
// repository that has neither of these yet holds nothing to compare against.
// It reaches GitHub as a tree nobody can build, on every reconcile.
// It reads the index the rules above filled in rather than building its own:
// what "the same name" means is one decision, in one place, in the function
// whose whole job is names that only differ by folding.
//
// Every ancestor of every path is looked up, rather than neighbours in a sorted
// list compared: "docs", "docs-2.md" and "docs/index.md" sort in that order, so
// the pair that is wrong is not the pair that is adjacent. Walked in
// configuration order, so which of two violations is reported does not depend
// on map iteration.
func (c FileConfig) validateNesting(managed foldedNames) error {
	for _, managedPath := range c.Managed() {
		for parent := parentPath(managedPath); parent != ""; parent = parentPath(parent) {
			if owner, held := managed[strings.ToLower(parent)]; held {
				return invalid(
					"%q sits under %q, so git would have to record %q as a file and "+
						"as a directory at once", managedPath, owner, owner)
			}
		}
	}

	return nil
}

// validateRetired checks the retired list against itself and against the files,
// and folds it into the index it was handed so the nesting rule reads one thing.
func (c FileConfig) validateRetired(files foldedNames) error {
	retired := foldedNames{}

	for index, retiredPath := range c.Retired {
		if err := validateFilePath("retired path", index, retiredPath); err != nil {
			return err
		}

		// Asked of the retired list first, so a path written twice there is
		// reported as that rather than as a collision with the file it is
		// about to be folded in beside.
		if earlier, clashed := retired.clash(retiredPath); clashed {
			return invalid("retired path %q is listed twice", earlier)
		}

		// A path cannot be both written and removed. Which one won would depend
		// on the order the two lists happened to be walked in, and the answer
		// somebody meant is not knowable from the document.
		if _, isFile := files[strings.ToLower(retiredPath)]; isFile {
			return invalid(
				"%q is configured as a file and as a retired path, so it would be "+
					"written and removed by the same change", retiredPath)
		}

		files[strings.ToLower(retiredPath)] = retiredPath
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
	for _, found := range placeholder.FindAllStringSubmatch(file.Content, -1) {
		// A `$` in front makes it somebody else's expression - a workflow's
		// `${{ GITHUB_TOKEN }}` reaches the repository as it is written.
		if found[1] == "$" {
			continue
		}

		// The exact spelling, because Render substitutes that and nothing else:
		// `{{ DEFAULT_BRANCH }}` known but not substituted is braces committed
		// to every repository, which is what this check is here to stop.
		canonical := "{{" + found[2] + "}}"

		if _, known := placeholders[canonical]; !known || found[0] != canonical {
			return invalid("file %q asks for %s, which nothing fills in",
				file.Path, canonical)
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

	case strings.ContainsFunc(filePath, unprintable):
		// A NUL or a control character survives every check above - it is not
		// a separator, not a dot, not whitespace path.Clean touches - and
		// reaches GitHub as an argument nothing sensible can be done with. It
		// is also invisible in the box somebody typed it into, which is the
		// part worth answering.
		return invalid("%s %q has a character in it that cannot be printed",
			noun, filePath)

	case cleaned == ".." || strings.HasPrefix(cleaned, "../"):
		// Asked of the cleaned path, so a traversal spelled the long way round
		// is answered as a traversal rather than as untidy punctuation.
		return invalid("%s %q climbs out of the repository", noun, filePath)

	case cleaned == ".":
		return invalid("%s %q names the repository rather than a file in it",
			noun, filePath)

	case strings.EqualFold(cleaned, ".git") ||
		strings.HasPrefix(strings.ToLower(cleaned), ".git/"):
		// git's own directory, which is not part of a checkout's contents. A
		// tree entry naming one is at best ignored and at worst a hook this
		// would be installing, so it is refused where it is typed rather than
		// discovered in somebody's repository. Folded, because a checkout on a
		// case-insensitive filesystem cannot tell .GIT from .git.
		return invalid("%s %q is inside git's own directory", noun, filePath)

	case cleaned != filePath:
		// A trailing separator, a doubled one, or a "./" - each is a way of
		// writing a path that is not the way git writes it.
		return invalid("%s %q is not a plain path; %q is the same place spelled once",
			noun, filePath, cleaned)
	}

	return nil
}

// unprintable reports a rune that has no business in a path a person typed.
func unprintable(character rune) bool { return !unicode.IsPrint(character) }

// Paths returns every configured file path, in configuration order.
func (c FileConfig) Paths() []string {
	paths := make([]string, 0, len(c.Files))
	for _, file := range c.Files {
		paths = append(paths, file.Path)
	}

	return paths
}

// Managed is every path this configuration writes or removes, which is the
// whole of what it asks a repository about.
func (c FileConfig) Managed() []string { return slices.Concat(c.Paths(), c.Retired) }

// workflowDirectory is where GitHub keeps a repository's workflows, and the one
// place in a repository that Contents access is not enough to write.
const workflowDirectory = ".github/workflows/"

// workflowPermission is GitHub's own spelling for being allowed to.
const workflowPermission = "workflows"

// PathPermission is what writing one path needs beyond the kind's own, or empty
// where nothing more is needed.
//
// On the kind, because which kinds are addressed by a path in a repository is
// the kind's own business: a label somebody named after a directory is still a
// label. Asked in two places - of a configuration before anything is planned,
// and of an action's subject before it is applied - and a second path-addressed
// kind knowing about only one of them is a plan somebody approves that GitHub
// then refuses.
//
// GitHub keeps workflow files behind a permission of their own and enforces it
// when the ref moves: a commit that creates or updates anything under
// .github/workflows is refused with a 422 naming `workflows`, however much
// Contents access the App holds. Unchecked, that lands after a person has read
// the plan and approved it - and because the apply failed, nothing is recorded,
// so the same plan is computed, approved and refused again on every reconcile
// after it, for ever.
//
// An exact prefix, because GitHub's is: a workflow is a file in that directory
// spelled that way and nowhere else.
func (k Kind) PathPermission(path string) string {
	if k == KindFiles && strings.HasPrefix(path, workflowDirectory) {
		return workflowPermission
	}

	return ""
}

// decodeFilePaths reads which files a stored document names, and nothing else.
//
// Beside FileConfig because it mirrors FileConfig's own tags, and a shape that
// copies another's tags from a different file is a shape that stops matching
// it. The templates are left undecoded on purpose: they are the bulk of the
// document - up to a megabyte - and the one caller reads this on every sweep
// tick of every installation, whether or not anything gets planned, to learn at
// most one permission name.
func decodeFilePaths(document []byte) (FileConfig, error) {
	var named struct {
		Files []struct {
			Path string `json:"path"`
		} `json:"files"`
		Retired  []string `json:"retired"`
		Excludes []string `json:"excludes"`
	}

	if err := json.Unmarshal(document, &named); err != nil {
		return FileConfig{}, err
	}

	config := FileConfig{Retired: named.Retired, Excludes: named.Excludes}
	for _, file := range named.Files {
		config.Files = append(config.Files, File{Path: file.Path})
	}

	return config, nil
}

// Permissions is what an installation must have granted for this configuration
// to run, beyond what the files kind itself needs.
//
// Retired paths count. Removing a workflow is writing the tree that no longer
// holds it, which GitHub refuses for the same reason it refuses adding one.
//
// Excluded paths do not. The planner skips them everywhere, so a workflow the
// configuration names and then excludes is never written - and asking for a
// permission on its account stands the whole kind down for the whole
// installation, over a file nothing was going to touch.
func (c FileConfig) Permissions() []string {
	var (
		wanted  []string
		leftOut = c.Exclusions()
	)

	for _, path := range c.Managed() {
		if leftOut.Matches(path) {
			continue
		}

		if permission := KindFiles.PathPermission(path); permission != "" &&
			!slices.Contains(wanted, permission) {
			wanted = append(wanted, permission)
		}
	}

	return wanted
}

// parentPath is the directory a path sits in, empty at the repository root.
//
// Every path reaching this has been through validateFilePath, so it is
// relative, cleaned and free of "..", which is what makes path.Dir the whole
// answer rather than most of it.
func parentPath(filePath string) string {
	parent := path.Dir(filePath)
	if parent == "." || parent == "/" || parent == filePath {
		return ""
	}

	return parent
}

// Adjusted is every path this override adjusts, in the order it names them.
func (o FileOverride) Adjusted() []string {
	paths := make([]string, 0, len(o.Merges))
	for _, merge := range o.Merges {
		paths = append(paths, merge.Path)
	}

	return paths
}

// Validate reports a repository's own adjustments that could never be applied.
//
// Checked against the installation's files, because an adjustment to a file
// nobody syncs is the same silence as a mistyped path: it reads as configured
// and does nothing.
func (o FileOverride) Validate(config FileConfig) error {
	return o.ValidateAgainst(config, nil)
}

// ValidateAgainst is the same check with the paths a repository already adjusts
// excused from one part of it.
//
// One part, and only one: whether the path is among the files the installation
// synchronizes. That is the single check here that turns on somebody else's
// configuration, and a repository that saved an adjustment while it did fit
// cannot be held to a document that moved underneath afterwards. Refusing the
// save takes this page away from the one person who needs it, because the
// repository whose adjustment no longer fits is exactly the one somebody has
// come to clean up or switch off - and the way out was to delete the
// customization to reach the switch.
//
// The path's shape, the duplicates, and whether the merge is one the file it
// names could take are checked as always. None of them is about the
// installation.
func (o FileOverride) ValidateAgainst(config FileConfig, keeping []string) error {
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

		if !slices.Contains(paths, merge.Path) && !slices.Contains(keeping, merge.Path) {
			return invalid("%q is adjusted here and is not one of the files synchronized",
				merge.Path)
		}

		if err := merge.Validate(merge.Path); err != nil {
			return invalid("adjusting %q: %s", merge.Path, err)
		}
	}

	return nil
}
