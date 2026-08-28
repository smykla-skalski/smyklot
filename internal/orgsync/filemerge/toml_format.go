package filemerge

import (
	"bytes"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2/unstable"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

type tomlArrayPiece struct {
	start   int
	value   *tomlValueRef
	comment *tomlSpan
}

func formatTOMLDocument(content []byte, policy config.FormattingPolicy) ([]byte, error) {
	before, comments, err := decodeTOMLSemantic(content)
	if err != nil {
		return nil, err
	}
	formatted, err := formatTOMLValues(content, policy)
	if err != nil {
		return nil, err
	}
	if policy.TOML.KeyOrder == "sort" {
		formatted, err = sortTOMLAssignments(formatted)
		if err != nil {
			return nil, err
		}
	}
	formatted, err = alignTOMLAssignments(formatted, policy)
	if err != nil {
		return nil, err
	}
	after, afterComments, err := decodeTOMLSemantic(formatted)
	if err != nil {
		return nil, fmt.Errorf("%w: formatted TOML did not parse: %w", ErrUnwritable, err)
	}
	if !tomlSemanticEqual(before, after) {
		return nil, fmt.Errorf("%w: formatting changed the TOML value", ErrUnwritable)
	}
	if !mapsEqual(comments, afterComments) {
		return nil, fmt.Errorf("%w: formatting changed TOML comments", ErrUnwritable)
	}

	return formatted, nil
}

func formatTOMLValues(content []byte, policy config.FormattingPolicy) ([]byte, error) {
	document, err := parseTOMLSyntax(content)
	if err != nil {
		return nil, err
	}
	indent := tomlIndentUnit(content, policy.Common)
	eol := tomlPreferredLineEnding(content, policy.Common)
	arrayEdits := make([]byteEdit, 0)
	changedArrays := make(map[*tomlValueRef]struct{})
	for _, value := range document.values {
		if value.kind != unstable.Array || tomlHasArrayAncestor(value) ||
			!tomlArrayFormattingNeeded(content, value, policy) {
			continue
		}
		replacement, renderErr := renderTOMLArray(content, value, policy, indent, eol, tomlBaseIndent(content, value.span.start))
		if renderErr != nil {
			return nil, renderErr
		}
		if !bytes.Equal(replacement, content[value.span.start:value.span.end]) {
			arrayEdits = append(arrayEdits, byteEdit{
				start: value.span.start, end: value.span.end, replacement: replacement,
			})
			changedArrays[value] = struct{}{}
		}
	}
	quoteEdits, err := tomlQuoteEdits(content, document.values, changedArrays, policy, indent)
	if err != nil {
		return nil, err
	}

	return applyByteEdits(content, append(arrayEdits, quoteEdits...))
}

func tomlQuoteEdits(
	content []byte,
	values []*tomlValueRef,
	changedArrays map[*tomlValueRef]struct{},
	policy config.FormattingPolicy,
	indent string,
) ([]byteEdit, error) {
	if policy.TOML.QuoteStyle == formatPreserve {
		return nil, nil
	}
	edits := make([]byteEdit, 0)
	for _, value := range values {
		if value.kind != unstable.String || tomlInsideChangedArray(value, changedArrays) {
			continue
		}
		decoded, err := decodeTOMLString(content[value.span.start:value.span.end])
		if err != nil {
			return nil, err
		}
		var replacement []byte
		if policy.TOML.QuoteStyle == "prefer_basic" {
			replacement, err = encodeTOMLBasicString(decoded)
		} else {
			replacement, err = encodeTOMLValue(decoded, false, indent)
		}
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(replacement, content[value.span.start:value.span.end]) {
			edits = append(edits, byteEdit{
				start: value.span.start, end: value.span.end, replacement: replacement,
			})
		}
	}

	return edits, nil
}

func tomlArrayFormattingNeeded(content []byte, value *tomlValueRef, policy config.FormattingPolicy) bool {
	if policy.TOML.Arrays != formatPreserve || policy.TOML.TrailingCommas != formatPreserve {
		return true
	}

	return policy.Common.IndentStyle != formatPreserve &&
		bytes.ContainsAny(content[value.span.start:value.span.end], "\r\n")
}

func renderTOMLArray(
	content []byte,
	value *tomlValueRef,
	policy config.FormattingPolicy,
	indent, eol, baseIndent string,
) ([]byte, error) {
	if policy.Common.LineEnding == formatPreserve {
		if nearby := tomlNearbyLineEnding(content, value.span); nearby != "" {
			eol = nearby
		}
	}
	children := make([][]byte, len(value.children))
	for index, child := range value.children {
		rendered, err := renderTOMLArrayChild(
			content, child, policy, indent, eol, baseIndent+indent,
		)
		if err != nil {
			return nil, err
		}
		children[index] = rendered
	}
	choice := tomlArrayLayout(content, value, children, policy)
	if choice == formatCompact {
		if len(value.comments) > 0 || tomlChildrenMultiline(children) {
			return nil, fmt.Errorf("%w: compact TOML arrays cannot preserve comments or multiline values safely", ErrUnwritable)
		}
		return compactTOMLArray(content, value, children, policy), nil
	}
	if choice == formatExpanded {
		return expandTOMLArray(content, value, children, policy, indent, eol, baseIndent), nil
	}
	if policy.TOML.TrailingCommas != formatPreserve {
		return formatPreservedTOMLTrailingComma(content, value, policy), nil
	}

	return bytes.Clone(content[value.span.start:value.span.end]), nil
}

func tomlNearbyLineEnding(content []byte, span tomlSpan) string {
	if nearby := firstLineEnding(content[span.start:span.end]); nearby != "" {
		return nearby
	}

	return firstLineEnding(content[span.end:])
}

func renderTOMLArrayChild(
	content []byte,
	child *tomlValueRef,
	policy config.FormattingPolicy,
	indent, eol, baseIndent string,
) ([]byte, error) {
	if child.kind == unstable.Array && tomlArrayFormattingNeeded(content, child, policy) {
		return renderTOMLArray(content, child, policy, indent, eol, baseIndent)
	}
	if child.kind != unstable.String || policy.TOML.QuoteStyle == formatPreserve {
		return bytes.Clone(content[child.span.start:child.span.end]), nil
	}
	decoded, err := decodeTOMLString(content[child.span.start:child.span.end])
	if err != nil {
		return nil, err
	}
	if policy.TOML.QuoteStyle == "prefer_basic" {
		return encodeTOMLBasicString(decoded)
	}

	return encodeTOMLValue(decoded, false, indent)
}

func formatPreservedTOMLTrailingComma(
	content []byte,
	value *tomlValueRef,
	policy config.FormattingPolicy,
) []byte {
	raw := bytes.Clone(content[value.span.start:value.span.end])
	if len(value.children) == 0 {
		return raw
	}
	lastEnd := value.children[len(value.children)-1].span.end
	segmentStart := lastEnd - value.span.start
	segment := raw[segmentStart : len(raw)-1]
	comma := tomlCommaOutsideComment(segment)
	switch policy.TOML.TrailingCommas {
	case formatRemove:
		if comma >= 0 {
			return slices.Concat(raw[:segmentStart+comma], raw[segmentStart+comma+1:])
		}
	case formatMultiline:
		if comma < 0 && bytes.ContainsAny(raw, "\r\n") {
			return slices.Concat(raw[:segmentStart], []byte{','}, raw[segmentStart:])
		}
	}

	return raw
}

func tomlArrayLayout(
	content []byte,
	value *tomlValueRef,
	children [][]byte,
	policy config.FormattingPolicy,
) string {
	choice := policy.TOML.Arrays
	if choice == formatPreserve && policy.Common.IndentStyle != formatPreserve &&
		bytes.ContainsAny(content[value.span.start:value.span.end], "\r\n") {
		return formatExpanded
	}
	if choice != formatAuto {
		return choice
	}
	if len(value.comments) > 0 || tomlChildrenMultiline(children) {
		return formatExpanded
	}
	compact := compactTOMLArray(content, value, children, policy)
	if tomlArrayStartColumn(content, value.span.start)+utf8.RuneCount(compact) <=
		policy.Common.LineWidth {
		return formatCompact
	}

	return formatExpanded
}

func tomlArrayStartColumn(content []byte, offset int) int {
	lineStart := bytes.LastIndexByte(content[:offset], '\n') + 1

	return utf8.RuneCount(content[lineStart:offset])
}

func compactTOMLArray(
	content []byte,
	value *tomlValueRef,
	children [][]byte,
	policy config.FormattingPolicy,
) []byte {
	var output bytes.Buffer
	output.WriteByte('[')
	for index, child := range children {
		if index > 0 {
			output.WriteString(", ")
		}
		output.Write(child)
	}
	if len(children) > 0 && policy.TOML.TrailingCommas == formatPreserve && tomlHasTrailingComma(content, value) {
		output.WriteByte(',')
	}
	output.WriteByte(']')

	return output.Bytes()
}

func expandTOMLArray(
	content []byte,
	value *tomlValueRef,
	children [][]byte,
	policy config.FormattingPolicy,
	indent, eol, baseIndent string,
) []byte {
	pieces := tomlArrayPieces(value)
	childIndex := make(map[*tomlValueRef]int, len(value.children))
	for index, child := range value.children {
		childIndex[child] = index
	}
	entryIndent := baseIndent + indent
	lines := make([][]byte, 0, len(pieces))
	lastValueLine := -1
	valueSeen := 0
	for _, piece := range pieces {
		if piece.comment != nil {
			comment := bytes.Clone(content[piece.comment.start:piece.comment.end])
			if lastValueLine >= 0 && tomlSameLine(content, value.children[valueSeen-1].span.end, piece.comment.start) {
				lines[lastValueLine] = append(lines[lastValueLine], ' ')
				lines[lastValueLine] = append(lines[lastValueLine], comment...)
			} else {
				lines = append(lines, append([]byte(entryIndent), comment...))
			}
			continue
		}
		index := childIndex[piece.value]
		line := append([]byte(entryIndent), children[index]...)
		valueSeen++
		if valueSeen < len(value.children) || tomlExpandedTrailingComma(content, value, policy) {
			line = append(line, ',')
		}
		lines = append(lines, line)
		lastValueLine = len(lines) - 1
	}
	if len(lines) == 0 {
		return []byte("[]")
	}

	return concatBytes([]byte{'['}, []byte(eol), bytes.Join(lines, []byte(eol)),
		[]byte(eol), []byte(baseIndent), []byte{']'})
}

func tomlArrayPieces(value *tomlValueRef) []tomlArrayPiece {
	pieces := make([]tomlArrayPiece, 0, len(value.children)+len(value.comments))
	for _, child := range value.children {
		pieces = append(pieces, tomlArrayPiece{start: child.span.start, value: child})
	}
	for index := range value.comments {
		comment := &value.comments[index]
		pieces = append(pieces, tomlArrayPiece{start: comment.start, comment: comment})
	}
	slices.SortFunc(pieces, func(one, two tomlArrayPiece) int { return one.start - two.start })

	return pieces
}

func tomlExpandedTrailingComma(content []byte, value *tomlValueRef, policy config.FormattingPolicy) bool {
	switch policy.TOML.TrailingCommas {
	case "multiline":
		return true
	case formatRemove:
		return false
	default:
		return tomlHasTrailingComma(content, value)
	}
}

func tomlHasTrailingComma(content []byte, value *tomlValueRef) bool {
	start := value.span.start + 1
	if len(value.children) > 0 {
		start = value.children[len(value.children)-1].span.end
	}
	segment := content[start : value.span.end-1]

	return tomlCommaOutsideComment(segment) >= 0
}

func tomlCommaOutsideComment(segment []byte) int {
	inComment := false
	for index, character := range segment {
		switch character {
		case '#':
			inComment = true
		case '\r', '\n':
			inComment = false
		case ',':
			if !inComment {
				return index
			}
		}
	}

	return -1
}

func tomlChildrenMultiline(children [][]byte) bool {
	for _, child := range children {
		if bytes.ContainsAny(child, "\r\n") {
			return true
		}
	}

	return false
}

func tomlHasArrayAncestor(value *tomlValueRef) bool {
	for parent := value.parent; parent != nil; parent = parent.parent {
		if parent.kind == unstable.Array {
			return true
		}
	}

	return false
}

func tomlInsideChangedArray(value *tomlValueRef, changed map[*tomlValueRef]struct{}) bool {
	for parent := value.parent; parent != nil; parent = parent.parent {
		if _, ok := changed[parent]; ok {
			return true
		}
	}

	return false
}

func tomlBaseIndent(content []byte, offset int) string {
	start := bytes.LastIndexByte(content[:offset], '\n') + 1
	end := start
	for end < len(content) && (content[end] == ' ' || content[end] == '\t') {
		end++
	}

	return string(content[start:end])
}

func tomlSameLine(content []byte, first, second int) bool {
	return !bytes.ContainsAny(content[first:second], "\r\n")
}

func tomlIndentUnit(content []byte, common config.FormattingCommonPolicy) string {
	if common.IndentStyle == formatTabs {
		return "\t"
	}
	if common.IndentStyle == formatSpaces {
		return strings.Repeat(" ", common.IndentWidth)
	}

	return strings.Repeat(" ", tomlDetectedIndent(content, common.IndentWidth))
}

func tomlDetectedIndent(content []byte, fallback int) int {
	minimum := 0
	for _, line := range bytes.Split(content, []byte{'\n'}) {
		spaces := len(line) - len(bytes.TrimLeft(line, " "))
		if spaces > 0 && (minimum == 0 || spaces < minimum) {
			minimum = spaces
		}
	}
	if minimum > 0 {
		return minimum
	}

	return fallback
}

func tomlPreferredLineEnding(content []byte, common config.FormattingCommonPolicy) string {
	if common.LineEnding == lineEndingCRLF {
		return crlf
	}
	if common.LineEnding == lineEndingLF {
		return lf
	}

	return map[bool]string{true: crlf, false: lf}[dominantLineEnding(content) == lineEndingCRLF]
}

func mapsEqual[K comparable, V any](one, two map[K]V) bool {
	return reflect.DeepEqual(one, two)
}
