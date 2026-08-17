package filemerge

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"

	yaml "go.yaml.in/yaml/v3"
)

// Format is how a structured file is written down.
type Format string

const (
	FormatJSON Format = "json"
	FormatYAML Format = "yaml"
)

// yamlIndent matches what the templates in the organization are written with,
// and what go-yaml would otherwise pick for itself, which is four.
const yamlIndent = 2

// errTrailingContent is a file holding more than the one document this reads.
var errTrailingContent = errors.New("it holds more than one document")

// parseDocument reads a structured file into a document.
//
// A file whose top level is not an object is refused. Merging overrides into a
// list or a scalar has no meaning, and answering with an empty document instead
// would write the overrides alone over whatever was there.
func parseDocument(format Format, data []byte) (map[string]any, error) {
	var (
		document map[string]any
		err      error
	)

	switch format {
	case FormatJSON:
		decoder := json.NewDecoder(bytes.NewReader(data))

		// Numbers as they were written, rather than as float64. A repository or
		// app identifier past 2^53 comes back from float64 as a different
		// number, and the file this writes is the one the repository keeps.
		decoder.UseNumber()

		if err = decoder.Decode(&document); err == nil && decoder.More() {
			// A decoder reads one value and stops, so a file holding two would
			// be merged as its first and written back without the rest. That
			// is the same silence a second YAML document was dropped in before
			// this, and the file it produces is one nobody wrote.
			err = errTrailingContent
		}

	case FormatYAML:
		err = decodeYAML(data, &document)

	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedFormat, format)
	}

	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}

	if document == nil {
		return nil, fmt.Errorf("%w: it holds no object", ErrUnreadable)
	}

	return document, nil
}

// decodeYAML reads the one document a file should hold.
//
// go-yaml's Unmarshal takes the first document and says nothing about the rest,
// so a file with a --- in it would be merged as its first half and written back
// without the second. That is the silence a repository's settings were dropped
// in before the configuration was reworked, arriving here through a different
// door.
func decodeYAML(data []byte, document *map[string]any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))

	if err := decoder.Decode(document); err != nil {
		if errors.Is(err, io.EOF) {
			// No document at all, which the caller reports as holding no
			// object rather than as unreadable punctuation.
			return nil
		}

		return err
	}

	var second any

	switch err := decoder.Decode(&second); {
	case errors.Is(err, io.EOF):
		return nil

	case err != nil:
		return err

	default:
		return errTrailingContent
	}
}

// renderDocument writes a document back out.
func renderDocument(format Format, document map[string]any) ([]byte, error) {
	switch format {
	case FormatJSON:
		var buffer bytes.Buffer

		encoder := json.NewEncoder(&buffer)

		// Unescaped, because these files hold regular expressions: escaping
		// would turn a renovate matchStrings pattern into one that no longer
		// matches what it was written for.
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")

		if err := encoder.Encode(document); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrUnwritable, err)
		}

		// Encode already ends the document with a newline, which is what a file
		// should end with.
		return buffer.Bytes(), nil

	case FormatYAML:
		var buffer bytes.Buffer

		encoder := yaml.NewEncoder(&buffer)
		encoder.SetIndent(yamlIndent)

		if err := encoder.Encode(asYAML(document)); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrUnwritable, err)
		}

		if err := encoder.Close(); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrUnwritable, err)
		}

		return buffer.Bytes(), nil

	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedFormat, format)
	}
}

// asYAML puts a document into the types go-yaml writes as numbers.
//
// Overrides are stored as JSON whatever the file they patch is written in, so
// they arrive holding json.Number - which is a string type, and which go-yaml
// would therefore quote. A schedule of 4 would be written into somebody's
// workflow as "4".
func asYAML(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		converted := make(map[string]any, len(typed))
		for key, nested := range typed {
			converted[key] = asYAML(nested)
		}

		return converted

	case []any:
		converted := make([]any, len(typed))
		for index, nested := range typed {
			converted[index] = asYAML(nested)
		}

		return converted

	case json.Number:
		return asYAMLNumber(typed)

	default:
		return value
	}
}

// asYAMLNumber reads a JSON number as the Go type go-yaml writes it back as.
//
// An integer too large for int64 comes out as a quoted string, and that is the
// least bad of the three answers available: converting it to float64 would
// write a different number into somebody's file, refusing it would refuse a
// value JSON allows, and a quoted literal keeps every digit and says on the
// face of it that something unusual happened.
func asYAMLNumber(value json.Number) any {
	if integer, ok := new(big.Int).SetString(value.String(), 10); ok {
		if integer.IsInt64() {
			return integer.Int64()
		}

		return value.String()
	}

	if float, err := value.Float64(); err == nil {
		return float
	}

	return value.String()
}
