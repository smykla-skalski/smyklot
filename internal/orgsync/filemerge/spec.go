package filemerge

import (
	"cmp"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

// Strategy is how a repository's copy of a template is built from it.
type Strategy string

const (
	// StrategyDeep merges objects key by key, all the way down. RFC 7396: a
	// null removes a key, and anything else replaces what was there.
	StrategyDeep Strategy = "deep-merge"

	// StrategyShallow replaces top-level keys rather than merging into them.
	StrategyShallow Strategy = "shallow-merge"

	// StrategyMarkdown edits a document by its headings.
	StrategyMarkdown Strategy = "markdown"
)

// There is no "overlay". The engine this replaces offered one, published it in
// its schema as a choice beside deep-merge, and implemented it as the same
// code path - so a repository that chose it got something other than what it
// picked from a list of two things that were one thing.

// ArrayStrategy is what happens to a list the template and the override both
// have. Without one, RFC 7396 replaces it, which is the default here too.
type ArrayStrategy string

const (
	ArrayReplace ArrayStrategy = "replace"
	ArrayAppend  ArrayStrategy = "append"
	ArrayPrepend ArrayStrategy = "prepend"
)

// SectionAction is what a markdown merge does to a document.
type SectionAction string

const (
	// SectionBefore puts content above a heading.
	SectionBefore SectionAction = "before"

	// SectionAfter puts content below a section, past its subsections.
	SectionAfter SectionAction = "after"

	// SectionReplace puts content where a section was, subsections included.
	SectionReplace SectionAction = "replace"

	// SectionDelete removes a section, subsections included.
	SectionDelete SectionAction = "delete"

	// SectionPatch substitutes text inside a section, below its heading.
	//
	// Every occurrence, code blocks included. Which section a patch belongs to
	// is decided by reading the document's headings, and that reading skips
	// fenced code so a heading inside one is not mistaken for a real one - but
	// what to replace once the section is found is a literal substitution over
	// its text. A command in a fenced block is one of the most useful things
	// to patch, and a substitution that stopped at a fence would leave the
	// repository with the template's version of it and say nothing.
	SectionPatch SectionAction = "patch"

	// SectionAppend puts content at the end of the document.
	SectionAppend SectionAction = "append"

	// SectionPrepend puts content at the start of it.
	SectionPrepend SectionAction = "prepend"
)

// ArrayRule is what to do with the list at one path.
//
// A list rather than a map from path to strategy, which is what the engine
// this replaces used. Ranging a map gave the rules no order, so two of them
// touching one document resolved differently between runs and the file a
// repository ended up with depended on nothing anybody could see.
type ArrayRule struct {
	Path     string        `json:"path"`
	Strategy ArrayStrategy `json:"strategy"`
}

// Patch is a literal substitution inside a section.
type Patch struct {
	Find string `json:"find"`

	// Replace is what to put there. Empty removes the text found.
	Replace string `json:"replace"`
}

// Section is one operation on a markdown document.
type Section struct {
	Action SectionAction `json:"action"`

	// Heading is written the way the document writes it, marks and all:
	// "## Usage". The marks are how "## Usage" is told from "### Usage", which
	// the engine this replaces could not do.
	Heading string `json:"heading,omitempty"`

	// Occurrence says which one, counting from one, where a document carries
	// the same heading more than once. Left out, a heading that appears twice
	// is refused rather than resolved to the first of them.
	Occurrence int `json:"occurrence,omitempty"`

	Content string  `json:"content,omitempty"`
	Patches []Patch `json:"patches,omitempty"`
}

// Spec is how one file is composed for one repository.
type Spec struct {
	Strategy Strategy `json:"strategy,omitempty"`

	// Overrides are the repository's adjustments to a structured template,
	// stored as JSON whatever the template is written in.
	//
	// Raw rather than decoded, because how a number should be read depends on
	// what it is being merged into: a YAML file wants Go's own integers, and a
	// JSON one wants the digits exactly as they were typed.
	Overrides json.RawMessage `json:"overrides,omitempty"`

	Arrays      []ArrayRule `json:"arrays,omitempty"`
	Deduplicate bool        `json:"deduplicate,omitempty"`

	Sections []Section `json:"sections,omitempty"`
}

// Empty reports a spec that composes nothing, so the template is what the
// repository should hold.
func (s Spec) Empty() bool {
	return s.Strategy == "" && !s.adjusts() &&
		len(s.Arrays) == 0 && !s.Deduplicate && len(s.Sections) == 0
}

// adjusts reports overrides that say something.
//
// Three spellings arrive for saying nothing - absent, null, and the empty
// object - and all three mean the same thing. Reading one of them as an
// adjustment would run the merge for it, and running the merge re-renders the
// file: a YAML template would come back with its keys reordered and its
// comments gone, proposed to a repository as a change nobody asked for.
func (s Spec) adjusts() bool {
	document := strings.TrimSpace(string(s.Overrides))

	return document != "" && document != "null" && document != "{}"
}

// Apply builds the copy of a template a repository should hold.
//
// The template is the only document read. What a repository currently has at
// that path decides whether the answer is worth writing, and nothing else: a
// merge that read it would give an answer that depended on its own last answer,
// and appending to a file that already holds what was appended grows it once
// per run.
func Apply(
	filePath string,
	template []byte,
	spec Spec,
	policy config.FormattingPolicy,
) ([]byte, error) {
	result, err := ApplyDetailed(filePath, template, spec, policy)
	if err != nil {
		return nil, err
	}

	return result.Final, nil
}

// ApplyResult keeps the semantic composition separate from presentation.
// Callers that explain formatting compliance compare Composed with Final;
// callers that only need the repository bytes continue to use Apply.
type ApplyResult struct {
	Composed []byte
	Final    []byte
}

// ApplyStage identifies which half of ApplyDetailed refused a document.
type ApplyStage string

const (
	ApplyStageMerge  ApplyStage = "merge"
	ApplyStageFormat ApplyStage = "format"
)

// ApplyError preserves the original error text and identity while carrying
// the stage a panel diagnostic should name.
type ApplyError struct {
	Stage ApplyStage
	Err   error
}

func (e *ApplyError) Error() string { return e.Err.Error() }
func (e *ApplyError) Unwrap() error { return e.Err }

// ApplyDetailed builds the semantic copy and its final formatted bytes.
func ApplyDetailed(
	filePath string,
	template []byte,
	spec Spec,
	policy config.FormattingPolicy,
) (ApplyResult, error) {
	if spec.Empty() && policy.AllPreserve() {
		return ApplyResult{Composed: template, Final: template}, nil
	}
	if spec.Empty() {
		formatted, err := FormatDocument(filePath, template, policy)
		if err != nil {
			return ApplyResult{}, &ApplyError{Stage: ApplyStageFormat, Err: err}
		}

		return ApplyResult{Composed: template, Final: formatted}, nil
	}

	if err := spec.Validate(filePath); err != nil {
		return ApplyResult{}, &ApplyError{Stage: ApplyStageMerge, Err: err}
	}

	var (
		merged []byte
		err    error
	)

	if spec.effective(filePath) == StrategyMarkdown {
		merged, err = mergeMarkdown(template, spec.Sections)
	} else {
		format, _ := formatOf(filePath)
		merged, err = mergeStructured(format, template, spec)
	}
	if err != nil {
		return ApplyResult{}, &ApplyError{Stage: ApplyStageMerge, Err: err}
	}

	formatted, err := formatWithSource(filePath, merged, template, policy)
	if err != nil {
		return ApplyResult{}, &ApplyError{Stage: ApplyStageFormat, Err: err}
	}

	return ApplyResult{Composed: merged, Final: formatted}, nil
}

// FormatDocument applies only configured presentation dimensions. Unsupported file
// extensions are returned byte-identically.
func FormatDocument(filePath string, content []byte, policy config.FormattingPolicy) ([]byte, error) {
	if policy.AllPreserve() || !SupportsFormatting(filePath) {
		return content, nil
	}

	return formatWithSource(filePath, content, content, policy)
}

// SupportsFormatting reports the extensions governed by FormattingPolicy.
func SupportsFormatting(filePath string) bool {
	switch strings.ToLower(path.Ext(filePath)) {
	case extJSON, extJSONC, extYAML, extYML, extTOML, extMD, extMarkdown:
		return true
	default:
		return false
	}
}

// Validate reports a merge nobody should be able to configure, for the file it
// would be applied to.
//
// The file decides most of it. A strategy is only meaningful for the sort of
// document it edits, and the engine this replaces let a markdown strategy be
// configured for a JSON file, discovered it at apply time, and wrote the raw
// template over the repository's copy.
func (s Spec) Validate(filePath string) error {
	_, structured := formatOf(filePath)
	markdown := isMarkdown(filePath)

	if !structured && !markdown {
		return fmt.Errorf(
			"%w: %s has no extension this can merge; JSON, YAML, TOML and Markdown can",
			ErrUnsupportedFormat, filePath)
	}

	if err := s.validateStrategy(filePath, markdown); err != nil {
		return err
	}

	if markdown {
		return s.validateSections()
	}

	return s.validateStructured()
}

// effective is the strategy a spec runs under, which the file decides where the
// spec does not say.
func (s Spec) effective(filePath string) Strategy {
	if s.Strategy != "" {
		return s.Strategy
	}

	if isMarkdown(filePath) {
		return StrategyMarkdown
	}

	return StrategyDeep
}

func (s Spec) validateStrategy(filePath string, markdown bool) error {
	switch s.effective(filePath) {
	case StrategyMarkdown:
		if !markdown {
			return fmt.Errorf(
				"%w: %s is not Markdown, so it cannot be merged by its headings",
				ErrInvalidSpec, filePath)
		}

	case StrategyDeep, StrategyShallow:
		if markdown {
			return fmt.Errorf(
				"%w: %s is Markdown, which has no keys to merge; use %q",
				ErrInvalidSpec, filePath, StrategyMarkdown)
		}

	default:
		return fmt.Errorf("%w: unknown strategy %q", ErrInvalidSpec, s.Strategy)
	}

	return nil
}

// validateStructured checks a JSON or YAML merge.
func (s Spec) validateStructured() error {
	if len(s.Sections) > 0 {
		return fmt.Errorf(
			"%w: sections edit Markdown headings, and this file has none", ErrInvalidSpec)
	}

	var document map[string]any
	if len(s.Overrides) > 0 {
		var err error
		document, err = decodeOverrides(s.Overrides)
		if err != nil {
			return fmt.Errorf("%w: the overrides are not an object: %w", ErrInvalidSpec, err)
		}
	}

	if s.Deduplicate && len(s.Arrays) == 0 {
		return fmt.Errorf(
			"%w: nothing is deduplicated without a list rule, because a list with no "+
				"rule is replaced whole", ErrInvalidSpec)
	}

	// A merge that merges nothing is a row somebody filled in half of. Refused
	// rather than run: running it would re-render the file and propose a
	// reordered, comment-stripped copy of it as a change.
	if !s.adjusts() && len(s.Arrays) == 0 {
		return fmt.Errorf(
			"%w: nothing is merged without overrides or a list rule", ErrInvalidSpec)
	}

	return s.validateArrays(document)
}

func (s Spec) validateArrays(document map[string]any) error {
	seen := make(map[string]struct{}, len(s.Arrays))

	for index, rule := range s.Arrays {
		switch rule.Strategy {
		case ArrayReplace, ArrayAppend, ArrayPrepend:
		default:
			return fmt.Errorf("%w: list rule %d has unknown strategy %q",
				ErrInvalidSpec, index+1, rule.Strategy)
		}

		if _, repeated := seen[rule.Path]; repeated {
			return fmt.Errorf("%w: %s has two list rules", ErrInvalidSpec, rule.Path)
		}

		seen[rule.Path] = struct{}{}

		// The same reading the merge does, against the one document a spec
		// already holds: its own overrides. A rule says what to do with the
		// repository's list where the template has one, so a rule whose path no
		// override sets has no list to work with - for every template, always.
		// Left to Apply, that lands as a warning in the service log that stops
		// the repository's whole file sync, so a typo in one path silently
		// stops every managed file. Here it lands under the box somebody typed
		// it into.
		keys, _, err := overrideListFor(document, rule, ErrInvalidSpec)
		if err != nil {
			return err
		}

		// A shallow merge replaces a top-level key with the override's value
		// whole, so nothing below one is ever merged and a rule pointing there
		// would describe work that cannot happen.
		if s.Strategy == StrategyShallow && len(keys) > 1 {
			return fmt.Errorf(
				"%w: %s is below the top level, and a shallow merge replaces "+
					"top-level keys whole", ErrInvalidSpec, rule.Path)
		}
	}

	return nil
}

// validateSections checks a Markdown merge.
func (s Spec) validateSections() error {
	if len(s.Overrides) > 0 || len(s.Arrays) > 0 || s.Deduplicate {
		return fmt.Errorf(
			"%w: Markdown is edited by its headings, not by keys and lists", ErrInvalidSpec)
	}

	if len(s.Sections) == 0 {
		return fmt.Errorf("%w: a Markdown merge with no sections changes nothing",
			ErrInvalidSpec)
	}

	for index, section := range s.Sections {
		if err := section.validate(index); err != nil {
			return err
		}
	}

	return nil
}

func (s Section) validate(index int) error {
	position := index + 1

	switch s.Action {
	case SectionBefore, SectionAfter, SectionReplace:
		// The first problem rather than all of them, so a section with two
		// mistakes reports the earlier one.
		return cmp.Or(s.needsHeading(position), s.needsContent(position))

	case SectionDelete:
		return s.needsHeading(position)

	case SectionPatch:
		return cmp.Or(s.needsHeading(position), s.needsPatches(position))

	case SectionAppend, SectionPrepend:
		if s.Heading != "" || s.Occurrence != 0 {
			return fmt.Errorf(
				"%w: section %d %ss to the document, so it addresses no heading",
				ErrInvalidSpec, position, s.Action)
		}

		return s.needsContent(position)

	default:
		return fmt.Errorf("%w: section %d has unknown action %q",
			ErrInvalidSpec, position, s.Action)
	}
}

func (s Section) needsHeading(position int) error {
	if _, _, ok := parseHeading(s.Heading); !ok {
		return fmt.Errorf(
			"%w: section %d addresses %q, which is not a heading; write it with its # marks",
			ErrInvalidSpec, position, s.Heading)
	}

	if s.Occurrence < 0 {
		return fmt.Errorf("%w: section %d wants occurrence %d",
			ErrInvalidSpec, position, s.Occurrence)
	}

	return nil
}

func (s Section) needsContent(position int) error {
	if strings.TrimSpace(s.Content) == "" {
		return fmt.Errorf("%w: section %d has nothing to %s",
			ErrInvalidSpec, position, s.Action)
	}

	return nil
}

func (s Section) needsPatches(position int) error {
	if len(s.Patches) == 0 {
		return fmt.Errorf("%w: section %d patches nothing", ErrInvalidSpec, position)
	}

	for index, patch := range s.Patches {
		if patch.Find == "" {
			return fmt.Errorf("%w: section %d patch %d finds nothing",
				ErrInvalidSpec, position, index+1)
		}
	}

	return nil
}

// formatOf reads a file's extension as the format it is written in.
func formatOf(filePath string) (Format, bool) {
	switch strings.ToLower(path.Ext(filePath)) {
	case ".json":
		return FormatJSON, true
	case ".jsonc":
		return FormatJSONC, true
	case ".yml", ".yaml":
		return FormatYAML, true
	case ".toml":
		return FormatTOML, true
	default:
		return "", false
	}
}

func isMarkdown(filePath string) bool {
	switch strings.ToLower(path.Ext(filePath)) {
	case ".md", ".markdown":
		return true
	default:
		return false
	}
}
