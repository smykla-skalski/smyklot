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
