package filemerge

import (
	"bytes"
	"fmt"
	"slices"
)

func encodeTOMLLocalEntry(key string, value any, indent, lineEnding string) ([]byte, error) {
	encoded, err := encodeTOMLDocument(map[string]any{key: value}, false, indent)
	if err != nil {
		return nil, err
	}
	document, err := parseTOMLSyntax(encoded)
	if err != nil {
		return nil, err
	}
	if len(document.assignments) != 1 || len(document.sections) != 1 ||
		!slices.Equal(document.assignments[0].path, []string{key}) {
		return nil, fmt.Errorf("%w: TOML encoder did not produce one local entry", ErrUnwritable)
	}
	line := document.assignments[0].line
	entry := bytes.Clone(encoded[line.start:line.end])

	return replaceTOMLLineEndings(entry, lineEnding), nil
}

func encodeTOMLDottedEntry(path []string, value any, indent, lineEnding string) ([]byte, error) {
	if len(path) < 2 {
		return nil, fmt.Errorf("%w: a dotted TOML entry needs at least two path components", ErrUnwritable)
	}
	wrapper := value
	for index := len(path) - 1; index >= 0; index-- {
		wrapper = map[string]any{path[index]: wrapper}
	}
	encoded, err := encodeTOMLDocument(wrapper, false, indent)
	if err != nil {
		return nil, err
	}
	document, err := parseTOMLSyntax(encoded)
	if err != nil {
		return nil, err
	}
	parent := path[:len(path)-1]
	var section *tomlSectionRef
	for index := range document.sections {
		if slices.Equal(document.sections[index].path, parent) {
			section = &document.sections[index]
			break
		}
	}
	if section == nil || len(document.assignments) != 1 ||
		!slices.Equal(document.assignments[0].path, path) {
		return nil, fmt.Errorf(
			"%w: TOML encoder did not produce one dotted entry for %s",
			ErrUnwritable, displayTOMLPath(path),
		)
	}
	assignment := document.assignments[0]
	entry := slices.Concat(
		encoded[section.key.start:section.key.end],
		[]byte{'.'},
		encoded[assignment.expression.start:assignment.line.end],
	)
	entry = replaceTOMLLineEndings(entry, lineEnding)
	validation, err := parseTOMLSyntax(entry)
	if err != nil || len(validation.assignments) != 1 ||
		!slices.Equal(validation.assignments[0].path, path) {
		return nil, fmt.Errorf(
			"%w: generated dotted TOML entry for %s did not validate",
			ErrUnwritable, displayTOMLPath(path),
		)
	}

	return entry, nil
}

func encodeTOMLInlineEntry(key string, value any, indent string) ([]byte, error) {
	encoded, err := encodeTOMLValue(map[string]any{key: value}, false, indent)
	if err != nil {
		return nil, err
	}
	document, err := wrapTOMLRawValue(encoded)
	if err != nil {
		return nil, err
	}
	root, err := oneTOMLRootAssignment(document, "__smyklot_value__")
	if err != nil {
		return nil, err
	}
	for _, assignment := range document.assignments {
		if assignment.inline && assignment.value.parent == root.value &&
			slices.Equal(assignment.path, appendPath(root.path, key)) {
			return bytes.Clone(document.content[assignment.expression.start:assignment.expression.end]), nil
		}
	}

	return nil, fmt.Errorf("%w: TOML encoder did not produce one inline member", ErrUnwritable)
}

func (renderer *tomlMergeRenderer) closestTOMLSection(
	parent, path []string,
) (tomlSectionRef, []string) {
	section := renderer.sections[""]
	for _, candidate := range renderer.sections {
		if len(candidate.path) > len(section.path) && pathHasPrefix(parent, candidate.path) {
			section = candidate
		}
	}

	return section, path[len(section.path):]
}

func encodeTOMLSubtree(path []string, value any, indent, lineEnding string) ([]byte, error) {
	wrapped := value
	for index := len(path) - 1; index >= 0; index-- {
		wrapped = map[string]any{path[index]: wrapped}
	}
	encoded, err := encodeTOMLDocument(wrapped, false, indent)
	if err != nil {
		return nil, err
	}
	document, err := parseTOMLSyntax(encoded)
	if err != nil {
		return nil, err
	}
	start := -1
	for _, section := range document.sections {
		if len(section.path) > 0 && slices.Equal(section.path, path) {
			start = section.header.start
			break
		}
	}
	if start < 0 {
		return nil, fmt.Errorf(
			"%w: TOML encoder did not produce a table for %s", ErrUnwritable, displayTOMLPath(path),
		)
	}
	addition := bytes.Clone(encoded[start:])

	return replaceTOMLLineEndings(addition, lineEnding), nil
}

func replaceTOMLLineEndings(content []byte, lineEnding string) []byte {
	content = bytes.ReplaceAll(content, []byte(crlf), []byte(lf))
	if lineEnding == crlf {
		content = bytes.ReplaceAll(content, []byte(lf), []byte(crlf))
	}

	return content
}
