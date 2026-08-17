package filemerge

import (
	"fmt"
	"strings"
)

// A path names one place in a decoded document: "$" for the whole of it, then a
// dot and a key for each level below. "$.packageRules" is the top-level key of
// that name; "$.hostRules.matchHost" is one level in.
//
// A backslash escapes the character after it, so a key that itself contains a
// dot is written "$.example\.com" and one containing a backslash "$.a\\b". The
// engine this replaces documented escaping and did not implement it: it split
// on every dot, so a key with one in it could not be addressed at all and the
// path silently matched nothing.
const (
	pathRoot      = '$'
	pathSeparator = '.'
	pathEscape    = '\\'
)

// parsePath reads a path into the keys it names.
//
// Every failure is refused rather than resolved to nothing. A path that matches
// nothing is the same silence as a path that is misspelled, and the engine this
// replaces answered both by leaving the array alone - so a typo in a merge
// configuration read as "this file needs no merging" for as long as nobody
// checked.
func parsePath(path string) ([]string, error) {
	if path == "" {
		return nil, fmt.Errorf("%w: a path cannot be empty", ErrInvalidSpec)
	}

	if path[0] != pathRoot {
		return nil, fmt.Errorf(
			"%w: path %q does not start with %q", ErrInvalidSpec, path, string(pathRoot))
	}

	if len(path) == 1 {
		return nil, fmt.Errorf(
			"%w: path %q names the whole document, which is never an array",
			ErrInvalidSpec, path)
	}

	if path[1] != pathSeparator {
		return nil, fmt.Errorf(
			"%w: path %q needs a %q after the %q",
			ErrInvalidSpec, path, string(pathSeparator), string(pathRoot))
	}

	keys, err := splitPath(path, path[2:])
	if err != nil {
		return nil, err
	}

	for _, key := range keys {
		if key == "" {
			return nil, fmt.Errorf("%w: path %q has an empty key", ErrInvalidSpec, path)
		}
	}

	return keys, nil
}

// splitPath cuts a path's keys apart on unescaped dots and unescapes each one.
func splitPath(path, rest string) ([]string, error) {
	var (
		keys    []string
		current strings.Builder
	)

	for index := 0; index < len(rest); index++ {
		switch character := rest[index]; character {
		case pathEscape:
			if index+1 == len(rest) {
				return nil, fmt.Errorf(
					"%w: path %q ends in a %q, which escapes nothing",
					ErrInvalidSpec, path, string(pathEscape))
			}

			index++

			current.WriteByte(rest[index])

		case pathSeparator:
			keys = append(keys, current.String())
			current.Reset()

		case '[':
			// An index cannot mean anything here. The strategies this addresses
			// change what an array holds, so a position in the result is not the
			// position anything was configured at: append puts the override's
			// first element after every element of the template's. Refused with
			// the reason, rather than accepted and quietly ignored, which is
			// what the engine this replaces did with them.
			return nil, fmt.Errorf(
				"%w: path %q indexes an array, and an array's positions move when "+
					"it is merged, so only keys can be addressed",
				ErrInvalidSpec, path)

		default:
			current.WriteByte(character)
		}
	}

	return append(keys, current.String()), nil
}

// overrideListFor reads the list a rule works on out of the overrides, which is
// the same preflight for every rule wherever it is asked.
//
// Three places ask: Validate, where somebody typed the rule, and the JSON and
// YAML merges, which check again because a spec reaching them unvalidated
// should fail rather than quietly replace a list with nothing. Written out at
// each of them, the three had already stopped agreeing on what to call a list.
//
// The sentinel is the caller's, because the same fact is a bad configuration
// where it is typed and a merge that addresses nothing where it is run.
func overrideListFor(
	override map[string]any,
	rule ArrayRule,
	sentinel error,
) (keys []string, items []any, err error) {
	if keys, err = parsePath(rule.Path); err != nil {
		return nil, nil, err
	}

	value, present := valueAt(override, keys)
	if !present {
		return nil, nil, fmt.Errorf("%w: no override sets %s, so there is no list to %s",
			sentinel, rule.Path, rule.Strategy)
	}

	items, isList := value.([]any)
	if !isList {
		return nil, nil, fmt.Errorf("%w: the override at %s is not a list",
			sentinel, rule.Path)
	}

	return keys, items, nil
}

// valueAt reads what a document holds at a path, and whether it holds anything.
func valueAt(document map[string]any, keys []string) (any, bool) {
	current, found := parentAt(document, keys)
	if !found {
		return nil, false
	}

	value, held := current[keys[len(keys)-1]]

	return value, held
}

// parentAt walks to the object the last key sits in, if every level on the way
// is one.
func parentAt(document map[string]any, keys []string) (map[string]any, bool) {
	current := document

	for _, key := range keys[:len(keys)-1] {
		nested, isMap := current[key].(map[string]any)
		if !isMap {
			return nil, false
		}

		current = nested
	}

	return current, true
}

// setValueAt writes a value at a path, reporting whether the path was there to
// write to.
//
// It never builds the levels above. A rule addresses an array the merged
// document already has; creating the branches on the way to one it does not
// would write a key nothing asked for into somebody's file.
func setValueAt(document map[string]any, keys []string, value any) bool {
	current, found := parentAt(document, keys)
	if !found {
		return false
	}

	current[keys[len(keys)-1]] = value

	return true
}
