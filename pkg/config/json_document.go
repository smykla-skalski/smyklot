package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"unicode/utf8"
)

const MaxFileDocumentBytes = 16 << 20

// DecodeJSONObject retains exact numbers and refuses ambiguous object keys.
// Configuration files are editable outside the panel, so last-key-wins decoding
// must not erase an earlier setting before domain validation can inspect it.
func DecodeJSONObject(document []byte) (map[string]any, error) {
	return decodeJSONObjectWithin(document, MaxFileDocumentBytes)
}

func decodeJSONObjectWithin(document []byte, maxBytes int) (map[string]any, error) {
	if maxBytes <= 0 || len(document) > maxBytes || !utf8.Valid(document) {
		return nil, errors.New("configuration JSON exceeds its size limit or contains invalid UTF-8")
	}
	reader := jsonObjectReader{source: document, decoder: json.NewDecoder(bytes.NewReader(document))}
	reader.decoder.UseNumber()
	value, err := reader.value(0)
	if err != nil {
		return nil, err
	}
	if _, err := reader.decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("configuration must contain one JSON object")
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("configuration JSON must be an object")
	}
	if _, err := encodeJSONDocumentWithin(object, maxBytes); err != nil {
		return nil, err
	}
	return object, nil
}

// EncodeJSONDocument applies the same budget to canonical output as input.
// Encoding can expand Unicode separators, so checking raw input alone would
// accept documents that cannot be read back after a merge. These bytes are data,
// never embedded in an HTML script, and need no HTML escaping.
func EncodeJSONDocument(value any) ([]byte, error) {
	return encodeJSONDocumentWithin(value, MaxFileDocumentBytes)
}

func encodeJSONDocumentWithin(value any, maxBytes int) ([]byte, error) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	encoded := bytes.TrimSuffix(output.Bytes(), []byte("\n"))
	if len(encoded) > maxBytes {
		return nil, errors.New("encoded configuration JSON exceeds its size limit")
	}
	return encoded, nil
}

type jsonObjectReader struct {
	source  []byte
	decoder *json.Decoder
	values  int
}

func (reader *jsonObjectReader) token() (json.Token, error) {
	start := reader.decoder.InputOffset()
	token, err := reader.decoder.Token()
	if err != nil {
		return nil, err
	}
	if _, ok := token.(string); ok {
		if err := validJSONSurrogates(reader.source[start:reader.decoder.InputOffset()]); err != nil {
			return nil, err
		}
	}
	if number, ok := token.(json.Number); ok && len(number) > 4096 {
		return nil, errors.New("configuration JSON number is too long")
	}
	return token, nil
}

func (reader *jsonObjectReader) value(depth int) (any, error) {
	reader.values++
	if depth > 128 || reader.values > 250000 {
		return nil, errors.New("configuration JSON is too deeply nested or contains too many values")
	}
	token, err := reader.token()
	if err != nil {
		return nil, err
	}
	switch token {
	case json.Delim('{'):
		return reader.object(depth)
	case json.Delim('['):
		array := []any{}
		for reader.decoder.More() {
			value, err := reader.value(depth + 1)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		_, err := reader.token()
		return array, err
	default:
		return token, nil
	}
}

func (reader *jsonObjectReader) object(depth int) (map[string]any, error) {
	object := make(map[string]any)
	for reader.decoder.More() {
		token, err := reader.token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, errors.New("configuration JSON object key must be a string")
		}
		if _, exists := object[key]; exists {
			return nil, fmt.Errorf("duplicate configuration JSON key %.80q", key)
		}
		value, err := reader.value(depth + 1)
		if err != nil {
			return nil, err
		}
		object[key] = value
	}
	_, err := reader.token()
	return object, err
}

// encoding/json replaces unpaired UTF-16 escapes with U+FFFD. Reject that
// lossy input, while accepting both literal and escaped valid Unicode.
func validJSONSurrogates(raw []byte) error {
	for index := 0; index < len(raw); index++ {
		if raw[index] != '\\' {
			continue
		}
		index++
		if index >= len(raw) || raw[index] != 'u' {
			continue
		}
		if index+4 >= len(raw) {
			return errors.New("incomplete Unicode escape in configuration JSON")
		}
		value, err := strconv.ParseUint(string(raw[index+1:index+5]), 16, 16)
		if err != nil {
			return err
		}
		index += 4
		if value < 0xD800 || value > 0xDFFF {
			continue
		}
		if value > 0xDBFF || index+6 >= len(raw) || raw[index+1] != '\\' || raw[index+2] != 'u' {
			return errors.New("unpaired Unicode surrogate in configuration JSON")
		}
		low, err := strconv.ParseUint(string(raw[index+3:index+7]), 16, 16)
		if err != nil || low < 0xDC00 || low > 0xDFFF {
			return errors.New("unpaired Unicode surrogate in configuration JSON")
		}
		index += 6
	}
	return nil
}
