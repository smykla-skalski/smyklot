package filemerge

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/goccy/go-yaml/lexer"
	"github.com/goccy/go-yaml/token"
	yaml "go.yaml.in/yaml/v3"
)

// An unterminated YAML block scalar can retain the new final line break as
// part of its value. Choose strip chomping for that scalar before adding the
// required file terminator, then prove that the decoded value did not change.
func terminateYAMLTemplate(content []byte) ([]byte, error) {
	final := TerminateTemplate(content)
	if bytes.Equal(content, final) {
		return content, nil
	}
	if _, err := decodeYAMLSemantic(content); err != nil {
		return nil, err
	}
	before, err := parseYAMLFormatDocument(content)
	if err != nil {
		return nil, err
	}
	after, err := parseYAMLFormatDocument(final)
	if err != nil {
		return nil, err
	}
	if equalYAMLTemplateValues(before, after) {
		return final, nil
	}
	last := lastYAMLBlockIndicator(content)
	if last == nil {
		return nil, fmt.Errorf("%w: the required final newline changes the YAML value", ErrUnwritable)
	}
	lines := strings.Split(string(content), "\n")
	line := lines[last.Position.Line-1]
	// Tagged scalars have unreliable column marks in both YAML parsers. The
	// lexer identifies the header line; match its validated indicator before
	// the optional trailing comment, preserving anchors, tags and indentation.
	match := yamlBlockIndicator.FindStringSubmatchIndex(line)
	if match == nil {
		return nil, fmt.Errorf("%w: cannot locate the final YAML block scalar", ErrUnwritable)
	}
	start, end := match[4], match[5]
	indicator := strings.ReplaceAll(line[start:end], "+", "")
	if !strings.Contains(indicator, "-") {
		indicator += "-"
	}
	lines[last.Position.Line-1] = line[:start] + indicator + line[end:]
	final = TerminateTemplate([]byte(strings.Join(lines, "\n")))
	after, err = parseYAMLFormatDocument(final)
	if err != nil || !equalYAMLTemplateValues(before, after) {
		return nil, fmt.Errorf("%w: the required final newline changes the YAML value", ErrUnwritable)
	}
	return final, nil
}

var yamlBlockIndicator = regexp.MustCompile(`(^|[ \t])([|>][1-9+-]*)[ \t]*(?:#.*)?\r?$`)

func lastYAMLBlockIndicator(content []byte) *token.Token {
	var last *token.Token
	for _, candidate := range lexer.Tokenize(string(content)) {
		if candidate.Type == token.LiteralType || candidate.Type == token.FoldedType {
			last = candidate
		}
	}
	return last
}

// Only termination and a block's chomping indicator may change here. Compare
// the parsed values directly, retaining NaN keys and values without passing
// through Go maps, where a NaN key cannot even retrieve its own value.
func equalYAMLTemplateValues(before, after *yaml.Node) bool {
	if before.Kind != after.Kind || before.Tag != after.Tag || before.Value != after.Value ||
		before.Anchor != after.Anchor || len(before.Content) != len(after.Content) {
		return false
	}
	for index, child := range before.Content {
		if !equalYAMLTemplateValues(child, after.Content[index]) {
			return false
		}
	}
	return true
}
