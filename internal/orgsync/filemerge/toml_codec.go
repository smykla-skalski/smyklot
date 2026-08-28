package filemerge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

type tomlFragment struct {
	Value any `toml:"__smyklot_value__,inline"`
}

type tomlRawFragment struct {
	Value unstable.RawMessage `toml:"__smyklot_value__"`
}

func decodeTOMLSemantic(content []byte) (map[string]any, map[string]int, error) {
	syntax, err := parseTOMLSyntax(content)
	if err != nil {
		return nil, nil, err
	}
	var document map[string]any
	if err := toml.Unmarshal(content, &document); err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}
	if document == nil {
		document = map[string]any{}
	}
	normalized, ok := normalizeTOMLSemanticCollections(document).(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("%w: TOML document holds no object", ErrUnreadable)
	}
	document = normalized

	return document, syntax.comments, nil
}

func normalizeTOMLSemanticCollections(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		normalized := make(map[string]any, len(typed))
		for key, child := range typed {
			normalized[key] = normalizeTOMLSemanticCollections(child)
		}

		return normalized
	case []any:
		normalized := make([]any, len(typed))
		for index, child := range typed {
			normalized[index] = normalizeTOMLSemanticCollections(child)
		}

		return normalized
	case []map[string]any:
		normalized := make([]any, len(typed))
		for index, child := range typed {
			normalized[index] = normalizeTOMLSemanticCollections(child)
		}

		return normalized
	default:
		return value
	}
}

func normalizeTOMLOverride(value any) (any, error) {
	return normalizeTOMLOverrideAtPath(value, nil)
}

func normalizeTOMLOverrideAtPath(value any, path []string) (any, error) {
	switch typed := value.(type) {
	case json.Number:
		text := typed.String()
		if !strings.ContainsAny(text, ".eE") {
			integer, err := strconv.ParseInt(text, 10, 64)
			if err != nil {
				return nil, fmt.Errorf(
					"%w: TOML integer at %s must fit in 64 bits: %s",
					ErrUnwritable, displayTOMLPath(path), text,
				)
			}

			return integer, nil
		}
		floating, err := strconv.ParseFloat(text, 64)
		if err != nil || math.IsInf(floating, 0) || math.IsNaN(floating) {
			return nil, fmt.Errorf(
				"%w: TOML cannot hold JSON number %s at %s",
				ErrUnwritable, text, displayTOMLPath(path),
			)
		}

		return floating, nil
	case map[string]any:
		return normalizeTOMLOverrideMap(typed, path)
	case []any:
		return normalizeTOMLOverrideList(typed, path)
	default:
		return value, nil
	}
}

func normalizeTOMLOverrideMap(value map[string]any, path []string) (map[string]any, error) {
	result := make(map[string]any, len(value))
	for key, child := range value {
		normalized, err := normalizeTOMLOverrideAtPath(child, appendPath(path, key))
		if err != nil {
			return nil, err
		}
		result[key] = normalized
	}

	return result, nil
}

func normalizeTOMLOverrideList(value []any, path []string) ([]any, error) {
	result := make([]any, len(value))
	for index, child := range value {
		normalized, err := normalizeTOMLOverrideAtPath(
			child, appendPath(path, fmt.Sprintf("[%d]", index)),
		)
		if err != nil {
			return nil, err
		}
		result[index] = normalized
	}

	return result, nil
}

func rejectTOMLDatetimeReplacement(base, override any, path []string) error {
	baseMap, baseIsMap := base.(map[string]any)
	overrideMap, overrideIsMap := override.(map[string]any)
	if baseIsMap && overrideIsMap {
		for key, replacement := range overrideMap {
			if replacement == nil {
				continue
			}
			if existing, ok := baseMap[key]; ok {
				if err := rejectTOMLDatetimeReplacement(
					existing, replacement, appendPath(path, key),
				); err != nil {
					return err
				}
			}
		}

		return nil
	}
	if isTOMLDatetime(base) {
		return fmt.Errorf(
			"%w: JSON overrides cannot replace TOML date or time at %s",
			ErrUnwritable, displayTOMLPath(path),
		)
	}

	return nil
}

func isTOMLDatetime(value any) bool {
	switch value.(type) {
	case time.Time, toml.LocalDate, toml.LocalTime, toml.LocalDateTime:
		return true
	default:
		return false
	}
}

func appendPath(path []string, key string) []string {
	result := make([]string, len(path), len(path)+1)
	copy(result, path)

	return append(result, key)
}

func displayTOMLPath(path []string) string {
	if len(path) == 0 {
		return "$"
	}
	var result strings.Builder
	result.WriteByte('$')
	for _, component := range path {
		if strings.HasPrefix(component, "[") {
			result.WriteString(component)
			continue
		}
		result.WriteByte('.')
		result.WriteString(component)
	}

	return result.String()
}

func encodeTOMLValue(value any, multilineArrays bool, indent string) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := toml.NewEncoder(&buffer).
		SetTablesInline(true).
		SetArraysMultiline(multilineArrays).
		SetIndentSymbol(indent)
	if err := encoder.Encode(tomlFragment{Value: value}); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnwritable, err)
	}
	syntax, err := parseTOMLSyntax(buffer.Bytes())
	if err != nil {
		return nil, fmt.Errorf("%w: generated TOML value did not parse: %w", ErrUnwritable, err)
	}
	root, err := oneTOMLRootAssignment(syntax, "__smyklot_value__")
	if err != nil {
		return nil, err
	}
	span := root.value.span

	return bytes.Clone(buffer.Bytes()[span.start:span.end]), nil
}

func encodeTOMLDocument(value any, multilineArrays bool, indent string) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := toml.NewEncoder(&buffer).
		SetArraysMultiline(multilineArrays).
		SetIndentSymbol(indent)
	if err := encoder.Encode(value); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnwritable, err)
	}
	if _, err := parseTOMLSyntax(buffer.Bytes()); err != nil {
		return nil, fmt.Errorf("%w: generated TOML document did not parse: %w", ErrUnwritable, err)
	}

	return buffer.Bytes(), nil
}

func encodeTOMLBasicString(value string) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnwritable, err)
	}
	encoded := bytes.TrimSuffix(buffer.Bytes(), []byte{'\n'})
	fragment, err := wrapTOMLRawValue(encoded)
	if err != nil {
		return nil, fmt.Errorf("%w: string has no safe basic TOML spelling: %w", ErrUnwritable, err)
	}
	span := fragment.assignments[0].value.span

	return bytes.Clone(fragment.content[span.start:span.end]), nil
}

func decodeTOMLString(raw []byte) (string, error) {
	fragment, err := wrapTOMLRawValue(raw)
	if err != nil {
		return "", err
	}
	var decoded struct {
		Value string `toml:"__smyklot_value__"`
	}
	if err := toml.Unmarshal(fragment.content, &decoded); err != nil {
		return "", fmt.Errorf("%w: %w", ErrUnreadable, err)
	}

	return decoded.Value, nil
}

func wrapTOMLRawValue(raw []byte) (*tomlSyntaxDocument, error) {
	var buffer bytes.Buffer
	encoder := toml.NewEncoder(&buffer).EnableMarshalerInterface()
	if err := encoder.Encode(tomlRawFragment{Value: unstable.RawMessage(raw)}); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnwritable, err)
	}
	document, err := parseTOMLSyntax(buffer.Bytes())
	if err != nil {
		return nil, err
	}
	if _, err := oneTOMLRootAssignment(document, "__smyklot_value__"); err != nil {
		return nil, fmt.Errorf("%w: TOML encoder did not produce one raw value: %w", ErrUnwritable, err)
	}

	return document, nil
}

func oneTOMLRootAssignment(document *tomlSyntaxDocument, key string) (*tomlAssignmentRef, error) {
	var root *tomlAssignmentRef
	for index := range document.assignments {
		assignment := &document.assignments[index]
		if assignment.inline || !slices.Equal(assignment.path, []string{key}) {
			continue
		}
		if root != nil {
			return nil, fmt.Errorf("%w: TOML encoder produced multiple root values", ErrUnwritable)
		}
		root = assignment
	}
	if root == nil {
		return nil, fmt.Errorf("%w: TOML encoder did not produce one value", ErrUnwritable)
	}

	return root, nil
}

func tomlSemanticEqual(one, other any) bool {
	if left, ok := one.(float64); ok {
		right, sameType := other.(float64)

		return sameType && (left == right || math.IsNaN(left) && math.IsNaN(right))
	}
	switch left := one.(type) {
	case map[string]any:
		return tomlMapsEqual(left, other)
	case []any:
		return tomlListsEqual(left, other)
	default:
		return reflect.DeepEqual(one, other)
	}
}

func tomlMapsEqual(left map[string]any, other any) bool {
	right, ok := other.(map[string]any)
	if !ok || len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if counterpart, exists := right[key]; !exists || !tomlSemanticEqual(value, counterpart) {
			return false
		}
	}

	return true
}

func tomlListsEqual(left []any, other any) bool {
	right, ok := other.([]any)
	if !ok || len(left) != len(right) {
		return false
	}
	for index, value := range left {
		if !tomlSemanticEqual(value, right[index]) {
			return false
		}
	}

	return true
}
