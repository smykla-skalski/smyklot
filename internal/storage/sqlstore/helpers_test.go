package sqlstore

import (
	"testing"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

// A row a strict decoder would refuse must not take the page with it.
// collectRows abandons a listing on the first row it cannot scan, and
// UpdateRepositorySettings reads the row back inside its own transaction, so a
// refusal here would leave an installation whose repositories will not render
// and cannot be repaired from the panel that exists to repair them.
func TestUnmarshalPatchDoesNotFailThePageOverOneRow(t *testing.T) {
	t.Parallel()

	for _, content := range []string{
		`{"not_a_setting":true}`,
		`{"runner":"workflow"}`,
	} {
		if _, err := unmarshalPatch(content); err != nil {
			t.Errorf("unmarshalPatch(%s) error = %v", content, err)
		}
	}
}

func TestUnmarshalPatchReadsWhatMarshalPatchWrote(t *testing.T) {
	t.Parallel()

	runner := config.RunnerAction
	commands := []string{"approve"}
	patch := config.Patch{Runner: &runner, AllowedCommands: &commands}

	content, err := marshalPatch(patch)
	if err != nil {
		t.Fatalf("marshalPatch() error = %v", err)
	}

	read, err := unmarshalPatch(content)
	if err != nil {
		t.Fatalf("unmarshalPatch(%s) error = %v", content, err)
	}

	if read.Runner == nil || *read.Runner != runner {
		t.Errorf("runner = %v, want %v", read.Runner, runner)
	}
	if read.AllowedCommands == nil || len(*read.AllowedCommands) != 1 {
		t.Errorf("allowed commands = %v", read.AllowedCommands)
	}
}

// The column defaults to an empty object, and a row predating a patch has one.
func TestUnmarshalPatchReadsAnEmptyPatch(t *testing.T) {
	t.Parallel()

	for _, content := range []string{"{}", ""} {
		patch, err := unmarshalPatch(content)
		if err != nil {
			t.Errorf("unmarshalPatch(%q) error = %v", content, err)
		}
		if len(patch.SetKeys()) != 0 {
			t.Errorf("unmarshalPatch(%q) set %v", content, patch.SetKeys())
		}
	}
}
