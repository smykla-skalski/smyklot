package filemerge

import (
	"bytes"
	"path"
	"strings"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

// NormalizeTemplate validates structured templates even when all presentation
// settings preserve the source, and supplies the mandatory file terminator.
func NormalizeTemplate(filePath string, content []byte) ([]byte, error) {
	var err error
	switch strings.ToLower(path.Ext(filePath)) {
	case extJSON, extJSONC:
		_, err = parseJSONSyntax(content, strings.EqualFold(path.Ext(filePath), extJSONC))
	case extYAML, extYML:
		if _, err = parseGoccyYAML(content); err != nil {
			return nil, err
		}
		if _, err = parseYAMLFormatDocument(content); err == nil {
			return terminateYAMLTemplate(content)
		}
	case extTOML:
		_, _, err = decodeTOMLSemantic(content)
	}
	if err != nil {
		return nil, err
	}

	return TerminateTemplate(content), nil
}

// TerminateTemplate ensures that a shared file ends with a line ending. The
// required terminator is content hygiene, independent of formatting preferences.
// Existing blank lines remain intact because they can be meaningful in YAML
// block scalars and other literal content.
func TerminateTemplate(content []byte) []byte {
	if bytes.HasSuffix(content, []byte("\n")) {
		return content
	}
	ending := []byte("\n")
	if dominantLineEnding(content) == lineEndingCRLF && !bytes.HasSuffix(content, []byte("\r")) {
		ending = []byte("\r\n")
	}

	return append(bytes.Clone(content), ending...)
}

// TemplateBody excludes only the required terminator, retaining intentional
// blank lines. A terminator's LF/CRLF spelling is not a formatting difference.
func TemplateBody(content []byte) []byte {
	if bytes.HasSuffix(content, []byte("\r\n")) {
		return content[:len(content)-2]
	}

	return bytes.TrimSuffix(content, []byte("\n"))
}

// ApplyTemplate is the shared-file boundary used by planning and previews.
// Legacy final_newline preferences are ignored here. Normalize both sides of
// the formatting comparison so the required terminator never becomes a change
// that the user has to format or approve.
func ApplyTemplate(
	filePath string,
	template []byte,
	spec Spec,
	policy config.FormattingPolicy,
) (ApplyResult, error) {
	policy.Common.FinalNewline = formatPreserve
	template, err := NormalizeTemplate(filePath, template)
	if err != nil {
		return ApplyResult{}, &ApplyError{Stage: ApplyStageMerge, Err: err}
	}
	result, err := ApplyDetailed(filePath, template, spec, policy)
	if err != nil {
		return ApplyResult{}, err
	}
	result.Composed, err = NormalizeTemplate(filePath, result.Composed)
	if err != nil {
		return ApplyResult{}, &ApplyError{Stage: ApplyStageMerge, Err: err}
	}
	result.Final, err = NormalizeTemplate(filePath, result.Final)
	if err != nil {
		return ApplyResult{}, &ApplyError{Stage: ApplyStageFormat, Err: err}
	}

	return result, nil
}
