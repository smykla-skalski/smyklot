package filemerge

import (
	"encoding/json"
	"math/big"
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

		return ok && equalObjects(left, right)

	case []any:
		right, ok := other.([]any)

		return ok && equalLists(left, right)

	default:
		return one == other
	}
}

func equalObjects(one, other map[string]any) bool {
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
