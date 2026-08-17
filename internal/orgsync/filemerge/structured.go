package filemerge

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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

// parseJSON reads a JSON file into a document.
//
// A file whose top level is not an object is refused. Merging overrides into a
// list or a scalar has no meaning, and answering with an empty document instead
// would write the overrides alone over whatever was there.
//
// JSON only. YAML is merged node by node, because decoding it into Go values
// and encoding those back does not write the file that was read - see
// mergeYAML. JSON survives the trip: its numbers are kept as the digits
// somebody typed, and it has no comments to lose. What it does not keep is key
// order, which is a diff nobody asked for and not a value nobody wrote.
func parseJSON(data []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))

	// Numbers as they were written, rather than as float64. A repository or app
	// identifier past 2^53 comes back from float64 as a different number, and
	// the file this writes is the one the repository keeps.
	decoder.UseNumber()

	var document map[string]any

	err := decoder.Decode(&document)
	if err == nil && decoder.More() {
		// A decoder reads one value and stops, so a file holding two would be
		// merged as its first and written back without the rest. That is the
		// same silence a second YAML document was dropped in before this, and
		// the file it produces is one nobody wrote.
		err = errTrailingContent
	}

	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}

	if document == nil {
		return nil, fmt.Errorf("%w: it holds no object", ErrUnreadable)
	}

	return document, nil
}

// renderJSON writes a document back out.
func renderJSON(document map[string]any) ([]byte, error) {
	var buffer bytes.Buffer

	encoder := json.NewEncoder(&buffer)

	// Unescaped, because these files hold regular expressions: escaping would
	// turn a renovate matchStrings pattern into one that no longer matches what
	// it was written for.
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(document); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnwritable, err)
	}

	// Encode already ends the document with a newline, which is what a file
	// should end with.
	return buffer.Bytes(), nil
}
