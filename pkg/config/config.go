// Package config is what Smyklot can be configured to do, and how a value for
// each setting is arrived at.
//
// Patch is the one hand-written description of the settings; Config, the key
// constants, the defaults, the flags and the JSON Schema are generated from it
// - see internal/configgen. LoadProcess resolves the layers a process reads,
// and Resolve continues with the account and the repository. PrecedenceDoc
// states the order, and is the only place it is stated.
package config

import (
	"bytes"
)

const (
	// EnvPrefix is the prefix for environment variables
	EnvPrefix = "SMYKLOT"

	// EnvConfig is the environment variable holding a whole configuration
	// document, as against the one variable per setting EnvVar names
	EnvConfig = "SMYKLOT_CONFIG"
)

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
