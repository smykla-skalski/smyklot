package filemerge

import (
	"bytes"
	"fmt"
	"slices"
	"unicode/utf8"
)

type byteEdit struct {
	start       int
	end         int
	replacement []byte
}

type sourceLine struct {
	start      int
	contentEnd int
	end        int
}

type sourceLayout struct {
	content []byte
	lines   []sourceLine
}

func newSourceLayout(content []byte) sourceLayout {
	lines := make([]sourceLine, 0, bytes.Count(content, []byte{'\n'})+1)
	start := 0
	for start < len(content) {
		newline := bytes.IndexByte(content[start:], '\n')
		if newline < 0 {
			lines = append(lines, sourceLine{start: start, contentEnd: len(content), end: len(content)})

			return sourceLayout{content: content, lines: lines}
		}
		newline += start
		contentEnd := newline
		if contentEnd > start && content[contentEnd-1] == '\r' {
			contentEnd--
		}
		lines = append(lines, sourceLine{start: start, contentEnd: contentEnd, end: newline + 1})
		start = newline + 1
	}
	if len(content) == 0 || start == len(content) {
		lines = append(lines, sourceLine{start: start, contentEnd: start, end: start})
	}

	return sourceLayout{content: content, lines: lines}
}

func (layout sourceLayout) index(line, column int) (int, error) {
	if line < 1 || line > len(layout.lines) || column < 1 {
		return 0, fmt.Errorf("%w: source position %d:%d is outside the document", ErrUnwritable, line, column)
	}
	entry := layout.lines[line-1]
	text := layout.content[entry.start:entry.contentEnd]
	index := 0
	for current := 1; current < column; current++ {
		if index >= len(text) {
			return 0, fmt.Errorf("%w: source position %d:%d is outside its line", ErrUnwritable, line, column)
		}
		_, width := utf8.DecodeRune(text[index:])
		if width == 0 {
			return 0, fmt.Errorf("%w: source position %d:%d cannot be decoded", ErrUnwritable, line, column)
		}
		index += width
	}

	return entry.start + index, nil
}

func (layout sourceLayout) contentEnd(line int) (int, error) {
	entry, err := layout.line(line)
	if err != nil {
		return 0, err
	}

	return entry.contentEnd, nil
}

func (layout sourceLayout) wholeLine(line int) (int, int, error) {
	entry, err := layout.line(line)
	if err != nil {
		return 0, 0, err
	}

	return entry.start, entry.end, nil
}

func (layout sourceLayout) line(line int) (sourceLine, error) {
	if line < 1 || line > len(layout.lines) {
		return sourceLine{}, fmt.Errorf("%w: source line %d is outside the document", ErrUnwritable, line)
	}

	return layout.lines[line-1], nil
}

func applyByteEdits(content []byte, edits []byteEdit) ([]byte, error) {
	if len(edits) == 0 {
		return content, nil
	}
	edits = slices.Clone(edits)
	slices.SortFunc(edits, func(one, two byteEdit) int {
		if one.start != two.start {
			return two.start - one.start
		}

		return two.end - one.end
	})
	previousStart := len(content)
	for _, edit := range edits {
		if edit.start < 0 || edit.end < edit.start || edit.end > len(content) {
			return nil, fmt.Errorf("%w: source edit [%d:%d] is outside a %d-byte document",
				ErrUnwritable, edit.start, edit.end, len(content))
		}
		if edit.end > previousStart {
			return nil, fmt.Errorf("%w: source edits overlap", ErrUnwritable)
		}
		content = slices.Concat(content[:edit.start], edit.replacement, content[edit.end:])
		previousStart = edit.start
	}

	return content, nil
}
