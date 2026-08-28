package filemerge

import (
	"bytes"
	"fmt"
	"slices"
	"sort"

	"github.com/pelletier/go-toml/v2/unstable"
)

type tomlInlineMember struct {
	content     []byte
	sourceIndex int
}

func (renderer *tomlMergeRenderer) replaceInlineTable(
	assignment tomlAssignmentRef,
	base, merged map[string]any,
) error {
	replacement, err := renderer.renderInlineTable(assignment.value, assignment.path, base, merged)
	if err != nil {
		return err
	}
	renderer.edits = append(renderer.edits, byteEdit{
		start: assignment.value.span.start, end: assignment.value.span.end,
		replacement: replacement,
	})

	return nil
}

func (renderer *tomlMergeRenderer) renderInlineTable(
	value *tomlValueRef,
	path []string,
	base, merged map[string]any,
) ([]byte, error) {
	assignments, err := renderer.directInlineAssignments(value, path)
	if err != nil {
		return nil, err
	}
	members, err := renderer.renderExistingInlineMembers(assignments, base, merged)
	if err != nil {
		return nil, err
	}
	additions := make([]string, 0)
	for key := range merged {
		if _, existed := base[key]; !existed {
			additions = append(additions, key)
		}
	}
	sort.Strings(additions)
	for _, key := range additions {
		encoded, encodeErr := encodeTOMLInlineEntry(key, merged[key], renderer.indent)
		if encodeErr != nil {
			return nil, encodeErr
		}
		members = append(members, tomlInlineMember{content: encoded, sourceIndex: -1})
	}

	return renderer.composeInlineTable(value, assignments, members), nil
}

func (renderer *tomlMergeRenderer) directInlineAssignments(
	parent *tomlValueRef,
	parentPath []string,
) ([]tomlAssignmentRef, error) {
	assignments := make([]tomlAssignmentRef, 0)
	for _, assignment := range renderer.document.assignments {
		if !assignment.inline || assignment.value.parent != parent {
			continue
		}
		if len(assignment.path) != len(parentPath)+1 || !pathHasPrefix(assignment.path, parentPath) {
			return nil, fmt.Errorf(
				"%w: structural edits to dotted inline TOML keys are not byte-safe",
				ErrUnwritable,
			)
		}
		assignments = append(assignments, assignment)
	}
	slices.SortFunc(assignments, func(one, two tomlAssignmentRef) int {
		return one.expression.start - two.expression.start
	})
	if len(assignments) != len(parent.children) {
		return nil, fmt.Errorf("%w: TOML inline syntax trees disagree", ErrUnwritable)
	}

	return assignments, nil
}

func (renderer *tomlMergeRenderer) renderExistingInlineMembers(
	assignments []tomlAssignmentRef,
	base, merged map[string]any,
) ([]tomlInlineMember, error) {
	members := make([]tomlInlineMember, 0, len(merged))
	for index, assignment := range assignments {
		key := assignment.path[len(assignment.path)-1]
		baseValue, existed := base[key]
		mergedValue, remains := merged[key]
		if !existed {
			return nil, fmt.Errorf("%w: TOML inline member has no semantic value", ErrUnwritable)
		}
		if !remains {
			continue
		}
		content := bytes.Clone(renderer.content[assignment.expression.start:assignment.expression.end])
		if !tomlSemanticEqual(baseValue, mergedValue) {
			replacement, err := renderer.renderInlineValue(assignment, baseValue, mergedValue)
			if err != nil {
				return nil, err
			}
			content = slices.Concat(
				renderer.content[assignment.expression.start:assignment.value.span.start],
				replacement,
				renderer.content[assignment.value.span.end:assignment.expression.end],
			)
		}
		members = append(members, tomlInlineMember{content: content, sourceIndex: index})
	}

	return members, nil
}

func (renderer *tomlMergeRenderer) renderInlineValue(
	assignment tomlAssignmentRef,
	base, merged any,
) ([]byte, error) {
	baseMap, baseIsMap := base.(map[string]any)
	mergedMap, mergedIsMap := merged.(map[string]any)
	if assignment.value.kind == unstable.InlineTable && baseIsMap && mergedIsMap {
		return renderer.renderInlineTable(assignment.value, assignment.path, baseMap, mergedMap)
	}
	if renderer.spanHasComment(assignment.value.span) {
		return nil, fmt.Errorf(
			"%w: changing %s cannot preserve comments inside its TOML value",
			ErrUnwritable, displayTOMLPath(assignment.path),
		)
	}
	replacement, err := encodeTOMLValue(merged, false, renderer.indent)
	if err != nil {
		return nil, err
	}
	if text, ok := merged.(string); ok && assignment.value.kind == unstable.String &&
		bytes.HasPrefix(renderer.content[assignment.value.span.start:assignment.value.span.end], []byte{'"'}) {
		return encodeTOMLBasicString(text)
	}

	return replacement, nil
}

func (renderer *tomlMergeRenderer) composeInlineTable(
	value *tomlValueRef,
	assignments []tomlAssignmentRef,
	members []tomlInlineMember,
) []byte {
	if len(assignments) == 0 {
		return composeEmptyOrNewInlineTable(renderer.content[value.span.start:value.span.end], members)
	}
	prefix := renderer.content[value.span.start:assignments[0].expression.start]
	suffix := renderer.content[assignments[len(assignments)-1].expression.end:value.span.end]
	separator := []byte(", ")
	if len(assignments) > 1 {
		separator = renderer.content[assignments[0].expression.end:assignments[1].expression.start]
	}
	var output bytes.Buffer
	output.Write(prefix)
	for index, member := range members {
		if index > 0 {
			previous := members[index-1]
			if previous.sourceIndex >= 0 && member.sourceIndex == previous.sourceIndex+1 {
				gapStart := assignments[previous.sourceIndex].expression.end
				gapEnd := assignments[member.sourceIndex].expression.start
				output.Write(renderer.content[gapStart:gapEnd])
			} else {
				output.Write(separator)
			}
		}
		output.Write(member.content)
	}
	if len(members) == 0 {
		return composeEmptyOrNewInlineTable(slices.Concat(prefix, suffix), nil)
	}
	output.Write(suffix)

	return output.Bytes()
}

func composeEmptyOrNewInlineTable(source []byte, members []tomlInlineMember) []byte {
	if len(members) > 0 {
		var output bytes.Buffer
		output.WriteString("{ ")
		for index, member := range members {
			if index > 0 {
				output.WriteString(", ")
			}
			output.Write(member.content)
		}
		output.WriteString(" }")

		return output.Bytes()
	}
	if len(bytes.TrimSpace(source)) > 2 {
		return []byte("{ }")
	}

	return []byte("{}")
}
