// Package configsync reconciles panel settings with their versioned GitHub files.
package configsync

import (
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"sort"
	"strings"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

// Conflict identifies one setting changed differently on both sides.
// Paths are segments, not dotted strings, so names containing dots stay intact.
type Conflict struct {
	Path []string `json:"path"`
}

type MergeResult struct {
	Document  []byte
	Conflicts []Conflict
}

type slot struct {
	value   any
	present bool
}

// Merge combines independent edits against a common baseline. Arrays are atomic:
// guessing how to combine reordered commands, rule lists or templates is unsafe.
// A caller must not publish Document while Conflicts is nonempty.
func Merge(base, panel, file []byte) (MergeResult, error) {
	inputs := make([]slot, 3)
	for index, document := range [][]byte{base, panel, file} {
		value, err := decode(document)
		if err != nil {
			return MergeResult{}, err
		}
		inputs[index] = slot{value: value, present: true}
	}
	var conflicts []Conflict
	merged := mergeSlot(inputs[0], inputs[1], inputs[2], nil, &conflicts)
	document, err := config.EncodeJSONDocument(merged.value)
	if err != nil {
		return MergeResult{}, fmt.Errorf("encode merged settings: %w", err)
	}
	return MergeResult{Document: document, Conflicts: conflicts}, nil
}

// Equivalent ignores JSON whitespace, object order and numeric spelling.
func Equivalent(left, right []byte) (bool, error) {
	a, err := decode(left)
	if err != nil {
		return false, err
	}
	b, err := decode(right)
	if err != nil {
		return false, err
	}
	return equal(a, b), nil
}

func decode(document []byte) (any, error) {
	return config.DecodeJSONObject(document)
}

func mergeSlot(base, panel, file slot, path []string, conflicts *[]Conflict) slot {
	switch {
	case sameSlot(panel, file):
		return panel
	case sameSlot(base, panel):
		return file
	case sameSlot(base, file):
		return panel
	}
	p, panelObject := panel.value.(map[string]any)
	f, fileObject := file.value.(map[string]any)
	b, baseObject := base.value.(map[string]any)
	if panel.present && file.present && panelObject && fileObject && (baseObject || !base.present) {
		merged := make(map[string]any)
		keys := make(map[string]struct{})
		for _, object := range []map[string]any{b, p, f} {
			for key := range object {
				keys[key] = struct{}{}
			}
		}
		ordered := make([]string, 0, len(keys))
		for key := range keys {
			ordered = append(ordered, key)
		}
		sort.Strings(ordered)
		for _, key := range ordered {
			childPath := append(append([]string{}, path...), key)
			next := mergeSlot(at(b, key), at(p, key), at(f, key), childPath, conflicts)
			if next.present {
				merged[key] = next.value
			}
		}
		return slot{value: merged, present: true}
	}
	*conflicts = append(*conflicts, Conflict{Path: append([]string{}, path...)})
	return panel
}

func at(object map[string]any, key string) slot {
	value, present := object[key]
	return slot{value: value, present: present}
}

func sameSlot(a, b slot) bool {
	return a.present == b.present && (!a.present || equal(a.value, b.value))
}

func equal(a, b any) bool {
	switch a := a.(type) {
	case json.Number:
		right, ok := b.(json.Number)
		if !ok {
			return false
		}
		return canonicalNumber(a) == canonicalNumber(right)
	case map[string]any:
		right, ok := b.(map[string]any)
		return ok && equalObjects(a, right)
	case []any:
		right, ok := b.([]any)
		return ok && equalLists(a, right)
	default:
		return reflect.DeepEqual(a, b)
	}
}

func equalObjects(left, right map[string]any) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		other, present := right[key]
		if !present || !equal(value, other) {
			return false
		}
	}
	return true
}

func equalLists(left, right []any) bool {
	if len(left) != len(right) {
		return false
	}
	for index, value := range left {
		if !equal(value, right[index]) {
			return false
		}
	}
	return true
}

// Normalize coefficient and exponent without constructing enormous powers of ten.
// JSON permits exponents far beyond the range or memory budget of a float/rational.
func canonicalNumber(number json.Number) string {
	mantissa, exponent, found := strings.Cut(strings.ToLower(string(number)), "e")
	scale := new(big.Int)
	if found {
		scale.SetString(exponent, 10)
	}
	integer, fraction, _ := strings.Cut(mantissa, ".")
	scale.Sub(scale, big.NewInt(int64(len(fraction))))
	negative := strings.HasPrefix(integer, "-")
	digits := strings.TrimLeft(strings.TrimPrefix(integer, "-")+fraction, "0")
	if digits == "" {
		return "0"
	}
	significant := strings.TrimRight(digits, "0")
	scale.Add(scale, big.NewInt(int64(len(digits)-len(significant))))
	sign := ""
	if negative {
		sign = "-"
	}
	return sign + significant + "e" + scale.String()
}
