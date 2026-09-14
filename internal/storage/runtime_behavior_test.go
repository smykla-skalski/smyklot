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
		`{"runner":"unknown"}`,
		`{"version":1,"overrides":{},"unknown":true}`,
		`{"version":1}`,
		`{"version":1,"overrides":null}`,
		`{"version":1,"overrides":{"quiet_success":null}}`,
		`{"version":1,"overrides":{"allowed_commands":[null]}}`,
		`{"version":1,"overrides":{"command_aliases":{"a":null}}}`,
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

func TestRuntimeBehaviorBeforeFormattingExisted(t *testing.T) {
	var value storage.RuntimeBehavior
	if err := json.Unmarshal([]byte(`{"quiet_success":true}`), &value); err != nil {
		t.Fatal(err)
	}
	deployment := config.Default()
	deployment.QuietPending = true
	resolved := value.Resolve(deployment)
	if !resolved.QuietSuccess || resolved.QuietPending || resolved.CommandPrefix != "" || len(resolved.AllowedCommands) != 0 || len(resolved.CommandAliases) != 0 {
		t.Fatalf("legacy zero values no longer pinned: %+v", resolved)
	}
	if resolved.Formatting != config.DefaultFormattingPolicy() {
		t.Fatal("pre-formatting records must preserve file presentation")
	}
}

func TestRuntimeBehaviorLegacyNullCollectionEntries(t *testing.T) {
	var value storage.RuntimeBehavior
	if err := json.Unmarshal([]byte(`{"allowed_commands":[null,"approve"],"command_aliases":{"a":null,"m":"merge"}}`), &value); err != nil {
		t.Fatal(err)
	}
	resolved := value.Resolve(config.Default())
	if !reflect.DeepEqual(resolved.AllowedCommands, []string{"", "approve"}) || !reflect.DeepEqual(resolved.CommandAliases, map[string]string{"a": "", "m": "merge"}) {
		t.Fatalf("legacy null entries changed: %+v", resolved)
	}
}

func TestRuntimeBehaviorLegacyFieldCase(t *testing.T) {
	var value storage.RuntimeBehavior
	document := `{"QUIET_SUCCESS":true,"quiet_success":null,"COMMAND_PREFIX":"!","ALLOWED_COMMANDS":["merge"],"allowed_commands":["approve"],"COMMAND_ALIASES":{"ship":"merge"},"command_aliases":{"ok":"approve"}}`
	if err := json.Unmarshal([]byte(document), &value); err != nil {
		t.Fatal(err)
	}
	resolved := value.Resolve(config.Default())
	if !resolved.QuietSuccess || resolved.CommandPrefix != "!" || !reflect.DeepEqual(resolved.AllowedCommands, []string{"approve"}) || !reflect.DeepEqual(resolved.CommandAliases, map[string]string{"ship": "merge", "ok": "approve"}) {
		t.Fatalf("legacy field case changed: %+v", resolved)
	}
	for _, invalid := range []string{`{"RUNNER":"unknown"}`, `{"QUIET_SUCCESS":"yes","quiet_success":true}`, `{"version":1,"overrides":{"QUIET_SUCCESS":true}}`, `{"version":1,"overrides":{"formatting":{"common":{"INDENT_WIDTH":4}}}}`} {
		if err := json.Unmarshal([]byte(invalid), &value); err == nil {
			t.Fatalf("invalid document accepted: %s", invalid)
		}
	}
}

func TestRuntimeBehaviorLegacyFormattingFields(t *testing.T) {
	policy, err := json.Marshal(config.DefaultFormattingPolicy())
	if err != nil {
		t.Fatal(err)
	}
	var value storage.RuntimeBehavior
	document := `{"formatting":` + string(policy) + `,"FORMATTING":{"COMMON":{"INDENT_WIDTH":4,"indent_width":null},"unknown":true}}`
	if err := json.Unmarshal([]byte(document), &value); err != nil {
		t.Fatal(err)
	}
	if got := value.Resolve(config.Default()).Formatting.Common.IndentWidth; got != 4 {
		t.Fatalf("got width %d", got)
	}
	if err := json.Unmarshal([]byte(`{"FORMATTING":{"COMMON":{"INDENT_WIDTH":6}}}`), &value); err != nil {
		t.Fatal(err)
	}
	if got := value.Resolve(config.Default()).Formatting.Common.IndentWidth; got != 6 {
		t.Fatalf("got width %d", got)
	}
	for _, document := range []string{`{"formatting":null}`, `{"formatting":{"common":{"indent_width":4}}}`, `{"formatting":` + string(policy) + `,"FORMATTING":{"common":{"indent_width":"bad"}}}`} {
		if err := json.Unmarshal([]byte(document), &value); err == nil {
			t.Fatalf("invalid formatting accepted: %s", document)
		}
	}
}
