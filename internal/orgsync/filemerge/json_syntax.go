package filemerge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"

	"github.com/tailscale/hujson"
)

func parseJSONSyntax(content []byte, jsonc bool) (*hujson.Value, error) {
	value, err := hujson.Parse(content)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}
	if !jsonc && !value.IsStandard() {
		return nil, fmt.Errorf("%w: strict JSON cannot contain comments or trailing commas", ErrUnreadable)
	}
	if err := rejectDuplicateJSONKeys(&value); err != nil {
		return nil, err
	}

	return &value, nil
}

func rejectDuplicateJSONKeys(value *hujson.Value) error {
	switch node := value.Value.(type) {
	case *hujson.Object:
		seen := make(map[string]struct{}, len(node.Members))
		for index := range node.Members {
			name, err := jsonMemberName(node.Members[index])
			if err != nil {
				return err
			}
			if _, exists := seen[name]; exists {
				return fmt.Errorf("%w: it names %q twice", ErrUnreadable, name)
			}
			seen[name] = struct{}{}
			if err := rejectDuplicateJSONKeys(&node.Members[index].Value); err != nil {
				return err
			}
		}

	case *hujson.Array:
		for index := range node.Elements {
			if err := rejectDuplicateJSONKeys(&node.Elements[index]); err != nil {
				return err
			}
		}
	}

	return nil
}

func jsonSyntaxValue(value *hujson.Value) (any, error) {
	copy := value.Clone()
	copy.Standardize()

	decoder := json.NewDecoder(bytes.NewReader(copy.Pack()))
	decoder.UseNumber()

	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}

	return decoded, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return err
	}

	return errTrailingContent
}

func syntaxForJSONValue(value any) (hujson.Value, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return hujson.Value{}, fmt.Errorf("%w: %w", ErrUnwritable, err)
	}

	parsed, err := hujson.Parse(bytes.TrimSuffix(buffer.Bytes(), []byte("\n")))
	if err != nil {
		return hujson.Value{}, fmt.Errorf("%w: %w", ErrUnwritable, err)
	}

	return parsed, nil
}

func mergeJSONSyntax(template []byte, spec Spec, jsonc bool) ([]byte, error) {
	root, err := parseJSONSyntax(template, jsonc)
	if err != nil {
		return nil, err
	}
	object, ok := root.Value.(*hujson.Object)
	if !ok {
		return nil, fmt.Errorf("%w: it holds no object", ErrUnreadable)
	}
	before := root.Clone()

	override, err := decodeOverrides(spec.Overrides)
	if err != nil {
		return nil, err
	}

	deep := spec.Strategy != StrategyShallow
	if err := mergeJSONObject(object, override, deep); err != nil {
		return nil, err
	}
	if err := applyJSONSyntaxArrayRules(root, &before, override, spec); err != nil {
		return nil, err
	}
	if err := rejectDuplicateJSONKeys(root); err != nil {
		return nil, err
	}

	return root.Pack(), nil
}

func mergeJSONObject(object *hujson.Object, patch map[string]any, deep bool) error {
	for _, key := range sortedKeys(patch) {
		value := patch[key]
		index, err := jsonMemberIndex(object, key)
		if err != nil {
			return err
		}
		if value == nil {
			deleteJSONMember(object, index)

			continue
		}

		nested, isObject := value.(map[string]any)
		merged, err := mergeNestedJSONObject(object, index, nested, deep && isObject)
		if err != nil {
			return err
		}
		if merged {
			continue
		}
		if deep && isObject {
			value = withoutNulls(nested)
		}

		built, err := syntaxForJSONValue(value)
		if err != nil {
			return err
		}
		if index >= 0 {
			if err := replaceJSONValue(&object.Members[index].Value, &built); err != nil {
				return err
			}
			continue
		}

		appendJSONMember(object, key, built)
	}

	return nil
}

func deleteJSONMember(object *hujson.Object, index int) {
	if index >= 0 {
		object.Members = slices.Delete(object.Members, index, index+1)
	}
}

func mergeNestedJSONObject(
	object *hujson.Object,
	index int,
	nested map[string]any,
	merge bool,
) (bool, error) {
	if !merge || index < 0 {
		return false, nil
	}
	held, ok := object.Members[index].Value.Value.(*hujson.Object)
	if !ok {
		return false, nil
	}
	if err := mergeJSONObject(held, nested, true); err != nil {
		return false, err
	}

	return true, nil
}

func jsonMemberIndex(object *hujson.Object, key string) (int, error) {
	for index := range object.Members {
		name, err := jsonMemberName(object.Members[index])
		if err != nil {
			return -1, err
		}
		if name == key {
			return index, nil
		}
	}

	return -1, nil
}

func replaceJSONValue(target, built *hujson.Value) error {
	if err := inheritJSONPresentation(target, built); err != nil {
		return err
	}
	target.Value = built.Value

	return nil
}

func inheritJSONPresentation(target, built *hujson.Value) error {
	switch source := target.Value.(type) {
	case *hujson.Object:
		if destination, ok := built.Value.(*hujson.Object); ok {
			return inheritJSONObjectPresentation(source, destination)
		}
	case *hujson.Array:
		if destination, ok := built.Value.(*hujson.Array); ok {
			return inheritJSONArrayPresentation(source, destination)
		}
	}

	return nil
}

func inheritJSONObjectPresentation(source, destination *hujson.Object) error {
	destination.AfterExtra = jsonWhitespaceOnly(source.AfterExtra)
	trailing := jsonHasTrailingComma(source)
	for index := range destination.Members {
		sample, err := jsonObjectStyleSample(source, destination.Members[index], index)
		if err != nil {
			return err
		}
		if sample == nil {
			continue
		}
		member := &destination.Members[index]
		member.Name.BeforeExtra = jsonWhitespaceOnly(sample.Name.BeforeExtra)
		member.Name.AfterExtra = jsonWhitespaceOnly(sample.Name.AfterExtra)
		member.Value.BeforeExtra = jsonWhitespaceOnly(sample.Value.BeforeExtra)
		member.Value.AfterExtra = nil
		if err := inheritJSONPresentation(&sample.Value, &member.Value); err != nil {
			return err
		}
	}
	if trailing && len(destination.Members) > 0 {
		destination.Members[len(destination.Members)-1].Value.AfterExtra = hujson.Extra{}
	}

	return nil
}

func jsonObjectStyleSample(
	source *hujson.Object,
	destination hujson.ObjectMember,
	index int,
) (*hujson.ObjectMember, error) {
	name, err := jsonMemberName(destination)
	if err != nil {
		return nil, err
	}
	matching, err := jsonMemberIndex(source, name)
	if err != nil {
		return nil, err
	}
	if matching >= 0 {
		return &source.Members[matching], nil
	}
	if len(source.Members) == 0 {
		return nil, nil
	}
	if index >= len(source.Members) {
		index = len(source.Members) - 1
	}

	return &source.Members[index], nil
}

func inheritJSONArrayPresentation(source, destination *hujson.Array) error {
	destination.AfterExtra = jsonWhitespaceOnly(source.AfterExtra)
	trailing := jsonArrayHasTrailingComma(source)
	for index := range destination.Elements {
		var sample *hujson.Value
		if index < len(source.Elements) {
			sample = &source.Elements[index]
		} else if len(source.Elements) > 0 {
			sample = &source.Elements[len(source.Elements)-1]
		}
		if sample != nil {
			destination.Elements[index].BeforeExtra = jsonWhitespaceOnly(sample.BeforeExtra)
			destination.Elements[index].AfterExtra = nil
			if err := inheritJSONPresentation(sample, &destination.Elements[index]); err != nil {
				return err
			}
		}
	}
	if trailing && len(destination.Elements) > 0 {
		destination.Elements[len(destination.Elements)-1].AfterExtra = hujson.Extra{}
	}

	return nil
}

func jsonWhitespaceOnly(extra hujson.Extra) hujson.Extra {
	if len(extra) == 0 || jsonExtraHasComment(extra) {
		return nil
	}

	return slices.Clone(extra)
}

func appendJSONMember(object *hujson.Object, key string, value hujson.Value) {
	trailing := jsonHasTrailingComma(object)
	nameExtra, valueExtra := inferJSONObjectSpacing(object)
	name := hujson.Value{Value: hujson.String(key), BeforeExtra: nameExtra}
	value.BeforeExtra = valueExtra
	value.AfterExtra = nil
	object.Members = append(object.Members, hujson.ObjectMember{Name: name, Value: value})
	if trailing {
		object.Members[len(object.Members)-1].Value.AfterExtra = hujson.Extra{}
	}
}

func inferJSONObjectSpacing(object *hujson.Object) (hujson.Extra, hujson.Extra) {
	if len(object.Members) > 0 {
		last := object.Members[len(object.Members)-1]
		return slices.Clone(last.Name.BeforeExtra), slices.Clone(last.Value.BeforeExtra)
	}
	if bytes.Contains(object.AfterExtra, []byte("\n")) {
		return slices.Clone(object.AfterExtra), hujson.Extra(" ")
	}

	return nil, hujson.Extra(" ")
}

func jsonHasTrailingComma(object *hujson.Object) bool {
	return len(object.Members) > 0 && object.Members[len(object.Members)-1].Value.AfterExtra != nil
}

func jsonArrayHasTrailingComma(array *hujson.Array) bool {
	return len(array.Elements) > 0 && array.Elements[len(array.Elements)-1].AfterExtra != nil
}
