package filemerge

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark/v2/ast"
	mdast "github.com/yuin/goldmark/v2/extension/ast"
)

type markdownTableRow struct {
	indent   string
	cells    []string
	leading  bool
	trailing bool
	ending   string
}

func formatMarkdownTables(content []byte, choice string) ([]byte, error) {
	document := parseMarkdownDocument(content)
	edits := make([]byteEdit, 0)
	err := ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		table, ok := node.(*mdast.Table)
		if !entering || !ok || table.Parent() == nil || table.Parent().Kind() != ast.KindDocument {
			return ast.WalkContinue, nil
		}
		span, rows, alignments, safe := readMarkdownTable(content, table)
		if !safe {
			return ast.WalkSkipChildren, nil
		}
		replacement := renderMarkdownTable(rows, alignments, choice)
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

func readMarkdownTable(
	content []byte,
	table *mdast.Table,
) (tomlSpan, []markdownTableRow, []mdast.Alignment, bool) {
	header, ok := table.FirstChild().(*mdast.TableHeader)
	if !ok {
		return tomlSpan{}, nil, nil, false
	}
	alignments, ok := markdownTableAlignments(header)
	if !ok {
		return tomlSpan{}, nil, nil, false
	}
	rowCount := 2
	if body, bodyOK := table.LastChild().(*mdast.TableBody); bodyOK {
		rowCount += body.ChildCount()
	}
	start := markdownPhysicalLineStart(content, table.Pos())
	lines, end, ok := markdownSourceRows(content, start, rowCount)
	if !ok {
		return tomlSpan{}, nil, nil, false
	}
	rows := make([]markdownTableRow, len(lines))
	for index, line := range lines {
		row, rowOK := parseMarkdownTableRow(line)
		if !rowOK || len(row.cells) != len(alignments) ||
			index > 0 && (row.leading != rows[0].leading || row.trailing != rows[0].trailing) {
			return tomlSpan{}, nil, nil, false
		}
		rows[index] = row
	}

	return tomlSpan{start: start, end: end}, rows, alignments, true
}

func markdownTableAlignments(header *mdast.TableHeader) ([]mdast.Alignment, bool) {
	alignments := make([]mdast.Alignment, 0, header.ChildCount())
	for child := header.FirstChild(); child != nil; child = child.NextSibling() {
		cell, ok := child.(*mdast.TableCell)
		if !ok {
			return nil, false
		}
		alignments = append(alignments, cell.Alignment)
	}

	return alignments, true
}

func markdownSourceRows(content []byte, start, count int) ([][]byte, int, bool) {
	rows := make([][]byte, 0, count)
	offset := start
	for range count {
		if offset >= len(content) {
			return nil, 0, false
		}
		newline := bytes.IndexByte(content[offset:], '\n')
		if newline < 0 {
			rows = append(rows, content[offset:])
			offset = len(content)
			continue
		}
		end := offset + newline + 1
		rows = append(rows, content[offset:end])
		offset = end
	}

	return rows, offset, true
}

func parseMarkdownTableRow(source []byte) (markdownTableRow, bool) {
	line := parseMarkdownLines(source)
	if len(line) != 1 || strings.ContainsAny(line[0].text, "`\t") || strings.Contains(line[0].text, `\|`) {
		return markdownTableRow{}, false
	}
	text := line[0].text
	trimmed := strings.TrimLeft(text, " ")
	indent := text[:len(text)-len(trimmed)]
	if len(indent) > widestIndent {
		return markdownTableRow{}, false
	}
	trimmed = strings.TrimSpace(trimmed)
	leading := strings.HasPrefix(trimmed, "|")
	trailing := strings.HasSuffix(trimmed, "|")
	if leading {
		trimmed = strings.TrimPrefix(trimmed, "|")
	}
	if trailing {
		trimmed = strings.TrimSuffix(trimmed, "|")
	}
	parts := strings.Split(trimmed, "|")
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}

	return markdownTableRow{
		indent: indent, cells: parts, leading: leading, trailing: trailing, ending: line[0].ending,
	}, true
}

func renderMarkdownTable(rows []markdownTableRow, alignments []mdast.Alignment, choice string) []byte {
	widths := markdownTableWidths(rows, alignments, choice)
	var output bytes.Buffer
	for index, row := range rows {
		cells := row.cells
		if index == 1 {
			cells = markdownTableDelimiterCells(widths, alignments, choice)
		}
		output.WriteString(row.indent)
		output.WriteString(renderMarkdownTableRow(cells, widths, row.leading, row.trailing, choice))
		output.WriteString(row.ending)
	}

	return output.Bytes()
}

func markdownTableWidths(
	rows []markdownTableRow,
	alignments []mdast.Alignment,
	choice string,
) []int {
	widths := make([]int, len(alignments))
	if choice == formatCompact {
		return widths
	}
	for index, alignment := range alignments {
		widths[index] = markdownTableMinimumWidth(alignment)
	}
	for rowIndex, row := range rows {
		if rowIndex == 1 {
			continue
		}
		for cellIndex, cell := range row.cells {
			widths[cellIndex] = max(widths[cellIndex], utf8.RuneCountInString(cell))
		}
	}

	return widths
}

func markdownTableMinimumWidth(alignment mdast.Alignment) int {
	switch alignment {
	case mdast.AlignCenter:
		return 5
	case mdast.AlignLeft, mdast.AlignRight:
		return 4
	default:
		return 3
	}
}

func markdownTableDelimiterCells(
	widths []int,
	alignments []mdast.Alignment,
	choice string,
) []string {
	cells := make([]string, len(widths))
	for index, alignment := range alignments {
		width := widths[index]
		if choice == formatCompact {
			width = markdownTableMinimumWidth(alignment)
		}
		switch alignment {
		case mdast.AlignLeft:
			cells[index] = ":" + strings.Repeat("-", width-1)
		case mdast.AlignRight:
			cells[index] = strings.Repeat("-", width-1) + ":"
		case mdast.AlignCenter:
			cells[index] = ":" + strings.Repeat("-", width-2) + ":"
		default:
			cells[index] = strings.Repeat("-", width)
		}
	}

	return cells
}

func renderMarkdownTableRow(
	cells []string,
	widths []int,
	leading, trailing bool,
	choice string,
) string {
	separator := " | "
	prefix, suffix := "", ""
	if choice == formatCompact {
		separator = "|"
		if leading {
			prefix = "|"
		}
		if trailing {
			suffix = "|"
		}
		return prefix + strings.Join(cells, separator) + suffix
	}
	padded := make([]string, len(cells))
	for index, cell := range cells {
		padded[index] = cell + strings.Repeat(" ", widths[index]-utf8.RuneCountInString(cell))
	}
	if leading {
		prefix = "| "
	}
	if trailing {
		suffix = " |"
	}

	return prefix + strings.Join(padded, separator) + suffix
}
