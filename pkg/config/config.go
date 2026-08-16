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
	// EnvPrefix is the prefix for environment variables
	EnvPrefix = "SMYKLOT"

	// EnvConfig is the environment variable for JSON configuration
	EnvConfig = "SMYKLOT_CONFIG"
)

// SetupViper configures Viper with default values and environment variable
// bindings. The defaults come from Default(), so a setting added to Patch
// reaches viper without this function being edited.
func SetupViper(v *viper.Viper) {
	defaults := Default()

	v.SetDefault(KeyQuietSuccess, defaults.QuietSuccess)
	v.SetDefault(KeyQuietReactions, defaults.QuietReactions)
	v.SetDefault(KeyQuietPending, defaults.QuietPending)
	v.SetDefault(KeyAllowedCommands, defaults.AllowedCommands)
	v.SetDefault(KeyCommandAliases, defaults.CommandAliases)
	v.SetDefault(KeyCommandPrefix, defaults.CommandPrefix)
	v.SetDefault(KeyDisableMentions, defaults.DisableMentions)
	v.SetDefault(KeyDisableBareCommands, defaults.DisableBareCommands)
	v.SetDefault(KeyDisableUnapprove, defaults.DisableUnapprove)
	v.SetDefault(KeyDisableReactions, defaults.DisableReactions)
	v.SetDefault(KeyDisableDeletedComments, defaults.DisableDeletedComments)
	v.SetDefault(KeyAllowSelfApproval, defaults.AllowSelfApproval)
	v.SetDefault(KeyRunner, string(defaults.Runner))

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
// repository checks the same settings into its own configuration file instead,
// and both entry points layer it the same way so a comment gets the same
// treatment whichever one handles it.
//
// Keys the file omits keep their value from base. Empty content returns base
// unchanged, so a repository without the file is unaffected.
func LoadRepoConfig(base *Config, format Format, content []byte) (*Config, error) {
	if len(bytes.TrimSpace(content)) == 0 {
		return base, nil
	}

	patch, err := ParsePatch(format, content)
	if err != nil {
		return nil, err
	}

	return ApplyPatch(base, patch), nil
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
