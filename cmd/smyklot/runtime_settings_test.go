package main

import (
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/bot"
	adminpanel "github.com/smykla-skalski/smyklot/internal/panel"
	"github.com/smykla-skalski/smyklot/internal/pendingci/gate"
	"github.com/smykla-skalski/smyklot/internal/storage/open"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func TestApplyRuntimeSettingsUpdatesAndWakesPendingCI(t *testing.T) {
	t.Parallel()
	store, err := open.Store(t.Context(), filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	}()

	level := &slog.LevelVar{}
	service := &server{
		logLevel: level, runtimeBotConfig: config.Default(),
		pollIntervalChanged: make(chan struct{}, 1),
		gate: gate.New(gate.Dependencies{
			Store: store, Gates: store, Checks: store, Transitions: store,
			Leases: store, Handoffs: store, Current: store,
			Coordinator: bot.NewCoordinator(),
			Logger:      slog.New(slog.DiscardHandler),
		}),
	}

	service.ApplyRuntimeSettings(adminpanel.RuntimeValues{
		BotConfig: config.Default(), LogLevel: slog.LevelDebug,
		PollInterval: time.Minute, PendingCIQuietPeriod: 45 * time.Second,
		SessionTTL: time.Hour,
	})

	if got := service.gate.PassingQuiet(); got != 45*time.Second {
		t.Fatalf("pending-CI quiet period = %s, want 45s", got)
	}
	if level.Level() != slog.LevelDebug {
		t.Fatalf("log level = %s, want debug", level.Level())
	}
	select {
	case <-service.pollIntervalChanged:
	default:
		t.Fatal("the poll loop was not told the interval changed")
	}
}
