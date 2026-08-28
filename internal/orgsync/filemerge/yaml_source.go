package filemerge

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/token"
	"github.com/smykla-skalski/smyklot/pkg/config"
	yaml "go.yaml.in/yaml/v3"
)

type yamlCollectionRef struct {
	source       ast.Node
	node         *yaml.Node
	mappingValue *ast.MappingValueNode
	parentSeq    *ast.SequenceNode
	root         bool
	currentFlow  bool
	targetFlow   bool
}

type yamlStringRef struct {
	source *ast.StringNode
	node   *yaml.Node
}

type yamlFormatTree struct {
	collections []yamlCollectionRef
	strings     []yamlStringRef
}

type yamlSourceTree struct {
	collections []yamlCollectionRef
	strings     []*ast.StringNode
}

type yamlRenderTree struct {
	collections []*yaml.Node
	strings     []*yaml.Node
}

func yamlSourceEdits(
	content []byte,
	document *ast.DocumentNode,
	renderDocument *yaml.Node,
	policy config.FormattingPolicy,
) ([]byteEdit, error) {
	layout := newSourceLayout(content)
	edits := make([]byteEdit, 0)

	if yamlNodeFormattingActive(policy) {
		nodeEdits, err := yamlNodeSourceEdits(layout, document.Body, renderDocument, policy)
		if err != nil {
			return nil, err
		}
		edits = append(edits, nodeEdits...)
	}

	documentEdit, changed, err := yamlDocumentStartEdit(layout, document, policy)
	if err != nil {
		return nil, err
	}
	if changed {
		edits = append(edits, documentEdit)
	}

	return edits, nil
}

func yamlNodeSourceEdits(
	layout sourceLayout,
	source ast.Node,
	renderDocument *yaml.Node,
	policy config.FormattingPolicy,
) ([]byteEdit, error) {
	tree, err := collectYAMLFormatTree(source, renderDocument)
	if err != nil {
		return nil, err
	}
	if err := configureYAMLStrings(tree.strings, policy.YAML.QuoteStyle); err != nil {
		return nil, err
	}
	if err := resolveYAMLCollectionStyles(tree.collections, policy); err != nil {
		return nil, err
	}

	indentWidth := yamlConfiguredIndentWidth(layout.content, policy)
	collectionEdits, err := yamlCollectionEdits(layout, tree.collections, policy, indentWidth)
	if err != nil {
		return nil, err
	}
	stringEdits, err := yamlStringEdits(layout, tree.strings, collectionEdits, indentWidth)
	if err != nil {
		return nil, err
	}

	return append(collectionEdits, stringEdits...), nil
}

func yamlConfiguredIndentWidth(content []byte, policy config.FormattingPolicy) int {
	if policy.Common.IndentStyle == formatPreserve {
		return yamlIndentWidth(content, policy.Common.IndentWidth)
	}

	return policy.Common.IndentWidth
}

func yamlCollectionEdits(
	layout sourceLayout,
	collections []yamlCollectionRef,
	policy config.FormattingPolicy,
	indentWidth int,
) ([]byteEdit, error) {
	edits := make([]byteEdit, 0, len(collections))
	for index := range collections {
		edit, changed, err := yamlCollectionEdit(layout, &collections[index], policy, indentWidth)
		if err != nil {
			return nil, err
		}
		if changed {
			edits = append(edits, edit)
		}
	}

	return outerYAMLCollectionEdits(edits), nil
}

func yamlStringEdits(
	layout sourceLayout,
	strings []yamlStringRef,
	collectionEdits []byteEdit,
	indentWidth int,
) ([]byteEdit, error) {
	edits := make([]byteEdit, 0, len(strings))
	for _, ref := range strings {
		edit, changed, err := yamlStringEdit(layout, ref, indentWidth)
		if err != nil {
			return nil, err
		}
		if changed && !yamlEditContained(edit, collectionEdits) {
			edits = append(edits, edit)
		}
	}

	return edits, nil
}

func collectYAMLFormatTree(source ast.Node, document *yaml.Node) (yamlFormatTree, error) {
	sourceTree := yamlSourceTree{}
	collectYAMLSourceNode(source, yamlCollectionRef{root: true}, &sourceTree)
	renderTree := yamlRenderTree{}
	collectYAMLRenderNode(yamlDocumentBody(document), &renderTree)
	if len(sourceTree.collections) != len(renderTree.collections) ||
		len(sourceTree.strings) != len(renderTree.strings) {
		return yamlFormatTree{}, fmt.Errorf(
			"%w: YAML syntax trees disagree about the document shape", ErrUnwritable,
		)
	}

	tree := yamlFormatTree{strings: make([]yamlStringRef, len(sourceTree.strings))}
	tree.collections = slices.Clone(sourceTree.collections)
	for index, node := range renderTree.collections {
		ref := &tree.collections[index]
		if !sameYAMLCollectionKind(ref.source, node) {
			return yamlFormatTree{}, fmt.Errorf(
				"%w: YAML syntax trees disagree about a collection", ErrUnwritable,
			)
		}
		ref.node = node
		ref.currentFlow = yamlNodeIsFlow(node)
		ref.targetFlow = ref.currentFlow
	}
	for index, node := range renderTree.strings {
		sourceNode := sourceTree.strings[index]
		if sourceNode.Value != node.Value {
			return yamlFormatTree{}, fmt.Errorf(
				"%w: YAML syntax trees disagree about a string scalar", ErrUnwritable,
			)
		}
		tree.strings[index] = yamlStringRef{source: sourceNode, node: node}
	}

	return tree, nil
}

func collectYAMLSourceNode(node ast.Node, context yamlCollectionRef, tree *yamlSourceTree) {
	if node == nil {
		return
	}
	if stringNode, ok := node.(*ast.StringNode); ok {
		if !strings.ContainsAny(stringNode.Value, "\r\n") {
			tree.strings = append(tree.strings, stringNode)
		}

		return
	}
	switch typed := node.(type) {
	case *ast.MappingNode:
		for _, value := range typed.Values {
			collectYAMLSourceNode(value.Key, yamlCollectionRef{}, tree)
			collectYAMLSourceNode(value.Value, yamlCollectionRef{mappingValue: value}, tree)
		}
		context.source = typed
		tree.collections = append(tree.collections, context)
	case *ast.SequenceNode:
		for _, value := range typed.Values {
			collectYAMLSourceNode(value, yamlCollectionRef{parentSeq: typed}, tree)
		}
		context.source = typed
		tree.collections = append(tree.collections, context)
	case *ast.MappingKeyNode:
		collectYAMLSourceNode(typed.Value, yamlCollectionRef{}, tree)
	case *ast.MappingValueNode:
		collectYAMLSourceNode(typed.Key, yamlCollectionRef{}, tree)
		collectYAMLSourceNode(typed.Value, yamlCollectionRef{mappingValue: typed}, tree)
	case *ast.DocumentNode:
		collectYAMLSourceNode(typed.Body, yamlCollectionRef{root: true}, tree)
	case *ast.AnchorNode:
		collectYAMLSourceNode(typed.Name, yamlCollectionRef{}, tree)
		collectYAMLSourceNode(typed.Value, yamlCollectionRef{}, tree)
	case *ast.AliasNode:
		collectYAMLSourceNode(typed.Value, yamlCollectionRef{}, tree)
	case *ast.TagNode:
		collectYAMLSourceNode(typed.Value, yamlCollectionRef{}, tree)
	}
}

func collectYAMLRenderNode(node *yaml.Node, tree *yamlRenderTree) {
	if node == nil {
		return
	}
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			collectYAMLRenderNode(child, tree)
		}
	case yaml.MappingNode:
		for _, child := range node.Content {
			collectYAMLRenderNode(child, tree)
		}
		tree.collections = append(tree.collections, node)
	case yaml.SequenceNode:
		for _, child := range node.Content {
			collectYAMLRenderNode(child, tree)
		}
		tree.collections = append(tree.collections, node)
	case yaml.ScalarNode:
		if node.Tag == tagString && node.Style&(yaml.LiteralStyle|yaml.FoldedStyle) == 0 {
			tree.strings = append(tree.strings, node)
		}
	}
}

func yamlDocumentBody(document *yaml.Node) *yaml.Node {
	if document != nil && document.Kind == yaml.DocumentNode && len(document.Content) == 1 {
		return document.Content[0]
	}

	return document
}

func sameYAMLCollectionKind(source ast.Node, render *yaml.Node) bool {
	switch source.(type) {
	case *ast.MappingNode:
		return render.Kind == yaml.MappingNode
	case *ast.SequenceNode:
		return render.Kind == yaml.SequenceNode
	default:
		return false
	}
}

func configureYAMLStrings(strings []yamlStringRef, choice string) error {
	for _, ref := range strings {
		style := ref.node.Style &^ (yaml.SingleQuotedStyle | yaml.DoubleQuotedStyle)
		switch choice {
		case formatPreserve:
			continue
		case "prefer_plain":
			if ref.node.Value == "" || token.IsNeedQuoted(ref.node.Value) {
				continue
			}
			ref.node.Style = style
		case "prefer_single":
			ref.node.Style = style | yaml.SingleQuotedStyle
		case "prefer_double":
			ref.node.Style = style | yaml.DoubleQuotedStyle
		default:
			return fmt.Errorf("%w: unknown YAML quote style %q", ErrUnwritable, choice)
		}
	}

	return nil
}

func resolveYAMLCollectionStyles(
	collections []yamlCollectionRef,
	policy config.FormattingPolicy,
) error {
	for index := range collections {
		ref := &collections[index]
		configured := yamlCollectionPolicy(ref.node, policy)
		switch configured {
		case formatPreserve:
			ref.targetFlow = ref.currentFlow
		case "flow":
			ref.targetFlow = true
		case "block":
			ref.targetFlow = false
		case formatAuto:
			ref.targetFlow = yamlCollectionFits(ref, policy.Common.LineWidth)
		default:
			return fmt.Errorf("%w: unknown YAML collection style %q", ErrUnwritable, configured)
		}
		setYAMLNodeFlow(ref.node, ref.targetFlow)
		if ref.targetFlow && yamlHasBlockCollectionChild(ref.node) {
			return fmt.Errorf(
				"%w: a YAML flow collection cannot contain a preserved block collection",
				ErrUnwritable,
			)
		}
	}

	return nil
}

func yamlCollectionFits(ref *yamlCollectionRef, width int) bool {
	setYAMLNodeFlow(ref.node, true)
	if yamlHasBlockCollectionChild(ref.node) {
		return false
	}
	rendered, err := encodeYAMLFragment(ref.node, 2)
	if err != nil || strings.ContainsAny(rendered, "\r\n") {
		return false
	}
	column := yamlCollectionStartToken(ref.source, ref.currentFlow).Position.Column

	return column-1+utf8.RuneCountInString(rendered) <= width
}

func yamlCollectionPolicy(node *yaml.Node, policy config.FormattingPolicy) string {
	if node.Kind == yaml.SequenceNode {
		return policy.YAML.Sequences
	}

	return policy.YAML.Mappings
}

func yamlNodeIsFlow(node *yaml.Node) bool { return node.Style&yaml.FlowStyle != 0 }

func setYAMLNodeFlow(node *yaml.Node, flow bool) {
	if flow {
		node.Style |= yaml.FlowStyle
	} else {
		node.Style &^= yaml.FlowStyle
	}
}

func yamlHasBlockCollectionChild(node *yaml.Node) bool {
	for _, child := range node.Content {
		if child.Kind == yaml.MappingNode || child.Kind == yaml.SequenceNode {
			if !yamlNodeIsFlow(child) || yamlHasBlockCollectionChild(child) {
				return true
			}
		}
	}

	return false
}

func yamlCollectionEdit(
	layout sourceLayout,
	ref *yamlCollectionRef,
	policy config.FormattingPolicy,
	indentWidth int,
) (byteEdit, bool, error) {
	if ref.currentFlow == ref.targetFlow {
		return byteEdit{}, false, nil
	}
	start, end, err := yamlCollectionSpan(layout, ref.source, ref.currentFlow)
	if err != nil {
		return byteEdit{}, false, err
	}
	rendered, err := encodeYAMLFragment(ref.node, indentWidth)
	if err != nil {
		return byteEdit{}, false, err
	}
	if !ref.targetFlow {
		indent := yamlCollectionIndent(*ref, ref.currentFlow, policy.YAML.SequenceIndent, indentWidth)
		rendered = indentYAMLFragment(rendered, indent)
	}
	lineEnding := yamlPreferredLineEnding(layout.content)
	rendered = strings.ReplaceAll(rendered, lf, lineEnding)
	if ref.currentFlow && !ref.targetFlow && yamlCollectionNeedsLeadingLine(*ref) {
		colon := ref.mappingValue.Start
		colonStart, positionErr := layout.index(colon.Position.Line, colon.Position.Column)
		if positionErr != nil {
			return byteEdit{}, false, positionErr
		}
		colonEnd := colonStart + 1
		if colonEnd > start || len(bytes.TrimSpace(layout.content[colonEnd:start])) != 0 {
			return byteEdit{}, false, fmt.Errorf(
				"%w: YAML collection position does not follow its mapping delimiter", ErrUnwritable,
			)
		}
		start = colonEnd
		rendered = lineEnding + rendered
	}

	return byteEdit{start: start, end: end, replacement: []byte(rendered)}, true, nil
}

func indentYAMLFragment(fragment string, spaces int) string {
	if spaces == 0 {
		return fragment
	}
	prefix := strings.Repeat(" ", spaces)
	lines := strings.Split(fragment, lf)
	for index := range lines {
		if lines[index] != "" {
			lines[index] = prefix + lines[index]
		}
	}

	return strings.Join(lines, lf)
}

func yamlCollectionIndent(
	ref yamlCollectionRef,
	currentFlow bool,
	sequenceIndent string,
	indentWidth int,
) int {
	if sequence, ok := ref.source.(*ast.SequenceNode); ok && ref.mappingValue != nil {
		keyIndent := ref.mappingValue.Key.GetToken().Position.Column - 1
		switch sequenceIndent {
		case "indentless":
			return keyIndent
		case "indented":
			return keyIndent + indentWidth
		case formatPreserve:
			if !currentFlow {
				return sequence.GetToken().Position.Column - 1
			}
		}

		return keyIndent + indentWidth
	}
	if ref.mappingValue != nil {
		return ref.mappingValue.Key.GetToken().Position.Column - 1 + indentWidth
	}
	if ref.parentSeq != nil {
		return ref.parentSeq.GetToken().Position.Column - 1 + indentWidth
	}
	if ref.root {
		return 0
	}

	return yamlCollectionStartToken(ref.source, currentFlow).Position.Column - 1
}

func yamlCollectionNeedsLeadingLine(ref yamlCollectionRef) bool {
	if ref.mappingValue == nil {
		return false
	}
	line := yamlCollectionStartToken(ref.source, ref.currentFlow).Position.Line

	return ref.mappingValue.Key.GetToken().Position.Line == line
}

func yamlCollectionSpan(layout sourceLayout, node ast.Node, flow bool) (int, int, error) {
	startToken := yamlCollectionStartToken(node, flow)
	if startToken == nil || startToken.Position == nil {
		return 0, 0, fmt.Errorf("%w: YAML collection has no source position", ErrUnwritable)
	}
	start, err := layout.index(startToken.Position.Line, startToken.Position.Column)
	if err != nil {
		return 0, 0, err
	}
	if flow {
		endToken := yamlCollectionEndToken(node)
		if endToken == nil || endToken.Position == nil {
			return 0, 0, fmt.Errorf("%w: flow collection has no closing token", ErrUnwritable)
		}
		end, err := layout.index(endToken.Position.Line, endToken.Position.Column)
		if err != nil {
			return 0, 0, err
		}
		closing := strings.TrimSpace(endToken.Origin)
		if closing == "" {
			closing = endToken.Value
		}

		return start, end + len(closing), nil
	}
	end, err := layout.contentEnd(yamlNodeLastLine(node))
	if err != nil {
		return 0, 0, err
	}

	return start, end, nil
}

func yamlCollectionStartToken(node ast.Node, flow bool) *token.Token {
	if flow {
		return node.GetToken()
	}
	switch typed := node.(type) {
	case *ast.MappingNode:
		if len(typed.Values) > 0 {
			return typed.Values[0].Key.GetToken()
		}
	case *ast.SequenceNode:
		return typed.Start
	}

	return node.GetToken()
}

func yamlCollectionEndToken(node ast.Node) *token.Token {
	switch typed := node.(type) {
	case *ast.MappingNode:
		return typed.End
	case *ast.SequenceNode:
		return typed.End
	default:
		return nil
	}
}

func yamlDocumentStartEdit(
	layout sourceLayout,
	document *ast.DocumentNode,
	policy config.FormattingPolicy,
) (byteEdit, bool, error) {
	switch policy.YAML.DocumentStart {
	case formatPreserve:
		return byteEdit{}, false, nil
	case formatInsert:
		if document.Start != nil {
			return byteEdit{}, false, nil
		}

		return byteEdit{replacement: []byte("---" + yamlPreferredLineEnding(layout.content))}, true, nil
	case formatRemove:
		if document.Start == nil {
			return byteEdit{}, false, nil
		}
		line := document.Start.Position.Line
		start, end, err := layout.wholeLine(line)
		if err != nil {
			return byteEdit{}, false, err
		}
		if string(bytes.TrimSpace(layout.content[start:end])) != "---" {
			return byteEdit{}, false, fmt.Errorf(
				"%w: a commented YAML document start cannot be removed safely", ErrUnwritable,
			)
		}

		return byteEdit{start: start, end: end}, true, nil
	default:
		return byteEdit{}, false, fmt.Errorf(
			"%w: unknown YAML document-start policy %q", ErrUnwritable, policy.YAML.DocumentStart,
		)
	}
}

func yamlPreferredLineEnding(content []byte) string {
	if dominantLineEnding(content) == lineEndingCRLF {
		return crlf
	}

	return lf
}

func outerYAMLCollectionEdits(edits []byteEdit) []byteEdit {
	edits = slices.Clone(edits)
	slices.SortFunc(edits, func(one, two byteEdit) int {
		if one.start != two.start {
			return one.start - two.start
		}

		return two.end - one.end
	})
	outer := make([]byteEdit, 0, len(edits))
	for _, candidate := range edits {
		if yamlEditContained(candidate, outer) {
			continue
		}
		outer = append(outer, candidate)
	}

	return outer
}

func yamlEditContained(candidate byteEdit, containers []byteEdit) bool {
	for _, container := range containers {
		if container.start <= candidate.start && container.end >= candidate.end {
			return true
		}
	}

	return false
}
