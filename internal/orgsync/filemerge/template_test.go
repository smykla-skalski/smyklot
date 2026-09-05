package filemerge_test

import (
	"bytes"
	"testing"

	yaml "go.yaml.in/yaml/v3"

	"github.com/smykla-skalski/smyklot/internal/orgsync/filemerge"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func TestSharedTemplatesAlwaysEndWithNewline(t *testing.T) {
	t.Parallel()
	for _, source := range []struct{ path, body string }{
		{"config.json", `{"enabled":true}`},
		{"config.jsonc", "// retained\n{\"enabled\":true}"},
		{"config.yaml", "enabled: true"},
		{"config.toml", "enabled = true"},
		{"README.md", "# Heading"},
		{"LICENSE", "Some text"},
		{"script.sh", "#!/bin/sh\nprintf hello"},
	} {
		for _, newline := range []string{"preserve", "insert", "remove"} {
			t.Run(source.path+"/"+newline, func(t *testing.T) {
				policy := config.Default().Formatting
				policy.Common.FinalNewline = newline
				result, err := filemerge.ApplyTemplate(source.path, []byte(source.body), filemerge.Spec{}, policy)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(result.Final, []byte(source.body+"\n")) {
					t.Fatalf("final = %q", result.Final)
				}
				if !bytes.Equal(result.Composed, result.Final) {
					t.Fatal("terminator became a formatting difference")
				}
			})
		}
	}
}

func TestTemplateTerminationPreservesYAMLValues(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		"value: .nan", "value: .NaN", "value: .NAN", "[.nan, .inf, -.inf]",
		"value: .nan\ntext: |\n  text", ".nan: |\n  text",
		"value: |\n  text", "value: >\n  first\n  second",
		"value: |+\n  text", "value: |-\n  text", "value: |2 # comment\n  text",
		"żółw: |\n  text", "nested:\n  value: >+\n    text",
		"value: &shared |\n  text", "value: !!str |\n  text",
		"value: !<tag:yaml.org,2002:str> >\n  text",
		"żółw: &shared !!str |2 # comment\n  text",
		"value: |\r\n  text", "value: |+\n  text\n\n",
	} {
		t.Run(source, func(t *testing.T) {
			result, err := filemerge.NormalizeTemplate("config.yaml", []byte(source))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.HasSuffix(result, []byte("\n")) {
				t.Fatalf("unterminated: %q", result)
			}
			var before, after yaml.Node
			if err := yaml.Unmarshal([]byte(source), &before); err != nil {
				t.Fatal(err)
			}
			if err := yaml.Unmarshal(result, &after); err != nil {
				t.Fatal(err)
			}
			if !sameTemplateScalars(&before, &after) {
				t.Fatalf("changed value: %q -> %q", source, result)
			}
			again, err := filemerge.NormalizeTemplate("config.yaml", result)
			if err != nil || !bytes.Equal(again, result) {
				t.Fatalf("not idempotent: %q, %v", again, err)
			}
		})
	}
}

func TestPreserveDoesNotAcceptInvalidStructuredTemplates(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ path, content string }{
		{"a.json", `{"a":1,"a":2}`},
		{"a.json", "// comment\n{}"},
		{"a.jsonc", "{ broken }"},
		{"a.yaml", "a: 1\na: 2\n"},
		{"a.yaml", "a: 1\n---\na: 2\n"},
		{"a.toml", "a = 1\na = 2\n"},
	} {
		t.Run(test.path+test.content, func(t *testing.T) {
			_, err := filemerge.ApplyTemplate(test.path, []byte(test.content), filemerge.Spec{}, config.Default().Formatting)
			if err == nil {
				t.Fatal("accepted invalid structured content")
			}
		})
	}
}

func TestTemplateTerminationPreservesLiteralContent(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ input, want string }{
		{"", "\n"},
		{"a", "a\n"},
		{"a\n", "a\n"},
		{"a\n\n", "a\n\n"},
		{"a\r\nb", "a\r\nb\r\n"},
		{"a\r\n", "a\r\n"},
		{"a\r", "a\r\n"},
		{"value: |+\n  text\n\n", "value: |+\n  text\n\n"},
	} {
		got := filemerge.TerminateTemplate([]byte(test.input))
		if string(got) != test.want {
			t.Errorf("%q: got %q, want %q", test.input, got, test.want)
		}
		if !bytes.Equal(got, filemerge.TerminateTemplate(got)) {
			t.Errorf("termination is not idempotent for %q", test.input)
		}
	}
}

func sameTemplateScalars(before, after *yaml.Node) bool {
	if before.Kind != after.Kind || before.Tag != after.Tag || before.Value != after.Value ||
		len(before.Content) != len(after.Content) {
		return false
	}
	for index := range before.Content {
		if !sameTemplateScalars(before.Content[index], after.Content[index]) {
			return false
		}
	}
	return true
}

func TestTemplateTerminationRetainsNaNKeyedBlockValue(t *testing.T) {
	t.Parallel()
	result, err := filemerge.NormalizeTemplate("config.yaml", []byte(".nan: |\n  text"))
	if err != nil {
		t.Fatal(err)
	}
	var values map[any]any
	if err := yaml.Unmarshal(result, &values); err != nil {
		t.Fatal(err)
	}
	for _, value := range values {
		if value != "text" {
			t.Fatalf("changed block scalar: %q", value)
		}
	}
}
