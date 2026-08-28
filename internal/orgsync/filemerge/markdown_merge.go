package filemerge

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
)

type markdownLine struct {
	text   string
	ending string
}

// mergeMarkdown applies section operations to a source-aware line document.
// Untouched lines retain their exact terminators; inserted lines inherit the
// replaced or nearest line before falling back to the document's dominant EOL.
func mergeMarkdown(template []byte, sections []Section) ([]byte, error) {
	lines := parseMarkdownLines(template)
	finalNewline := bytes.HasSuffix(template, []byte{'\n'})
	for index, section := range sections {
		applied, err := applyMarkdownSection(lines, section)
		if err != nil {
			return nil, fmt.Errorf("section %d (%s): %w", index+1, section.Action, err)
		}
		lines = applied
	}
	if len(lines) == 0 {
		return nil, nil
	}
	if finalNewline {
		if lines[len(lines)-1].ending == "" {
			lines[len(lines)-1].ending = markdownNearbyEOL(lines, len(lines)-1, len(lines))
		}
	} else {
		lines[len(lines)-1].ending = ""
	}

	return joinMarkdownLines(lines), nil
}

func applyMarkdownSection(lines []markdownLine, section Section) ([]markdownLine, error) {
	if section.Action == SectionAppend {
		return appendMarkdownContent(lines, section), nil
	}
	if section.Action == SectionPrepend {
		return prependMarkdownContent(lines, section), nil
	}
	written := headings(markdownLineTexts(lines))
	found, err := locate(written, section)
	if err != nil {
		return nil, err
	}
	start := written[found].line
	end := sectionEnd(written, found, len(lines))
	eol := markdownNearbyEOL(lines, start, end)
	content := markdownContentLines(section.Content, eol)
	switch section.Action {
	case SectionBefore:
		return replaceMarkdownLines(lines, start, start, content, eol), nil
	case SectionAfter:
		return replaceMarkdownLines(lines, end, end, content, eol), nil
	case SectionReplace:
		return replaceMarkdownLines(lines, start, end, content, eol), nil
	case SectionDelete:
		return replaceMarkdownLines(lines, start, end, nil, eol), nil
	case SectionPatch:
		return patchMarkdownBody(lines, start+written[found].span, end, section, eol)
	default:
		return nil, fmt.Errorf("%w: unknown action %q", ErrInvalidSpec, section.Action)
	}
}

func appendMarkdownContent(lines []markdownLine, section Section) []markdownLine {
	eol := markdownNearbyEOL(lines, len(lines), len(lines))
	content := markdownContentLines(section.Content, eol)
	body := trimMarkdownBlankTail(lines)
	if markdownEndsWith(body, content) {
		return lines
	}

	return joinMarkdownBlocks(eol, body, content)
}

func prependMarkdownContent(lines []markdownLine, section Section) []markdownLine {
	eol := markdownNearbyEOL(lines, 0, 0)
	content := markdownContentLines(section.Content, eol)
	body := trimMarkdownBlankHead(lines)
	if markdownStartsWith(body, content) {
		return lines
	}

	return joinMarkdownBlocks(eol, content, body)
}

func patchMarkdownBody(
	lines []markdownLine,
	body, end int,
	section Section,
	eol string,
) ([]markdownLine, error) {
	original := joinMarkdownLines(lines[body:end])
	patched := bytes.Clone(original)
	for index, patch := range section.Patches {
		find := markdownPatchBytes(patch.Find, eol)
		if !bytes.Contains(patched, find) {
			return nil, fmt.Errorf(
				"%w: patch %d does not find %q under %q",
				ErrNothingAddressed, index+1, patch.Find, section.Heading,
			)
		}
		replacement := markdownPatchBytes(patch.Replace, eol)
		patched = bytes.ReplaceAll(patched, find, replacement)
	}
	result := make([]markdownLine, 0, len(lines))
	result = append(result, lines[:body]...)
	result = append(result, parseMarkdownLines(patched)...)
	result = append(result, lines[end:]...)

	return ensureMarkdownInternalEOL(result, eol), nil
}

func markdownPatchBytes(value, eol string) []byte {
	normalized := strings.ReplaceAll(value, crlf, lf)
	if eol == crlf {
		normalized = strings.ReplaceAll(normalized, lf, crlf)
	}

	return []byte(normalized)
}

func replaceMarkdownLines(
	lines []markdownLine,
	start, end int,
	content []markdownLine,
	eol string,
) []markdownLine {
	content = trimMarkdownBlankHead(trimMarkdownBlankTail(content))
	result := append([]markdownLine(nil), lines[:start]...)
	result = appendMarkdownBlock(result, content, eol)
	result = appendMarkdownBlock(result, lines[end:], eol)

	return ensureMarkdownInternalEOL(result, eol)
}

func appendMarkdownBlock(result, block []markdownLine, eol string) []markdownLine {
	if len(block) == 0 {
		return result
	}
	if len(result) == 0 {
		return append(result, block...)
	}
	last := len(result) - 1
	if result[last].ending == "" {
		result[last].ending = eol
	}
	if strings.TrimSpace(result[last].text) != "" && strings.TrimSpace(block[0].text) != "" {
		result = append(result, markdownLine{text: "", ending: eol})
	}

	return append(result, block...)
}

func joinMarkdownBlocks(eol string, blocks ...[]markdownLine) []markdownLine {
	result := make([]markdownLine, 0)
	for _, block := range blocks {
		if len(block) == 0 {
			continue
		}
		if len(result) > 0 {
			result = append(result, markdownLine{text: "", ending: eol})
		}
		result = append(result, block...)
	}

	return ensureMarkdownInternalEOL(result, eol)
}

func ensureMarkdownInternalEOL(lines []markdownLine, eol string) []markdownLine {
	for index := range lines[:max(0, len(lines)-1)] {
		if lines[index].ending == "" {
			lines[index].ending = eol
		}
	}

	return lines
}

func parseMarkdownLines(content []byte) []markdownLine {
	lines := make([]markdownLine, 0, bytes.Count(content, []byte{'\n'})+1)
	for len(content) > 0 {
		newline := bytes.IndexByte(content, '\n')
		if newline < 0 {
			lines = append(lines, markdownLine{text: string(content)})
			break
		}
		ending := lf
		stop := newline
		if newline > 0 && content[newline-1] == '\r' {
			ending = crlf
			stop--
		}
		lines = append(lines, markdownLine{text: string(content[:stop]), ending: ending})
		content = content[newline+1:]
	}

	return lines
}

func markdownContentLines(content, eol string) []markdownLine {
	content = strings.TrimRight(content, "\r\n")
	if content == "" {
		return nil
	}
	lines := parseMarkdownLines([]byte(content))
	for index := range lines {
		lines[index].ending = eol
	}

	return lines
}

func joinMarkdownLines(lines []markdownLine) []byte {
	var output bytes.Buffer
	for _, line := range lines {
		output.WriteString(line.text)
		output.WriteString(line.ending)
	}

	return output.Bytes()
}

func markdownLineTexts(lines []markdownLine) []string {
	texts := make([]string, len(lines))
	for index, line := range lines {
		texts[index] = line.text
	}

	return texts
}

func markdownNearbyEOL(lines []markdownLine, start, end int) string {
	for index := start; index < end; index++ {
		if lines[index].ending != "" {
			return lines[index].ending
		}
	}
	for distance := 1; distance <= len(lines); distance++ {
		if before := start - distance; before >= 0 && lines[before].ending != "" {
			return lines[before].ending
		}
		if after := end + distance - 1; after < len(lines) && lines[after].ending != "" {
			return lines[after].ending
		}
	}

	return lf
}

func trimMarkdownBlankTail(lines []markdownLine) []markdownLine {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1].text) == "" {
		lines = lines[:len(lines)-1]
	}

	return lines
}

func trimMarkdownBlankHead(lines []markdownLine) []markdownLine {
	for len(lines) > 0 && strings.TrimSpace(lines[0].text) == "" {
		lines = lines[1:]
	}

	return lines
}

func markdownEndsWith(lines, tail []markdownLine) bool {
	if len(tail) > len(lines) {
		return false
	}

	return slices.Equal(markdownLineTexts(lines[len(lines)-len(tail):]), markdownLineTexts(tail))
}

func markdownStartsWith(lines, head []markdownLine) bool {
	if len(head) > len(lines) {
		return false
	}

	return slices.Equal(markdownLineTexts(lines[:len(head)]), markdownLineTexts(head))
}
