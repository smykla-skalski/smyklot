package configgen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const (
	// SchemaDir is where RenderSchema's output belongs, relative to the
	// repository root. pkg/config embeds it, so what the service publishes is
	// what the binary was built from.
	SchemaDir = "pkg/config/schema"

	// SchemaName carries a version because the document is published and an
	// editor pins it: changing what v1 means underneath a repository is how a
	// schema comes to describe a type that has moved on.
	SchemaName = "repository-v1.json"

	// SchemaFile is where RenderSchema's output belongs.
	SchemaFile = SchemaDir + "/" + SchemaName

	// SchemaOrigin is the host the document is published from. Fixed rather
	// than read from a deployment's own origin, because the URL is written into
	// other people's repositories and has to resolve from a laptop that has
	// never heard of the installation that wrote it.
	SchemaOrigin = "https://smyklot.com"

	// SchemaPath is where the service serves the document, and the last
	// segments of its $id. One constant, so a schema cannot advertise an
	// address the service does not answer at.
	SchemaPath = "/schema/" + SchemaName

	// SchemaID is where the document says it lives. dotsync's pointed at a file
	// on a branch of another repository, which is how it came to describe types
	// that no longer existed. This one is served by the binary that validates
	// against it.
	SchemaID = SchemaOrigin + SchemaPath

	// SchemaDialect is the JSON Schema version the document declares.
	SchemaDialect = "https://json-schema.org/draft/2020-12/schema"
)

// The JSON Schema type names, spelled once.
const (
	jsonBoolean = "boolean"
	jsonString  = "string"
	jsonArray   = "array"
	jsonObject  = "object"
)

// RenderSchema produces the JSON Schema for a repository's configuration file.
//
// The bytes are canonical and ordered by hand rather than left to a formatter.
// dotsync's generator round-tripped through a map and emitted alphabetically
// while the committed file was in a formatter's order, so regenerating locally
// always showed a diff that was not a change - which is why its schema came to
// be hand-edited and then described types that had moved on.
//
// additionalProperties is false because the strict decoders reject an unknown
// key. The schema and the decoder are two statements of the same thing, and a
// schema that accepted what the code refuses would send somebody to write a
// file that an editor called valid and Smyklot called broken.
func RenderSchema(model Model) ([]byte, error) {
	var buffer bytes.Buffer

	buffer.WriteString("{\n")
	writeHeader(&buffer, "$schema", SchemaDialect)
	writeHeader(&buffer, "$id", SchemaID)
	writeHeader(&buffer, "title", "Smyklot repository configuration")
	writeHeader(&buffer, "description", schemaDescription)
	writeHeader(&buffer, "type", jsonObject)
	writeRaw(&buffer, 1, "additionalProperties", "false", true)

	buffer.WriteString("  \"properties\": {\n")

	for index, field := range model.Fields {
		property, err := renderProperty(field)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", field.Key, err)
		}

		writeRaw(&buffer, 2, field.Key, property, index < len(model.Fields)-1)
	}

	buffer.WriteString("  }\n}\n")

	// Parsing what was just written is cheap and catches a template mistake
	// here rather than in an editor that silently ignores a broken schema.
	if !json.Valid(buffer.Bytes()) {
		return nil, errInvalidSchema
	}

	return buffer.Bytes(), nil
}

const schemaDescription = "Settings a repository may set for Smyklot, in .smyklot.toml, " +
	".smyklot/config.toml, .github/.smyklot.toml or the legacy .github/smyklot.yaml."

// renderProperty renders one setting as a compact one-line schema, so the
// document reads as a table of settings rather than as nested punctuation.
func renderProperty(field Field) (string, error) {
	var parts []string

	add := func(key string, value any) error {
		encoded, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("encode %s: %w", key, err)
		}

		parts = append(parts, strconv.Quote(key)+": "+string(encoded))

		return nil
	}

	if err := add("type", jsonType(field)); err != nil {
		return "", err
	}

	if err := add("description", field.Description); err != nil {
		return "", err
	}

	switch field.Kind {
	case KindStringSlice:
		parts = append(parts, `"items": {"type": "string"}`)

	case KindStringMap:
		parts = append(parts, `"additionalProperties": {"type": "string"}`)

	case KindEnum:
		if err := add("enum", field.Enum); err != nil {
			return "", err
		}
	}

	if err := add("default", defaultValue(field)); err != nil {
		return "", err
	}

	return "{" + strings.Join(parts, ", ") + "}", nil
}

func jsonType(field Field) string {
	switch field.Kind {
	case KindBool:
		return jsonBoolean

	case KindStringSlice:
		return jsonArray

	case KindStringMap:
		return jsonObject

	default:
		return jsonString
	}
}

// defaultValue is the schema's default, in the JSON type the setting takes.
//
// It comes from the same tag Default() is generated from, which is what a test
// asserts: a schema whose defaults are decorative is worse than one with none,
// because a reader trusts it.
func defaultValue(field Field) any {
	switch field.Kind {
	case KindBool:
		return field.Default == "true"

	case KindStringSlice:
		return []string{}

	case KindStringMap:
		return map[string]string{}

	default:
		return field.Default
	}
}

// writeHeader writes one of the document's own top-level strings. Every one of
// them is followed by something, so the comma is not a decision.
func writeHeader(buffer *bytes.Buffer, key, value string) {
	writeRaw(buffer, 1, key, strconv.Quote(value), true)
}

func writeRaw(buffer *bytes.Buffer, depth int, key, value string, more bool) {
	for range depth {
		buffer.WriteString("  ")
	}

	buffer.WriteString(strconv.Quote(key))
	buffer.WriteString(": ")
	buffer.WriteString(value)

	if more {
		buffer.WriteString(",")
	}

	buffer.WriteString("\n")
}
