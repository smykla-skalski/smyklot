package config_test

import (
	"encoding/json"
	"testing"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

type exactJSONChild struct {
	Enabled bool `json:"enabled"`
}

type exactJSONSettings struct {
	exactJSONChild
	Settings map[string]exactJSONChild `json:"settings"`
	Entries  []exactJSONChild          `json:"entries"`
	Raw      json.RawMessage           `json:"raw"`
	Values   map[string]any            `json:"values"`
}

func TestExactJSONRejectsAliasesAcrossTypedBoundaries(t *testing.T) {
	for _, document := range []string{
		`{"Enabled":false}`, `{"enabled":true,"Enabled":false}`,
		`{"settings":{"repo":{"ENABLED":false}}}`,
		`{"entries":[{"enabled":true,"Enabled":false}]}`,
		`{"enabled":true,"enabled":false}`, `{"unknown":true}`,
		`{"enabled":true} {}`, `{"raw":{"x":1,"x":2}}`,
		`{"enabled":true,"entries":[{"enabled":"no"}]}`,
	} {
		t.Run(document, func(t *testing.T) {
			value := exactJSONSettings{}
			if err := config.DecodeExactJSON([]byte(document), &value); err == nil {
				t.Fatal("accepted an ambiguous or invalid typed document")
			}
			if value.Enabled {
				t.Fatal("failed decoding left a partially updated destination")
			}
		})
	}
}

func TestExactJSONKeepsCaseSensitiveUserKeys(t *testing.T) {
	var value exactJSONSettings
	document := []byte(`{"enabled":true,"settings":{"UPPER":{"enabled":false}},"raw":{"Files":1,"files":2},"values":{"UPPER":9007199254740993,"upper":null}}`)
	if err := config.DecodeExactJSON(document, &value); err != nil {
		t.Fatal(err)
	}
	if !value.Enabled || len(value.Values) != 2 || value.Values["UPPER"] != json.Number("9007199254740993") ||
		string(value.Raw) != `{"Files":1,"files":2}` || len(value.Settings) != 1 {
		t.Fatalf("lost case-sensitive content or numeric precision: %+v", value)
	}
}
