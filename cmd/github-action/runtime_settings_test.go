package main

import (
	"io"
	"log/slog"
	"testing"
	"time"

	adminpanel "github.com/smykla-skalski/smyklot/internal/panel"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func TestApplyRuntimeSettingsUpdatesAndWakesPendingCI(t *testing.T) {
	t.Parallel()
	timing := defaultPendingCITiming()
	reconciler := newPendingCIReconciler(
		&reconcilerTestStore{}, reconcilerTestObserver{}, &reconcilerTestEffects{},
		newPendingCICoordinator(), timing,
	)
	scheduler := newPendingCIScheduler(
		&schedulerTestStore{}, reconciler,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	level := &slog.LevelVar{}
	service := &server{
		logLevel: level, runtimeBotConfig: config.Default(),
		pollIntervalChanged: make(chan struct{}, 1),
		pendingCIReconciler: reconciler, pendingCI: scheduler,
	}

	service.ApplyRuntimeSettings(adminpanel.RuntimeValues{
		BotConfig: config.Default(), LogLevel: slog.LevelDebug,
		PollInterval: time.Minute, PendingCIQuietPeriod: 45 * time.Second,
		SessionTTL: time.Hour,
	})

	if got := reconciler.currentTiming().PassingQuiet; got != 45*time.Second {
		t.Fatalf("pending-CI quiet period = %s, want 45s", got)
	}
	if level.Level() != slog.LevelDebug {
		t.Fatalf("log level = %s, want debug", level.Level())
	}
	select {
	case <-scheduler.wake:
	default:
		t.Fatal("pending-CI scheduler was not woken after a quiet-period change")
	}
	scheduler.retuneMu.Lock()
	retune := scheduler.retune
	scheduler.retuneMu.Unlock()
	if retune == nil || retune.PassingQuiet != 45*time.Second {
		t.Fatalf("pending-CI retune = %#v, want 45s", retune)
	}
}
