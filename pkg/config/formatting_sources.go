package config

func formattingSources(sources map[string]Source) FormattingSources {
	return FormattingSources{
		Preset: sources["formatting.preset"],
		Common: FormattingCommonSources{
			IndentStyle:  sources["formatting.common.indent_style"],
			IndentWidth:  sources["formatting.common.indent_width"],
			LineWidth:    sources["formatting.common.line_width"],
			LineEnding:   sources["formatting.common.line_ending"],
			FinalNewline: sources["formatting.common.final_newline"],
		},
		JSON: FormattingJSONSources{
			Arrays:   sources["formatting.json.arrays"],
			Objects:  sources["formatting.json.objects"],
			KeyOrder: sources["formatting.json.key_order"],
		},
		JSONC: FormattingJSONCSources{
			TrailingCommas: sources["formatting.jsonc.trailing_commas"],
		},
		YAML: FormattingYAMLSources{
			Sequences:      sources["formatting.yaml.sequences"],
			Mappings:       sources["formatting.yaml.mappings"],
			QuoteStyle:     sources["formatting.yaml.quote_style"],
			SequenceIndent: sources["formatting.yaml.sequence_indent"],
			DocumentStart:  sources["formatting.yaml.document_start"],
		},
		TOML: FormattingTOMLSources{
			Arrays:         sources["formatting.toml.arrays"],
			TrailingCommas: sources["formatting.toml.trailing_commas"],
			QuoteStyle:     sources["formatting.toml.quote_style"],
			AlignEntries:   sources["formatting.toml.align_entries"],
			AlignComments:  sources["formatting.toml.align_comments"],
			KeyOrder:       sources["formatting.toml.key_order"],
		},
		Markdown: FormattingMarkdownSources{
			ProseWrap:   sources["formatting.markdown.prose_wrap"],
			ListSpacing: sources["formatting.markdown.list_spacing"],
			Tables:      sources["formatting.markdown.tables"],
		},
	}
}
