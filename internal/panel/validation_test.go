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

	// One key at a time. Building a full patch inside this loop asserted the
	// same thing on every pass - that a patch setting everything is refused -
	// which a single denied key is enough to make true. A key that stopped
	// being refused would not have shown up.
	for _, key := range denied {
		if err := validatePatch(patchSetting(t, key)); err == nil {
			t.Errorf("a patch setting only %q was accepted", key)
		}
	}
}

// fullPatch returns a patch that sets every setting, so a test can prove
// something holds for all of them without naming any.
func fullPatch(t *testing.T) config.Patch {
	t.Helper()

	var patch config.Patch

	value := reflect.ValueOf(&patch).Elem()
	for index := range value.NumField() {
		field := value.Field(index)
		field.Set(reflect.New(field.Type().Elem()))
	}

	return patch
}

// patchSetting returns a patch that sets exactly one key, found by the json tag
// the key is addressed by rather than by a name spelled here.
func patchSetting(t *testing.T, key string) config.Patch {
	t.Helper()

	var patch config.Patch

	value := reflect.ValueOf(&patch).Elem()
	fields := value.Type()

	for index := range value.NumField() {
		tag, _, _ := strings.Cut(fields.Field(index).Tag.Get("json"), ",")
		if tag != key {
			continue
		}

		field := value.Field(index)
		field.Set(reflect.New(field.Type().Elem()))

		return patch
	}

	t.Fatalf("no field on config.Patch is addressed by %q", key)

	return patch
}
