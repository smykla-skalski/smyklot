package filemerge

import (
	"bytes"
	"fmt"

	"github.com/tailscale/hujson"
)

type jsonArrayItem struct {
	semantic      any
	syntax        hujson.Value
	fromBase      bool
	originalIndex int
}

func applyJSONSyntaxArrayRules(
	merged, base *hujson.Value,
	override map[string]any,
	spec Spec,
) error {
	for _, rule := range spec.Arrays {
		keys, overrideItems, err := overrideListFor(override, rule, ErrNothingAddressed)
		if err != nil {
			return err
		}

		baseValue, _, err := jsonValueAt(base, keys)
		if err != nil {
			return err
		}
		var baseArray *hujson.Array
		if baseValue != nil {
			baseArray, _ = baseValue.Value.(*hujson.Array)
		}
		target, found, err := jsonValueAt(merged, keys)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("%w: nothing in the merged file holds %s", ErrNothingAddressed, rule.Path)
		}

		items, err := combineJSONArrayItems(baseArray, overrideItems, rule, spec.Deduplicate)
		if err != nil {
			return err
		}
		array := arrayFromItems(baseArray, items)
		target.Value = array
	}

	return nil
}

func combineJSONArrayItems(
	base *hujson.Array,
	override []any,
	rule ArrayRule,
	deduplicate bool,
) ([]jsonArrayItem, error) {
	baseItems, err := syntaxArrayItems(base)
	if err != nil {
		return nil, err
	}
	overrideItems, err := builtArrayItems(override)
	if err != nil {
		return nil, err
	}

	var combined []jsonArrayItem
	switch rule.Strategy {
	case ArrayAppend:
		combined = append(combined, baseItems...)
		combined = append(combined, overrideItems...)
	case ArrayPrepend:
		combined = append(combined, overrideItems...)
		combined = append(combined, baseItems...)
	default:
		combined = append(combined, overrideItems...)
	}
	if !deduplicate {
		return combined, nil
	}

	kept := make([]jsonArrayItem, 0, len(combined))
	for _, candidate := range combined {
		duplicate := false
		for _, existing := range kept {
			if holdsEqual([]any{existing.semantic}, candidate.semantic) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			kept = append(kept, candidate)
		}
	}

	return kept, nil
}

func syntaxArrayItems(array *hujson.Array) ([]jsonArrayItem, error) {
	if array == nil {
		return nil, nil
	}
	items := make([]jsonArrayItem, 0, len(array.Elements))
	for index := range array.Elements {
		value := array.Elements[index].Clone()
		semantic, err := jsonSyntaxValue(&value)
		if err != nil {
			return nil, err
		}
		items = append(items, jsonArrayItem{
			semantic: semantic, syntax: value, fromBase: true, originalIndex: index,
		})
	}

	return items, nil
}

func builtArrayItems(values []any) ([]jsonArrayItem, error) {
	items := make([]jsonArrayItem, 0, len(values))
	for _, value := range values {
		syntax, err := syntaxForJSONValue(value)
		if err != nil {
			return nil, err
		}
		items = append(items, jsonArrayItem{semantic: value, syntax: syntax})
	}

	return items, nil
}

func arrayFromItems(base *hujson.Array, items []jsonArrayItem) *hujson.Array {
	result := &hujson.Array{}
	trailing := false
	if base != nil {
		result.AfterExtra = append(hujson.Extra(nil), base.AfterExtra...)
		trailing = jsonArrayHasTrailingComma(base)
	}

	for index, item := range items {
		value := item.syntax.Clone()
		if !item.fromBase || (item.originalIndex == 0 && index > 0) {
			value.BeforeExtra = inferredJSONArraySpacing(base, index)
		}
		value.AfterExtra = nil
		result.Elements = append(result.Elements, value)
	}
	if trailing && len(result.Elements) > 0 {
		result.Elements[len(result.Elements)-1].AfterExtra = hujson.Extra{}
	}

	return result
}

func inferredJSONArraySpacing(base *hujson.Array, index int) hujson.Extra {
	if base == nil || len(base.Elements) == 0 {
		if index > 0 {
			return hujson.Extra(" ")
		}
		return nil
	}
	if index == 0 {
		return jsonWhitespaceOnly(base.Elements[0].BeforeExtra)
	}
	for baseIndex := 1; baseIndex < len(base.Elements); baseIndex++ {
		if spacing := jsonWhitespaceOnly(base.Elements[baseIndex].BeforeExtra); spacing != nil {
			return spacing
		}
	}
	if bytes.Contains(base.Elements[0].BeforeExtra, []byte("\n")) {
		return jsonWhitespaceOnly(base.Elements[0].BeforeExtra)
	}

	return nil
}

func jsonValueAt(root *hujson.Value, keys []string) (*hujson.Value, bool, error) {
	current := root
	for _, key := range keys {
		object, ok := current.Value.(*hujson.Object)
		if !ok {
			return nil, false, nil
		}
		index, err := jsonMemberIndex(object, key)
		if err != nil {
			return nil, false, err
		}
		if index < 0 {
			return nil, false, nil
		}
		current = &object.Members[index].Value
	}

	return current, true, nil
}
