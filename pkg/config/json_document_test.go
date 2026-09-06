package config_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

func TestConfigurationJSONRejectsAmbiguityAndLoss(t *testing.T) {
	cases := []string{
		`{"override":1,"override":2}`,
		`{"nested":[{"key":false,"key":true}]}`,
		`{"key":1,"\u006bey":2}`,
		`{"content":"\ud800"}`, `{"\udfff":1}`, `{"content":"\ud800\u0041"}`,
		"{\"content\":\"\xff\"}",
		`{"x":1} {"x":2}`, `{"x":]}`, `[]`, `null`, ``,
		`{"x":` + strings.Repeat("[", 130) + `0` + strings.Repeat("]", 130) + `}`,
		`{"x":1e` + strings.Repeat("9", 4096) + `}`,
	}
	for _, input := range cases {
		if _, err := config.DecodeJSONObject([]byte(input)); err == nil {
			t.Errorf("accepted ambiguous or lossy JSON: %.120q", input)
		}
	}
}

func TestConfigurationJSONPreservesValues(t *testing.T) {
	cases := map[string]string{
		`{"text":"\ud83d\ude00"}`: "😀",
		`{"text":"😀"}`:            "😀",
		`{"text":"\ufffd"}`:       "�",
		`{"text":"\\ud800"}`:      `\ud800`,
		`{"text":"\\\\ud800"}`:    `\\ud800`,
		`{"text":"é"}`:            "é",
	}
	for input, expected := range cases {
		object, err := config.DecodeJSONObject([]byte(input))
		if err != nil || object["text"] != expected {
			t.Errorf("changed valid text %s: %#v (%v)", input, object, err)
		}
	}
	object, err := config.DecodeJSONObject([]byte(`{"number":9007199254740993,"delete":null,"items":[],"empty":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	if object["number"] != json.Number("9007199254740993") {
		t.Fatalf("rounded number: %v", object)
	}
	if _, exists := object["delete"]; !exists {
		t.Fatal("lost null key")
	}
	if object["delete"] != nil || len(object["items"].([]any)) != 0 || len(object["empty"].(map[string]any)) != 0 {
		t.Fatalf("changed empty values: %v", object)
	}
}

func FuzzConfigurationJSONRoundTrip(f *testing.F) {
	for _, seed := range []string{`{}`, `{"x":null}`, `{"x":1e999}`, `{"x":"\ud800"}`, `{"a":1,"a":2}`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, document string) {
		parsed, err := config.DecodeJSONObject([]byte(document))
		if err != nil {
			return
		}
		encoded, err := config.EncodeJSONDocument(parsed)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := config.DecodeJSONObject(encoded); err != nil {
			t.Fatalf("accepted a value that cannot round trip: %v", err)
		}
	})
}

func TestConfigurationJSONEncodedBudgetIsStable(t *testing.T) {
	for _, character := range []string{"<", "\u2028", "\u2029"} {
		source := `{"text":"` + strings.Repeat(character, 3<<20) + `"}`
		parsed, err := config.DecodeJSONObject([]byte(source))
		if err != nil {
			if character == "<" {
				t.Fatalf("HTML text should fit without escaping: %v", err)
			}
			continue
		}
		encoded, err := config.EncodeJSONDocument(parsed)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := config.DecodeJSONObject(encoded); err != nil {
			t.Fatalf("accepted a value too large to read back: %v", err)
		}
	}
}
