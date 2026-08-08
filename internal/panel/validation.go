package panel

import (
	"errors"
	"regexp"
	"strings"
	"unicode"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

const (
	maxAliases      = 100
	maxAliasLength  = 64
	maxPrefixLength = 64
)

var aliasPattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

var errRunnerManagedByRepository = errors.New("runner can only be configured in the repository file")

var canonicalCommands = map[string]struct{}{
	"approve":   {},
	"merge":     {},
	"squash":    {},
	"rebase":    {},
	"unapprove": {},
	"cleanup":   {},
	"help":      {},
}

func validatePatch(patch config.Patch) error {
	if err := validateRunner(patch.Runner); err != nil {
		return err
	}
	if err := validatePrefix(patch.CommandPrefix); err != nil {
		return err
	}
	if err := validateCommands(patch.AllowedCommands); err != nil {
		return err
	}

	return validateAliases(patch.CommandAliases)
}

func validateRunner(runner *config.Runner) error {
	if runner == nil {
		return nil
	}

	return errRunnerManagedByRepository
}

func validatePrefix(prefix *string) error {
	if prefix == nil {
		return nil
	}
	if *prefix == "" || len(*prefix) > maxPrefixLength ||
		strings.ContainsFunc(*prefix, unicode.IsControl) {
		return errors.New("command prefix must contain 1 to 64 printable characters")
	}

	return nil
}

func validateCommands(commands *[]string) error {
	if commands == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(*commands))
	for _, command := range *commands {
		if _, ok := canonicalCommands[command]; !ok {
			return errors.New("allowed commands contain an unknown command")
		}
		if _, duplicate := seen[command]; duplicate {
			return errors.New("allowed commands contain a duplicate")
		}
		seen[command] = struct{}{}
	}

	return nil
}

func validateAliases(aliases *map[string]string) error {
	if aliases == nil {
		return nil
	}
	if len(*aliases) > maxAliases {
		return errors.New("command aliases contain too many entries")
	}
	for alias, command := range *aliases {
		if len(alias) == 0 || len(alias) > maxAliasLength || !aliasPattern.MatchString(alias) {
			return errors.New("alias names may contain only letters, numbers, and underscores")
		}
		if _, ok := canonicalCommands[command]; !ok {
			return errors.New("command aliases contain an unknown command")
		}
	}

	return nil
}
