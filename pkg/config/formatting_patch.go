package config

import "fmt"

// Validate reports a formatting layer that cannot be resolved safely.
func (p FormattingPatch) Validate() error {
	copy := p

	return copy.normalize()
}

// FormattingKeys returns every formatting leaf in declaration order.
func FormattingKeys() []string {
	return []string{
		"formatting.preset",
		"formatting.common.indent_style", "formatting.common.indent_width",
		"formatting.common.line_width", "formatting.common.line_ending",
		"formatting.common.final_newline",
		"formatting.json.arrays", "formatting.json.objects", "formatting.json.key_order",
		"formatting.jsonc.trailing_commas",
		"formatting.yaml.sequences", "formatting.yaml.mappings",
		"formatting.yaml.quote_style", "formatting.yaml.sequence_indent",
		"formatting.yaml.document_start",
		"formatting.toml.arrays", "formatting.toml.trailing_commas",
		"formatting.toml.quote_style", "formatting.toml.align_entries",
		"formatting.toml.align_comments", "formatting.toml.key_order",
		"formatting.markdown.prose_wrap", "formatting.markdown.list_spacing",
		"formatting.markdown.tables",
	}
}

// SetKeys returns the leaf keys this formatting layer explicitly sets.
func (p FormattingPatch) SetKeys() []string {
	keys := make([]string, 0, len(FormattingKeys()))
	appendKey(&keys, "formatting.preset", p.Preset)
	if p.Common != nil {
		p.Common.appendKeys(&keys)
	}
	if p.JSON != nil {
		p.JSON.appendKeys(&keys)
	}
	if p.JSONC != nil {
		appendKey(&keys, "formatting.jsonc.trailing_commas", p.JSONC.TrailingCommas)
	}
	if p.YAML != nil {
		p.YAML.appendKeys(&keys)
	}
	if p.TOML != nil {
		p.TOML.appendKeys(&keys)
	}
	if p.Markdown != nil {
		p.Markdown.appendKeys(&keys)
	}

	return keys
}

func (p FormattingCommonPatch) appendKeys(keys *[]string) {
	appendKey(keys, "formatting.common.indent_style", p.IndentStyle)
	appendKey(keys, "formatting.common.indent_width", p.IndentWidth)
	appendKey(keys, "formatting.common.line_width", p.LineWidth)
	appendKey(keys, "formatting.common.line_ending", p.LineEnding)
	appendKey(keys, "formatting.common.final_newline", p.FinalNewline)
}

func (p FormattingJSONPatch) appendKeys(keys *[]string) {
	appendKey(keys, "formatting.json.arrays", p.Arrays)
	appendKey(keys, "formatting.json.objects", p.Objects)
	appendKey(keys, "formatting.json.key_order", p.KeyOrder)
}

func (p FormattingYAMLPatch) appendKeys(keys *[]string) {
	appendKey(keys, "formatting.yaml.sequences", p.Sequences)
	appendKey(keys, "formatting.yaml.mappings", p.Mappings)
	appendKey(keys, "formatting.yaml.quote_style", p.QuoteStyle)
	appendKey(keys, "formatting.yaml.sequence_indent", p.SequenceIndent)
	appendKey(keys, "formatting.yaml.document_start", p.DocumentStart)
}

func (p FormattingTOMLPatch) appendKeys(keys *[]string) {
	appendKey(keys, "formatting.toml.arrays", p.Arrays)
	appendKey(keys, "formatting.toml.trailing_commas", p.TrailingCommas)
	appendKey(keys, "formatting.toml.quote_style", p.QuoteStyle)
	appendKey(keys, "formatting.toml.align_entries", p.AlignEntries)
	appendKey(keys, "formatting.toml.align_comments", p.AlignComments)
	appendKey(keys, "formatting.toml.key_order", p.KeyOrder)
}

func (p FormattingMarkdownPatch) appendKeys(keys *[]string) {
	appendKey(keys, "formatting.markdown.prose_wrap", p.ProseWrap)
	appendKey(keys, "formatting.markdown.list_spacing", p.ListSpacing)
	appendKey(keys, "formatting.markdown.tables", p.Tables)
}

func appendKey[T any](keys *[]string, key string, value *T) {
	if value != nil {
		*keys = append(*keys, key)
	}
}

func (p *FormattingPatch) normalize() error {
	if err := enumSetting("formatting.preset", p.Preset, "preserve", "conventional"); err != nil {
		return err
	}
	if p.Common != nil {
		if err := p.Common.normalize(); err != nil {
			return err
		}
	}
	if p.JSON != nil {
		if err := p.JSON.normalize(); err != nil {
			return err
		}
	}
	if p.JSONC != nil {
		if err := enumSetting("formatting.jsonc.trailing_commas", p.JSONC.TrailingCommas, "preserve", "insert", "remove"); err != nil {
			return err
		}
	}
	if p.YAML != nil {
		if err := p.YAML.normalize(); err != nil {
			return err
		}
	}
	if p.TOML != nil {
		if err := p.TOML.normalize(); err != nil {
			return err
		}
	}
	if p.Markdown != nil {
		return p.Markdown.normalize()
	}

	return nil
}

func (p *FormattingCommonPatch) normalize() error {
	checks := []error{
		enumSetting("formatting.common.indent_style", p.IndentStyle, "preserve", "spaces", "tabs"),
		boundedSetting("formatting.common.indent_width", p.IndentWidth, 1, 16),
		boundedSetting("formatting.common.line_width", p.LineWidth, 40, 320),
		enumSetting("formatting.common.line_ending", p.LineEnding, "preserve", "lf", "crlf"),
		enumSetting("formatting.common.final_newline", p.FinalNewline, "preserve", "insert", "remove"),
	}

	return firstError(checks)
}

func (p *FormattingJSONPatch) normalize() error {
	collection := []string{preserve, auto, "compact", "expanded"}
	return firstError([]error{
		enumSetting("formatting.json.arrays", p.Arrays, collection...),
		enumSetting("formatting.json.objects", p.Objects, collection...),
		enumSetting("formatting.json.key_order", p.KeyOrder, "preserve", "sort"),
	})
}

func (p *FormattingYAMLPatch) normalize() error {
	return firstError([]error{
		enumSetting("formatting.yaml.sequences", p.Sequences, preserve, auto, "flow", "block"),
		enumSetting("formatting.yaml.mappings", p.Mappings, preserve, auto, "flow", "block"),
		enumSetting("formatting.yaml.quote_style", p.QuoteStyle, "preserve", "prefer_plain", "prefer_single", "prefer_double"),
		enumSetting("formatting.yaml.sequence_indent", p.SequenceIndent, "preserve", "indented", "indentless"),
		enumSetting("formatting.yaml.document_start", p.DocumentStart, "preserve", "insert", "remove"),
	})
}

func (p *FormattingTOMLPatch) normalize() error {
	collection := []string{preserve, auto, "compact", "expanded"}
	return firstError([]error{
		enumSetting("formatting.toml.arrays", p.Arrays, collection...),
		enumSetting("formatting.toml.trailing_commas", p.TrailingCommas, "preserve", "multiline", "remove"),
		enumSetting("formatting.toml.quote_style", p.QuoteStyle, "preserve", "prefer_basic", "prefer_literal"),
		enumSetting("formatting.toml.align_entries", p.AlignEntries, "preserve", "align", "compact"),
		enumSetting("formatting.toml.align_comments", p.AlignComments, "preserve", "align", "compact"),
		enumSetting("formatting.toml.key_order", p.KeyOrder, "preserve", "sort"),
	})
}

func (p *FormattingMarkdownPatch) normalize() error {
	return firstError([]error{
		enumSetting("formatting.markdown.prose_wrap", p.ProseWrap, "preserve", "always", "never"),
		enumSetting("formatting.markdown.list_spacing", p.ListSpacing, "preserve", "tight", "loose"),
		enumSetting("formatting.markdown.tables", p.Tables, "preserve", "align", "compact"),
	})
}

func enumSetting(key string, value *string, choices ...string) error {
	if value == nil {
		return nil
	}
	for _, choice := range choices {
		if *value == choice {
			return nil
		}
	}
	return fmt.Errorf("%w for %s: %q is not one of %v", ErrInvalidValue, key, *value, choices)
}

func boundedSetting(key string, value *int, minimum, maximum int) error {
	if value == nil || (*value >= minimum && *value <= maximum) {
		return nil
	}
	return fmt.Errorf("%w for %s: %d is outside %d..%d", ErrInvalidValue, key, *value, minimum, maximum)
}

func firstError(errors []error) error {
	for _, err := range errors {
		if err != nil {
			return err
		}
	}
	return nil
}

// AsPatch returns a complete sparse representation of this policy.
func (p FormattingPolicy) AsPatch() FormattingPatch {
	common := FormattingCommonPatch{IndentStyle: &p.Common.IndentStyle, IndentWidth: &p.Common.IndentWidth, LineWidth: &p.Common.LineWidth, LineEnding: &p.Common.LineEnding, FinalNewline: &p.Common.FinalNewline}
	json := FormattingJSONPatch{Arrays: &p.JSON.Arrays, Objects: &p.JSON.Objects, KeyOrder: &p.JSON.KeyOrder}
	jsonc := FormattingJSONCPatch{TrailingCommas: &p.JSONC.TrailingCommas}
	yaml := FormattingYAMLPatch{Sequences: &p.YAML.Sequences, Mappings: &p.YAML.Mappings, QuoteStyle: &p.YAML.QuoteStyle, SequenceIndent: &p.YAML.SequenceIndent, DocumentStart: &p.YAML.DocumentStart}
	toml := FormattingTOMLPatch{Arrays: &p.TOML.Arrays, TrailingCommas: &p.TOML.TrailingCommas, QuoteStyle: &p.TOML.QuoteStyle, AlignEntries: &p.TOML.AlignEntries, AlignComments: &p.TOML.AlignComments, KeyOrder: &p.TOML.KeyOrder}
	markdown := FormattingMarkdownPatch{ProseWrap: &p.Markdown.ProseWrap, ListSpacing: &p.Markdown.ListSpacing, Tables: &p.Markdown.Tables}
	return FormattingPatch{Preset: &p.Preset, Common: &common, JSON: &json, JSONC: &jsonc, YAML: &yaml, TOML: &toml, Markdown: &markdown}
}
