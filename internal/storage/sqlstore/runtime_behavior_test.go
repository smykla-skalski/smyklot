package sqlstore

import (
	"encoding/json"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

func TestRuntimeFormattingInvalidationTracksOwnership(t *testing.T) {
	decode := func(content string) *storage.RuntimeBehavior {
		t.Helper()
		var value storage.RuntimeBehavior
		if err := json.Unmarshal([]byte(content), &value); err != nil {
			t.Fatal(err)
		}
		return &value
	}
	explicit := decode(`{"version":1,"overrides":{"formatting":{"common":{"line_ending":"preserve"}}}}`)
	equal := decode(`{"version":1,"overrides":{"formatting":{"common":{"line_ending":"preserve"}},"quiet_success":false}}`)
	unrelated := decode(`{"version":1,"overrides":{"quiet_success":false}}`)
	for _, test := range []struct {
		name          string
		before, after *storage.RuntimeBehavior
		changed       bool
	}{
		{"start pinning a default", nil, explicit, true},
		{"resume inheriting", explicit, nil, true},
		{"equal formatting with different behavior", explicit, equal, false},
		{"unrelated behavior", nil, unrelated, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := runtimeFormattingChanged(test.before, test.after); got != test.changed {
				t.Fatalf("formatting invalidation = %v, want %v", got, test.changed)
			}
		})
	}
}
