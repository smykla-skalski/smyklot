// Package config provides configuration management for Smyklot using Viper
//
// It supports loading configuration from multiple sources with precedence:
// CLI flags > Environment variables > Config file > Defaults
package config

import (
	"bytes"
	"encoding/json"
	"os"

	"github.com/spf13/viper"
)

const (
	// KeyQuietSuccess is the config key for quiet_success setting
	KeyQuietSuccess = "quiet_success"

	// KeyQuietReactions is the config key for quiet_reactions setting
	KeyQuietReactions = "quiet_reactions"

	// KeyQuietPending is the config key for quiet_pending setting
	KeyQuietPending = "quiet_pending"

	// KeyAllowedCommands is the config key for allowed_commands setting
	KeyAllowedCommands = "allowed_commands"

	// KeyCommandAliases is the config key for command_aliases setting
	KeyCommandAliases = "command_aliases"

	// KeyCommandPrefix is the config key for command_prefix setting
	KeyCommandPrefix = "command_prefix"

	// KeyDisableMentions is the config key for disable_mentions setting
	KeyDisableMentions = "disable_mentions"

	// KeyDisableBareCommands is the config key for disable_bare_commands setting
	KeyDisableBareCommands = "disable_bare_commands"

	// KeyDisableUnapprove is the config key for disable_unapprove setting
	KeyDisableUnapprove = "disable_unapprove"

	// KeyDisableReactions is the config key for disable_reactions setting
	KeyDisableReactions = "disable_reactions"

	// KeyDisableDeletedComments is the config key for disable_deleted_comments setting
	KeyDisableDeletedComments = "disable_deleted_comments"

	// KeyAllowSelfApproval is the config key for allow_self_approval setting
	KeyAllowSelfApproval = "allow_self_approval"

	// KeyRunner is the config key for the runner setting
	KeyRunner = "runner"

	// EnvPrefix is the prefix for environment variables
	EnvPrefix = "SMYKLOT"

	// EnvConfig is the environment variable for JSON configuration
	EnvConfig = "SMYKLOT_CONFIG"
)

// SetupViper configures Viper with default values and environment variable bindings
func SetupViper(v *viper.Viper) {
	// Set defaults
	v.SetDefault(KeyQuietSuccess, false)
	v.SetDefault(KeyQuietReactions, false)
	v.SetDefault(KeyQuietPending, false)
	v.SetDefault(KeyAllowedCommands, []string{})
	v.SetDefault(KeyCommandAliases, map[string]string{})
	v.SetDefault(KeyCommandPrefix, DefaultCommandPrefix)
	v.SetDefault(KeyDisableMentions, false)
	v.SetDefault(KeyDisableBareCommands, false)
	v.SetDefault(KeyDisableUnapprove, false)
	v.SetDefault(KeyDisableReactions, false)
	v.SetDefault(KeyDisableDeletedComments, false)
	v.SetDefault(KeyAllowSelfApproval, false)
	v.SetDefault(KeyRunner, string(DefaultRunner))

	// Enable environment variable support
	v.SetEnvPrefix(EnvPrefix)
	v.AutomaticEnv()
}

// LoadFromViper creates a Config from Viper settings
//
// Every path that builds a Config from settings comes through here - the
// process-wide environment, the JSON blob, and a repository's own file - so
// this is where a value that cannot mean anything is rejected. A setting
// validated anywhere else would be validated on one of those paths and not the
// others.
func LoadFromViper(v *viper.Viper) (*Config, error) {
	runner, err := ParseRunner(v.GetString(KeyRunner))
	if err != nil {
		return nil, err
	}

	return &Config{
		QuietSuccess:           v.GetBool(KeyQuietSuccess),
		QuietReactions:         v.GetBool(KeyQuietReactions),
		QuietPending:           v.GetBool(KeyQuietPending),
		AllowedCommands:        v.GetStringSlice(KeyAllowedCommands),
		CommandAliases:         v.GetStringMapString(KeyCommandAliases),
		CommandPrefix:          v.GetString(KeyCommandPrefix),
		DisableMentions:        v.GetBool(KeyDisableMentions),
		DisableBareCommands:    v.GetBool(KeyDisableBareCommands),
		DisableUnapprove:       v.GetBool(KeyDisableUnapprove),
		DisableReactions:       v.GetBool(KeyDisableReactions),
		DisableDeletedComments: v.GetBool(KeyDisableDeletedComments),
		AllowSelfApproval:      v.GetBool(KeyAllowSelfApproval),
		Runner:                 runner,
	}, nil
}

// LoadRepoConfig layers a repository's own configuration file over base
//
// The Action reads per-repository behaviour from a repository variable a
// workflow injects, which a service running outside Actions cannot see. A
// repository checks the same settings into .github/smyklot.yaml instead, and
// both entry points layer it the same way so a comment gets the same treatment
// whichever one handles it.
//
// Keys the file omits keep their value from base. Empty content returns base
// unchanged, so a repository without the file is unaffected.
func LoadRepoConfig(base *Config, content []byte) (*Config, error) {
	if len(bytes.TrimSpace(content)) == 0 {
		return base, nil
	}

	if base == nil {
		base = Default()
	}

	// Seed Viper from base through Config's own JSON tags, so a new setting
	// cannot be forgotten here
	seed, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}

	v := viper.New()
	v.SetConfigType("json")

	if err := v.ReadConfig(bytes.NewReader(seed)); err != nil {
		return nil, err
	}

	v.SetConfigType("yaml")

	if err := v.MergeConfig(bytes.NewReader(content)); err != nil {
		return nil, err
	}

	return LoadFromViper(v)
}

// LoadJSONConfig reads and parses JSON configuration from SMYKLOT_CONFIG environment variable
func LoadJSONConfig(v *viper.Viper) error {
	configJSON := os.Getenv(EnvConfig)
	if configJSON == "" {
		return nil // No JSON config provided
	}

	// Parse JSON into a map
	var configMap map[string]interface{}
	if err := json.Unmarshal([]byte(configJSON), &configMap); err != nil {
		return err
	}

	// Merge each config value into Viper
	for key, value := range configMap {
		// Viper expects snake_case keys
		v.Set(key, value)
	}

	return nil
}
