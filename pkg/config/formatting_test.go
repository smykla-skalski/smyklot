package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

var _ = Describe("Formatting configuration [Unit]", func() {
	BeforeEach(clearEnv)

	It("defaults every presentation dimension to preserve", func() {
		policy := config.DefaultFormattingPolicy()

		Expect(policy.AllPreserve()).To(BeTrue())
		Expect(config.Default().Formatting).To(Equal(policy))
		Expect(policy.Common.IndentWidth).To(Equal(2))
		Expect(policy.Common.LineWidth).To(Equal(100))
	})

	It("resets every leaf at a preset before applying sibling settings", func() {
		lower := config.ApplyFormattingPatch(
			config.DefaultFormattingPolicy(),
			fullFormattingPatch(),
		)
		preset := "conventional"
		preserve := "preserve"
		patch := config.FormattingPatch{
			Preset: &preset,
			Common: &config.FormattingCommonPatch{LineEnding: &preserve},
			JSON:   &config.FormattingJSONPatch{Arrays: &preserve},
		}

		resolved := config.ApplyFormattingPatch(lower, patch)

		Expect(resolved.Preset).To(Equal("conventional"))
		Expect(resolved.Common.IndentStyle).To(Equal("spaces"))
		Expect(resolved.Common.LineEnding).To(Equal("preserve"))
		Expect(resolved.Common.FinalNewline).To(Equal("insert"))
		Expect(resolved.JSON.Arrays).To(Equal("preserve"))
		Expect(resolved.JSON.Objects).To(Equal("auto"))
		Expect(resolved.YAML.Sequences).To(Equal("auto"))
		Expect(resolved.YAML.Mappings).To(Equal("block"))
		Expect(resolved.TOML.Arrays).To(Equal("auto"))
		Expect(resolved.Markdown.Tables).To(Equal("align"))
		Expect(resolved.JSON.KeyOrder).To(Equal("preserve"))
		Expect(resolved.YAML.QuoteStyle).To(Equal("preserve"))
		Expect(resolved.TOML.QuoteStyle).To(Equal("preserve"))
	})

	It("lets explicit preserve cancel a lower normalization", func() {
		compact := "compact"
		preserve := "preserve"
		lower := config.ApplyFormattingPatch(config.DefaultFormattingPolicy(), config.FormattingPatch{
			JSON: &config.FormattingJSONPatch{Arrays: &compact},
		})

		resolved := config.ApplyFormattingPatch(lower, config.FormattingPatch{
			JSON: &config.FormattingJSONPatch{Arrays: &preserve},
		})

		Expect(resolved.JSON.Arrays).To(Equal("preserve"))
	})

	DescribeTable("strictly decodes every formatting leaf",
		func(format config.Format) {
			patch := fullFormattingPatch()
			var (
				document []byte
				err      error
			)
			if format == config.FormatTOML {
				document, err = config.RenderTOML(config.Patch{Formatting: &patch})
			} else {
				document, err = yaml.Marshal(config.Patch{Formatting: &patch})
			}
			Expect(err).NotTo(HaveOccurred())

			parsed, err := config.ParsePatch(format, document)
			Expect(err).NotTo(HaveOccurred())
			Expect(parsed.Formatting).NotTo(BeNil())
			Expect(parsed.Formatting.SetKeys()).To(Equal(config.FormattingKeys()))
			Expect(config.ApplyFormattingPatch(config.DefaultFormattingPolicy(), *parsed.Formatting)).
				To(Equal(fullFormattingPolicy()))
		},
		Entry("from TOML", config.FormatTOML),
		Entry("from YAML", config.FormatYAML),
	)

	DescribeTable("rejects unknown nested formatting fields",
		func(format config.Format, document string) {
			_, err := config.ParsePatch(format, []byte(document))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown_rule"))
		},
		Entry("in TOML", config.FormatTOML, "[formatting.json]\nunknown_rule = true\n"),
		Entry("in YAML", config.FormatYAML, "formatting:\n  json:\n    unknown_rule: true\n"),
	)

	DescribeTable("rejects integer values outside their documented bounds",
		func(format config.Format, document string, key string) {
			_, err := config.ParsePatch(format, []byte(document))
			Expect(err).To(MatchError(config.ErrInvalidValue))
			Expect(err.Error()).To(ContainSubstring(key))
		},
		Entry("TOML indent below", config.FormatTOML, "[formatting.common]\nindent_width = 0\n", config.KeyFormattingCommonIndentWidth),
		Entry("TOML indent above", config.FormatTOML, "[formatting.common]\nindent_width = 17\n", config.KeyFormattingCommonIndentWidth),
		Entry("YAML width below", config.FormatYAML, "formatting:\n  common:\n    line_width: 39\n", config.KeyFormattingCommonLineWidth),
		Entry("YAML width above", config.FormatYAML, "formatting:\n  common:\n    line_width: 321\n", config.KeyFormattingCommonLineWidth),
	)

	It("publishes exact environment and flag names for nested leaves", func() {
		Expect(config.EnvVar(config.KeyFormattingJSONArrays)).To(Equal("SMYKLOT_FORMATTING_JSON_ARRAYS"))
		Expect(config.FlagName(config.KeyFormattingJSONArrays)).To(Equal("formatting-json-arrays"))
	})

	DescribeTable("loads every formatting leaf through each process layer",
		func(setup func() []string) {
			flags := newFlags(setup()...)
			loaded, err := config.LoadProcess(flags)
			Expect(err).NotTo(HaveOccurred())
			Expect(loaded.Formatting).To(Equal(fullFormattingPolicy()))
			resolved := config.Resolve(loaded)
			for _, key := range config.FormattingKeys() {
				Expect(resolved.Sources).NotTo(HaveKey(key))
			}
			Expect(resolved.Formatting).To(Equal(fullFormattingSources(config.SourceProcess)))
		},
		Entry("process file", func() []string {
			setEnv(config.EnvConfigFile, writeFullFormattingFile())
			return nil
		}),
		Entry("process document", func() []string {
			setEnv(config.EnvConfig, fullFormattingTOML())
			return nil
		}),
		Entry("process environment", func() []string {
			for key, value := range fullFormattingValues() {
				setEnv(config.EnvVar(key), value)
			}
			return nil
		}),
		Entry("process flags", func() []string {
			arguments := make([]string, 0, len(config.FormattingKeys()))
			values := fullFormattingValues()
			for _, key := range config.FormattingKeys() {
				arguments = append(arguments, "--"+config.FlagName(key)+"="+values[key])
			}
			return arguments
		}),
	)

	DescribeTable("resolves every formatting leaf through each persisted layer",
		func(source config.Source) {
			patch := fullFormattingPatch()
			resolved := config.Resolve(config.Default(), config.Layer{
				Source: source,
				Patch:  config.Patch{Formatting: &patch},
			})

			Expect(resolved.Values.Formatting).To(Equal(fullFormattingPolicy()))
			for _, key := range config.FormattingKeys() {
				Expect(resolved.Sources).NotTo(HaveKey(key))
			}
			Expect(resolved.Formatting).To(Equal(fullFormattingSources(source)))
		},
		Entry("account settings", config.SourceTarget),
		Entry("repository file", config.SourceRepositoryFile),
		Entry("repository settings", config.SourceRepositoryPanel),
	)

	It("copies a complete policy into a sparse patch without aliasing it", func() {
		policy := fullFormattingPolicy()
		patch := policy.AsPatch()

		Expect(patch.SetKeys()).To(Equal(config.FormattingKeys()))
		*patch.Common.IndentWidth = 9
		Expect(policy.Common.IndentWidth).To(Equal(4))
	})
})

func fullFormattingSources(source config.Source) config.FormattingSources {
	return config.FormattingSources{
		Preset: source,
		Common: config.FormattingCommonSources{
			IndentStyle: source, IndentWidth: source, LineWidth: source,
			LineEnding: source, FinalNewline: source,
		},
		JSON: config.FormattingJSONSources{
			Arrays: source, Objects: source, KeyOrder: source,
		},
		JSONC: config.FormattingJSONCSources{TrailingCommas: source},
		YAML: config.FormattingYAMLSources{
			Sequences: source, Mappings: source, QuoteStyle: source,
			SequenceIndent: source, DocumentStart: source,
		},
		TOML: config.FormattingTOMLSources{
			Arrays: source, TrailingCommas: source, QuoteStyle: source,
			AlignEntries: source, AlignComments: source, KeyOrder: source,
		},
		Markdown: config.FormattingMarkdownSources{
			ProseWrap: source, ListSpacing: source, Tables: source,
		},
	}
}

func fullFormattingPatch() config.FormattingPatch {
	values := fullFormattingValues()
	stringValue := func(key string) *string { value := values[key]; return &value }
	intValue := func(key string) *int {
		value := 4
		if key == config.KeyFormattingCommonLineWidth {
			value = 120
		}
		return &value
	}

	return config.FormattingPatch{
		Preset: stringValue(config.KeyFormattingPreset),
		Common: &config.FormattingCommonPatch{
			IndentStyle:  stringValue(config.KeyFormattingCommonIndentStyle),
			IndentWidth:  intValue(config.KeyFormattingCommonIndentWidth),
			LineWidth:    intValue(config.KeyFormattingCommonLineWidth),
			LineEnding:   stringValue(config.KeyFormattingCommonLineEnding),
			FinalNewline: stringValue(config.KeyFormattingCommonFinalNewline),
		},
		JSON: &config.FormattingJSONPatch{
			Arrays: stringValue(config.KeyFormattingJSONArrays), Objects: stringValue(config.KeyFormattingJSONObjects),
			KeyOrder: stringValue(config.KeyFormattingJSONKeyOrder),
		},
		JSONC: &config.FormattingJSONCPatch{TrailingCommas: stringValue(config.KeyFormattingJSONCTrailingCommas)},
		YAML: &config.FormattingYAMLPatch{
			Sequences: stringValue(config.KeyFormattingYAMLSequences), Mappings: stringValue(config.KeyFormattingYAMLMappings),
			QuoteStyle: stringValue(config.KeyFormattingYAMLQuoteStyle), SequenceIndent: stringValue(config.KeyFormattingYAMLSequenceIndent),
			DocumentStart: stringValue(config.KeyFormattingYAMLDocumentStart),
		},
		TOML: &config.FormattingTOMLPatch{
			Arrays: stringValue(config.KeyFormattingTOMLArrays), TrailingCommas: stringValue(config.KeyFormattingTOMLTrailingCommas),
			QuoteStyle: stringValue(config.KeyFormattingTOMLQuoteStyle), AlignEntries: stringValue(config.KeyFormattingTOMLAlignEntries),
			AlignComments: stringValue(config.KeyFormattingTOMLAlignComments), KeyOrder: stringValue(config.KeyFormattingTOMLKeyOrder),
		},
		Markdown: &config.FormattingMarkdownPatch{
			ProseWrap: stringValue(config.KeyFormattingMarkdownProseWrap), ListSpacing: stringValue(config.KeyFormattingMarkdownListSpacing),
			Tables: stringValue(config.KeyFormattingMarkdownTables),
		},
	}
}

func fullFormattingPolicy() config.FormattingPolicy {
	return config.ApplyFormattingPatch(config.DefaultFormattingPolicy(), fullFormattingPatch())
}

func fullFormattingValues() map[string]string {
	return map[string]string{
		config.KeyFormattingPreset:              "preserve",
		config.KeyFormattingCommonIndentStyle:   "tabs",
		config.KeyFormattingCommonIndentWidth:   "4",
		config.KeyFormattingCommonLineWidth:     "120",
		config.KeyFormattingCommonLineEnding:    "crlf",
		config.KeyFormattingCommonFinalNewline:  "remove",
		config.KeyFormattingJSONArrays:          "compact",
		config.KeyFormattingJSONObjects:         "expanded",
		config.KeyFormattingJSONKeyOrder:        "sort",
		config.KeyFormattingJSONCTrailingCommas: "insert",
		config.KeyFormattingYAMLSequences:       "flow",
		config.KeyFormattingYAMLMappings:        "block",
		config.KeyFormattingYAMLQuoteStyle:      "prefer_single",
		config.KeyFormattingYAMLSequenceIndent:  "indentless",
		config.KeyFormattingYAMLDocumentStart:   "insert",
		config.KeyFormattingTOMLArrays:          "expanded",
		config.KeyFormattingTOMLTrailingCommas:  "multiline",
		config.KeyFormattingTOMLQuoteStyle:      "prefer_literal",
		config.KeyFormattingTOMLAlignEntries:    "align",
		config.KeyFormattingTOMLAlignComments:   "compact",
		config.KeyFormattingTOMLKeyOrder:        "sort",
		config.KeyFormattingMarkdownProseWrap:   "never",
		config.KeyFormattingMarkdownListSpacing: "loose",
		config.KeyFormattingMarkdownTables:      "compact",
	}
}

func fullFormattingTOML() string {
	document, err := config.RenderTOML(config.Patch{Formatting: ptr(fullFormattingPatch())})
	Expect(err).NotTo(HaveOccurred())

	return string(document)
}

func writeFullFormattingFile() string {
	return writeConfigFile("formatting.toml", fullFormattingTOML())
}
