package config

//go:generate go run github.com/smykla-skalski/smyklot/internal/configgen/cmd/generate

import (
	"bytes"
	"fmt"

	"go.yaml.in/yaml/v3"
)

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

// ParsePatch decodes a repository or panel configuration layer. Unknown keys
// are rejected so a typo cannot silently leave a security-relevant default in
// effect.
func ParsePatch(content []byte) (Patch, error) {
	var patch Patch

	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)

	if err := decoder.Decode(&patch); err != nil {
		return Patch{}, fmt.Errorf("decode config patch: %w", err)
	}
	if patch.Runner != nil {
		runner, err := ParseRunner(string(*patch.Runner))
		if err != nil {
			return Patch{}, err
		}

		patch.Runner = &runner
	}

	return patch, nil
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
