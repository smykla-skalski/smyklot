package panel

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
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

// errManagedByRepository reports a setting the panel must not write.
//
// Which settings those are is not decided here: a field carries `panel:"deny"`
// on config.Patch and config.PanelDeniedKeys reports it, so the rule lives
// beside the field rather than in a list this package would have to remember to
// update. The message names whichever key was refused for the same reason - it
// is what a person reads, and hardcoding "runner" would make it wrong for the
// next field that opts out rather than merely incomplete.
var errManagedByRepository = errors.New("can only be configured in the repository file")

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
	if err := validateDeniedKeys(patch); err != nil {
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

// validateDeniedKeys refuses a patch that writes a setting the panel does not
// own, whichever setting that turns out to be.
func validateDeniedKeys(patch config.Patch) error {
	denied := config.PanelDeniedKeys()

	for _, key := range patch.SetKeys() {
		if slices.Contains(denied, key) {
			return fmt.Errorf("%s %w", key, errManagedByRepository)
		}
	}

	return nil
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
