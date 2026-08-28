package filemerge

import (
	"fmt"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

func formatMarkdownDocument(content []byte, policy config.FormattingPolicy) ([]byte, error) {
	before, err := markdownSemanticDigest(content)
	if err != nil {
		return nil, err
	}
	formatted := content
	if policy.Markdown.ProseWrap != formatPreserve {
		formatted, err = formatMarkdownProse(formatted, policy)
		if err != nil {
			return nil, err
		}
	}
	if policy.Markdown.ListSpacing != formatPreserve {
		formatted, err = formatMarkdownLists(formatted, policy.Markdown.ListSpacing)
		if err != nil {
			return nil, err
		}
	}
	if policy.Markdown.Tables != formatPreserve {
		formatted, err = formatMarkdownTables(formatted, policy.Markdown.Tables)
		if err != nil {
			return nil, err
		}
	}
	after, err := markdownSemanticDigest(formatted)
	if err != nil {
		return nil, fmt.Errorf("%w: formatted Markdown did not parse: %w", ErrUnwritable, err)
	}
	if before != after {
		return nil, fmt.Errorf("%w: formatting changed the Markdown document", ErrUnwritable)
	}

	return formatted, nil
}

func markdownFormattingActive(policy config.FormattingPolicy) bool {
	return policy.Markdown.ProseWrap != formatPreserve ||
		policy.Markdown.ListSpacing != formatPreserve ||
		policy.Markdown.Tables != formatPreserve
}
