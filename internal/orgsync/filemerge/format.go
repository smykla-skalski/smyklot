package filemerge

import (
	"bytes"
	"fmt"
	"path"
	"strings"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

const (
	extJSON  = ".json"
	extJSONC = ".jsonc"

	formatPreserve = "preserve"
	formatAuto     = "auto"
	formatCompact  = "compact"
	formatExpanded = "expanded"
	formatInsert   = "insert"
	formatRemove   = "remove"

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

func proveFormattedSemantics(filePath string, before, after []byte) error {
	if bytes.Equal(before, after) {
		return nil
	}
	switch strings.ToLower(path.Ext(filePath)) {
	case extJSON, extJSONC:
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
