package config

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

// Patch is a sparse configuration layer. Pointer fields distinguish an
// omitted setting from an explicit zero value such as false or an empty list.
type Patch struct {
	QuietSuccess           *bool              `json:"quiet_success,omitempty" yaml:"quiet_success,omitempty"`
	QuietReactions         *bool              `json:"quiet_reactions,omitempty" yaml:"quiet_reactions,omitempty"`
	QuietPending           *bool              `json:"quiet_pending,omitempty" yaml:"quiet_pending,omitempty"`
	AllowedCommands        *[]string          `json:"allowed_commands,omitempty" yaml:"allowed_commands,omitempty"`
	CommandAliases         *map[string]string `json:"command_aliases,omitempty" yaml:"command_aliases,omitempty"`
	CommandPrefix          *string            `json:"command_prefix,omitempty" yaml:"command_prefix,omitempty"`
	DisableMentions        *bool              `json:"disable_mentions,omitempty" yaml:"disable_mentions,omitempty"`
	DisableBareCommands    *bool              `json:"disable_bare_commands,omitempty" yaml:"disable_bare_commands,omitempty"`
	DisableUnapprove       *bool              `json:"disable_unapprove,omitempty" yaml:"disable_unapprove,omitempty"`
	DisableReactions       *bool              `json:"disable_reactions,omitempty" yaml:"disable_reactions,omitempty"`
	DisableDeletedComments *bool              `json:"disable_deleted_comments,omitempty" yaml:"disable_deleted_comments,omitempty"`
	AllowSelfApproval      *bool              `json:"allow_self_approval,omitempty" yaml:"allow_self_approval,omitempty"`
	Runner                 *Runner            `json:"runner,omitempty" yaml:"runner,omitempty"`
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

func processSources() map[string]Source {
	return map[string]Source{
		KeyQuietSuccess:           SourceProcess,
		KeyQuietReactions:         SourceProcess,
		KeyQuietPending:           SourceProcess,
		KeyAllowedCommands:        SourceProcess,
		KeyCommandAliases:         SourceProcess,
		KeyCommandPrefix:          SourceProcess,
		KeyDisableMentions:        SourceProcess,
		KeyDisableBareCommands:    SourceProcess,
		KeyDisableUnapprove:       SourceProcess,
		KeyDisableReactions:       SourceProcess,
		KeyDisableDeletedComments: SourceProcess,
		KeyAllowSelfApproval:      SourceProcess,
		KeyRunner:                 SourceProcess,
	}
}

func applyPatch(values *Config, patch Patch, sources map[string]Source, source Source) {
	set(&values.QuietSuccess, patch.QuietSuccess, sources, KeyQuietSuccess, source)
	set(&values.QuietReactions, patch.QuietReactions, sources, KeyQuietReactions, source)
	set(&values.QuietPending, patch.QuietPending, sources, KeyQuietPending, source)
	setSlice(&values.AllowedCommands, patch.AllowedCommands, sources, KeyAllowedCommands, source)
	setMap(&values.CommandAliases, patch.CommandAliases, sources, KeyCommandAliases, source)
	set(&values.CommandPrefix, patch.CommandPrefix, sources, KeyCommandPrefix, source)
	set(&values.DisableMentions, patch.DisableMentions, sources, KeyDisableMentions, source)
	set(&values.DisableBareCommands, patch.DisableBareCommands, sources, KeyDisableBareCommands, source)
	set(&values.DisableUnapprove, patch.DisableUnapprove, sources, KeyDisableUnapprove, source)
	set(&values.DisableReactions, patch.DisableReactions, sources, KeyDisableReactions, source)
	set(
		&values.DisableDeletedComments,
		patch.DisableDeletedComments,
		sources,
		KeyDisableDeletedComments,
		source,
	)
	set(&values.AllowSelfApproval, patch.AllowSelfApproval, sources, KeyAllowSelfApproval, source)
	set(&values.Runner, patch.Runner, sources, KeyRunner, source)
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
