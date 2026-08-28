package filemerge

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/yuin/goldmark/v2/ast"
)

func formatMarkdownProse(content []byte, policy config.FormattingPolicy) ([]byte, error) {
	document := parseMarkdownDocument(content)
	frontmatterEnd := markdownFrontmatterEnd(content)
	edits := make([]byteEdit, 0)
	err := ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		paragraph, ok := node.(*ast.Paragraph)
		if !entering || !ok || !safeMarkdownProse(paragraph, frontmatterEnd) {
			return ast.WalkContinue, nil
		}
		span, ok := markdownBlockSpan(paragraph, content)
		if !ok {
			return ast.WalkContinue, nil
		}
		replacement := renderMarkdownProse(content[span.start:span.end], policy, markdownNearbyLineEnding(content, span))
		if !bytes.Equal(replacement, content[span.start:span.end]) {
			edits = append(edits, byteEdit{start: span.start, end: span.end, replacement: replacement})
		}

		return ast.WalkSkipChildren, nil
	})
	if err != nil {
		return nil, err
	}

	return applyByteEdits(content, edits)
}

func safeMarkdownProse(paragraph *ast.Paragraph, frontmatterEnd int) bool {
	if paragraph.Parent() == nil || paragraph.Parent().Kind() != ast.KindDocument ||
		paragraph.Pos() < frontmatterEnd || paragraph.FirstChild() == nil {
		return false
	}
	for child := paragraph.FirstChild(); child != nil; child = child.NextSibling() {
		text, ok := child.(*ast.Text)
		if !ok || text.HardLineBreak() {
			return false
		}
	}

	return true
}

func renderMarkdownProse(content []byte, policy config.FormattingPolicy, eol string) []byte {
	lines := parseMarkdownLines(content)
	words := make([]string, 0)
	for _, line := range lines {
		words = append(words, strings.Fields(line.text)...)
	}
	if len(words) == 0 {
		return bytes.Clone(content)
	}
	if policy.Markdown.ProseWrap == "never" {
		return []byte(strings.Join(words, " "))
	}
	wrapped := make([]string, 0)
	current := words[0]
	for _, word := range words[1:] {
		if utf8.RuneCountInString(current)+1+utf8.RuneCountInString(word) <= policy.Common.LineWidth {
			current += " " + word
			continue
		}
		wrapped = append(wrapped, current)
		current = word
	}
	wrapped = append(wrapped, current)

	return []byte(strings.Join(wrapped, eol))
}
