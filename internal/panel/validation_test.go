package panel

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

func TestValidatePatchRejectsRunner(t *testing.T) {
	t.Parallel()

	for _, runner := range []config.Runner{"workflow", config.RunnerService, config.RunnerAction} {
		runner := runner
		err := validatePatch(config.Patch{Runner: &runner})
		if !errors.Is(err, errManagedByRepository) {
			t.Fatalf("validatePatch(%q) error = %v, want %v", runner, err, errManagedByRepository)
		}
	}
}

// The message is what a person reads, and the check is generic now, so it has
// to name the key it refused rather than assume which one that was.
func TestDeniedKeyErrorNamesTheKey(t *testing.T) {
	runner := config.RunnerAction

	err := validatePatch(config.Patch{Runner: &runner})
	if err == nil {
		t.Fatal("validatePatch accepted a runner")
	}

	if !strings.Contains(err.Error(), config.KeyRunner) {
		t.Errorf("error %q does not name the setting it refused", err)
	}
}

// Every key the tags deny must actually be refused, so adding one is enough.
func TestEveryDeniedKeyIsRefused(t *testing.T) {
	denied := config.PanelDeniedKeys()
	if len(denied) == 0 {
		t.Fatal("no key is denied, so this test proves nothing")
	}

	for _, key := range denied {
		full := config.Patch{}
		value := reflect.ValueOf(&full).Elem()

		for index := range value.NumField() {
			field := value.Field(index)
			field.Set(reflect.New(field.Type().Elem()))
		}

		if err := validatePatch(full); err == nil {
			t.Errorf("a patch setting %q was accepted", key)
		}
	}
}
