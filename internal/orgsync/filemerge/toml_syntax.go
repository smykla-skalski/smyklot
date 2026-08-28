package filemerge

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2/unstable"
)

type tomlSpan struct {
	start int
	end   int
}

type tomlValueRef struct {
	kind     unstable.Kind
	span     tomlSpan
	parent   *tomlValueRef
	children []*tomlValueRef
	comments []tomlSpan
}

type tomlAssignmentRef struct {
	path       []string
	value      *tomlValueRef
	expression tomlSpan
	line       tomlSpan
	comment    *tomlSpan
	arrayTable bool
	inline     bool
}

type tomlSectionRef struct {
	path       []string
	arrayTable bool
	header     tomlSpan
	key        tomlSpan
	bodyStart  int
	end        int
}

type tomlSyntaxDocument struct {
	content      []byte
	values       []*tomlValueRef
	assignments  []tomlAssignmentRef
	sections     []tomlSectionRef
	comments     map[string]int
	commentSpans []tomlSpan
}

func parseTOMLSyntax(content []byte) (*tomlSyntaxDocument, error) {
	document := &tomlSyntaxDocument{
		content: content, comments: make(map[string]int),
		sections: []tomlSectionRef{{bodyStart: 0, end: len(content)}},
	}
	parser := unstable.Parser{KeepComments: true}
	parser.Reset(content)
	var tablePath []string
	arrayTable := false
	for parser.NextExpression() {
		expression := parser.Expression()
		if expression == nil {
			continue
		}
		collectTOMLComments(parser.Data(), expression, document.comments, &document.commentSpans)
		if sibling := expression.Next(); sibling != nil {
			collectTOMLComments(parser.Data(), sibling, document.comments, &document.commentSpans)
		}
		switch expression.Kind {
		case unstable.Table, unstable.ArrayTable:
			section, err := tomlSectionFromNode(content, expression)
			if err != nil {
				return nil, err
			}
			document.sections = append(document.sections, section)
			tablePath = slices.Clone(section.path)
			arrayTable = section.arrayTable
		case unstable.KeyValue:
			assignment, err := tomlAssignment(content, expression, tablePath, arrayTable)
			if err != nil {
				return nil, err
			}
			document.assignments = append(document.assignments, assignment)
			inline, err := tomlInlineAssignments(
				content, expression.Value(), assignment.value, assignment.path, arrayTable,
			)
			if err != nil {
				return nil, err
			}
			document.assignments = append(document.assignments, inline...)
			collectTOMLValues(assignment.value, &document.values)
		}
	}
	if err := parser.Error(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}
	for index := range document.sections[:len(document.sections)-1] {
		document.sections[index].end = document.sections[index+1].header.start
	}

	return document, nil
}

func tomlSectionFromNode(content []byte, node *unstable.Node) (tomlSectionRef, error) {
	path := tomlNodeKey(node)
	keyNode := node.Child()
	if keyNode == nil {
		return tomlSectionRef{}, fmt.Errorf("%w: TOML table has no key", ErrUnreadable)
	}
	key, err := checkedTOMLRange(content, keyNode.Raw)
	if err != nil {
		return tomlSectionRef{}, err
	}
	header, err := tomlHeaderSpan(content, node, path)
	if err != nil {
		return tomlSectionRef{}, err
	}

	return tomlSectionRef{
		path: slices.Clone(path), arrayTable: node.Kind == unstable.ArrayTable,
		header: header, key: key, bodyStart: header.end, end: len(content),
	}, nil
}

func tomlInlineAssignments(
	content []byte,
	node *unstable.Node,
	ref *tomlValueRef,
	path []string,
	arrayTable bool,
) ([]tomlAssignmentRef, error) {
	if node.Kind != unstable.InlineTable {
		return nil, nil
	}
	assignments := make([]tomlAssignmentRef, 0)
	childIndex := 0
	children := node.Children()
	for children.Next() {
		child := children.Node()
		if child.Kind != unstable.KeyValue {
			continue
		}
		if childIndex >= len(ref.children) {
			return nil, fmt.Errorf("%w: TOML inline syntax trees disagree", ErrUnwritable)
		}
		childRef := ref.children[childIndex]
		childIndex++
		expression, err := checkedTOMLRange(content, child.Raw)
		if err != nil {
			return nil, err
		}
		childPath := append(slices.Clone(path), tomlNodeKey(child)...)
		assignment := tomlAssignmentRef{
			path: childPath, value: childRef, expression: expression, line: expression,
			arrayTable: arrayTable, inline: true,
		}
		assignments = append(assignments, assignment)
		nested, err := tomlInlineAssignments(
			content, child.Value(), childRef, childPath, arrayTable,
		)
		if err != nil {
			return nil, err
		}
		assignments = append(assignments, nested...)
	}

	return assignments, nil
}

func tomlAssignment(
	content []byte,
	node *unstable.Node,
	tablePath []string,
	arrayTable bool,
) (tomlAssignmentRef, error) {
	expression, err := checkedTOMLRange(content, node.Raw)
	if err != nil {
		return tomlAssignmentRef{}, err
	}
	valueStart, err := tomlKeyValueStart(content, node)
	if err != nil {
		return tomlAssignmentRef{}, err
	}
	value, err := tomlValueFromNode(content, node.Value(), valueStart)
	if err != nil {
		return tomlAssignmentRef{}, err
	}
	path := append(slices.Clone(tablePath), tomlNodeKey(node)...)
	line := tomlWholeLines(content, expression.start, expression.end)
	var comment *tomlSpan
	if sibling := node.Next(); sibling != nil && sibling.Kind == unstable.Comment {
		span, rangeErr := checkedTOMLRange(content, sibling.Raw)
		if rangeErr != nil {
			return tomlAssignmentRef{}, rangeErr
		}
		comment = &span
	}

	return tomlAssignmentRef{
		path: path, value: value, expression: expression, line: line,
		comment: comment, arrayTable: arrayTable,
	}, nil
}

func tomlValueFromNode(content []byte, node *unstable.Node, sourceStart int) (*tomlValueRef, error) {
	span := tomlSpan{start: sourceStart, end: sourceStart}
	var err error
	if node.Kind != unstable.Array && node.Kind != unstable.InlineTable {
		span, err = checkedTOMLRange(content, node.Raw)
		if err != nil {
			return nil, err
		}
	}
	if node.Kind == unstable.Array || node.Kind == unstable.InlineTable {
		span.end, err = scanTOMLCollectionEnd(content, span.start)
		if err != nil {
			return nil, err
		}
	}
	ref := &tomlValueRef{kind: node.Kind, span: span}
	if err := populateTOMLValueChildren(content, node, ref); err != nil {
		return nil, err
	}

	return ref, nil
}

func populateTOMLValueChildren(
	content []byte,
	node *unstable.Node,
	ref *tomlValueRef,
) error {
	cursor := ref.span.start + 1
	children := node.Children()
	for children.Next() {
		var err error
		cursor, err = appendTOMLValueChild(content, node.Kind, children.Node(), ref, cursor)
		if err != nil {
			return err
		}
	}

	return nil
}

func appendTOMLValueChild(
	content []byte,
	parentKind unstable.Kind,
	child *unstable.Node,
	ref *tomlValueRef,
	cursor int,
) (int, error) {
	if child.Kind == unstable.Comment {
		comments, err := tomlNodeCommentSpans(content, child)
		if err != nil {
			return 0, err
		}
		ref.comments = append(ref.comments, comments...)
		for _, comment := range comments {
			cursor = max(cursor, comment.end)
		}

		return cursor, nil
	}
	child, cursor, err := locateTOMLChildValue(content, parentKind, child, cursor, ref.span.end)
	if err != nil {
		return 0, err
	}
	value, err := tomlValueFromNode(content, child, cursor)
	if err != nil {
		return 0, err
	}
	value.parent = ref
	ref.children = append(ref.children, value)

	return value.span.end, nil
}

func locateTOMLChildValue(
	content []byte,
	parentKind unstable.Kind,
	child *unstable.Node,
	cursor, parentEnd int,
) (*unstable.Node, int, error) {
	if parentKind == unstable.InlineTable && child.Kind == unstable.KeyValue {
		start, err := tomlKeyValueStart(content, child)

		return child.Value(), start, err
	}
	if child.Kind == unstable.Array {
		return child, tomlNextArrayValueStart(content, cursor, parentEnd), nil
	}
	span, err := checkedTOMLRange(content, child.Raw)

	return child, span.start, err
}

func tomlKeyValueStart(content []byte, node *unstable.Node) (int, error) {
	expression, err := checkedTOMLRange(content, node.Raw)
	if err != nil {
		return 0, err
	}
	keyEnd := expression.start
	keys := node.Key()
	for keys.Next() {
		key, rangeErr := checkedTOMLRange(content, keys.Node().Raw)
		if rangeErr != nil {
			return 0, rangeErr
		}
		keyEnd = max(keyEnd, key.end)
	}
	equals := bytes.IndexByte(content[keyEnd:expression.end], '=')
	if equals < 0 {
		return 0, fmt.Errorf("%w: TOML assignment has no separator", ErrUnreadable)
	}
	start := keyEnd + equals + 1
	for start < expression.end && (content[start] == ' ' || content[start] == '\t') {
		start++
	}

	return start, nil
}

func tomlNextArrayValueStart(content []byte, start, end int) int {
	for start < end {
		switch content[start] {
		case ' ', '\t', '\r', '\n', ',':
			start++
		default:
			return start
		}
	}

	return start
}

func tomlNodeCommentSpans(content []byte, node *unstable.Node) ([]tomlSpan, error) {
	span, err := checkedTOMLRange(content, node.Raw)
	if err != nil {
		return nil, err
	}
	spans := []tomlSpan{span}
	children := node.Children()
	for children.Next() {
		childSpans, childErr := tomlNodeCommentSpans(content, children.Node())
		if childErr != nil {
			return nil, childErr
		}
		spans = append(spans, childSpans...)
	}

	return spans, nil
}

func collectTOMLValues(value *tomlValueRef, values *[]*tomlValueRef) {
	for _, child := range value.children {
		collectTOMLValues(child, values)
	}
	*values = append(*values, value)
}

func collectTOMLComments(
	content []byte,
	node *unstable.Node,
	comments map[string]int,
	spans *[]tomlSpan,
) {
	if node == nil {
		return
	}
	if node.Kind == unstable.Comment {
		span, err := checkedTOMLRange(content, node.Raw)
		if err == nil {
			comments[string(content[span.start:span.end])]++
			*spans = append(*spans, span)
		}
	}
	children := node.Children()
	for children.Next() {
		collectTOMLComments(content, children.Node(), comments, spans)
	}
}

func checkedTOMLRange(content []byte, raw unstable.Range) (tomlSpan, error) {
	start := uint64(raw.Offset)
	end := start + uint64(raw.Length)
	if end > uint64(len(content)) {
		return tomlSpan{}, fmt.Errorf("%w: TOML parser returned an invalid source range", ErrUnwritable)
	}
	rawBytes := content[raw.Offset : raw.Offset+raw.Length]
	startIndex := cap(content) - cap(rawBytes)

	return tomlSpan{start: startIndex, end: startIndex + len(rawBytes)}, nil
}

func tomlNodeKey(node *unstable.Node) []string {
	var path []string
	keys := node.Key()
	for keys.Next() {
		path = append(path, string(keys.Node().Data))
	}

	return path
}

func tomlHeaderSpan(content []byte, node *unstable.Node, path []string) (tomlSpan, error) {
	key := node.Child()
	if key == nil || len(path) == 0 {
		return tomlSpan{}, fmt.Errorf("%w: TOML table has no key", ErrUnreadable)
	}
	keySpan, err := checkedTOMLRange(content, key.Raw)
	if err != nil {
		return tomlSpan{}, err
	}
	line := tomlWholeLines(content, keySpan.start, keySpan.end)
	opening := bytes.IndexByte(content[line.start:keySpan.start], '[')
	if opening < 0 {
		return tomlSpan{}, fmt.Errorf("%w: TOML table header has no opening bracket", ErrUnreadable)
	}

	return tomlSpan{start: line.start + opening, end: line.end}, nil
}

func tomlWholeLines(content []byte, start, end int) tomlSpan {
	lineStart := bytes.LastIndexByte(content[:start], '\n') + 1
	lineEnd := len(content)
	if newline := bytes.IndexByte(content[end:], '\n'); newline >= 0 {
		lineEnd = end + newline + 1
	}

	return tomlSpan{start: lineStart, end: lineEnd}
}

func scanTOMLCollectionEnd(content []byte, start int) (int, error) {
	if start < 0 || start >= len(content) || (content[start] != '[' && content[start] != '{') {
		return 0, fmt.Errorf("%w: TOML collection has an invalid source range", ErrUnwritable)
	}
	stack := []byte{content[start]}
	for at := start + 1; at < len(content); {
		switch content[at] {
		case '#':
			at = tomlSkipComment(content, at)
		case '\'', '"':
			var err error
			at, err = tomlSkipString(content, at)
			if err != nil {
				return 0, err
			}
		case '[', '{':
			stack = append(stack, content[at])
			at++
		case ']', '}':
			expected := byte('[')
			if content[at] == '}' {
				expected = '{'
			}
			if len(stack) == 0 || stack[len(stack)-1] != expected {
				return 0, fmt.Errorf("%w: TOML collection delimiters disagree", ErrUnreadable)
			}
			stack = stack[:len(stack)-1]
			at++
			if len(stack) == 0 {
				return at, nil
			}
		default:
			at++
		}
	}

	return 0, fmt.Errorf("%w: TOML collection is not closed", ErrUnreadable)
}

func tomlSkipComment(content []byte, start int) int {
	if newline := bytes.IndexByte(content[start:], '\n'); newline >= 0 {
		return start + newline + 1
	}

	return len(content)
}

func tomlSkipString(content []byte, start int) (int, error) {
	quote := content[start]
	delimiter := 1
	if start+2 < len(content) && content[start+1] == quote && content[start+2] == quote {
		delimiter = 3
	}
	for at := start + delimiter; at < len(content); at++ {
		if quote == '"' && content[at] == '\\' {
			at++
			continue
		}
		if content[at] != quote {
			continue
		}
		if delimiter == 1 || at+2 < len(content) && content[at+1] == quote && content[at+2] == quote {
			return at + delimiter, nil
		}
	}

	return 0, fmt.Errorf("%w: TOML string is not closed", ErrUnreadable)
}

func tomlPathKey(path []string) string { return strings.Join(path, "\x00") }
