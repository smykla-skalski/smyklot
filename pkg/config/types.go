package config

import "fmt"

const (
	// DefaultCommandPrefix is the default prefix for slash-style commands
	DefaultCommandPrefix = "/"
)

// Runner names the entry point that acts on a repository's comments.
//
// Both entry points read this from the same file, and each one stands down
// unless it is the one named. Without it a repository that has a workflow file
// and is installed on a running service gets two of everything: two reactions,
// two approvals, two comments.
type Runner string

const (
	// RunnerService leaves every comment to the webhook service
	RunnerService Runner = "service"

	// RunnerAction leaves every comment to the GitHub Action
	RunnerAction Runner = "action"

	// DefaultRunner is the service. The Action is the fallback a repository
	// opts back into, not the other way round, so a repository that says
	// nothing is served by the service the App is installed on
	DefaultRunner = RunnerService
)

// ParseRunner reads a runner name, rejecting anything it does not know.
//
// A typo must not quietly pick an entry point. Left to compare equal to
// neither name it would take both of them out, and a repository would go silent
// with nothing to say why.
func ParseRunner(name string) (Runner, error) {
	switch runner := Runner(name); runner {
	case RunnerService, RunnerAction:
		return runner, nil

	case "":
		return DefaultRunner, nil

	default:
		return "", fmt.Errorf("%w: %q (want %q or %q)",
			ErrUnknownRunner, name, RunnerService, RunnerAction)
	}
}

// Config represents the configuration for Smyklot
type Config struct {
	// QuietSuccess disables success comments (keeps reactions only)
	QuietSuccess bool `json:"quiet_success"`

	// QuietReactions disables reaction-based approval/merge comments
	QuietReactions bool `json:"quiet_reactions"`

	// QuietPending disables pending CI comments (keeps reactions only)
	QuietPending bool `json:"quiet_pending"`

	// AllowedCommands is a list of allowed command names
	// an Empty list means all commands are allowed
	AllowedCommands []string `json:"allowed_commands"`

	// CommandAliases maps aliases to command names,
	// For example, {"app": "approve", "a": "approve"}
	CommandAliases map[string]string `json:"command_aliases"`

	// CommandPrefix is the prefix for slash-style commands
	// The default is "/" (e.g., /approve)
	CommandPrefix string `json:"command_prefix"`

	// DisableMentions disables mention-style commands (@smyklot approve)
	DisableMentions bool `json:"disable_mentions"`

	// DisableBareCommands disables bare commands (approve, lgtm, merge)
	DisableBareCommands bool `json:"disable_bare_commands"`

	// DisableUnapprove disables unapprove/disapprove commands
	DisableUnapprove bool `json:"disable_unapprove"`

	// DisableReactions disables reaction-based approvals/merges
	DisableReactions bool `json:"disable_reactions"`

	// DisableDeletedComments disables comments about deleted commands
	DisableDeletedComments bool `json:"disable_deleted_comments"`

	// AllowSelfApproval allows PR authors to approve their own PRs
	// Default is false (self-approval is not allowed)
	AllowSelfApproval bool `json:"allow_self_approval"`

	// Runner names the entry point that acts on this repository, so the other
	// one stands down. Empty means DefaultRunner
	Runner Runner `json:"runner"`
}

// Default returns a Config with default values
func Default() *Config {
	return &Config{
		QuietSuccess:           false,
		QuietReactions:         false,
		QuietPending:           false,
		AllowedCommands:        []string{},
		CommandAliases:         make(map[string]string),
		CommandPrefix:          DefaultCommandPrefix,
		DisableMentions:        false,
		DisableBareCommands:    false,
		DisableUnapprove:       false,
		DisableReactions:       false,
		DisableDeletedComments: false,
		AllowSelfApproval:      false,
		Runner:                 DefaultRunner,
	}
}

// EffectiveRunner is the entry point that acts on this repository.
//
// An unset runner reads as the default rather than as nothing, so a Config
// built in code behaves the same as one loaded from a file that omits the key.
func (c *Config) EffectiveRunner() Runner {
	if c.Runner == "" {
		return DefaultRunner
	}

	return c.Runner
}

// RunBy reports whether the given entry point should act on this repository.
func (c *Config) RunBy(runner Runner) bool {
	return c.EffectiveRunner() == runner
}

// IsCommandAllowed checks if a command is allowed
// If AllowedCommands is empty, all commands are allowed
func (c *Config) IsCommandAllowed(command string) bool {
	if len(c.AllowedCommands) == 0 {
		return true
	}

	for _, allowed := range c.AllowedCommands {
		if allowed == command {
			return true
		}
	}

	return false
}

// ResolveAlias resolves a command alias to the actual command name
// If no alias exists, returns the original command
func (c *Config) ResolveAlias(command string) string {
	if resolved, ok := c.CommandAliases[command]; ok {
		return resolved
	}

	return command
}
