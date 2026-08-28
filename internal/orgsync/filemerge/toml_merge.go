package filemerge

import (
	"bytes"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2/unstable"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

type tomlMergeRenderer struct {
	content     []byte
	document    *tomlSyntaxDocument
	assignments map[string]tomlAssignmentRef
	sections    map[string]tomlSectionRef
	edits       []byteEdit
	insertions  map[int][][]byte
	additions   [][]byte
	deletions   []tomlSpan
	indent      string
	lineEnding  string
}

func mergeTOML(template []byte, spec Spec) ([]byte, error) {
	base, _, err := decodeTOMLSemantic(template)
	if err != nil {
		return nil, err
	}
	override, err := decodeOverrides(spec.Overrides)
	if err != nil {
		return nil, err
	}
	normalized, err := normalizeTOMLOverride(override)
	if err != nil {
		return nil, err
	}
	override, ok := normalized.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: TOML overrides hold no object", ErrUnwritable)
	}
	if err := rejectTOMLDatetimeReplacement(base, override, nil); err != nil {
		return nil, err
	}
	var merged map[string]any
	if spec.Strategy == StrategyShallow {
		merged = mergeShallow(base, override)
	} else {
		merged = mergePatch(base, override)
	}
	if err := applyArrayRules(merged, base, override, spec); err != nil {
		return nil, err
	}

	return renderTOMLMerge(template, base, merged)
}

func renderTOMLMerge(content []byte, base, merged map[string]any) ([]byte, error) {
	if tomlSemanticEqual(base, merged) {
		return content, nil
	}
	document, err := parseTOMLSyntax(content)
	if err != nil {
		return nil, err
	}
	renderer := newTOMLMergeRenderer(content, document)
	if err := renderer.diffMap(nil, base, merged); err != nil {
		return nil, err
	}
	result, err := renderer.apply()
	if err != nil {
		return nil, err
	}
	actual, comments, err := decodeTOMLSemantic(result)
	if err != nil {
		return nil, fmt.Errorf("%w: merged TOML did not parse: %w", ErrUnwritable, err)
	}
	if !tomlSemanticEqual(actual, merged) {
		return nil, fmt.Errorf("%w: source edits did not produce the requested TOML merge", ErrUnwritable)
	}
	expectedComments := renderer.expectedComments()
	if !reflect.DeepEqual(comments, expectedComments) {
		return nil, fmt.Errorf("%w: TOML merge changed comments outside a deleted value", ErrUnwritable)
	}

	return result, nil
}

func newTOMLMergeRenderer(content []byte, document *tomlSyntaxDocument) *tomlMergeRenderer {
	renderer := &tomlMergeRenderer{
		content: content, document: document,
		assignments: make(map[string]tomlAssignmentRef, len(document.assignments)),
		sections:    make(map[string]tomlSectionRef, len(document.sections)),
		insertions:  make(map[int][][]byte),
		indent:      strings.Repeat(" ", tomlDetectedIndent(content, 2)),
		lineEnding:  map[bool]string{true: crlf, false: lf}[dominantLineEnding(content) == lineEndingCRLF],
	}
	for _, assignment := range document.assignments {
		if !assignment.arrayTable {
			renderer.assignments[tomlPathKey(assignment.path)] = assignment
		}
	}
	for _, section := range document.sections {
		if !section.arrayTable {
			renderer.sections[tomlPathKey(section.path)] = section
		}
	}

	return renderer
}

func (renderer *tomlMergeRenderer) diffMap(
	path []string,
	base, merged map[string]any,
) error {
	keys := make(map[string]any, len(base)+len(merged))
	for key := range base {
		keys[key] = nil
	}
	for key := range merged {
		keys[key] = nil
	}
	for _, key := range sortedKeys(keys) {
		baseValue, existed := base[key]
		mergedValue, remains := merged[key]
		if existed && remains && tomlSemanticEqual(baseValue, mergedValue) {
			continue
		}
		childPath := appendPath(path, key)
		if err := renderer.diffValue(childPath, baseValue, existed, mergedValue, remains); err != nil {
			return err
		}
	}

	return nil
}

func (renderer *tomlMergeRenderer) diffValue(
	path []string,
	base any,
	existed bool,
	merged any,
	remains bool,
) error {
	if assignment, ok := renderer.assignments[tomlPathKey(path)]; ok {
		if !remains {
			if assignment.inline {
				return fmt.Errorf("%w: inline TOML member deletion requires its parent", ErrUnwritable)
			}
			renderer.deleteSpan(assignment.line)

			return nil
		}

		baseMap, baseIsMap := base.(map[string]any)
		mergedMap, mergedIsMap := merged.(map[string]any)
		if assignment.value.kind == unstable.InlineTable && baseIsMap && mergedIsMap {
			if sameTOMLMapKeys(baseMap, mergedMap) {
				return renderer.diffMap(path, baseMap, mergedMap)
			}

			return renderer.replaceInlineTable(assignment, baseMap, mergedMap)
		}

		return renderer.replaceAssignment(assignment, merged)
	}
	baseMap, baseIsMap := base.(map[string]any)
	mergedMap, mergedIsMap := merged.(map[string]any)
	if existed && remains && baseIsMap && mergedIsMap && !renderer.hasArrayTable(path) {
		return renderer.diffMap(path, baseMap, mergedMap)
	}
	if existed {
		renderer.deletePath(path)
	}
	if remains {
		return renderer.addPath(path, merged)
	}

	return nil
}

func sameTOMLMapKeys(one, other map[string]any) bool {
	if len(one) != len(other) {
		return false
	}
	for key := range one {
		if _, ok := other[key]; !ok {
			return false
		}
	}

	return true
}

func (renderer *tomlMergeRenderer) replaceAssignment(
	assignment tomlAssignmentRef,
	value any,
) error {
	if renderer.spanHasComment(assignment.value.span) {
		return fmt.Errorf(
			"%w: changing %s cannot preserve comments inside its TOML value",
			ErrUnwritable, displayTOMLPath(assignment.path),
		)
	}
	if _, mapValue := value.(map[string]any); mapValue && assignment.value.kind != unstable.InlineTable {
		renderer.deleteSpan(assignment.line)

		return renderer.addPath(assignment.path, value)
	}
	multiline := assignment.value.kind == unstable.Array &&
		bytes.ContainsAny(renderer.content[assignment.value.span.start:assignment.value.span.end], "\r\n")
	replacement, err := encodeTOMLValue(value, multiline, renderer.indent)
	if err != nil {
		return err
	}
	if assignment.value.kind == unstable.Array {
		if nearby := firstLineEnding(
			renderer.content[assignment.value.span.start:assignment.value.span.end],
		); nearby != "" {
			replacement = replaceTOMLLineEndings(replacement, nearby)
		}
		replacement, err = inheritTOMLArrayTrailingComma(
			replacement,
			tomlHasTrailingComma(renderer.content, assignment.value),
			renderer.indent,
		)
		if err != nil {
			return err
		}
	}
	if text, ok := value.(string); ok && assignment.value.kind == unstable.String {
		source := renderer.content[assignment.value.span.start:assignment.value.span.end]
		if bytes.HasPrefix(source, []byte{'"'}) {
			replacement, err = encodeTOMLBasicString(text)
			if err != nil {
				return err
			}
		}
	}
	renderer.edits = append(renderer.edits, byteEdit{
		start: assignment.value.span.start, end: assignment.value.span.end,
		replacement: replacement,
	})

	return nil
}

func inheritTOMLArrayTrailingComma(value []byte, trailing bool, indent string) ([]byte, error) {
	fragment, err := wrapTOMLRawValue(value)
	if err != nil {
		return nil, err
	}
	policy := config.DefaultFormattingPolicy()
	policy.Common.IndentStyle = formatSpaces
	policy.Common.IndentWidth = len(indent)
	if trailing {
		policy.TOML.TrailingCommas = formatMultiline
	} else {
		policy.TOML.TrailingCommas = formatRemove
	}
	formatted, err := formatTOMLValues(fragment.content, policy)
	if err != nil {
		return nil, err
	}
	document, err := parseTOMLSyntax(formatted)
	if err != nil {
		return nil, err
	}
	if len(document.assignments) != 1 {
		return nil, fmt.Errorf("%w: TOML array fragment changed shape", ErrUnwritable)
	}
	span := document.assignments[0].value.span

	return bytes.Clone(formatted[span.start:span.end]), nil
}

func (renderer *tomlMergeRenderer) addPath(path []string, value any) error {
	if len(path) == 0 {
		return fmt.Errorf("%w: TOML merge cannot add an empty path", ErrUnwritable)
	}
	_, object := value.(map[string]any)
	if object || isTOMLArrayOfTables(value) {
		addition, err := encodeTOMLSubtree(path, value, renderer.indent, renderer.lineEnding)
		if err != nil {
			return err
		}
		renderer.additions = append(renderer.additions, addition)

		return nil
	}
	parent := path[:len(path)-1]
	section, relativePath := renderer.closestTOMLSection(parent, path)
	var entry []byte
	var err error
	if len(relativePath) == 1 {
		entry, err = encodeTOMLLocalEntry(relativePath[0], value, renderer.indent, renderer.lineEnding)
	} else {
		entry, err = encodeTOMLDottedEntry(relativePath, value, renderer.indent, renderer.lineEnding)
	}
	if err != nil {
		return err
	}
	offset := renderer.insertionOffset(section)
	renderer.insertions[offset] = append(renderer.insertions[offset], entry)

	return nil
}

func (renderer *tomlMergeRenderer) insertionOffset(section tomlSectionRef) int {
	offset := section.end
	for _, assignment := range renderer.document.assignments {
		if !assignment.arrayTable && !assignment.inline &&
			assignment.expression.start >= section.bodyStart &&
			assignment.expression.start < section.end {
			offset = assignment.line.end
		}
	}

	return offset
}

func (renderer *tomlMergeRenderer) deletePath(path []string) {
	for _, section := range renderer.document.sections {
		if len(section.path) > 0 && pathHasPrefix(section.path, path) {
			renderer.deleteSpan(tomlSpan{start: section.header.start, end: section.end})
		}
	}
	for _, assignment := range renderer.document.assignments {
		if !assignment.arrayTable && !assignment.inline && pathHasPrefix(assignment.path, path) &&
			!renderer.coveredByDeletedSection(assignment.line) {
			renderer.deleteSpan(assignment.line)
		}
	}
}

func (renderer *tomlMergeRenderer) deleteSpan(span tomlSpan) {
	for _, existing := range renderer.deletions {
		if span.start >= existing.start && span.end <= existing.end {
			return
		}
	}
	renderer.deletions = append(renderer.deletions, span)
	renderer.edits = append(renderer.edits, byteEdit{start: span.start, end: span.end})
}

func (renderer *tomlMergeRenderer) coveredByDeletedSection(span tomlSpan) bool {
	for _, deletion := range renderer.deletions {
		if span.start >= deletion.start && span.end <= deletion.end {
			return true
		}
	}

	return false
}

func (renderer *tomlMergeRenderer) hasArrayTable(path []string) bool {
	for _, section := range renderer.document.sections {
		if section.arrayTable && slices.Equal(section.path, path) {
			return true
		}
	}

	return false
}

func (renderer *tomlMergeRenderer) spanHasComment(span tomlSpan) bool {
	for _, comment := range renderer.document.commentSpans {
		if comment.start >= span.start && comment.end <= span.end {
			return true
		}
	}

	return false
}

func (renderer *tomlMergeRenderer) apply() ([]byte, error) {
	renderer.addInsertionEdits()
	renderer.addAdditionEdit()

	return applyByteEdits(renderer.content, renderer.edits)
}

func (renderer *tomlMergeRenderer) addInsertionEdits() {
	for offset, entries := range renderer.insertions {
		var replacement bytes.Buffer
		if offset > 0 && renderer.content[offset-1] != '\n' {
			replacement.WriteString(renderer.lineEnding)
		}
		for _, entry := range entries {
			replacement.Write(entry)
		}
		if offset == len(renderer.content) && !bytes.HasSuffix(renderer.content, []byte{'\n'}) {
			trimTOMLTerminalLineEnding(&replacement, renderer.lineEnding)
		}
		renderer.edits = append(renderer.edits, byteEdit{
			start: offset, end: offset, replacement: replacement.Bytes(),
		})
	}
}

func (renderer *tomlMergeRenderer) addAdditionEdit() {
	if len(renderer.additions) == 0 {
		return
	}
	var replacement bytes.Buffer
	prefix := renderer.remainingTOMLPrefix()
	if len(prefix) > 0 {
		switch {
		case bytes.HasSuffix(prefix, []byte(renderer.lineEnding+renderer.lineEnding)):
		case bytes.HasSuffix(prefix, []byte(renderer.lineEnding)):
			replacement.WriteString(renderer.lineEnding)
		default:
			replacement.WriteString(renderer.lineEnding + renderer.lineEnding)
		}
	}
	for index, addition := range renderer.additions {
		if index > 0 {
			replacement.WriteString(renderer.lineEnding)
		}
		replacement.Write(addition)
	}
	if !bytes.HasSuffix(renderer.content, []byte{'\n'}) {
		trimTOMLTerminalLineEnding(&replacement, renderer.lineEnding)
	}
	renderer.edits = append(renderer.edits, byteEdit{
		start: len(renderer.content), end: len(renderer.content), replacement: replacement.Bytes(),
	})
}

func (renderer *tomlMergeRenderer) remainingTOMLPrefix() []byte {
	end := len(renderer.content)
	for {
		changed := false
		for _, deletion := range renderer.deletions {
			if deletion.end == end {
				end = deletion.start
				changed = true
			}
		}
		if !changed {
			return renderer.content[:end]
		}
	}
}

func trimTOMLTerminalLineEnding(buffer *bytes.Buffer, lineEnding string) {
	content := buffer.Bytes()
	if bytes.HasSuffix(content, []byte(lineEnding)) {
		buffer.Truncate(len(content) - len(lineEnding))
	}
}

func (renderer *tomlMergeRenderer) expectedComments() map[string]int {
	expected := make(map[string]int, len(renderer.document.comments))
	for comment, count := range renderer.document.comments {
		expected[comment] = count
	}
	for _, span := range renderer.document.commentSpans {
		if renderer.spanDeleted(span) {
			text := string(renderer.content[span.start:span.end])
			expected[text]--
			if expected[text] == 0 {
				delete(expected, text)
			}
		}
	}

	return expected
}

func (renderer *tomlMergeRenderer) spanDeleted(span tomlSpan) bool {
	for _, deletion := range renderer.deletions {
		if span.start >= deletion.start && span.end <= deletion.end {
			return true
		}
	}

	return false
}

func pathHasPrefix(path, prefix []string) bool {
	return len(path) >= len(prefix) && slices.Equal(path[:len(prefix)], prefix)
}

func isTOMLArrayOfTables(value any) bool {
	array, ok := value.([]map[string]any)
	if ok {
		return len(array) > 0
	}
	values, ok := value.([]any)
	if !ok || len(values) == 0 {
		return false
	}
	for _, item := range values {
		if _, ok := item.(map[string]any); !ok {
			return false
		}
	}

	return true
}
