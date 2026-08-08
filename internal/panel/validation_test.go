package panel

import (
	"errors"
	"testing"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

func TestValidatePatchRejectsRunner(t *testing.T) {
	t.Parallel()

	for _, runner := range []config.Runner{"workflow", config.RunnerService, config.RunnerAction} {
		runner := runner
		err := validatePatch(config.Patch{Runner: &runner})
		if !errors.Is(err, errRunnerManagedByRepository) {
			t.Fatalf("validatePatch(%q) error = %v, want %v", runner, err, errRunnerManagedByRepository)
		}
	}
}
