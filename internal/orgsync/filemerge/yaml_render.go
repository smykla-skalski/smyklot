package filemerge

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/token"
	yaml "go.yaml.in/yaml/v3"
)

func encodeYAMLFragment(node *yaml.Node, indentWidth int) (string, error) {
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(indentWidth)
	if err := encoder.Encode(node); err != nil {
		return "", fmt.Errorf("%w: could not render YAML syntax tree: %w", ErrUnwritable, err)
	}
	if err := encoder.Close(); err != nil {
		return "", fmt.Errorf("%w: could not finish YAML syntax tree: %w", ErrUnwritable, err)
	}

	return strings.TrimSuffix(output.String(), lf), nil
}

func yamlStringEdit(
	layout sourceLayout,
	ref yamlStringRef,
	indentWidth int,
) (byteEdit, bool, error) {
	start, end, err := yamlScalarSpan(layout, ref.source.GetToken())
	if err != nil {
		return byteEdit{}, false, err
	}
	origin := string(layout.content[start:end])
	node := *ref.node
	node.HeadComment = ""
	node.LineComment = ""
	node.FootComment = ""
	replacement, err := encodeYAMLFragment(&node, indentWidth)
	if err != nil {
		return byteEdit{}, false, err
	}
	if replacement == origin {
		return byteEdit{}, false, nil
	}

	return byteEdit{start: start, end: end, replacement: []byte(replacement)}, true, nil
}

func yamlScalarSpan(layout sourceLayout, scalar *token.Token) (int, int, error) {
	if scalar == nil || scalar.Position == nil {
		return 0, 0, fmt.Errorf("%w: YAML scalar has no source position", ErrUnwritable)
	}
	start, err := layout.index(scalar.Position.Line, scalar.Position.Column)
	if err != nil {
		return 0, 0, err
	}
	end := len(layout.content)
	for next := scalar.Next; next != nil; next = next.Next {
		if next.Position == nil {
			continue
		}
		candidate, positionErr := layout.index(next.Position.Line, next.Position.Column)
		if positionErr != nil {
			return 0, 0, positionErr
		}
		if candidate > start {
			end = candidate
			break
		}
	}
	for end > start && strings.ContainsRune(" \t\r\n", rune(layout.content[end-1])) {
		end--
	}
	if end == start {
		return 0, 0, fmt.Errorf("%w: YAML scalar has an empty source span", ErrUnwritable)
	}

	return start, end, nil
}

type yamlVisitorFunc func(ast.Node) ast.Visitor

func (visit yamlVisitorFunc) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return nil
	}

	return visit(node)
}

func yamlNodeLastLine(node ast.Node) int {
	start := yamlCollectionStartToken(node, yamlCollectionSourceIsFlow(node))
	last := start.Position.Line
	var visitor yamlVisitorFunc
	visitor = func(current ast.Node) ast.Visitor {
		if currentToken := current.GetToken(); currentToken != nil && currentToken.Position != nil &&
			currentToken.Position.Line > last {
			last = currentToken.Position.Line
		}

		return visitor
	}
	ast.Walk(visitor, node)

	return last
}

func yamlCollectionSourceIsFlow(node ast.Node) bool {
	switch typed := node.(type) {
	case *ast.MappingNode:
		return typed.IsFlowStyle
	case *ast.SequenceNode:
		return typed.IsFlowStyle
	default:
		return false
	}
}
