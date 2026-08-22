package gate

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/bot"
)

func TestRetuneQuietPeriodReachesReconcilerAndScheduler(t *testing.T) {
	t.Parallel()

	// Given a runtime holding the default quiet period
	reconciler := newReconciler(
		&reconcilerTestStore{}, reconcilerTestObserver{}, &reconcilerTestEffects{},
		bot.NewCoordinator(), defaultTiming(),
	)
	scheduler := newScheduler(
		&schedulerTestStore{}, reconciler,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	gate := &Gate{Reconciler: reconciler, Scheduler: scheduler}

	// When an operator retunes it
	changed := gate.RetuneQuietPeriod(45 * time.Second)

	// Then the reconciler decides with the new value and the scheduler is woken
	if !changed {
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

	// And the same value again is not a change
	if gate.RetuneQuietPeriod(45 * time.Second) {
		t.Fatal("retune reported a change for the value already in force")
	}
}
