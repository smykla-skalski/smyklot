package storage_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func TestRuntimeBehaviorPreservesOwnership(t *testing.T) {
	var value storage.RuntimeBehavior
	if err := json.Unmarshal([]byte(`{"version":1,"overrides":{"quiet_success":false,"allowed_commands":[],"command_aliases":{},"command_prefix":""}}`), &value); err != nil {
		t.Fatal(err)
	}
	deployment := config.Default()
	deployment.QuietSuccess = true
	deployment.QuietPending = true
	deployment.CommandPrefix = "!"
	deployment.AllowedCommands = []string{"approve"}
	deployment.CommandAliases = map[string]string{"a": "approve"}
	resolved := value.Resolve(deployment)
	if resolved.QuietSuccess || !resolved.QuietPending || resolved.CommandPrefix != "" || len(resolved.AllowedCommands) != 0 || len(resolved.CommandAliases) != 0 {
		t.Fatalf("ownership lost: %+v", resolved)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded storage.RuntimeBehavior
	if err := json.Unmarshal(encoded, &reloaded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reloaded.Resolve(deployment), resolved) {
		t.Fatal("round trip changed behavior")
	}
	if !deployment.QuietSuccess || deployment.CommandPrefix != "!" {
		t.Fatal("mutated deployment")
	}
}

func TestRuntimeBehaviorCopiesPatches(t *testing.T) {
	enabled := true
	aliases := map[string]string{"a": "approve"}
	value, err := storage.NewRuntimeBehavior(config.Patch{QuietSuccess: &enabled, CommandAliases: &aliases})
	if err != nil {
		t.Fatal(err)
	}
	enabled = false
	aliases["a"] = "merge"
	patch := value.Patch()
	*patch.QuietSuccess = false
	(*patch.CommandAliases)["a"] = "cleanup"
	resolved := value.Resolve(config.Default())
	if !resolved.QuietSuccess || resolved.CommandAliases["a"] != "approve" {
		t.Fatal("patch aliases escaped")
	}
}

func TestRuntimeBehaviorLegacyPinsCompleteConfig(t *testing.T) {
	legacy := config.Default()
	legacy.QuietSuccess = true
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	var value storage.RuntimeBehavior
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	deployment := config.Default()
	deployment.QuietPending = !legacy.QuietPending
	deployment.Runner = config.RunnerAction
	expected := *legacy
	expected.Runner = deployment.Runner
	if !reflect.DeepEqual(value.Resolve(deployment), &expected) {
		t.Fatal("legacy behavior changed")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"version":1`) {
		t.Fatal("legacy record not upgraded")
	}
}

func TestRuntimeBehaviorRejectsAmbiguousOverrides(t *testing.T) {
	for _, document := range []string{
		`{"version":2,"overrides":{}}`,
		`{"version":1,"overrides":{},"unknown":true}`,
		`{"version":1}`,
		`{"version":1,"overrides":null}`,
		`{"version":1,"overrides":{"quiet_success":null}}`,
		`{"version":1,"overrides":{"quiet_success":false,"quiet_success":true}}`,
		`{"version":1,"overrides":{"runner":"action"}}`,
		`{"version":1,"overrides":{"unknown":true}}`,
		`{"version":1,"overrides":{"quiet_success":"yes"}}`,
		`{"version":1,"overrides":{"formatting":{"common":{"line_width":null}}}}`,
	} {
		t.Run(document, func(t *testing.T) {
			var value storage.RuntimeBehavior
			if err := json.Unmarshal([]byte(document), &value); err == nil {
				t.Fatal("invalid override accepted")
			}
		})
	}
}
