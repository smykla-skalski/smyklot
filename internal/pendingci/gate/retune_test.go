package gate

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/bot"
)

// The quiet period is the one runtime setting an operator can change while the
// process is running, and it has to reach two places: the reconciler decides
// with it, and the scheduler has to be told to look again rather than sleep out
// its tick with the old value.
func TestRetuneQuietPeriodReachesReconcilerAndScheduler(t *testing.T) {
	t.Parallel()
	reconciler := newReconciler(
		&reconcilerTestStore{}, reconcilerTestObserver{}, &reconcilerTestEffects{},
		bot.NewCoordinator(), defaultTiming(),
	)
	scheduler := newScheduler(
		&schedulerTestStore{}, reconciler,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	gate := &Gate{Reconciler: reconciler, Scheduler: scheduler}

	if !gate.RetuneQuietPeriod(45 * time.Second) {
		t.Fatal("retune reported no change")
	}
	if got := gate.PassingQuiet(); got != 45*time.Second {
		t.Fatalf("quiet period = %s, want 45s", got)
	}
	select {
	case <-scheduler.wake:
	default:
		t.Fatal("scheduler was not woken after a quiet-period change")
	}

	// The same value twice is not a change, and waking on it would turn a
	// panel that saves without editing into a wake per save.
	if gate.RetuneQuietPeriod(45 * time.Second) {
		t.Fatal("retune reported a change for the value already in force")
	}
}
