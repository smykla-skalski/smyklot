package filemerge

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/token"
	yaml "go.yaml.in/yaml/v3"
)

type yamlSyntaxRef struct {
	outer        ast.Node
	core         ast.Node
	mappingValue *ast.MappingValueNode
}

type yamlMergeRenderer struct {
	content     []byte
	layout      sourceLayout
	refs        map[*yaml.Node]yamlSyntaxRef
	indentWidth int
	lineEnding  string
}

func renderYAMLMerge(content []byte, source, merged *yaml.Node) ([]byte, error) {
	if sameWriting(source, merged) {
		return content, nil
	}
	syntax, err := parseGoccyYAML(content)
	if err != nil {
		return nil, err
	}
	refs := make(map[*yaml.Node]yamlSyntaxRef)
	if err := pairYAMLSyntax(syntax.Docs[0], source, yamlSyntaxRef{}, refs); err != nil {
		return nil, err
	}
	renderer := yamlMergeRenderer{
		content: content, layout: newSourceLayout(content), refs: refs,
		indentWidth: yamlIndentWidth(content, yamlIndent),
		lineEnding:  yamlPreferredLineEnding(content),
	}
	sourceRoot, mergedRoot := yamlDocumentBody(source), yamlDocumentBody(merged)
	if sourceRoot == nil || mergedRoot == nil ||
		sourceRoot.Kind != yaml.MappingNode || mergedRoot.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%w: YAML merge did not produce an object", ErrUnwritable)
	}
	edits, err := renderer.mappingEdits(sourceRoot, mergedRoot)
	if err != nil {
		return nil, err
	}
	result, err := applyByteEdits(content, edits)
	if err != nil {
		return nil, err
	}
	if _, err := parseGoccyYAML(result); err != nil {
		return nil, fmt.Errorf("%w: merged YAML did not parse: %w", ErrUnwritable, err)
	}
	actual, err := parseYAMLFormatDocument(result)
	if err != nil {
		return nil, err
	}
	unwriteMergeTags(actual)
	if !sameYAMLMergeWriting(actual, merged) {
		return nil, fmt.Errorf("%w: source edits did not produce the requested YAML merge", ErrUnwritable)
	}

	return result, nil
}

func sameYAMLMergeWriting(actual, expected *yaml.Node) bool {
	if actual == nil || expected == nil {
		return actual == expected
	}
	if actual.Kind != expected.Kind || actual.Tag != expected.Tag || actual.Value != expected.Value ||
		!sameYAMLMergeStyle(actual, expected) || actual.Anchor != expected.Anchor ||
		actual.HeadComment != expected.HeadComment || actual.LineComment != expected.LineComment ||
		actual.FootComment != expected.FootComment || len(actual.Content) != len(expected.Content) {
		return false
	}
	for index := range actual.Content {
		if !sameYAMLMergeWriting(actual.Content[index], expected.Content[index]) {
			return false
		}
	}

	return true
}

func sameYAMLMergeStyle(actual, expected *yaml.Node) bool {
	if actual.Style == expected.Style {
		return true
	}
	if expected.Style != 0 {
		return false
	}
	switch expected.Kind {
	case yaml.MappingNode, yaml.SequenceNode:
		return len(expected.Content) == 0 && actual.Style == yaml.FlowStyle
	case yaml.ScalarNode:
		quoted := actual.Style & (yaml.SingleQuotedStyle | yaml.DoubleQuotedStyle)

		return expected.Tag == tagString && quoted != 0 && token.IsNeedQuoted(expected.Value)
	default:
		return false
	}
}

func pairYAMLSyntax(
	source ast.Node,
	node *yaml.Node,
	context yamlSyntaxRef,
	refs map[*yaml.Node]yamlSyntaxRef,
) error {
	if source == nil || node == nil {
		return fmt.Errorf("%w: YAML syntax trees disagree about an empty node", ErrUnwritable)
	}
	if node.Kind == yaml.DocumentNode {
		return pairYAMLDocumentSyntax(source, node, refs)
	}
	outer, core := source, unwrapYAMLSyntax(source)
	ref := context
	ref.outer, ref.core = outer, core
	refs[node] = ref
	switch node.Kind {
	case yaml.MappingNode:
		return pairYAMLMappingSyntax(core, node, refs)
	case yaml.SequenceNode:
		return pairYAMLSequenceSyntax(core, node, refs)
	}

	return nil
}

func pairYAMLDocumentSyntax(
	source ast.Node,
	node *yaml.Node,
	refs map[*yaml.Node]yamlSyntaxRef,
) error {
	document, ok := source.(*ast.DocumentNode)
	if !ok || len(node.Content) != 1 {
		return yamlSyntaxShapeError()
	}
	refs[node] = yamlSyntaxRef{outer: source, core: source}

	return pairYAMLSyntax(document.Body, node.Content[0], yamlSyntaxRef{}, refs)
}

func pairYAMLMappingSyntax(
	source ast.Node,
	node *yaml.Node,
	refs map[*yaml.Node]yamlSyntaxRef,
) error {
	mapping, ok := source.(*ast.MappingNode)
	if !ok || len(node.Content) != len(mapping.Values)*2 {
		return yamlSyntaxShapeError()
	}
	for index, value := range mapping.Values {
		if err := pairYAMLSyntax(value.Key, node.Content[index*2], yamlSyntaxRef{}, refs); err != nil {
			return err
		}
		context := yamlSyntaxRef{mappingValue: value}
		if err := pairYAMLSyntax(value.Value, node.Content[index*2+1], context, refs); err != nil {
			return err
		}
	}

	return nil
}

func pairYAMLSequenceSyntax(
	source ast.Node,
	node *yaml.Node,
	refs map[*yaml.Node]yamlSyntaxRef,
) error {
	sequence, ok := source.(*ast.SequenceNode)
	if !ok || len(node.Content) != len(sequence.Values) {
		return yamlSyntaxShapeError()
	}
	for index, value := range sequence.Values {
		if err := pairYAMLSyntax(value, node.Content[index], yamlSyntaxRef{}, refs); err != nil {
			return err
		}
	}

	return nil
}

func unwrapYAMLSyntax(node ast.Node) ast.Node {
	for {
		switch typed := node.(type) {
		case *ast.DocumentNode:
			node = typed.Body
		case *ast.MappingKeyNode:
			node = typed.Value
		case *ast.MappingValueNode:
			node = typed.Value
		case *ast.AnchorNode:
			if typed.Value == nil {
				return node
			}
			node = typed.Value
		case *ast.TagNode:
			if typed.Value == nil {
				return node
			}
			node = typed.Value
		default:
			return node
		}
	}
}

func yamlSyntaxShapeError() error {
	return fmt.Errorf("%w: YAML syntax trees disagree about the document shape", ErrUnwritable)
}

func (renderer yamlMergeRenderer) mappingEdits(
	source, merged *yaml.Node,
) ([]byteEdit, error) {
	if sameWriting(source, merged) {
		return nil, nil
	}
	ref, ok := renderer.refs[source]
	if !ok {
		return nil, yamlSyntaxShapeError()
	}
	sourceSyntax, ok := ref.core.(*ast.MappingNode)
	if !ok {
		return nil, yamlSyntaxShapeError()
	}
	wholeEdit, err := renderer.mappingNeedsWholeEdit(sourceSyntax)
	if err != nil {
		return nil, err
	}
	if source.Style&yaml.FlowStyle != 0 || merged.Style&yaml.FlowStyle != 0 ||
		len(source.Content) == 0 || len(merged.Content) == 0 ||
		wholeEdit {
		edit, err := renderer.replaceNode(source, merged)
		return []byteEdit{edit}, err
	}

	sourceEntries := yamlMappingEntries(source)
	mergedEntries := yamlMappingEntries(merged)
	edits, err := renderer.existingMappingEdits(sourceSyntax, sourceEntries, mergedEntries)
	if err != nil {
		return nil, err
	}
	added := addedYAMLMappingEntries(sourceEntries, mergedEntries)
	if len(added) == 0 {
		return edits, nil
	}
	edit, err := renderer.insertEntries(sourceSyntax, added)
	if err != nil {
		return nil, err
	}

	return append(edits, edit), nil
}

func (renderer yamlMergeRenderer) existingMappingEdits(
	sourceSyntax *ast.MappingNode,
	sourceEntries, mergedEntries yamlMapEntries,
) ([]byteEdit, error) {
	edits := make([]byteEdit, 0, len(sourceEntries.ordered))
	for _, entry := range sourceEntries.ordered {
		replacement, exists := mergedEntries.byKey[entry.key.Value]
		if !exists {
			edit, err := renderer.deleteEntry(sourceSyntax, entry.index)
			if err != nil {
				return nil, err
			}
			edits = append(edits, edit)
			continue
		}
		if sameWriting(entry.value, replacement.value) {
			continue
		}
		if entry.value.Kind == yaml.MappingNode && replacement.value.Kind == yaml.MappingNode &&
			entry.value.Style&yaml.FlowStyle == 0 && replacement.value.Style&yaml.FlowStyle == 0 {
			nested, err := renderer.mappingEdits(entry.value, replacement.value)
			if err != nil {
				return nil, err
			}
			edits = append(edits, nested...)
			continue
		}
		edit, err := renderer.replaceNode(entry.value, replacement.value)
		if err != nil {
			return nil, err
		}
		edits = append(edits, edit)
	}

	return edits, nil
}

func addedYAMLMappingEntries(sourceEntries, mergedEntries yamlMapEntries) []yamlMapEntry {
	added := make([]yamlMapEntry, 0, len(mergedEntries.ordered))
	for _, entry := range mergedEntries.ordered {
		if _, exists := sourceEntries.byKey[entry.key.Value]; !exists {
			added = append(added, entry)
		}
	}

	return added
}

type yamlMapEntry struct {
	key, value *yaml.Node
	index      int
}

type yamlMapEntries struct {
	ordered []yamlMapEntry
	byKey   map[string]yamlMapEntry
}

func yamlMappingEntries(mapping *yaml.Node) yamlMapEntries {
	entries := yamlMapEntries{byKey: make(map[string]yamlMapEntry, len(mapping.Content)/2)}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		entry := yamlMapEntry{key: mapping.Content[index], value: mapping.Content[index+1], index: index / 2}
		entries.ordered = append(entries.ordered, entry)
		entries.byKey[entry.key.Value] = entry
	}

	return entries
}

func (renderer yamlMergeRenderer) mappingNeedsWholeEdit(mapping *ast.MappingNode) (bool, error) {
	if len(mapping.Values) == 0 {
		return true, nil
	}
	start := mapping.Values[0].Key.GetToken().Position
	index, err := renderer.layout.index(start.Line, start.Column)
	if err != nil {
		return false, err
	}
	line, err := renderer.layout.line(start.Line)
	if err != nil {
		return false, err
	}
	prefix := renderer.content[line.start:index]

	return len(bytes.TrimSpace(prefix)) != 0, nil
}

func (renderer yamlMergeRenderer) replaceNode(source, merged *yaml.Node) (byteEdit, error) {
	ref, ok := renderer.refs[source]
	if !ok {
		return byteEdit{}, yamlSyntaxShapeError()
	}
	start, end, err := renderer.nodeSpan(source, ref)
	if err != nil {
		return byteEdit{}, err
	}
	renderNode := cloneNode(merged)
	clearYAMLLeadingHeadComments(source, renderNode)
	rendered, err := encodeYAMLFragment(renderNode, renderer.indentWidth)
	if err != nil {
		return byteEdit{}, err
	}
	rendered = strings.ReplaceAll(rendered, lf, renderer.lineEnding)
	if ref.mappingValue == nil {
		if strings.Contains(rendered, renderer.lineEnding) {
			indent := yamlMergeNodeStart(ref).Position.Column - 1
			rendered = indentYAMLFragmentAfterFirst(rendered, indent, renderer.lineEnding)
		}

		return byteEdit{start: start, end: end, replacement: []byte(rendered)}, nil
	}

	keyLine := ref.mappingValue.Key.GetToken().Position.Line
	valueLine := yamlMergeNodeStart(ref).Position.Line
	colonStart, err := renderer.layout.index(
		ref.mappingValue.Start.Position.Line,
		ref.mappingValue.Start.Position.Column,
	)
	if err != nil {
		return byteEdit{}, err
	}
	colonEnd := colonStart + 1
	multiline := yamlNodeNeedsOwnLine(merged) || strings.Contains(rendered, renderer.lineEnding)
	keyIndent := ref.mappingValue.Key.GetToken().Position.Column - 1
	switch {
	case keyLine == valueLine && multiline:
		start = colonEnd
		rendered = renderer.lineEnding + indentYAMLFragmentWithEnding(
			rendered, keyIndent+renderer.indentWidth, renderer.lineEnding,
		)
	case keyLine != valueLine && !multiline:
		start = colonEnd
		rendered = " " + rendered
	case keyLine != valueLine && multiline:
		indent := yamlMergeNodeStart(ref).Position.Column - 1
		rendered = indentYAMLFragmentAfterFirst(rendered, indent, renderer.lineEnding)
	}

	return byteEdit{start: start, end: end, replacement: []byte(rendered)}, nil
}

// clearYAMLLeadingHeadComments omits comments that precede the source span.
// A collection starts at its first key or dash, while yaml.v3 attaches a
// comment immediately above that token to the first child. The original bytes
// remain before the replacement, so asking the encoder to write the same
// comment would duplicate it.
func clearYAMLLeadingHeadComments(source, rendered *yaml.Node) {
	if source == nil || rendered == nil {
		return
	}
	rendered.HeadComment = ""
	if source.Kind != rendered.Kind || len(source.Content) == 0 || len(rendered.Content) == 0 {
		return
	}
	switch source.Kind {
	case yaml.MappingNode, yaml.SequenceNode:
		clearYAMLLeadingHeadComments(source.Content[0], rendered.Content[0])
	}
}

func yamlNodeNeedsOwnLine(node *yaml.Node) bool {
	switch node.Kind {
	case yaml.MappingNode, yaml.SequenceNode:
		return len(node.Content) > 0 && node.Style&yaml.FlowStyle == 0
	case yaml.ScalarNode:
		return node.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0
	default:
		return false
	}
}

func indentYAMLFragmentWithEnding(fragment string, spaces int, ending string) string {
	if spaces == 0 {
		return fragment
	}
	prefix := strings.Repeat(" ", spaces)
	lines := strings.Split(fragment, ending)
	for index := range lines {
		if lines[index] != "" {
			lines[index] = prefix + lines[index]
		}
	}

	return strings.Join(lines, ending)
}

func indentYAMLFragmentAfterFirst(fragment string, spaces int, ending string) string {
	if spaces == 0 {
		return fragment
	}
	prefix := strings.Repeat(" ", spaces)
	lines := strings.Split(fragment, ending)
	for index := 1; index < len(lines); index++ {
		if lines[index] != "" {
			lines[index] = prefix + lines[index]
		}
	}

	return strings.Join(lines, ending)
}

func (renderer yamlMergeRenderer) nodeSpan(
	node *yaml.Node,
	ref yamlSyntaxRef,
) (int, int, error) {
	startToken := yamlMergeNodeStart(ref)
	if startToken == nil || startToken.Position == nil {
		return 0, 0, fmt.Errorf("%w: YAML merge node has no source position", ErrUnwritable)
	}
	start, err := renderer.layout.index(startToken.Position.Line, startToken.Position.Column)
	if err != nil {
		return 0, 0, err
	}
	if isYAMLScalarSyntax(ref.core) && ref.outer == ref.core && node.LineComment == "" {
		return yamlScalarSpan(renderer.layout, ref.core.GetToken())
	}
	end, err := renderer.layout.contentEnd(yamlNodeLastLine(ref.outer))
	if err != nil {
		return 0, 0, err
	}

	return start, end, nil
}

func yamlMergeNodeStart(ref yamlSyntaxRef) *token.Token {
	if ref.outer != nil && ref.outer.GetToken() != nil {
		return ref.outer.GetToken()
	}

	return ref.core.GetToken()
}

func isYAMLScalarSyntax(node ast.Node) bool {
	switch node.(type) {
	case *ast.StringNode, *ast.IntegerNode, *ast.FloatNode, *ast.BoolNode,
		*ast.NullNode, *ast.InfinityNode, *ast.NanNode:
		return true
	default:
		return false
	}
}

func (renderer yamlMergeRenderer) deleteEntry(
	mapping *ast.MappingNode,
	index int,
) (byteEdit, error) {
	if index < 0 || index >= len(mapping.Values) {
		return byteEdit{}, yamlSyntaxShapeError()
	}
	entry := mapping.Values[index]
	startLine := entry.Key.GetToken().Position.Line
	endLine := yamlNodeLastLine(entry)
	start, _, err := renderer.layout.wholeLine(startLine)
	if err != nil {
		return byteEdit{}, err
	}
	_, end, err := renderer.layout.wholeLine(endLine)
	if err != nil {
		return byteEdit{}, err
	}

	return byteEdit{start: start, end: end}, nil
}

func (renderer yamlMergeRenderer) insertEntries(
	mapping *ast.MappingNode,
	entries []yamlMapEntry,
) (byteEdit, error) {
	if len(mapping.Values) == 0 || mapping.FootComment != nil {
		return byteEdit{}, fmt.Errorf(
			"%w: cannot add YAML keys around an empty or foot-commented mapping safely",
			ErrUnwritable,
		)
	}
	fragment := &yaml.Node{Kind: yaml.MappingNode, Tag: tagMap}
	for _, entry := range entries {
		fragment.Content = append(fragment.Content, entry.key, entry.value)
	}
	rendered, err := encodeYAMLFragment(fragment, renderer.indentWidth)
	if err != nil {
		return byteEdit{}, err
	}
	indent := mapping.Values[0].Key.GetToken().Position.Column - 1
	rendered = indentYAMLFragment(rendered, indent)
	rendered = strings.ReplaceAll(rendered, lf, renderer.lineEnding)
	lastLine := yamlNodeLastLine(mapping.Values[len(mapping.Values)-1])
	line, err := renderer.layout.line(lastLine)
	if err != nil {
		return byteEdit{}, err
	}
	if line.end > line.contentEnd {
		return byteEdit{
			start: line.end, end: line.end,
			replacement: []byte(rendered + renderer.lineEnding),
		}, nil
	}

	return byteEdit{
		start: line.contentEnd, end: line.contentEnd,
		replacement: []byte(renderer.lineEnding + rendered),
	}, nil
}

func inheritYAMLMergePresentation(source, merged *yaml.Node) {
	if source == nil || merged == nil {
		return
	}
	if merged.HeadComment == "" {
		merged.HeadComment = source.HeadComment
	}
	if merged.LineComment == "" {
		merged.LineComment = source.LineComment
	}
	if merged.FootComment == "" {
		merged.FootComment = source.FootComment
	}
	if merged.Anchor == "" {
		merged.Anchor = source.Anchor
	}
	if source.Kind != merged.Kind {
		return
	}
	switch source.Kind {
	case yaml.ScalarNode:
		inheritYAMLScalarPresentation(source, merged)
	case yaml.MappingNode:
		inheritYAMLMappingPresentation(source, merged)
	case yaml.SequenceNode:
		merged.Style = mergeYAMLFlowStyle(source.Style, merged.Style)
	}
}

func inheritYAMLScalarPresentation(source, merged *yaml.Node) {
	if source.Tag == merged.Tag && source.Value == merged.Value {
		blockStyle := source.Style & (yaml.LiteralStyle | yaml.FoldedStyle)
		if blockStyle != 0 {
			merged.Style &^= yaml.LiteralStyle | yaml.FoldedStyle
			merged.Style |= blockStyle
		}
	}
	if source.Tag != tagString || merged.Tag != tagString {
		return
	}
	quoted := source.Style & (yaml.SingleQuotedStyle | yaml.DoubleQuotedStyle)
	if quoted != 0 {
		merged.Style &^= yaml.SingleQuotedStyle | yaml.DoubleQuotedStyle
		merged.Style |= quoted
	}
}

func inheritYAMLMappingPresentation(source, merged *yaml.Node) {
	merged.Style = mergeYAMLFlowStyle(source.Style, merged.Style)
	sourceEntries := yamlMappingEntries(source)
	for _, entry := range yamlMappingEntries(merged).ordered {
		if held, ok := sourceEntries.byKey[entry.key.Value]; ok {
			inheritYAMLMergePresentation(held.key, entry.key)
			inheritYAMLMergePresentation(held.value, entry.value)
		}
	}
}

func mergeYAMLFlowStyle(source, merged yaml.Style) yaml.Style {
	merged &^= yaml.FlowStyle

	return merged | source&yaml.FlowStyle
}
