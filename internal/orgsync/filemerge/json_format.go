package filemerge

import (
	"bytes"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/tailscale/hujson"
)

type jsonFormatContext struct {
	policy     config.FormattingPolicy
	indent     string
	defaultEOL string
	jsonc      bool
}

func formatJSONDocument(
	content []byte,
	jsonc bool,
	policy config.FormattingPolicy,
) ([]byte, error) {
	root, err := parseJSONSyntax(content, jsonc)
	if err != nil {
		return nil, err
	}
	before, err := jsonSyntaxValue(root)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}
	commentsBefore := jsonCommentCounts(root)

	ctx := jsonFormatContext{
		policy:     policy,
		indent:     jsonIndentUnit(content, policy.Common),
		defaultEOL: jsonDefaultLineEnding(content, policy.Common),
		jsonc:      jsonc,
	}
	if err := ctx.formatValue(root, 0); err != nil {
		return nil, err
	}
	if jsonc {
		setJSONCTrailingCommas(root, policy.JSONC.TrailingCommas)
	}

	formatted := root.Pack()
	afterRoot, err := parseJSONSyntax(formatted, jsonc)
	if err != nil {
		return nil, fmt.Errorf("%w: formatted JSON did not parse: %w", ErrUnwritable, err)
	}
	after, err := jsonSyntaxValue(afterRoot)
	if err != nil || !holdsEqual([]any{before}, after) {
		return nil, fmt.Errorf("%w: formatting changed the JSON value", ErrUnwritable)
	}
	if jsonc && !sameJSONComments(commentsBefore, afterRoot) {
		return nil, fmt.Errorf("%w: formatting changed JSONC comments", ErrUnwritable)
	}

	return formatted, nil
}

func (ctx jsonFormatContext) formatValue(value *hujson.Value, depth int) error {
	switch node := value.Value.(type) {
	case *hujson.Object:
		if ctx.policy.JSON.KeyOrder == "sort" {
			if err := sortJSONObjectMembers(node); err != nil {
				return err
			}
		}
		for index := range node.Members {
			if err := ctx.formatValue(&node.Members[index].Value, depth+1); err != nil {
				return err
			}
		}

		return ctx.formatObject(node, depth)

	case *hujson.Array:
		for index := range node.Elements {
			if err := ctx.formatValue(&node.Elements[index], depth+1); err != nil {
				return err
			}
		}

		return ctx.formatArray(node, depth)
	}

	return nil
}

func (ctx jsonFormatContext) formatObject(object *hujson.Object, depth int) error {
	value := objectValue(object)

	return ctx.formatCollection(
		ctx.policy.JSON.Objects,
		value,
		depth,
		jsonObjectHasComments(object),
		"objects",
		func() { compactJSONObject(object) },
		func() { expandJSONObject(object, depth, ctx.indent, ctx.lineEnding(value)) },
	)
}

func (ctx jsonFormatContext) formatArray(array *hujson.Array, depth int) error {
	value := arrayValue(array)

	return ctx.formatCollection(
		ctx.policy.JSON.Arrays,
		value,
		depth,
		jsonArrayHasComments(array),
		"arrays",
		func() { compactJSONArray(array) },
		func() { expandJSONArray(array, depth, ctx.indent, ctx.lineEnding(value)) },
	)
}

func (ctx jsonFormatContext) formatCollection(
	configured string,
	value hujson.Value,
	depth int,
	hasComments bool,
	kind string,
	compact, expand func(),
) error {
	choice := ctx.collectionLayout(configured, value, depth)
	switch choice {
	case formatCompact:
		if hasComments {
			return fmt.Errorf(
				"%w: compact JSON %s cannot preserve comments safely", ErrUnwritable, kind,
			)
		}
		compact()

	case formatExpanded:
		expand()

	case formatPreserve:
		if commonIndentationActive(ctx.policy) && bytes.Contains(value.Pack(), []byte("\n")) {
			expand()
		}
	}

	return nil
}

func (ctx jsonFormatContext) lineEnding(value hujson.Value) string {
	if ctx.policy.Common.LineEnding == lineEndingCRLF {
		return crlf
	}
	if ctx.policy.Common.LineEnding == lineEndingLF {
		return lf
	}

	packed := value.Pack()
	if ending := firstLineEnding(packed); ending != "" {
		return ending
	}

	return ctx.defaultEOL
}

func (ctx jsonFormatContext) collectionLayout(choice string, value hujson.Value, depth int) string {
	if choice != formatAuto {
		return choice
	}
	if jsonValueHasComments(&value) {
		return formatExpanded
	}

	copy := value.Clone()
	compactJSONValue(&copy)
	if depth*len(ctx.indent)+len(copy.Pack()) <= ctx.policy.Common.LineWidth {
		return formatCompact
	}

	return formatExpanded
}

func compactJSONObject(object *hujson.Object) {
	trailing := jsonHasTrailingComma(object)
	for index := range object.Members {
		member := &object.Members[index]
		if index == 0 {
			member.Name.BeforeExtra = nil
		} else {
			member.Name.BeforeExtra = hujson.Extra(" ")
		}
		member.Name.AfterExtra = nil
		member.Value.BeforeExtra = hujson.Extra(" ")
		member.Value.AfterExtra = nil
	}
	object.AfterExtra = nil
	if trailing && len(object.Members) > 0 {
		object.Members[len(object.Members)-1].Value.AfterExtra = hujson.Extra{}
	}
}

func compactJSONArray(array *hujson.Array) {
	trailing := jsonArrayHasTrailingComma(array)
	for index := range array.Elements {
		if index == 0 {
			array.Elements[index].BeforeExtra = nil
		} else {
			array.Elements[index].BeforeExtra = hujson.Extra(" ")
		}
		array.Elements[index].AfterExtra = nil
	}
	array.AfterExtra = nil
	if trailing && len(array.Elements) > 0 {
		array.Elements[len(array.Elements)-1].AfterExtra = hujson.Extra{}
	}
}

func compactJSONValue(value *hujson.Value) {
	switch node := value.Value.(type) {
	case *hujson.Object:
		for index := range node.Members {
			compactJSONValue(&node.Members[index].Value)
		}
		compactJSONObject(node)
	case *hujson.Array:
		for index := range node.Elements {
			compactJSONValue(&node.Elements[index])
		}
		compactJSONArray(node)
	}
}

func expandJSONObject(object *hujson.Object, depth int, indent, eol string) {
	if len(object.Members) == 0 {
		return
	}
	entryPrefix := eol + strings.Repeat(indent, depth+1)
	closePrefix := eol + strings.Repeat(indent, depth)
	for index := range object.Members {
		member := &object.Members[index]
		member.Name.BeforeExtra = formatJSONExtra(member.Name.BeforeExtra, entryPrefix)
		if !jsonExtraHasComment(member.Name.AfterExtra) {
			member.Name.AfterExtra = nil
		}
		if !jsonExtraHasComment(member.Value.BeforeExtra) {
			member.Value.BeforeExtra = hujson.Extra(" ")
		}
	}
	object.AfterExtra = formatJSONExtra(object.AfterExtra, closePrefix)
}

func expandJSONArray(array *hujson.Array, depth int, indent, eol string) {
	if len(array.Elements) == 0 {
		return
	}
	entryPrefix := eol + strings.Repeat(indent, depth+1)
	closePrefix := eol + strings.Repeat(indent, depth)
	for index := range array.Elements {
		array.Elements[index].BeforeExtra = formatJSONExtra(
			array.Elements[index].BeforeExtra, entryPrefix)
	}
	array.AfterExtra = formatJSONExtra(array.AfterExtra, closePrefix)
}

func formatJSONExtra(extra hujson.Extra, prefix string) hujson.Extra {
	comments := jsonComments(extra)
	if len(comments) == 0 {
		return hujson.Extra(prefix)
	}

	var output strings.Builder
	for _, comment := range comments {
		output.WriteString(prefix)
		output.Write(comment)
	}
	output.WriteString(prefix)

	return hujson.Extra(output.String())
}

func jsonComments(extra hujson.Extra) [][]byte {
	var comments [][]byte
	for at := 0; at < len(extra); {
		switch {
		case at+1 < len(extra) && extra[at] == '/' && extra[at+1] == '/':
			newline := bytes.IndexByte(extra[at:], '\n')
			if newline < 0 {
				comments = append(comments, slices.Clone(extra[at:]))
				at = len(extra)

				continue
			}
			end := newline
			if end > 0 && extra[at+end-1] == '\r' {
				end--
			}
			comments = append(comments, slices.Clone(extra[at:at+end]))
			at += newline + 1
		case at+1 < len(extra) && extra[at] == '/' && extra[at+1] == '*':
			end := bytes.Index(extra[at+2:], []byte("*/"))
			if end < 0 {
				return comments
			}
			end += 4
			comments = append(comments, slices.Clone(extra[at:at+end]))
			at += end
		default:
			at++
		}
	}

	return comments
}

func setJSONCTrailingCommas(value *hujson.Value, choice string) {
	switch node := value.Value.(type) {
	case *hujson.Object:
		for index := range node.Members {
			setJSONCTrailingCommas(&node.Members[index].Value, choice)
		}
		if len(node.Members) > 0 {
			setJSONTrailing(&node.Members[len(node.Members)-1].Value, &node.AfterExtra, choice)
		}
	case *hujson.Array:
		for index := range node.Elements {
			setJSONCTrailingCommas(&node.Elements[index], choice)
		}
		if len(node.Elements) > 0 {
			setJSONTrailing(&node.Elements[len(node.Elements)-1], &node.AfterExtra, choice)
		}
	}
}

func setJSONTrailing(last *hujson.Value, closing *hujson.Extra, choice string) {
	switch choice {
	case formatInsert:
		if last.AfterExtra == nil {
			last.AfterExtra = hujson.Extra{}
		}
	case formatRemove:
		if last.AfterExtra != nil {
			*closing = append(slices.Clone(last.AfterExtra), (*closing)...)
			last.AfterExtra = nil
		}
	}
}

func objectValue(object *hujson.Object) hujson.Value { return hujson.Value{Value: object} }
func arrayValue(array *hujson.Array) hujson.Value    { return hujson.Value{Value: array} }

func jsonMemberName(member hujson.ObjectMember) (string, error) {
	literal, ok := member.Name.Value.(hujson.Literal)
	if !ok || literal.Kind() != '"' {
		return "", fmt.Errorf("%w: JSON object contains a non-string member name", ErrUnreadable)
	}

	return literal.String(), nil
}

type namedJSONMember struct {
	name   string
	member hujson.ObjectMember
}

func sortJSONObjectMembers(object *hujson.Object) error {
	named := make([]namedJSONMember, len(object.Members))
	for index, member := range object.Members {
		name, err := jsonMemberName(member)
		if err != nil {
			return err
		}
		named[index] = namedJSONMember{name: name, member: member}
	}
	sort.SliceStable(named, func(one, two int) bool {
		return named[one].name < named[two].name
	})
	for index := range named {
		object.Members[index] = named[index].member
	}

	return nil
}

func jsonExtraHasComment(extra hujson.Extra) bool { return len(jsonComments(extra)) > 0 }

func jsonObjectHasComments(object *hujson.Object) bool {
	return jsonValueHasComments(&hujson.Value{Value: object})
}

func jsonArrayHasComments(array *hujson.Array) bool {
	return jsonValueHasComments(&hujson.Value{Value: array})
}

func jsonValueHasComments(value *hujson.Value) bool {
	if jsonExtraHasComment(value.BeforeExtra) || jsonExtraHasComment(value.AfterExtra) {
		return true
	}
	switch node := value.Value.(type) {
	case *hujson.Object:
		return jsonObjectHasNestedComments(node)
	case *hujson.Array:
		return jsonArrayHasNestedComments(node)
	}

	return false
}

func jsonObjectHasNestedComments(object *hujson.Object) bool {
	if jsonExtraHasComment(object.AfterExtra) {
		return true
	}
	for index := range object.Members {
		if jsonValueHasComments(&object.Members[index].Name) ||
			jsonValueHasComments(&object.Members[index].Value) {
			return true
		}
	}

	return false
}

func jsonArrayHasNestedComments(array *hujson.Array) bool {
	if jsonExtraHasComment(array.AfterExtra) {
		return true
	}
	for index := range array.Elements {
		if jsonValueHasComments(&array.Elements[index]) {
			return true
		}
	}

	return false
}

func sameJSONComments(before map[string]int, after *hujson.Value) bool {
	afterCounts := jsonCommentCounts(after)
	if len(before) != len(afterCounts) {
		return false
	}
	for comment, count := range before {
		if afterCounts[comment] != count {
			return false
		}
	}

	return true
}

func jsonCommentCounts(value *hujson.Value) map[string]int {
	counts := make(map[string]int)
	collectJSONCommentCounts(value, counts)

	return counts
}

func collectJSONCommentCounts(value *hujson.Value, counts map[string]int) {
	countJSONExtraComments(value.BeforeExtra, counts)
	countJSONExtraComments(value.AfterExtra, counts)
	switch node := value.Value.(type) {
	case *hujson.Object:
		countJSONExtraComments(node.AfterExtra, counts)
		for index := range node.Members {
			collectJSONCommentCounts(&node.Members[index].Name, counts)
			collectJSONCommentCounts(&node.Members[index].Value, counts)
		}
	case *hujson.Array:
		countJSONExtraComments(node.AfterExtra, counts)
		for index := range node.Elements {
			collectJSONCommentCounts(&node.Elements[index], counts)
		}
	}
}

func countJSONExtraComments(extra hujson.Extra, counts map[string]int) {
	for _, comment := range jsonComments(extra) {
		counts[string(comment)]++
	}
}

func jsonIndentUnit(content []byte, common config.FormattingCommonPolicy) string {
	if common.IndentStyle == formatTabs {
		return "\t"
	}
	if common.IndentStyle == formatSpaces {
		return strings.Repeat(" ", common.IndentWidth)
	}
	minimumSpaces := 0
	for _, line := range bytes.Split(content, []byte("\n")) {
		trimmed := bytes.TrimLeft(line, " \t")
		if len(trimmed) == len(line) || len(trimmed) == 0 {
			continue
		}
		leading := line[:len(line)-len(trimmed)]
		if leading[0] == '\t' {
			return "\t"
		}
		if minimumSpaces == 0 || len(leading) < minimumSpaces {
			minimumSpaces = len(leading)
		}
	}
	if minimumSpaces > 0 {
		return strings.Repeat(" ", minimumSpaces)
	}
	return strings.Repeat(" ", common.IndentWidth)
}

func jsonDefaultLineEnding(content []byte, common config.FormattingCommonPolicy) string {
	switch common.LineEnding {
	case lineEndingCRLF:
		return crlf
	case lineEndingLF:
		return lf
	default:
		if dominantLineEnding(content) == lineEndingCRLF {
			return crlf
		}
		return lf
	}
}

func firstLineEnding(content []byte) string {
	for index, current := range content {
		if current != '\n' {
			continue
		}
		if index > 0 && content[index-1] == '\r' {
			return crlf
		}
		return lf
	}

	return ""
}
