package orgsync

import "strings"

// Excludes are the subjects a repository has asked to be left alone.
//
// An entry is either a literal name or a pattern containing `*`, which stands
// for any run of characters. That is the whole of the syntax, on purpose.
//
// The tool this replaces compared exclusions with `==`, so `ci/*` excluded a
// label literally called "ci/*" and nothing else, and neither its schema nor
// its documentation said so. Somebody would have written that entry, seen the
// sync succeed, and never learned it had done nothing.
//
// `*` crosses every character including `/`, unlike a path glob. A label is not
// a path: `kind/*` and `*/wip` are both names somebody means literally, and
// borrowing filepath.Match's separator rule would make one of them silently
// narrower than it reads.
type Excludes struct {
	Patterns []string
}

// Matches reports a subject that configuration has asked to leave alone.
func (e Excludes) Matches(subject string) bool {
	for _, pattern := range e.Patterns {
		if matchPattern(pattern, subject) {
			return true
		}
	}

	return false
}

// Validate refuses a pattern that cannot mean anything, at the point somebody
// writes it.
//
// There is no malformed pattern in this syntax, which is a reason to have
// chosen it: a character class or an unbalanced bracket would give a matcher
// somewhere to fail silently, and a pattern that quietly matches nothing is
// indistinguishable from one that works until the day it matters.
func (e Excludes) Validate() error {
	for index, pattern := range e.Patterns {
		if strings.TrimSpace(pattern) == "" {
			return invalid("exclusion %d is empty", index+1)
		}
	}

	return nil
}

// matchPattern reports whether subject matches a pattern whose only
// metacharacter is `*`.
//
// Written out rather than compiled to a regular expression, because escaping a
// name into a pattern language is how a label containing `.` or `+` comes to
// match something it does not equal.
func matchPattern(pattern, subject string) bool {
	if !strings.Contains(pattern, "*") {
		return pattern == subject
	}

	parts := strings.Split(pattern, "*")

	// The first part is anchored to the start and the last to the end;
	// everything between may float, taking the earliest match so the remaining
	// parts get the most room.
	if !strings.HasPrefix(subject, parts[0]) {
		return false
	}
	subject = subject[len(parts[0]):]

	last := parts[len(parts)-1]
	middle := parts[1 : len(parts)-1]

	for _, part := range middle {
		index := strings.Index(subject, part)
		if index < 0 {
			return false
		}
		subject = subject[index+len(part):]
	}

	return strings.HasSuffix(subject, last)
}
