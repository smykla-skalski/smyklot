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

// EffectiveRunner is the entry point that acts on this repository.
//
// Anything this does not recognise reads as the default, which is what makes a
// wrong value harmless rather than silencing. Both entry points decide whether
// to act by comparing this to their own name, so a third value matches neither
// and the repository goes quiet with nothing anywhere to say why.
//
// Every place a runner is read from text refuses one it does not know -
// ParsePatch does it for a file and a document, and the generated normalize
// does it for a variable and a flag - so this should be unreachable. It is here
// because "unreachable" is a claim about today's callers, and the failure it
// prevents is a repository nobody can see has stopped working.
func (c *Config) EffectiveRunner() Runner {
	switch c.Runner {
	case RunnerService, RunnerAction:
		return c.Runner

	default:
		return DefaultRunner
	}
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
