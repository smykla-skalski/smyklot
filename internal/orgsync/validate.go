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
//
// what is the word for what the name is - a name for a label, a path for a
// file - because a message that arrives beside the field somebody typed has to
// use their word for what they typed.
func validateName(noun, what string, index int, name string, longest int) error {
	trimmed := strings.TrimSpace(name)

	switch {
	case trimmed == "":
		return invalid("%s %d has no %s", noun, index+1, what)

	case trimmed != name:
		return invalid("%s %q has leading or trailing whitespace", noun, name)

	case len(name) > longest:
		return invalid("%s %q is longer than %d characters", noun, name, longest)
	}

	return nil
}

// foldedNames remembers the names a document has used so far.
//
// Shared because every kind is keyed by a name somebody typed, and answered
// separately by each because the reason differs. Labels clash because GitHub
// itself folds them and will not hold both. Rulesets clash because GitHub will
// happily hold both, and then nothing downstream can say which one a
// configuration entry meant.
//
// Asked one name at a time rather than handed the whole list, so a document
// with more than one thing wrong reports the first of them in configuration
// order. Checking the list afterwards was tidier to read and reported a
// mistake on line nine ahead of the one on line two.
type foldedNames map[string]string

// clash records a name and reports the earlier one it cannot be told apart
// from. earlier equals the name itself where the two are spelled identically,
// which is the plain duplicate rather than a case collision.
func (n foldedNames) clash(name string) (earlier string, clashed bool) {
	folded := strings.ToLower(name)
	if first, found := n[folded]; found {
		return first, true
	}

	n[folded] = name

	return "", false
}
