package panel

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

// A refresh interval is range-checked as seconds, before it is multiplied.
//
// A Duration is int64 nanoseconds, so 18446744074 seconds multiplied first
// comes to exactly 0 - which a validator reading the product then accepts, and
// 0 is "check every sweep", the most expensive answer there is, reached by
// typing a number nobody meant.
//
// Asked of `pathIndexOverride` because that is what every handler calls: the
// field rides on a request struct four of them decode, so a check that is not
// on the path all four take is a check a fifth handler will not have.
func TestPathIndexDurationRefusesWhatOverflows(t *testing.T) {
	t.Parallel()

	longest := int64(MaxPathIndexInterval / time.Second)

	for _, one := range []struct {
		name    string
		seconds int64
		refused bool
	}{
		{name: "an hour", seconds: 3600},
		{name: "zero, which is every sweep", seconds: 0},
		{name: "the longest it may be", seconds: longest},
		{name: "one second past that", seconds: longest + 1, refused: true},
		{name: "negative", seconds: -1, refused: true},
		{name: "a count that multiplies to zero", seconds: 18446744074, refused: true},
		{name: "the largest there is", seconds: math.MaxInt64, refused: true},
	} {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()

			seconds := one.seconds
			_, err := pathIndexOverride(nil, nullableValue[int64]{
				Present: true, Value: &seconds,
			})

			if one.refused && err == nil {
				t.Fatalf("%d seconds was accepted", one.seconds)
			}

			if !one.refused && err != nil {
				t.Fatalf("%d seconds was refused: %v", one.seconds, err)
			}
		})
	}
}

// A request that does not mention the interval leaves the stored one alone,
// which is what carries a workspace's setting through a save of something
// else on the same page.
func TestPathIndexOverrideKeepsWhatIsStored(t *testing.T) {
	t.Parallel()

	stored := 42 * time.Minute
	kept, err := pathIndexOverride(&stored, nullableValue[int64]{})
	if err != nil {
		t.Fatalf("carrying the stored value through: %v", err)
	}
	if kept == nil || *kept != stored {
		t.Fatalf("kept = %v, wanted %s", kept, stored)
	}

	// And an explicit null is inheriting, which is a value rather than an
	// absence: it takes the override off.
	cleared, err := pathIndexOverride(&stored, nullableValue[int64]{Present: true})
	if err != nil {
		t.Fatalf("clearing the override: %v", err)
	}
	if cleared != nil {
		t.Fatalf("cleared = %v, wanted nothing", *cleared)
	}
}

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

	fillSparseStruct(reflect.ValueOf(&patch).Elem())

	return patch
}

func fillSparseStruct(value reflect.Value) {
	for index := range value.NumField() {
		field := value.Field(index)
		field.Set(reflect.New(field.Type().Elem()))
		if field.Elem().Kind() == reflect.Struct {
			fillSparseStruct(field.Elem())
		}
	}
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
