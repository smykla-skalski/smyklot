package filemerge

import (
	"bytes"

	"github.com/yuin/goldmark/v2/ast"
)

func formatMarkdownLists(content []byte, spacing string) ([]byte, error) {
	document := parseMarkdownDocument(content)
	edits := make([]byteEdit, 0)
	err := ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		list, ok := node.(*ast.List)
		if !entering || !ok {
			return ast.WalkContinue, nil
		}
		paragraphs, safe := safeMarkdownListParagraphs(list)
		if !safe {
			return ast.WalkContinue, nil
		}
		edits = append(edits, markdownListSpacingEdits(content, paragraphs, spacing)...)

		return ast.WalkSkipChildren, nil
	})
	if err != nil {
		return nil, err
	}

	return applyByteEdits(content, edits)
}

func safeMarkdownListParagraphs(list *ast.List) ([]*ast.Paragraph, bool) {
	if list.ChildCount() < 2 {
		return nil, false
	}
	paragraphs := make([]*ast.Paragraph, 0, list.ChildCount())
	for child := list.FirstChild(); child != nil; child = child.NextSibling() {
		item, ok := child.(*ast.ListItem)
		if !ok || item.ChildCount() != 1 {
			return nil, false
		}
		paragraph, ok := item.FirstChild().(*ast.Paragraph)
		if !ok {
			return nil, false
		}
		paragraphs = append(paragraphs, paragraph)
	}

	return paragraphs, true
}

func markdownListSpacingEdits(
	content []byte,
	paragraphs []*ast.Paragraph,
	spacing string,
) []byteEdit {
	edits := make([]byteEdit, 0, len(paragraphs)-1)
	for index := 1; index < len(paragraphs); index++ {
		previous, previousOK := markdownBlockSpan(paragraphs[index-1], content)
		current, currentOK := markdownBlockSpan(paragraphs[index], content)
		if !previousOK || !currentOK {
			continue
		}
		start := previous.end
		end := markdownPhysicalLineStart(content, current.start)
		if start > end || len(bytes.TrimSpace(content[start:end])) != 0 {
			continue
		}
		eol := markdownNearbyLineEnding(content, tomlSpan{start: start, end: end})
		replacement := []byte(eol)
		if spacing == "loose" {
			replacement = []byte(eol + eol)
		}
		if !bytes.Equal(content[start:end], replacement) {
			edits = append(edits, byteEdit{start: start, end: end, replacement: replacement})
		}
	}

	return edits
}
