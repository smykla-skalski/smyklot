package filemerge

import (
	"bytes"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	"github.com/yuin/goldmark/v2/parser"
)

func parseMarkdownDocument(content []byte) ast.Node {
	return parser.New(parser.WithExtensions(extension.GFMParser)).Parse(content)
}

func markdownSemanticDigest(content []byte) (string, error) {
	document := parseMarkdownDocument(content)
	var digest strings.Builder
	if err := writeMarkdownSemanticNode(&digest, document, content); err != nil {
		return "", fmt.Errorf("%w: %w", ErrUnreadable, err)
	}

	return digest.String(), nil
}

func writeMarkdownSemanticNode(output *strings.Builder, node ast.Node, source []byte) error {
	if text, ok := node.(*ast.Text); ok {
		writeMarkdownSemanticText(output, text, source)

		return nil
	}
	output.WriteByte('(')
	output.WriteString(node.Kind().String())
	dump := node.Dump(source)
	keys := make([]string, 0, len(dump.Properties))
	for key := range dump.Properties {
		if !markdownPresentationProperty(node, key) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		output.WriteByte('|')
		output.WriteString(key)
		output.WriteByte('=')
		if err := writeMarkdownSemanticValue(output, dump.Properties[key], source); err != nil {
			return err
		}
	}
	attributes := slices.Clone(node.Attributes())
	sort.Slice(attributes, func(one, other int) bool {
		return attributes[one].Name < attributes[other].Name
	})
	for _, attribute := range attributes {
		output.WriteString("|@")
		output.WriteString(attribute.Name)
		output.WriteByte('=')
		output.WriteString(strconv.Quote(normalizeMarkdownEOL(attribute.Value.Str(source))))
	}
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if err := writeMarkdownSemanticNode(output, child, source); err != nil {
			return err
		}
	}
	output.WriteByte(')')

	return nil
}

func markdownPresentationProperty(node ast.Node, key string) bool {
	return node.Kind() == ast.KindList && key == "Tight" ||
		node.Kind() == ast.KindHeading && key == "HeadingKind" ||
		node.Kind() == ast.KindCodeBlock && key == "CodeBlockKind"
}

func writeMarkdownSemanticText(output *strings.Builder, node *ast.Text, source []byte) {
	raw := node.Value.Str(source)
	leadingSpace := len(strings.TrimLeftFunc(raw, unicode.IsSpace)) != len(raw)
	trailingSpace := len(strings.TrimRightFunc(raw, unicode.IsSpace)) != len(raw)
	value := strings.Join(strings.Fields(raw), " ")
	if leadingSpace {
		output.WriteByte(' ')
	}
	if value != "" {
		output.WriteString(value)
	}
	if node.HardLineBreak() {
		output.WriteString("<hard-break>")
	} else if node.SoftLineBreak() || trailingSpace {
		output.WriteByte(' ')
	}
}

func writeMarkdownSemanticValue(output *strings.Builder, value any, source []byte) error {
	switch typed := value.(type) {
	case interface{ Str([]byte) string }:
		output.WriteString(strconv.Quote(normalizeMarkdownEOL(typed.Str(source))))
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		output.WriteByte('{')
		for _, key := range keys {
			output.WriteString(strconv.Quote(key))
			output.WriteByte(':')
			if err := writeMarkdownSemanticValue(output, typed[key], source); err != nil {
				return err
			}
		}
		output.WriteByte('}')
	case []any:
		output.WriteByte('[')
		for _, child := range typed {
			if err := writeMarkdownSemanticValue(output, child, source); err != nil {
				return err
			}
		}
		output.WriteByte(']')
	case string:
		output.WriteString(strconv.Quote(normalizeMarkdownEOL(typed)))
	default:
		if _, err := fmt.Fprint(output, typed); err != nil {
			return err
		}
	}

	return nil
}

func normalizeMarkdownEOL(value string) string {
	return strings.ReplaceAll(value, crlf, lf)
}

func markdownFrontmatterEnd(content []byte) int {
	lines := parseMarkdownLines(content)
	if len(lines) < 2 || (lines[0].text != "---" && lines[0].text != "+++") {
		return 0
	}
	delimiter := lines[0].text
	offset := len(lines[0].text) + len(lines[0].ending)
	for _, line := range lines[1:] {
		offset += len(line.text) + len(line.ending)
		if line.text == delimiter {
			return offset
		}
	}

	return 0
}

func markdownBlockSpan(block ast.BlockNode, content []byte) (tomlSpan, bool) {
	segments := block.Source()
	if len(segments) == 0 {
		return tomlSpan{}, false
	}
	start := segments[0].Start
	end := segments[len(segments)-1].Stop
	if start < 0 || end < start || end > len(content) {
		return tomlSpan{}, false
	}
	for end > start && (content[end-1] == '\n' || content[end-1] == '\r') {
		end--
	}

	return tomlSpan{start: start, end: end}, true
}

func markdownPhysicalLineStart(content []byte, offset int) int {
	return bytes.LastIndexByte(content[:offset], '\n') + 1
}

func markdownNearbyLineEnding(content []byte, span tomlSpan) string {
	if ending := firstLineEnding(content[span.start:span.end]); ending != "" {
		return ending
	}
	if ending := firstLineEnding(content[span.end:]); ending != "" {
		return ending
	}
	if ending := lastLineEnding(content[:span.start]); ending != "" {
		return ending
	}

	return lf
}

func lastLineEnding(content []byte) string {
	newline := bytes.LastIndexByte(content, '\n')
	if newline < 0 {
		return ""
	}
	if newline > 0 && content[newline-1] == '\r' {
		return crlf
	}

	return lf
}
