package filemerge

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// mergeStructured composes a JSON or YAML template with a repository's
// overrides.
//
// YAML goes through its own path, which edits the document's nodes. JSON is
// decoded and re-encoded: its numbers survive that because the decoder is told
// to keep them as text, and it has no comments to lose. YAML survives neither,
// which is why it is not read this way.
func mergeStructured(format Format, template []byte, spec Spec) ([]byte, error) {
	if format == FormatYAML {
		return mergeYAML(template, spec)
	}

	base, err := parseJSON(template)
	if err != nil {
		return nil, err
	}

	override, err := decodeOverrides(spec.Overrides)
	if err != nil {
		return nil, err
	}

	var merged map[string]any

	switch spec.Strategy {
	case StrategyShallow:
		merged = mergeShallow(base, override)
	default:
		merged = mergePatch(base, override)
	}

	if err := applyArrayRules(merged, base, override, spec); err != nil {
		return nil, err
	}

	return renderJSON(merged)
}

// decodeOverrides reads the adjustments, which are stored as JSON whatever the
// file they patch is written in.
func decodeOverrides(overrides json.RawMessage) (map[string]any, error) {
	if len(overrides) == 0 {
		return map[string]any{}, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(overrides))
	decoder.UseNumber()

	var document map[string]any
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("%w: reading the overrides: %w", ErrUnreadable, err)
	}

	if document == nil {
		return map[string]any{}, nil
	}

	return document, nil
}

// mergePatch applies RFC 7396 JSON Merge Patch: objects merge key by key, a
// null removes a key, and anything else replaces what was there.
//
// Written out rather than taken from a library, because the library route goes
// through JSON and back. That round trip is what turned every number into a
// float64 - so an identifier past 2^53 came back as a different number, and a
// deduplicated array compared int(1) against float64(1) and kept both.
func mergePatch(base, patch map[string]any) map[string]any {
	merged := cloneOf(base, len(patch))

	for key, value := range patch {
		if value == nil {
			delete(merged, key)

			continue
		}

		nested, isObject := value.(map[string]any)
		if !isObject {
			merged[key] = clone(value)

			continue
		}

		// An object patched onto anything that is not an object replaces it,
		// and the nulls inside the patch remove nothing because there is
		// nothing there to remove. That is what the RFC says, and it is the
		// only reading that keeps a null meaning one thing.
		existing, wasObject := merged[key].(map[string]any)
		if !wasObject {
			existing = map[string]any{}
		}

		merged[key] = mergePatch(existing, nested)
	}

	return merged
}

// mergeShallow replaces top-level keys rather than merging into them.
//
// A null at the top level removes the key, as it does in a deep merge. A null
// anywhere below one is a null value, because a shallow merge does not look
// below the top level: what it writes there is the override's value, whole and
// exactly as it was written.
func mergeShallow(base, override map[string]any) map[string]any {
	merged := cloneOf(base, len(override))

	for key, value := range override {
		if value == nil {
			delete(merged, key)

			continue
		}

		merged[key] = clone(value)
	}

	return merged
}

// applyArrayRules re-merges the arrays a spec names, which RFC 7396 would
// otherwise have replaced.
//
// In configured order. The engine this replaces ranged a map, so two rules that
// touched the same document could resolve either way and the same inputs
// produced different files on different runs.
func applyArrayRules(merged, base, override map[string]any, spec Spec) error {
	for _, rule := range spec.Arrays {
		// Read rather than trusted: Validate has already read every path, and a
		// spec reaching here unvalidated should fail rather than skip.
		keys, overrideArray, err := overrideListFor(override, rule, ErrNothingAddressed)
		if err != nil {
			return err
		}

		// The template may not carry the array at all, and that is ordinary:
		// appending to nothing is the override's own list.
		baseArray, _ := valueAt(base, keys)
		existing, _ := baseArray.([]any)

		combined := mergeArrays(existing, overrideArray, rule.Strategy, spec.Deduplicate)

		if !setValueAt(merged, keys, combined) {
			return fmt.Errorf(
				"%w: nothing in the merged file holds %s", ErrNothingAddressed, rule.Path)
		}
	}

	return nil
}

// mergeArrays combines the template's list with the repository's.
func mergeArrays(base, override []any, strategy ArrayStrategy, deduplicate bool) []any {
	var combined []any

	switch strategy {
	case ArrayAppend:
		combined = concat(base, override)

	case ArrayPrepend:
		combined = concat(override, base)

	default:
		combined = concat(nil, override)
	}

	if !deduplicate {
		return combined
	}

	return deduplicated(combined)
}

// concat copies two lists into one that shares nothing with either.
func concat(first, second []any) []any {
	joined := make([]any, 0, len(first)+len(second))

	for _, value := range first {
		joined = append(joined, clone(value))
	}

	for _, value := range second {
		joined = append(joined, clone(value))
	}

	return joined
}

// deduplicated keeps the first of every value that appears more than once.
func deduplicated(values []any) []any {
	kept := make([]any, 0, len(values))

	for _, value := range values {
		if !holdsEqual(kept, value) {
			kept = append(kept, value)
		}
	}

	return kept
}
