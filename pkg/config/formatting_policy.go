package config

const (
	preserve = "preserve"
	auto     = "auto"
)

// DefaultFormattingPolicy keeps every presentation dimension untouched. The
// numeric defaults are still concrete because auto formatting needs them once
// a higher layer opts into it.
func DefaultFormattingPolicy() FormattingPolicy {
	return formattingPresetPolicy(preserve)
}

// ApplyFormattingPatch returns base with one sparse layer applied. A preset is
// applied first, then its sibling leaves, which makes a layer such as
// {preset: conventional, common: {line_ending: preserve}} deterministic.
func ApplyFormattingPatch(base FormattingPolicy, patch FormattingPatch) FormattingPolicy {
	applyFormattingPatch(&base, patch, nil, "")

	return base
}

func applyFormattingPatch(
	target *FormattingPolicy,
	patch FormattingPatch,
	sources map[string]Source,
	source Source,
) {
	if patch.Preset != nil {
		*target = formattingPresetPolicy(*patch.Preset)

		for _, key := range FormattingKeys() {
			setSource(sources, key, source)
		}
	}

	setFormatting(&target.Preset, patch.Preset, sources, "formatting.preset", source)
	applyCommonFormatting(&target.Common, patch.Common, sources, source)
	applyJSONFormatting(&target.JSON, patch.JSON, sources, source)
	applyJSONCFormatting(&target.JSONC, patch.JSONC, sources, source)
	applyYAMLFormatting(&target.YAML, patch.YAML, sources, source)
	applyTOMLFormatting(&target.TOML, patch.TOML, sources, source)
	applyMarkdownFormatting(&target.Markdown, patch.Markdown, sources, source)
}

func applyCommonFormatting(target *FormattingCommonPolicy, patch *FormattingCommonPatch, sources map[string]Source, source Source) {
	if patch == nil {
		return
	}
	setFormatting(&target.IndentStyle, patch.IndentStyle, sources, "formatting.common.indent_style", source)
	setFormatting(&target.IndentWidth, patch.IndentWidth, sources, "formatting.common.indent_width", source)
	setFormatting(&target.LineWidth, patch.LineWidth, sources, "formatting.common.line_width", source)
	setFormatting(&target.LineEnding, patch.LineEnding, sources, "formatting.common.line_ending", source)
	setFormatting(&target.FinalNewline, patch.FinalNewline, sources, "formatting.common.final_newline", source)
}

func applyJSONFormatting(target *FormattingJSONPolicy, patch *FormattingJSONPatch, sources map[string]Source, source Source) {
	if patch == nil {
		return
	}
	setFormatting(&target.Arrays, patch.Arrays, sources, "formatting.json.arrays", source)
	setFormatting(&target.Objects, patch.Objects, sources, "formatting.json.objects", source)
	setFormatting(&target.KeyOrder, patch.KeyOrder, sources, "formatting.json.key_order", source)
}

func applyJSONCFormatting(target *FormattingJSONCPolicy, patch *FormattingJSONCPatch, sources map[string]Source, source Source) {
	if patch != nil {
		setFormatting(&target.TrailingCommas, patch.TrailingCommas, sources, "formatting.jsonc.trailing_commas", source)
	}
}

func applyYAMLFormatting(target *FormattingYAMLPolicy, patch *FormattingYAMLPatch, sources map[string]Source, source Source) {
	if patch == nil {
		return
	}
	setFormatting(&target.Sequences, patch.Sequences, sources, "formatting.yaml.sequences", source)
	setFormatting(&target.Mappings, patch.Mappings, sources, "formatting.yaml.mappings", source)
	setFormatting(&target.QuoteStyle, patch.QuoteStyle, sources, "formatting.yaml.quote_style", source)
	setFormatting(&target.SequenceIndent, patch.SequenceIndent, sources, "formatting.yaml.sequence_indent", source)
	setFormatting(&target.DocumentStart, patch.DocumentStart, sources, "formatting.yaml.document_start", source)
}

func applyTOMLFormatting(target *FormattingTOMLPolicy, patch *FormattingTOMLPatch, sources map[string]Source, source Source) {
	if patch == nil {
		return
	}
	setFormatting(&target.Arrays, patch.Arrays, sources, "formatting.toml.arrays", source)
	setFormatting(&target.TrailingCommas, patch.TrailingCommas, sources, "formatting.toml.trailing_commas", source)
	setFormatting(&target.QuoteStyle, patch.QuoteStyle, sources, "formatting.toml.quote_style", source)
	setFormatting(&target.AlignEntries, patch.AlignEntries, sources, "formatting.toml.align_entries", source)
	setFormatting(&target.AlignComments, patch.AlignComments, sources, "formatting.toml.align_comments", source)
	setFormatting(&target.KeyOrder, patch.KeyOrder, sources, "formatting.toml.key_order", source)
}

func applyMarkdownFormatting(target *FormattingMarkdownPolicy, patch *FormattingMarkdownPatch, sources map[string]Source, source Source) {
	if patch == nil {
		return
	}
	setFormatting(&target.ProseWrap, patch.ProseWrap, sources, "formatting.markdown.prose_wrap", source)
	setFormatting(&target.ListSpacing, patch.ListSpacing, sources, "formatting.markdown.list_spacing", source)
	setFormatting(&target.Tables, patch.Tables, sources, "formatting.markdown.tables", source)
}

func setFormatting[T any](target *T, value *T, sources map[string]Source, key string, source Source) {
	if value == nil {
		return
	}
	*target = *value
	setSource(sources, key, source)
}

// AllPreserve reports whether rendering is allowed to return the input without
// parsing. Numeric values do not alter bytes by themselves.
func (p FormattingPolicy) AllPreserve() bool {
	return p.Common.IndentStyle == preserve && p.Common.LineEnding == preserve &&
		p.Common.FinalNewline == preserve && p.JSON.Arrays == preserve &&
		p.JSON.Objects == preserve && p.JSON.KeyOrder == preserve &&
		p.JSONC.TrailingCommas == preserve && p.YAML.Sequences == preserve &&
		p.YAML.Mappings == preserve && p.YAML.QuoteStyle == preserve &&
		p.YAML.SequenceIndent == preserve && p.YAML.DocumentStart == preserve &&
		p.TOML.Arrays == preserve && p.TOML.TrailingCommas == preserve &&
		p.TOML.QuoteStyle == preserve && p.TOML.AlignEntries == preserve &&
		p.TOML.AlignComments == preserve && p.TOML.KeyOrder == preserve &&
		p.Markdown.ProseWrap == preserve && p.Markdown.ListSpacing == preserve &&
		p.Markdown.Tables == preserve
}
