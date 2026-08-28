package filemerge

import (
	"bytes"
	"fmt"
	"path"
	"reflect"
	"strings"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

const (
	extJSON     = ".json"
	extJSONC    = ".jsonc"
	extYAML     = ".yaml"
	extYML      = ".yml"
	extTOML     = ".toml"
	extMD       = ".md"
	extMarkdown = ".markdown"

	formatPreserve  = "preserve"
	formatAuto      = "auto"
	formatCompact   = "compact"
	formatExpanded  = "expanded"
	formatInsert    = "insert"
	formatRemove    = "remove"
	formatSpaces    = "spaces"
	formatTabs      = "tabs"
	formatMultiline = "multiline"

	lineEndingLF   = "lf"
	lineEndingCRLF = "crlf"
	lf             = "\n"
	crlf           = "\r\n"
)

func formatWithSource(
	filePath string,
	content, source []byte,
	policy config.FormattingPolicy,
) ([]byte, error) {
	var (
		formatted []byte
		err       error
	)

	switch strings.ToLower(path.Ext(filePath)) {
	case extJSON:
		if jsonFormattingActive(policy) || commonIndentationActive(policy) {
			formatted, err = formatJSONDocument(content, false, policy)
		} else {
			formatted = content
		}
	case extJSONC:
		if jsonFormattingActive(policy) || commonIndentationActive(policy) ||
			policy.JSONC.TrailingCommas != formatPreserve {
			formatted, err = formatJSONDocument(content, true, policy)
		} else {
			formatted = content
		}
	case extYAML, extYML:
		if yamlFormattingActive(policy) {
			formatted, err = formatYAMLDocument(content, policy)
		} else {
			formatted = content
		}
	case extTOML:
		if tomlFormattingActive(policy) {
			formatted, err = formatTOMLDocument(content, policy)
		} else {
			formatted = content
		}
	case extMD, extMarkdown:
		if markdownFormattingActive(policy) {
			formatted, err = formatMarkdownDocument(content, policy)
		} else {
			formatted = content
		}
	default:
		formatted = content
	}
	if err != nil {
		return nil, err
	}

	formatted = applyCommonFormatting(formatted, source, policy.Common)
	if err := proveFormattedSemantics(filePath, content, formatted); err != nil {
		return nil, err
	}

	return formatted, nil
}

func commonIndentationActive(policy config.FormattingPolicy) bool {
	return policy.Common.IndentStyle != formatPreserve
}

func jsonFormattingActive(policy config.FormattingPolicy) bool {
	return policy.JSON.Arrays != formatPreserve || policy.JSON.Objects != formatPreserve ||
		policy.JSON.KeyOrder != formatPreserve
}

func yamlFormattingActive(policy config.FormattingPolicy) bool {
	return yamlASTFormattingActive(policy) || policy.YAML.DocumentStart != formatPreserve
}

func tomlFormattingActive(policy config.FormattingPolicy) bool {
	return policy.Common.IndentStyle != formatPreserve ||
		policy.TOML.Arrays != formatPreserve ||
		policy.TOML.TrailingCommas != formatPreserve ||
		policy.TOML.QuoteStyle != formatPreserve ||
		policy.TOML.AlignEntries != formatPreserve ||
		policy.TOML.AlignComments != formatPreserve ||
		policy.TOML.KeyOrder != formatPreserve
}

func proveFormattedSemantics(filePath string, before, after []byte) error {
	if bytes.Equal(before, after) {
		return nil
	}
	switch strings.ToLower(path.Ext(filePath)) {
	case extJSON, extJSONC:
		return proveJSONFormatting(filePath, before, after)
	case extYAML, extYML:
		return proveYAMLFormatting(before, after)
	case extTOML:
		return proveTOMLFormatting(before, after)
	case extMD, extMarkdown:
		return proveMarkdownFormatting(before, after)
	}

	return nil
}

func proveMarkdownFormatting(before, after []byte) error {
	beforeDigest, err := markdownSemanticDigest(before)
	if err != nil {
		return err
	}
	afterDigest, err := markdownSemanticDigest(after)
	if err != nil {
		return fmt.Errorf("%w: formatted Markdown did not parse: %w", ErrUnwritable, err)
	}
	if beforeDigest != afterDigest {
		return fmt.Errorf("%w: formatting changed the Markdown document", ErrUnwritable)
	}

	return nil
}

func proveJSONFormatting(filePath string, before, after []byte) error {
	jsonc := strings.HasSuffix(strings.ToLower(filePath), extJSONC)
	beforeRoot, err := parseJSONSyntax(before, jsonc)
	if err != nil {
		return err
	}
	afterRoot, err := parseJSONSyntax(after, jsonc)
	if err != nil {
		return fmt.Errorf("%w: formatted JSON did not parse: %w", ErrUnwritable, err)
	}
	beforeValue, beforeErr := jsonSyntaxValue(beforeRoot)
	afterValue, afterErr := jsonSyntaxValue(afterRoot)
	if beforeErr != nil || afterErr != nil || !holdsEqual([]any{beforeValue}, afterValue) {
		return fmt.Errorf("%w: formatting changed the JSON value", ErrUnwritable)
	}

	return nil
}

func proveYAMLFormatting(before, after []byte) error {
	if _, err := parseGoccyYAML(before); err != nil {
		return err
	}
	if _, err := parseGoccyYAML(after); err != nil {
		return fmt.Errorf("%w: formatted YAML did not parse: %w", ErrUnwritable, err)
	}
	beforeValue, beforeErr := decodeYAMLSemantic(before)
	afterValue, afterErr := decodeYAMLSemantic(after)
	if beforeErr != nil || afterErr != nil || !reflect.DeepEqual(beforeValue, afterValue) {
		return fmt.Errorf("%w: formatting changed the YAML value", ErrUnwritable)
	}
	if !yamlPresentationOf(before).equal(yamlPresentationOf(after)) {
		return fmt.Errorf("%w: formatting changed YAML comments or special syntax", ErrUnwritable)
	}

	return nil
}

func proveTOMLFormatting(before, after []byte) error {
	beforeValue, beforeComments, err := decodeTOMLSemantic(before)
	if err != nil {
		return err
	}
	afterValue, afterComments, err := decodeTOMLSemantic(after)
	if err != nil {
		return fmt.Errorf("%w: formatted TOML did not parse: %w", ErrUnwritable, err)
	}
	if !tomlSemanticEqual(beforeValue, afterValue) {
		return fmt.Errorf("%w: formatting changed the TOML value", ErrUnwritable)
	}
	if !reflect.DeepEqual(beforeComments, afterComments) {
		return fmt.Errorf("%w: formatting changed TOML comments", ErrUnwritable)
	}

	return nil
}

func applyCommonFormatting(
	content, source []byte,
	policy config.FormattingCommonPolicy,
) []byte {
	if policy.LineEnding != formatPreserve {
		content = bytes.ReplaceAll(content, []byte(crlf), []byte(lf))
		if policy.LineEnding == lineEndingCRLF {
			content = bytes.ReplaceAll(content, []byte(lf), []byte(crlf))
		}
	}

	if policy.FinalNewline == formatPreserve {
		return content
	}

	content = bytes.TrimRight(content, "\r\n")
	if policy.FinalNewline == formatInsert {
		lineEnding := policy.LineEnding
		if lineEnding == formatPreserve {
			lineEnding = dominantLineEnding(source)
		}
		if lineEnding == lineEndingCRLF {
			content = append(content, '\r', '\n')
		} else {
			content = append(content, '\n')
		}
	}

	return content
}

func dominantLineEnding(content []byte) string {
	crlf := bytes.Count(content, []byte("\r\n"))
	lf := bytes.Count(content, []byte("\n")) - crlf
	if crlf > lf {
		return lineEndingCRLF
	}

	return lineEndingLF
}
