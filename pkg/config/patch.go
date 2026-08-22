package config

//go:generate go run github.com/smykla-skalski/smyklot/internal/configgen/cmd/generate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"go.yaml.in/yaml/v3"
)

// RepoConfigPaths are the places a repository's own configuration may live, in
// the order they are looked for.
//
// The first match wins, so a repository that has both a TOML file and the
// legacy YAML one is read as TOML and told about the other. The legacy path is
// last for exactly that reason: it is what a repository migrates away from.
var RepoConfigPaths = []string{
	".smyklot.toml",
	".smyklot/config.toml",
	".github/.smyklot.toml",
	".github/smyklot.yaml",
}

// Format is how a configuration document is written.
type Format string

const (
	// FormatTOML is the format every configuration file Smyklot asks for is
	// written in.
	FormatTOML Format = "toml"

	// FormatYAML is .github/smyklot.yaml, which is still read and still
	// layered the same way, so a repository that has not migrated keeps
	// working. Nothing new is written in it.
	FormatYAML Format = "yaml"

	// formatJSON is the shape SMYKLOT_CONFIG used to be written in. It is
	// unexported because no file may be written in it: it exists for one
	// variable that is already deployed, and FormatOf must never hand it back
	// for a path.
	formatJSON Format = "json"
)

// FormatOf reports how the file at path is written.
//
// The extension decides, because it is the only thing known before the file is
// fetched - which is what lets discovery ask for a path and know what it will
// get back.
func FormatOf(filePath string) (Format, error) {
	switch strings.ToLower(path.Ext(filePath)) {
	case ".toml":
		return FormatTOML, nil

	case ".yaml", ".yml":
		return FormatYAML, nil

	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownFormat, filePath)
	}
}

// Source identifies the layer that supplied an effective setting.
type Source string

const (
	SourceProcess         Source = "process"
	SourceTarget          Source = "target"
	SourceRepositoryFile  Source = "repository_file"
	SourceRepositoryPanel Source = "repository_panel"
)

// Patch is a sparse configuration layer, and the authoritative description of
// what Smyklot can be configured to do. Pointer fields distinguish an omitted
// setting from an explicit zero value such as false or an empty list.
//
// Config, Default, the Key constants, applyPatch, the source map and the JSON
// Schema are all generated from this type - see internal/configgen. Adding a
// setting here and running `mise run generate` adds it everywhere; adding it
// anywhere else fails the completeness test rather than half-working.
//
// The tags beyond the encoders' own carry what the generator cannot infer:
//
//	default  the value Default() reports and the schema publishes
//	enum     the complete set of accepted values
//	flag     "-" for a setting with no command-line flag
//	panel    "deny" for a setting the panel must refuse to write
type Patch struct {
	// QuietSuccess drops the comment a successful command would post, leaving
	// only its reaction. Errors and warnings still comment.
	QuietSuccess *bool `json:"quiet_success,omitempty" yaml:"quiet_success,omitempty" toml:"quiet_success,omitempty"`

	// QuietReactions drops the comment an approval or merge driven by a
	// reaction would post.
	QuietReactions *bool `json:"quiet_reactions,omitempty" yaml:"quiet_reactions,omitempty" toml:"quiet_reactions,omitempty"`

	// QuietPending drops the comment announcing that a merge is waiting on CI,
	// leaving only its reaction.
	QuietPending *bool `json:"quiet_pending,omitempty" yaml:"quiet_pending,omitempty" toml:"quiet_pending,omitempty"`

	// AllowedCommands narrows what may be run. An empty list allows every
	// command; naming any command forbids the rest.
	AllowedCommands *[]string `json:"allowed_commands,omitempty" yaml:"allowed_commands,omitempty" toml:"allowed_commands,omitempty"`

	// CommandAliases maps a spelling somebody uses onto the command it means,
	// such as "app" onto "approve".
	CommandAliases *map[string]string `json:"command_aliases,omitempty" yaml:"command_aliases,omitempty" toml:"command_aliases,omitempty"`

	// CommandPrefix opens a slash-style command, as "/" does in "/approve".
	CommandPrefix *string `json:"command_prefix,omitempty" yaml:"command_prefix,omitempty" toml:"command_prefix,omitempty" default:"/"`

	// DisableMentions stops Smyklot answering a mention, such as
	// "@smyklot approve".
	DisableMentions *bool `json:"disable_mentions,omitempty" yaml:"disable_mentions,omitempty" toml:"disable_mentions,omitempty"`

	// DisableBareCommands stops Smyklot answering an unprefixed word such as
	// "approve" or "lgtm" on a line of its own.
	DisableBareCommands *bool `json:"disable_bare_commands,omitempty" yaml:"disable_bare_commands,omitempty" toml:"disable_bare_commands,omitempty"`

	// DisableUnapprove withdraws the unapprove and disapprove commands.
	DisableUnapprove *bool `json:"disable_unapprove,omitempty" yaml:"disable_unapprove,omitempty" toml:"disable_unapprove,omitempty"`

	// DisableReactions stops a reaction on the pull request body counting as a
	// command at all.
	DisableReactions *bool `json:"disable_reactions,omitempty" yaml:"disable_reactions,omitempty" toml:"disable_reactions,omitempty"`

	// DisableDeletedComments stops Smyklot reporting that a comment carrying a
	// command was deleted.
	DisableDeletedComments *bool `json:"disable_deleted_comments,omitempty" yaml:"disable_deleted_comments,omitempty" toml:"disable_deleted_comments,omitempty"`

	// AllowSelfApproval lets the author of a pull request approve it. Off by
	// default: an approval is meant to be a second pair of eyes.
	AllowSelfApproval *bool `json:"allow_self_approval,omitempty" yaml:"allow_self_approval,omitempty" toml:"allow_self_approval,omitempty"`

	// Runner names the entry point that acts on this repository, so the other
	// one stands down. It is settable only in the repository's own file: the
	// panel cannot write it, because a repository that has moved back to the
	// Action must be able to say so itself.
	Runner *Runner `json:"runner,omitempty" yaml:"runner,omitempty" toml:"runner,omitempty" default:"service" enum:"service,action" flag:"-" panel:"deny"`
}

// Layer associates a sparse patch with its provenance.
type Layer struct {
	Source Source
	Patch  Patch
}

// Resolved contains effective values and the source of every setting.
type Resolved struct {
	Values  Config            `json:"values"`
	Sources map[string]Source `json:"sources"`
}

// ParsePatch decodes a configuration layer written in format.
//
// The format is told rather than sniffed. A caller always knows which file it
// read, and guessing turns "this TOML file has a syntax error on line 4" into
// "this file is not valid YAML either", which names neither the format nor the
// line.
//
// Unknown keys are rejected in both formats, so a typo cannot silently leave a
// security-relevant default in effect. That is the same reason the file is
// fail-closed: it is where allowed_commands is narrowed.
func ParsePatch(format Format, content []byte) (Patch, error) {
	var patch Patch

	if err := decode(format, content, &patch); err != nil {
		return Patch{}, fmt.Errorf("decode %s config patch: %w", format, err)
	}

	if err := patch.normalize(); err != nil {
		return Patch{}, err
	}

	return patch, nil
}

// RenderTOML writes a patch as the file Smyklot asks repositories to keep.
//
// Only what the patch sets is written, because that is what the file means: a
// setting it omits keeps whatever the layers below it said, and writing out
// the full set would silently pin twelve defaults a repository never chose.
//
// This is what converts a repository's legacy YAML, so it is held to a
// round-trip: what ParsePatch reads back has to be what went in, or the
// migration would quietly change how a repository behaves.
func RenderTOML(patch Patch) ([]byte, error) {
	content, err := toml.Marshal(patch)
	if err != nil {
		return nil, fmt.Errorf("render toml config patch: %w", err)
	}

	return content, nil
}

// byteOrderMark is what several editors write at the start of a UTF-8 file.
//
// It is metadata rather than content, and invisible to whoever saved the file.
// go-toml reads it as the first character of a key and refuses the document
// with "invalid character at start of key: U+00EF", which under a fail-closed
// file means a repository goes quiet over a mark nobody can see. go-yaml skips
// it, so stripping it here is also what keeps the two formats agreeing.
var byteOrderMark = []byte{0xEF, 0xBB, 0xBF}

// decode reads content into patch.
//
// A document that sets nothing - blank, or nothing but comments - is not an
// error. The YAML decoder reports it as io.EOF, having found no values, and a
// file somebody created and has not filled in yet has to read as "nothing set".
// It used to be an error, which meant a repository that commented its settings
// out was told its configuration was invalid and had every command refused.
func decode(format Format, content []byte, patch *Patch) error {
	content = bytes.TrimPrefix(content, byteOrderMark)

	switch format {
	case FormatTOML:
		decoder := toml.NewDecoder(bytes.NewReader(content))
		decoder.DisallowUnknownFields()

		return emptyIsNothing(unknownSettings(decoder.Decode(patch)))

	case FormatYAML:
		decoder := yaml.NewDecoder(bytes.NewReader(content))
		decoder.KnownFields(true)

		if err := emptyIsNothing(decoder.Decode(patch)); err != nil {
			return err
		}

		return rejectLaterSettings(decoder)

	case formatJSON:
		decoder := json.NewDecoder(bytes.NewReader(content))
		decoder.DisallowUnknownFields()

		if err := emptyIsNothing(decoder.Decode(patch)); err != nil {
			return err
		}

		return rejectTrailingDocument(decoder)

	default:
		return fmt.Errorf("%w: %q", ErrUnknownFormat, format)
	}
}

// rejectLaterSettings refuses a YAML stream whose settings continue past the
// first document.
//
// A decoder reads one document, so everything after a `---` was being dropped
// without a word: `quiet_success: true` followed by `---` and
// `allowed_commands: [approve]` narrowed nothing, and a file whose first
// document was empty was ignored in its entirety. Both read as "this repository
// configured nothing", which grants back every command the file was there to
// take away.
//
// A later document that sets nothing is left alone, because a trailing `---` is
// legal, means nothing, and works today. Only a document carrying settings
// Smyklot would not otherwise honour is refused, and it is refused rather than
// merged: a file that says two things is one somebody should be told about.
func rejectLaterSettings(decoder *yaml.Decoder) error {
	for {
		var later Patch

		switch err := decoder.Decode(&later); {
		case errors.Is(err, io.EOF):
			return nil

		case err != nil:
			return err

		case len(later.SetKeys()) > 0:
			return fmt.Errorf("%w: %s", ErrMultipleDocuments, strings.Join(later.SetKeys(), ", "))
		}
	}
}

// rejectTrailingDocument refuses JSON with anything after the first value.
//
// A decoder stops at the end of that value and says nothing about what
// follows, so two documents in one variable would silently be one - the same
// hole rejectLaterSettings closes for YAML.
func rejectTrailingDocument(decoder *json.Decoder) error {
	if err := decoder.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) {
		return ErrTrailingContent
	}

	return nil
}

// emptyIsNothing reads "the decoder found no values" as setting nothing.
//
// Only a genuinely empty or comment-only document produces io.EOF; every
// truncated or malformed one reports a real error, which is what makes this
// safe. Reading a broken file as "nothing configured" would hand back every
// command the file was there to take away. go-toml answers an empty document
// with nil rather than io.EOF, so for TOML this is a no-op - it is applied to
// both so the two formats cannot drift apart.
func emptyIsNothing(err error) error {
	if errors.Is(err, io.EOF) {
		return nil
	}

	return err
}

// unknownSettings restates a TOML strict-mode failure so it names the keys.
//
// go-toml reports "fields in the document are missing in the target struct",
// which says nothing a repository owner could act on - and this message is
// shown to them, on the pull request, as the reason nothing ran. The keys are
// in the error, one level down.
func unknownSettings(err error) error {
	var missing *toml.StrictMissingError
	if !errors.As(err, &missing) {
		return err
	}

	keys := make([]string, 0, len(missing.Errors))
	for _, one := range missing.Errors {
		keys = append(keys, strings.Join(one.Key(), "."))
	}

	return fmt.Errorf("%w: %s", ErrUnknownSetting, strings.Join(keys, ", "))
}

// ApplyPatch returns a deep copy of base with every present patch field
// replaced. The input configuration is never mutated.
func ApplyPatch(base *Config, patch Patch) *Config {
	result := cloneConfig(base)
	applyPatch(result, patch, nil, "")

	return result
}

// Resolve applies ordered layers to base and records the winning source for
// each setting. Later layers have higher precedence.
func Resolve(base *Config, layers ...Layer) Resolved {
	values := cloneConfig(base)
	sources := processSources()

	for _, layer := range layers {
		applyPatch(values, layer.Patch, sources, layer.Source)
	}

	return Resolved{Values: *values, Sources: sources}
}

func cloneConfig(base *Config) *Config {
	if base == nil {
		base = Default()
	}

	result := *base
	result.AllowedCommands = append([]string{}, base.AllowedCommands...)
	result.CommandAliases = cloneAliases(base.CommandAliases)

	return &result
}

func cloneAliases(aliases map[string]string) map[string]string {
	result := make(map[string]string, len(aliases))
	for alias, command := range aliases {
		result[alias] = command
	}

	return result
}

func set[T any](target *T, value *T, sources map[string]Source, key string, source Source) {
	if value == nil {
		return
	}

	*target = *value
	setSource(sources, key, source)
}

func setSlice(
	target *[]string,
	value *[]string,
	sources map[string]Source,
	key string,
	source Source,
) {
	if value == nil {
		return
	}

	*target = append([]string{}, (*value)...)
	setSource(sources, key, source)
}

func setMap(
	target *map[string]string,
	value *map[string]string,
	sources map[string]Source,
	key string,
	source Source,
) {
	if value == nil {
		return
	}

	*target = cloneAliases(*value)
	setSource(sources, key, source)
}

func setSource(sources map[string]Source, key string, source Source) {
	if sources != nil {
		sources[key] = source
	}
}
