package orgsync

import (
	"fmt"
	"strings"
)

// invalid is how every document reports configuration nobody should be able to
// save.
func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidConfig, fmt.Sprintf(format, args...))
}

// validateName refuses a name GitHub would not keep as it was written.
//
// The whitespace check is the one that is not obvious. GitHub trims silently,
// so a configured " bug" is created as "bug", looks missing on the next
// reconcile, and is created again - for ever, once per tick, on every
// repository.
//
// index is where it sits in configuration, counted from zero. It is only used
// to name something that has no name yet.
func validateName(noun string, index int, name string, longest int) error {
	trimmed := strings.TrimSpace(name)

	switch {
	case trimmed == "":
		return invalid("%s %d has no name", noun, index+1)

	case trimmed != name:
		return invalid("%s %q has leading or trailing whitespace", noun, name)

	case len(name) > longest:
		return invalid("%s %q is longer than %d characters", noun, name, longest)
	}

	return nil
}

// firstFoldClash reports the first two names that differ at most in case, in
// configuration order.
//
// Shared because every kind is keyed by a name somebody typed, and answered
// separately by each because the reason differs. Labels clash because GitHub
// itself folds them and will not hold both. Rulesets clash because GitHub will
// happily hold both, and then nothing downstream can say which one a
// configuration entry meant.
//
// first equals second when the two are spelled identically, which is the plain
// duplicate rather than a case collision.
func firstFoldClash(names []string) (first, second string, clashed bool) {
	seen := make(map[string]string, len(names))

	for _, name := range names {
		folded := strings.ToLower(name)
		if earlier, found := seen[folded]; found {
			return earlier, name, true
		}

		seen[folded] = name
	}

	return "", "", false
}
