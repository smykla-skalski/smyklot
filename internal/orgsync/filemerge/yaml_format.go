package filemerge

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/lexer"
	"github.com/goccy/go-yaml/parser"
	"github.com/goccy/go-yaml/token"
	"github.com/smykla-skalski/smyklot/pkg/config"
	yaml "go.yaml.in/yaml/v3"
)

type yamlPresentation struct {
	tokens map[string]int
}

func formatYAMLDocument(content []byte, policy config.FormattingPolicy) ([]byte, error) {
	before, err := decodeYAMLSemantic(content)
	if err != nil {
		return nil, err
	}
	presentation := yamlPresentationOf(content)
	formatted, err := reindentYAML(content, policy.Common)
	if err != nil {
		return nil, err
	}
	file, err := parseGoccyYAML(formatted)
	if err != nil {
		return nil, err
	}
	renderDocument, err := parseYAMLFormatDocument(formatted)
	if err != nil {
		return nil, err
	}
	edits, err := yamlSourceEdits(formatted, file.Docs[0], renderDocument, policy)
	if err != nil {
		return nil, err
	}
	formatted, err = applyByteEdits(formatted, edits)
	if err != nil {
		return nil, err
	}
	formatted, err = reindentYAMLSequences(formatted, policy)
	if err != nil {
		return nil, err
	}

	if _, err := parseGoccyYAML(formatted); err != nil {
		return nil, fmt.Errorf("%w: formatted YAML did not parse: %w", ErrUnwritable, err)
	}
	after, err := decodeYAMLSemantic(formatted)
	if err != nil || !reflect.DeepEqual(before, after) {
		return nil, fmt.Errorf("%w: formatting changed the YAML value", ErrUnwritable)
	}
	afterPresentation := yamlPresentationOf(formatted)
	if !presentation.equal(afterPresentation) {
		return nil, fmt.Errorf("%w: formatting changed YAML comments or special syntax", ErrUnwritable)
	}

	return formatted, nil
}

func parseGoccyYAML(content []byte) (*ast.File, error) {
	tokens := lexer.Tokenize(string(content))
	if invalid := tokens.InvalidToken(); invalid != nil {
		return nil, fmt.Errorf("%w: %s", ErrUnreadable, invalid.Error)
	}
	file, err := parser.Parse(tokens, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}
	if len(file.Docs) != 1 || file.Docs[0] == nil || file.Docs[0].Body == nil {
		if len(file.Docs) > 1 {
			return nil, fmt.Errorf("%w: %w", ErrUnreadable, errTrailingContent)
		}

		return nil, fmt.Errorf("%w: it holds no document", ErrUnreadable)
	}

	return file, nil
}

func yamlASTFormattingActive(policy config.FormattingPolicy) bool {
	return policy.Common.IndentStyle != formatPreserve ||
		policy.YAML.Sequences != formatPreserve || policy.YAML.Mappings != formatPreserve ||
		policy.YAML.QuoteStyle != formatPreserve || policy.YAML.SequenceIndent != formatPreserve
}

func yamlNodeFormattingActive(policy config.FormattingPolicy) bool {
	return policy.YAML.Sequences != formatPreserve ||
		policy.YAML.Mappings != formatPreserve ||
		policy.YAML.QuoteStyle != formatPreserve
}

func yamlPresentationOf(content []byte) yamlPresentation {
	presentation := yamlPresentation{tokens: make(map[string]int)}
	for _, item := range lexer.Tokenize(string(content)) {
		switch item.Type {
		case token.CommentType:
			presentation.tokens[item.Type.String()+"\x00"+item.Value]++
		case token.AnchorType, token.AliasType, token.TagType,
			token.DirectiveType, token.MergeKeyType, token.LiteralType, token.FoldedType,
			token.DocumentEndType:
			presentation.tokens[item.Type.String()+"\x00"+strings.TrimSpace(item.Origin)]++
		}
	}

	return presentation
}

func (presentation yamlPresentation) equal(other yamlPresentation) bool {
	return reflect.DeepEqual(presentation.tokens, other.tokens)
}

func parseYAMLFormatDocument(content []byte) (*yaml.Node, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}
	var trailing yaml.Node
	switch err := decoder.Decode(&trailing); {
	case errors.Is(err, io.EOF):
	case err != nil:
		return nil, fmt.Errorf("%w: %w", ErrUnreadable, err)
	default:
		return nil, fmt.Errorf("%w: %w", ErrUnreadable, errTrailingContent)
	}
	if err := refuseRepeatedKeys(&document); err != nil {
		return nil, err
	}

	return &document, nil
}

func reindentYAML(content []byte, common config.FormattingCommonPolicy) ([]byte, error) {
	if common.IndentStyle == formatPreserve {
		return content, nil
	}
	if common.IndentStyle == formatTabs {
		return nil, fmt.Errorf("%w: YAML indentation cannot use tabs", ErrUnwritable)
	}
	current := yamlIndentWidth(content, common.IndentWidth)
	if current == common.IndentWidth {
		return content, nil
	}
	lines := bytes.SplitAfter(content, []byte{'\n'})
	for index, line := range lines {
		body := bytes.TrimSuffix(line, []byte{'\n'})
		body = bytes.TrimSuffix(body, []byte{'\r'})
		leading := len(body) - len(bytes.TrimLeft(body, " "))
		if leading == 0 || len(bytes.TrimSpace(body)) == 0 {
			continue
		}
		if leading%current != 0 {
			return nil, fmt.Errorf(
				"%w: YAML line %d has ambiguous %d-space indentation", ErrUnwritable, index+1, leading,
			)
		}
		replacement := bytes.Repeat([]byte{' '}, leading/current*common.IndentWidth)
		lines[index] = concatBytes(replacement, line[leading:])
	}

	return bytes.Join(lines, nil), nil
}

func concatBytes(parts ...[]byte) []byte {
	length := 0
	for _, part := range parts {
		length += len(part)
	}
	joined := make([]byte, 0, length)
	for _, part := range parts {
		joined = append(joined, part...)
	}

	return joined
}

func yamlIndentWidth(content []byte, fallback int) int {
	minimum := 0
	for _, line := range bytes.Split(content, []byte{'\n'}) {
		body := bytes.TrimSuffix(line, []byte{'\r'})
		leading := len(body) - len(bytes.TrimLeft(body, " "))
		if leading == 0 || len(bytes.TrimSpace(body)) == 0 {
			continue
		}
		if minimum == 0 || leading < minimum {
			minimum = leading
		}
	}
	if minimum > 0 {
		return minimum
	}

	return fallback
}

func decodeYAMLSemantic(content []byte) (any, error) {
	var value any
	if err := yaml.Unmarshal(content, &value); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}

	return value, nil
}
