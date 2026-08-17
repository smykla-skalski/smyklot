package filemerge

import (
	"encoding/json"
	"math/big"
	"slices"
)

// clone copies a decoded document so the result shares nothing with it.
//
// Every merge returns a document built only from clones. The engine this
// replaces returned a map whose untouched branches were the caller's own maps,
// so writing to the result wrote through to the template - and the template is
// read once and merged for every repository, which made the second repository's
// answer depend on the first's.
func clone(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		copied := make(map[string]any, len(typed))
		for key, nested := range typed {
			copied[key] = clone(nested)
		}

		return copied

	case []any:
		copied := make([]any, len(typed))
		for index, nested := range typed {
			copied[index] = clone(nested)
		}

		return copied

	default:
		// Scalars are immutable in Go, so a copy is the value itself.
		return value
	}
}

// cloneOf is a document to build a merge on: every key of the original, none
// of them still pointing at it, and room for what is about to be added.
func cloneOf(document map[string]any, extra int) map[string]any {
	copied := make(map[string]any, len(document)+extra)
	for key, value := range document {
		copied[key] = clone(value)
	}

	return copied
}

// equal reports two decoded values as saying the same thing.
//
// Numbers compare by value rather than by Go type, and that is the whole reason
// this exists. A JSON template decodes 1 one way and a YAML one another, so the
// engine this replaces compared int(1) against float64(1), called them
// different, and left both in an array it had been asked to deduplicate.
func equal(one, other any) bool {
	if left, right, both := numbers(one, other); both {
		return left.Cmp(right) == 0
	}

	switch left := one.(type) {
	case map[string]any:
		right, ok := other.(map[string]any)

		return ok && equalMaps(left, right)

	case map[any]any:
		// go-yaml answers with this for any mapping that has a key which is not
		// a string. Without it the comparison fell through to == on two maps,
		// which is a run-time panic rather than a wrong answer.
		right, ok := other.(map[any]any)

		return ok && equalMaps(left, right)

	case []any:
		right, ok := other.([]any)

		return ok && equalLists(left, right)

	default:
		if !canCompare(one) || !canCompare(other) {
			// Nothing left that can be compared with ==, and reaching it would
			// take the process down. Two values this cannot read are not the
			// same value, which for a deduplication means both are kept.
			return false
		}

		return one == other
	}
}

// canCompare reports a value == will not panic on.
//
// A whitelist rather than a list of what to avoid: a shape nobody anticipated
// answers false and is kept, where a blacklist would answer true and crash.
//
// Not named for the builtin constraint it sounds like: this is a question about
// one runtime value, and shadowing `comparable` puts the constraint out of
// reach of every generic function in the package.
func canCompare(value any) bool {
	switch value.(type) {
	case nil, bool, string, int, int64, uint64, float64, json.Number:
		return true

	default:
		return false
	}
}

// holdsEqual reports a value already among these, by the same comparison the
// merge uses everywhere else.
//
// Written out where both deduplications need it, because they are one rule -
// what counts as the same entry - and the JSON half and the YAML half judging
// it differently is a list deduplicated one way in one file and another way in
// the next.
func holdsEqual(values []any, wanted any) bool {
	return slices.ContainsFunc(values, func(value any) bool {
		return equal(value, wanted)
	})
}

// equalMaps compares two objects key by key.
//
// Generic over the key, because a document arrives with string keys from JSON
// and, where go-yaml met a key that is not a string, with any keys from YAML.
// The comparison is the same one either way.
func equalMaps[K comparable](one, other map[K]any) bool {
	if len(one) != len(other) {
		return false
	}

	for key, value := range one {
		counterpart, present := other[key]
		if !present || !equal(value, counterpart) {
			return false
		}
	}

	return true
}

func equalLists(one, other []any) bool {
	if len(one) != len(other) {
		return false
	}

	for index, value := range one {
		if !equal(value, other[index]) {
			return false
		}
	}

	return true
}

// numbers reads a pair as exact decimals, reporting whether both are numbers.
func numbers(one, other any) (left, right *big.Rat, both bool) {
	left, isNumber := number(one)
	if !isNumber {
		return nil, nil, false
	}

	right, isNumber = number(other)
	if !isNumber {
		return nil, nil, false
	}

	return left, right, true
}

// number reads a decoded value as an exact decimal.
//
// Exact, because the values this compares include GitHub actor and app
// identifiers, which pass 2^53 and stop being distinguishable as float64. The
// list is every numeric shape the two parsers produce: encoding/json answers
// with json.Number here because the decoder is told to, and go-yaml answers
// with int, int64, uint64 or float64 depending on what it read.
func number(value any) (*big.Rat, bool) {
	switch typed := value.(type) {
	case json.Number:
		return new(big.Rat).SetString(typed.String())

	case int:
		return new(big.Rat).SetInt64(int64(typed)), true

	case int64:
		return new(big.Rat).SetInt64(typed), true

	case uint64:
		return new(big.Rat).SetUint64(typed), true

	case float64:
		// Infinities and NaN have no rational value. They cannot come from
		// JSON, which has no spelling for them, but YAML does.
		rational := new(big.Rat).SetFloat64(typed)

		return rational, rational != nil

	default:
		return nil, false
	}
}
