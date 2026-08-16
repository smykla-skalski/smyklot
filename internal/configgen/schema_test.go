package configgen_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/configgen"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

// schemaDocument is only as much of JSON Schema as these tests read.
type schemaDocument struct {
	Schema               string                    `json:"$schema"`
	ID                   string                    `json:"$id"`
	Type                 string                    `json:"type"`
	AdditionalProperties bool                      `json:"additionalProperties"`
	Properties           map[string]schemaProperty `json:"properties"`
}

type schemaProperty struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Enum        []string `json:"enum"`
	Default     any      `json:"default"`
}

func readSchema(t *testing.T) schemaDocument {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(repoRoot, configgen.SchemaFile))
	if err != nil {
		t.Fatalf("read %s: %v", configgen.SchemaFile, err)
	}

	var document schemaDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("parse %s: %v", configgen.SchemaFile, err)
	}

	return document
}

func TestSchemaFileIsCurrent(t *testing.T) {
	rendered, err := configgen.RenderSchema(parse(t))
	if err != nil {
		t.Fatalf("RenderSchema() error = %v", err)
	}

	path := filepath.Join(repoRoot, configgen.SchemaFile)

	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	if string(rendered) != string(onDisk) {
		t.Errorf("%s is stale - run `mise run generate`", configgen.SchemaFile)
	}
}

func TestSchemaRenderIsDeterministic(t *testing.T) {
	model := parse(t)

	first, err := configgen.RenderSchema(model)
	if err != nil {
		t.Fatalf("RenderSchema() error = %v", err)
	}

	for range 8 {
		again, err := configgen.RenderSchema(model)
		if err != nil {
			t.Fatalf("RenderSchema() error = %v", err)
		}

		if string(again) != string(first) {
			t.Fatal("RenderSchema() is not deterministic")
		}
	}
}

// The completeness test for the published document. A setting missing from it
// is one an editor cannot complete and cannot check, and the reverse - a
// property with no setting behind it - is a document describing a version of
// Smyklot that no longer exists, which is exactly how dotsync's schema rotted.
func TestSchemaCoversEverySetting(t *testing.T) {
	document := readSchema(t)

	if len(document.Properties) != len(config.Keys()) {
		t.Errorf("schema has %d properties, there are %d settings",
			len(document.Properties), len(config.Keys()))
	}

	for _, key := range config.Keys() {
		property, ok := document.Properties[key]
		if !ok {
			t.Errorf("schema is missing %q", key)

			continue
		}

		if property.Description == "" {
			t.Errorf("schema describes %q with nothing", key)
		}

		if property.Type == "" {
			t.Errorf("schema gives %q no type", key)
		}
	}

	declared := make(map[string]struct{}, len(config.Keys()))
	for _, key := range config.Keys() {
		declared[key] = struct{}{}
	}

	for key := range document.Properties {
		if _, ok := declared[key]; !ok {
			t.Errorf("schema publishes %q, which is not a setting", key)
		}
	}
}

// A schema whose defaults are decorative is worse than one with none, because a
// reader trusts it. These are the values Default() actually returns.
func TestSchemaDefaultsAreTheRealDefaults(t *testing.T) {
	document := readSchema(t)
	defaults := reflect.ValueOf(*config.Default())

	for _, field := range parse(t).Fields {
		property, ok := document.Properties[field.Key]
		if !ok {
			continue
		}

		value := defaults.FieldByName(field.GoName)

		switch field.Kind {
		case configgen.KindBool:
			if property.Default != value.Bool() {
				t.Errorf("schema defaults %q to %v, Default() says %v",
					field.Key, property.Default, value.Bool())
			}

		case configgen.KindString, configgen.KindEnum:
			if property.Default != value.String() {
				t.Errorf("schema defaults %q to %v, Default() says %q",
					field.Key, property.Default, value.String())
			}

		case configgen.KindStringSlice, configgen.KindStringMap:
			if reflect.ValueOf(property.Default).Len() != 0 {
				t.Errorf("schema defaults %q to %v, Default() says empty",
					field.Key, property.Default)
			}
		}
	}
}

// The schema and the strict decoder are two statements of the same rule. If
// they disagree, somebody writes a file an editor calls valid and Smyklot calls
// broken - or worse, the other way round.
func TestSchemaAgreesWithTheDecoder(t *testing.T) {
	document := readSchema(t)

	if document.AdditionalProperties {
		t.Error("schema accepts unknown keys, the decoders reject them")
	}

	// Every published key has to be one the decoder accepts.
	for key, property := range document.Properties {
		document := key + " = " + tomlLiteral(property)

		if _, err := config.ParsePatch(config.FormatTOML, []byte(document)); err != nil {
			t.Errorf("schema publishes %q but the decoder refuses %q: %v", key, document, err)
		}
	}

	// And a key it does not publish has to be one the decoder refuses.
	if _, err := config.ParsePatch(config.FormatTOML, []byte("not_a_setting = true")); err == nil {
		t.Error("the decoder accepted a key the schema does not publish")
	}
}

// An enum's published values have to be values the decoder accepts, or the
// schema offers a completion that breaks the file.
func TestSchemaEnumsAreAccepted(t *testing.T) {
	for key, property := range readSchema(t).Properties {
		for _, value := range property.Enum {
			document := key + " = " + jsonString(value)

			if _, err := config.ParsePatch(config.FormatTOML, []byte(document)); err != nil {
				t.Errorf("schema offers %s = %q but the decoder refuses it: %v", key, value, err)
			}
		}
	}
}

// tomlLiteral renders a value of the property's type, for probing the decoder.
func tomlLiteral(property schemaProperty) string {
	switch property.Type {
	case "boolean":
		return "true"

	case "array":
		return "[]"

	case "object":
		return "{}"

	default:
		if len(property.Enum) > 0 {
			return jsonString(property.Enum[0])
		}

		return `"x"`
	}
}

func jsonString(value string) string {
	encoded, _ := json.Marshal(value)

	return string(encoded)
}
