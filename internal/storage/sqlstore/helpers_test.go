package sqlstore

import (
	"errors"
	"strings"
	"testing"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

// A stored patch reaches Resolve without passing anything else, so this is the
// last place a value nobody vouched for can be turned away. It used to be a
// bare json.Unmarshal.
func TestUnmarshalPatchRefusesWhatItCannotVouchFor(t *testing.T) {
	t.Parallel()

	// Both entry points compare the runner to their own name, so a third value
	// takes both of them out and the repository goes silent with nothing
	// anywhere to say why.
	t.Run("a runner naming no entry point", func(t *testing.T) {
		t.Parallel()

		if _, err := unmarshalPatch(`{"runner":"workflow"}`); !errors.Is(
			err, config.ErrUnknownRunner,
		) {
			t.Errorf("unmarshalPatch() error = %v, want %v", err, config.ErrUnknownRunner)
		}
	})

	t.Run("a setting that does not exist", func(t *testing.T) {
		t.Parallel()

		_, err := unmarshalPatch(`{"not_a_setting":true}`)
		if err == nil {
			t.Fatal("unmarshalPatch() accepted a setting that does not exist")
		}
		if !strings.Contains(err.Error(), "not_a_setting") {
			t.Errorf("error %q does not name the offending key", err)
		}
	})
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
