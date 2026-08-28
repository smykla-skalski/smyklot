package filemerge

import (
	"bytes"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

type tomlAssignmentBlock struct {
	key  string
	span tomlSpan
}

func sortTOMLAssignments(content []byte) ([]byte, error) {
	document, err := parseTOMLSyntax(content)
	if err != nil {
		return nil, err
	}
	edits := make([]byteEdit, 0)
	for _, section := range document.sections {
		assignments := tomlSectionAssignments(document.assignments, section)
		for _, group := range tomlAssignmentGroups(content, assignments, false) {
			if len(group) < 2 {
				continue
			}
			blocks := tomlSortableBlocks(group)
			sorted := slices.Clone(blocks)
			slices.SortStableFunc(sorted, func(one, two tomlAssignmentBlock) int {
				return strings.Compare(one.key, two.key)
			})
			if slices.EqualFunc(blocks, sorted, func(one, two tomlAssignmentBlock) bool {
				return one.key == two.key
			}) {
				continue
			}
			var replacement bytes.Buffer
			for _, block := range sorted {
				replacement.Write(content[block.span.start:block.span.end])
			}
			edits = append(edits, byteEdit{
				start: blocks[0].span.start, end: blocks[len(blocks)-1].span.end,
				replacement: replacement.Bytes(),
			})
		}
	}

	return applyByteEdits(content, edits)
}

func alignTOMLAssignments(content []byte, policy config.FormattingPolicy) ([]byte, error) {
	var err error
	if policy.TOML.AlignEntries != formatPreserve {
		content, err = alignTOMLEntries(content, policy.TOML.AlignEntries)
		if err != nil {
			return nil, err
		}
	}
	if policy.TOML.AlignComments != formatPreserve {
		content, err = alignTOMLComments(content, policy.TOML.AlignComments)
		if err != nil {
			return nil, err
		}
	}

	return content, nil
}

func alignTOMLEntries(content []byte, choice string) ([]byte, error) {
	document, err := parseTOMLSyntax(content)
	if err != nil {
		return nil, err
	}
	edits := make([]byteEdit, 0)
	for _, section := range document.sections {
		assignments := tomlSectionAssignments(document.assignments, section)
		for _, group := range tomlAssignmentGroups(content, assignments, true) {
			maximum := 0
			for _, assignment := range group {
				keyEnd := tomlEntrySeparator(content, assignment)
				maximum = max(maximum, utf8.RuneCount(content[assignment.expression.start:keyEnd]))
			}
			for _, assignment := range group {
				keyEnd := tomlEntrySeparator(content, assignment)
				spaces := 1
				if choice == "align" {
					spaces += maximum - utf8.RuneCount(content[assignment.expression.start:keyEnd])
				}
				replacement := strings.Repeat(" ", spaces) + "= "
				edits = append(edits, byteEdit{
					start: keyEnd, end: assignment.value.span.start,
					replacement: []byte(replacement),
				})
			}
		}
	}

	return applyByteEdits(content, edits)
}

func alignTOMLComments(content []byte, choice string) ([]byte, error) {
	document, err := parseTOMLSyntax(content)
	if err != nil {
		return nil, err
	}
	edits := make([]byteEdit, 0)
	for _, section := range document.sections {
		assignments := tomlSectionAssignments(document.assignments, section)
		for _, group := range tomlAssignmentGroups(content, assignments, true) {
			edits = append(edits, tomlCommentAlignmentEdits(content, group, choice)...)
		}
	}

	return applyByteEdits(content, edits)
}

func tomlCommentAlignmentEdits(
	content []byte,
	group []tomlAssignmentRef,
	choice string,
) []byteEdit {
	maximum := 0
	for _, assignment := range group {
		if assignment.comment != nil {
			maximum = max(maximum, tomlExpressionWidth(content, assignment))
		}
	}
	edits := make([]byteEdit, 0, len(group))
	for _, assignment := range group {
		if assignment.comment == nil {
			continue
		}
		spaces := 1
		if choice == "align" {
			spaces += maximum - tomlExpressionWidth(content, assignment)
		}
		edits = append(edits, byteEdit{
			start: assignment.expression.end, end: assignment.comment.start,
			replacement: []byte(strings.Repeat(" ", spaces)),
		})
	}

	return edits
}

func tomlSectionAssignments(
	all []tomlAssignmentRef,
	section tomlSectionRef,
) []tomlAssignmentRef {
	assignments := make([]tomlAssignmentRef, 0)
	for _, assignment := range all {
		if !assignment.arrayTable && !assignment.inline &&
			assignment.expression.start >= section.bodyStart &&
			assignment.expression.start < section.end {
			assignments = append(assignments, assignment)
		}
	}

	return assignments
}

func tomlAssignmentGroups(
	content []byte,
	assignments []tomlAssignmentRef,
	singleLine bool,
) [][]tomlAssignmentRef {
	groups := make([][]tomlAssignmentRef, 0)
	startNewGroup := false
	for _, assignment := range assignments {
		if singleLine && bytes.ContainsAny(
			content[assignment.expression.start:assignment.expression.end], "\r\n",
		) {
			startNewGroup = true
			continue
		}
		if len(groups) == 0 || startNewGroup {
			groups = append(groups, []tomlAssignmentRef{assignment})
			startNewGroup = false
			continue
		}
		group := groups[len(groups)-1]
		previous := group[len(group)-1]
		gap := content[previous.line.end:assignment.line.start]
		if tomlGapBreaksGroup(gap, singleLine) {
			groups = append(groups, []tomlAssignmentRef{assignment})
		} else {
			groups[len(groups)-1] = append(group, assignment)
		}
	}

	return groups
}

func tomlGapBreaksGroup(gap []byte, commentsBreak bool) bool {
	if commentsBreak && bytes.Contains(gap, []byte{'#'}) {
		return true
	}
	normalized := bytes.ReplaceAll(gap, []byte(crlf), []byte(lf))
	lines := bytes.Split(normalized, []byte(lf))
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	for _, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			return true
		}
	}

	return false
}

func tomlSortableBlocks(assignments []tomlAssignmentRef) []tomlAssignmentBlock {
	blocks := make([]tomlAssignmentBlock, len(assignments))
	for index, assignment := range assignments {
		start := assignment.line.start
		if index > 0 {
			start = assignments[index-1].line.end
		}
		blocks[index] = tomlAssignmentBlock{
			key: tomlPathKey(assignment.path), span: tomlSpan{start: start, end: assignment.line.end},
		}
	}

	return blocks
}

func tomlEntrySeparator(content []byte, assignment tomlAssignmentRef) int {
	prefix := content[assignment.expression.start:assignment.value.span.start]
	relative := bytes.LastIndexByte(prefix, '=')
	equals := assignment.expression.start + relative
	keyEnd := equals
	for keyEnd > assignment.expression.start &&
		(content[keyEnd-1] == ' ' || content[keyEnd-1] == '\t') {
		keyEnd--
	}

	return keyEnd
}

func tomlExpressionWidth(content []byte, assignment tomlAssignmentRef) int {
	return utf8.RuneCount(content[assignment.line.start:assignment.expression.end])
}
