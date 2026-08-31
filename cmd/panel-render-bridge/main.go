// Command panel-render-bridge serves the panel dev mock over JSON lines.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/smykla-skalski/smyklot/internal/panelrenderbridge"
)

const maxMessageBytes = 4 << 20

func main() {
	if err := run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "panel-render-bridge:", err)
		os.Exit(1)
	}
}

func run(input io.Reader, output io.Writer) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), maxMessageBytes)
	encoder := json.NewEncoder(output)
	for scanner.Scan() {
		request, err := decodeRequest(scanner.Bytes())
		if err != nil {
			var envelope struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(scanner.Bytes(), &envelope)
			if encodeErr := encoder.Encode(panelrenderbridge.InvalidRequest(envelope.ID, err.Error())); encodeErr != nil {
				return fmt.Errorf("encode invalid response: %w", encodeErr)
			}
			continue
		}
		if err := encoder.Encode(panelrenderbridge.Render(request)); err != nil {
			return fmt.Errorf("encode response: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read request: %w", err)
	}
	return nil
}

func decodeRequest(content []byte) (panelrenderbridge.Request, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var request panelrenderbridge.Request
	if err := decoder.Decode(&request); err != nil {
		return request, fmt.Errorf("decode request: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return request, errors.New("decode request: trailing JSON value")
	}
	return request, nil
}
