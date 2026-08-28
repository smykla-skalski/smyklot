package configgen

import (
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
	jsonInteger = "integer"
	jsonArray   = "array"
	jsonObject  = "object"
)

// RenderSchema produces the JSON Schema for a repository's configuration file.
//
// The bytes come from one typed schema model and one encoding/json boundary.
// encoding/json sorts map keys, so the output is deterministic without a
// second formatter whose ordering could disagree with the generator.
//
// additionalProperties is false because the strict decoders reject an unknown
// key. The schema and the decoder are two statements of the same thing, and a
// schema that accepted what the code refuses would send somebody to write a
// file that an editor called valid and Smyklot called broken.
func RenderSchema(model Model) ([]byte, error) {
	properties := make(map[string]schemaProperty, len(model.Fields))
	for _, field := range model.Fields {
		property, err := schemaPropertyFor(field)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", field.Key, err)
		}
		properties[field.Key] = property
	}

	document, err := json.MarshalIndent(schemaDocument{
		Dialect:              SchemaDialect,
		ID:                   SchemaID,
		Title:                "Smyklot repository configuration",
		Description:          schemaDescription,
		Type:                 jsonObject,
		AdditionalProperties: false,
		Properties:           properties,
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode schema: %w", err)
	}

	return append(document, '\n'), nil
}

const schemaDescription = "Settings a repository may set for Smyklot, in .smyklot.toml, " +
	".smyklot/config.toml, .github/.smyklot.toml or the legacy .github/smyklot.yaml."

type schemaDocument struct {
	Dialect              string                    `json:"$schema"`
	ID                   string                    `json:"$id"`
	Title                string                    `json:"title"`
	Description          string                    `json:"description"`
	Type                 string                    `json:"type"`
	AdditionalProperties bool                      `json:"additionalProperties"`
	Properties           map[string]schemaProperty `json:"properties"`
}

type schemaProperty struct {
	Type                 string                    `json:"type"`
	Description          string                    `json:"description,omitempty"`
	Enum                 []string                  `json:"enum,omitempty"`
	Items                *schemaProperty           `json:"items,omitempty"`
	AdditionalProperties any                       `json:"additionalProperties,omitempty"`
	Minimum              *int                      `json:"minimum,omitempty"`
	Maximum              *int                      `json:"maximum,omitempty"`
	Properties           map[string]schemaProperty `json:"properties,omitempty"`
	Default              *any                      `json:"default,omitempty"`
}

func schemaPropertyFor(field Field) (schemaProperty, error) {
	defaultValue := defaultValue(field)
	property := schemaProperty{
		Type:        jsonType(field),
		Description: field.Description,
		Default:     &defaultValue,
	}
	switch field.Kind {
	case KindStringSlice:
		property.Items = &schemaProperty{Type: jsonString}

	case KindStringMap:
		property.AdditionalProperties = schemaProperty{Type: jsonString}

	case KindEnum:
		property.Enum = field.Enum

	case KindInt:
		var err error
		if field.Min != "" {
			property.Minimum, err = schemaInteger(field.Min)
			if err != nil {
				return schemaProperty{}, fmt.Errorf("minimum: %w", err)
			}
		}
		if field.Max != "" {
			property.Maximum, err = schemaInteger(field.Max)
			if err != nil {
				return schemaProperty{}, fmt.Errorf("maximum: %w", err)
			}
		}

	case KindObject:
		property.Type = jsonObject
		property.AdditionalProperties = false
		property.Properties = make(map[string]schemaProperty, len(field.Children))
		for _, child := range field.Children {
			childProperty, err := schemaPropertyFor(child)
			if err != nil {
				return schemaProperty{}, fmt.Errorf("%s: %w", child.Key, err)
			}
			property.Properties[localKey(child.Key)] = childProperty
		}
		objectDefault := any(objectDefault(field))
		property.Default = &objectDefault
	}

	return property, nil
}

func schemaInteger(value string) (*int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return nil, err
	}

	return &parsed, nil
}

func localKey(key string) string {
	if at := strings.LastIndexByte(key, '.'); at >= 0 {
		return key[at+1:]
	}

	return key
}

func objectDefault(field Field) map[string]any {
	result := make(map[string]any, len(field.Children))
	for _, child := range field.Children {
		if child.Kind == KindObject {
			result[localKey(child.Key)] = objectDefault(child)
			continue
		}

		result[localKey(child.Key)] = defaultValue(child)
	}

	return result
}

func jsonType(field Field) string {
	switch field.Kind {
	case KindBool:
		return jsonBoolean

	case KindInt:
		return jsonInteger

	case KindStringSlice:
		return jsonArray

	case KindStringMap:
		return jsonObject

	case KindObject:
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

	case KindInt:
		value, _ := strconv.Atoi(field.Default)

		return value

	case KindStringSlice:
		return []string{}

	case KindStringMap:
		return map[string]string{}

	default:
		return field.Default
	}
}
