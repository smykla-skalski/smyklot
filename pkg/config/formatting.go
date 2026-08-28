package config

// FormattingPatch is one sparse formatting layer. A nil leaf inherits from
// the layer below it; an explicit "preserve" is a value and therefore cancels
// a lower formatting choice.
type FormattingPatch struct {
	// Preset resets every formatting leaf before sibling overrides are applied.
	Preset *string `json:"preset,omitempty" yaml:"preset,omitempty" toml:"preset,omitempty" default:"preserve" enum:"preserve,conventional" presets:"conventional=conventional"`
	// Common carries presentation choices shared by every supported format.
	Common *FormattingCommonPatch `json:"common,omitempty" yaml:"common,omitempty" toml:"common,omitempty"`
	// JSON carries strict JSON presentation choices.
	JSON *FormattingJSONPatch `json:"json,omitempty" yaml:"json,omitempty" toml:"json,omitempty"`
	// JSONC carries comment-aware JSON presentation choices.
	JSONC *FormattingJSONCPatch `json:"jsonc,omitempty" yaml:"jsonc,omitempty" toml:"jsonc,omitempty"`
	// YAML carries YAML presentation choices.
	YAML *FormattingYAMLPatch `json:"yaml,omitempty" yaml:"yaml,omitempty" toml:"yaml,omitempty"`
	// TOML carries TOML presentation choices.
	TOML *FormattingTOMLPatch `json:"toml,omitempty" yaml:"toml,omitempty" toml:"toml,omitempty"`
	// Markdown carries Markdown presentation choices.
	Markdown *FormattingMarkdownPatch `json:"markdown,omitempty" yaml:"markdown,omitempty" toml:"markdown,omitempty"`
}

type FormattingCommonPatch struct {
	// IndentStyle chooses spaces, tabs, or the document's existing indentation.
	IndentStyle *string `json:"indent_style,omitempty" yaml:"indent_style,omitempty" toml:"indent_style,omitempty" default:"preserve" enum:"preserve,spaces,tabs" presets:"conventional=spaces"`
	// IndentWidth is the number of spaces represented by one indentation level.
	IndentWidth *int `json:"indent_width,omitempty" yaml:"indent_width,omitempty" toml:"indent_width,omitempty" default:"2" min:"1" max:"16"`
	// LineWidth is the target width used by automatic collection and prose layout.
	LineWidth *int `json:"line_width,omitempty" yaml:"line_width,omitempty" toml:"line_width,omitempty" default:"100" min:"40" max:"320"`
	// LineEnding chooses LF, CRLF, or the document's existing endings.
	LineEnding *string `json:"line_ending,omitempty" yaml:"line_ending,omitempty" toml:"line_ending,omitempty" default:"preserve" enum:"preserve,lf,crlf" presets:"conventional=lf"`
	// FinalNewline inserts, removes, or preserves the last line ending.
	FinalNewline *string `json:"final_newline,omitempty" yaml:"final_newline,omitempty" toml:"final_newline,omitempty" default:"preserve" enum:"preserve,insert,remove" presets:"conventional=insert"`
}

type FormattingJSONPatch struct {
	// Arrays controls JSON array layout.
	Arrays *string `json:"arrays,omitempty" yaml:"arrays,omitempty" toml:"arrays,omitempty" default:"preserve" enum:"preserve,auto,compact,expanded" presets:"conventional=auto"`
	// Objects controls JSON object layout.
	Objects *string `json:"objects,omitempty" yaml:"objects,omitempty" toml:"objects,omitempty" default:"preserve" enum:"preserve,auto,compact,expanded" presets:"conventional=auto"`
	// KeyOrder preserves insertion order or sorts object keys.
	KeyOrder *string `json:"key_order,omitempty" yaml:"key_order,omitempty" toml:"key_order,omitempty" default:"preserve" enum:"preserve,sort"`
}

type FormattingJSONCPatch struct {
	// TrailingCommas inserts, removes, or preserves JSONC trailing commas.
	TrailingCommas *string `json:"trailing_commas,omitempty" yaml:"trailing_commas,omitempty" toml:"trailing_commas,omitempty" default:"preserve" enum:"preserve,insert,remove"`
}

type FormattingYAMLPatch struct {
	// Sequences controls YAML sequence layout.
	Sequences *string `json:"sequences,omitempty" yaml:"sequences,omitempty" toml:"sequences,omitempty" default:"preserve" enum:"preserve,auto,flow,block" presets:"conventional=auto"`
	// Mappings controls YAML mapping layout.
	Mappings *string `json:"mappings,omitempty" yaml:"mappings,omitempty" toml:"mappings,omitempty" default:"preserve" enum:"preserve,auto,flow,block" presets:"conventional=block"`
	// QuoteStyle controls safe scalar quote preference.
	QuoteStyle *string `json:"quote_style,omitempty" yaml:"quote_style,omitempty" toml:"quote_style,omitempty" default:"preserve" enum:"preserve,prefer_plain,prefer_single,prefer_double"`
	// SequenceIndent controls indentation of block sequence markers.
	SequenceIndent *string `json:"sequence_indent,omitempty" yaml:"sequence_indent,omitempty" toml:"sequence_indent,omitempty" default:"preserve" enum:"preserve,indented,indentless"`
	// DocumentStart inserts, removes, or preserves the YAML document marker.
	DocumentStart *string `json:"document_start,omitempty" yaml:"document_start,omitempty" toml:"document_start,omitempty" default:"preserve" enum:"preserve,insert,remove"`
}

type FormattingTOMLPatch struct {
	// Arrays controls TOML array layout.
	Arrays *string `json:"arrays,omitempty" yaml:"arrays,omitempty" toml:"arrays,omitempty" default:"preserve" enum:"preserve,auto,compact,expanded" presets:"conventional=auto"`
	// TrailingCommas controls commas in multiline TOML arrays.
	TrailingCommas *string `json:"trailing_commas,omitempty" yaml:"trailing_commas,omitempty" toml:"trailing_commas,omitempty" default:"preserve" enum:"preserve,multiline,remove"`
	// QuoteStyle controls safe TOML string quote preference.
	QuoteStyle *string `json:"quote_style,omitempty" yaml:"quote_style,omitempty" toml:"quote_style,omitempty" default:"preserve" enum:"preserve,prefer_basic,prefer_literal"`
	// AlignEntries aligns or compacts neighbouring TOML assignments.
	AlignEntries *string `json:"align_entries,omitempty" yaml:"align_entries,omitempty" toml:"align_entries,omitempty" default:"preserve" enum:"preserve,align,compact"`
	// AlignComments aligns or compacts neighbouring TOML comments.
	AlignComments *string `json:"align_comments,omitempty" yaml:"align_comments,omitempty" toml:"align_comments,omitempty" default:"preserve" enum:"preserve,align,compact"`
	// KeyOrder preserves insertion order or sorts TOML keys.
	KeyOrder *string `json:"key_order,omitempty" yaml:"key_order,omitempty" toml:"key_order,omitempty" default:"preserve" enum:"preserve,sort"`
}

type FormattingMarkdownPatch struct {
	// ProseWrap wraps safe prose, removes soft wraps, or preserves it.
	ProseWrap *string `json:"prose_wrap,omitempty" yaml:"prose_wrap,omitempty" toml:"prose_wrap,omitempty" default:"preserve" enum:"preserve,always,never"`
	// ListSpacing makes safe lists tight or loose, or preserves their spacing.
	ListSpacing *string `json:"list_spacing,omitempty" yaml:"list_spacing,omitempty" toml:"list_spacing,omitempty" default:"preserve" enum:"preserve,tight,loose"`
	// Tables aligns or compacts safe GFM tables, or preserves them.
	Tables *string `json:"tables,omitempty" yaml:"tables,omitempty" toml:"tables,omitempty" default:"preserve" enum:"preserve,align,compact" presets:"conventional=align"`
}

// FormattingPolicy is a complete formatting decision. It contains no sparse
// pointers, so code that renders a file never has to repeat inheritance rules.
type FormattingPolicy struct {
	Preset   string                   `json:"preset"`
	Common   FormattingCommonPolicy   `json:"common"`
	JSON     FormattingJSONPolicy     `json:"json"`
	JSONC    FormattingJSONCPolicy    `json:"jsonc"`
	YAML     FormattingYAMLPolicy     `json:"yaml"`
	TOML     FormattingTOMLPolicy     `json:"toml"`
	Markdown FormattingMarkdownPolicy `json:"markdown"`
}

type FormattingCommonPolicy struct {
	IndentStyle  string `json:"indent_style"`
	IndentWidth  int    `json:"indent_width"`
	LineWidth    int    `json:"line_width"`
	LineEnding   string `json:"line_ending"`
	FinalNewline string `json:"final_newline"`
}

type FormattingJSONPolicy struct {
	Arrays   string `json:"arrays"`
	Objects  string `json:"objects"`
	KeyOrder string `json:"key_order"`
}

type FormattingJSONCPolicy struct {
	TrailingCommas string `json:"trailing_commas"`
}

type FormattingYAMLPolicy struct {
	Sequences      string `json:"sequences"`
	Mappings       string `json:"mappings"`
	QuoteStyle     string `json:"quote_style"`
	SequenceIndent string `json:"sequence_indent"`
	DocumentStart  string `json:"document_start"`
}

type FormattingTOMLPolicy struct {
	Arrays         string `json:"arrays"`
	TrailingCommas string `json:"trailing_commas"`
	QuoteStyle     string `json:"quote_style"`
	AlignEntries   string `json:"align_entries"`
	AlignComments  string `json:"align_comments"`
	KeyOrder       string `json:"key_order"`
}

type FormattingMarkdownPolicy struct {
	ProseWrap   string `json:"prose_wrap"`
	ListSpacing string `json:"list_spacing"`
	Tables      string `json:"tables"`
}

// FormattingSources mirrors FormattingPolicy and records the winning layer
// for every leaf.
type FormattingSources struct {
	Preset   Source                    `json:"preset"`
	Common   FormattingCommonSources   `json:"common"`
	JSON     FormattingJSONSources     `json:"json"`
	JSONC    FormattingJSONCSources    `json:"jsonc"`
	YAML     FormattingYAMLSources     `json:"yaml"`
	TOML     FormattingTOMLSources     `json:"toml"`
	Markdown FormattingMarkdownSources `json:"markdown"`
}

type FormattingCommonSources struct {
	IndentStyle  Source `json:"indent_style"`
	IndentWidth  Source `json:"indent_width"`
	LineWidth    Source `json:"line_width"`
	LineEnding   Source `json:"line_ending"`
	FinalNewline Source `json:"final_newline"`
}

type FormattingJSONSources struct {
	Arrays   Source `json:"arrays"`
	Objects  Source `json:"objects"`
	KeyOrder Source `json:"key_order"`
}

type FormattingJSONCSources struct {
	TrailingCommas Source `json:"trailing_commas"`
}

type FormattingYAMLSources struct {
	Sequences      Source `json:"sequences"`
	Mappings       Source `json:"mappings"`
	QuoteStyle     Source `json:"quote_style"`
	SequenceIndent Source `json:"sequence_indent"`
	DocumentStart  Source `json:"document_start"`
}

type FormattingTOMLSources struct {
	Arrays         Source `json:"arrays"`
	TrailingCommas Source `json:"trailing_commas"`
	QuoteStyle     Source `json:"quote_style"`
	AlignEntries   Source `json:"align_entries"`
	AlignComments  Source `json:"align_comments"`
	KeyOrder       Source `json:"key_order"`
}

type FormattingMarkdownSources struct {
	ProseWrap   Source `json:"prose_wrap"`
	ListSpacing Source `json:"list_spacing"`
	Tables      Source `json:"tables"`
}
