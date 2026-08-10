package panel

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/logging"
)

// RuntimeValues are the effective settings consumed by the running service.
type RuntimeValues struct {
	BotConfig    *config.Config
	LogLevel     slog.Level
	PollInterval time.Duration
	SessionTTL   time.Duration
}

func resolveRuntimeValues(cfg Config, persisted storage.RuntimeSettings) (RuntimeValues, error) {
	values := RuntimeValues{
		BotConfig:    cloneRuntimeConfig(cfg.ProcessConfig),
		LogLevel:     cfg.LogLevel,
		PollInterval: cfg.PollInterval,
		SessionTTL:   cfg.SessionTTL,
	}
	if persisted.BotConfig != nil {
		values.BotConfig = cloneRuntimeConfig(persisted.BotConfig)
		values.BotConfig.Runner = cfg.ProcessConfig.EffectiveRunner()
	}
	if persisted.LogLevel != nil {
		level, err := logging.ParseLevel(*persisted.LogLevel)
		if err != nil {
			return RuntimeValues{}, err
		}
		values.LogLevel = level
	}
	if persisted.PollInterval != nil {
		values.PollInterval = *persisted.PollInterval
	}
	if persisted.SessionTTL != nil {
		values.SessionTTL = *persisted.SessionTTL
	}
	if values.PollInterval < 0 || values.SessionTTL < time.Minute {
		return RuntimeValues{}, fmt.Errorf("persisted runtime durations are invalid")
	}

	return values, nil
}

func (s *Server) applyRuntimeSettings(settings storage.RuntimeSettings) error {
	values, err := resolveRuntimeValues(s.cfg, settings)
	if err != nil {
		return err
	}
	if s.controller != nil {
		s.controller.ApplyRuntimeSettings(values)
	}
	s.runtimeMu.Lock()
	s.runtime = values
	s.runtimeMu.Unlock()

	return nil
}

func (s *Server) runtimeValues() RuntimeValues {
	s.runtimeMu.RLock()
	defer s.runtimeMu.RUnlock()
	values := s.runtime
	values.BotConfig = cloneRuntimeConfig(values.BotConfig)

	return values
}

func (s *Server) processConfig() *config.Config {
	return s.runtimeValues().BotConfig
}

func (s *Server) sessionTTL() time.Duration {
	return s.runtimeValues().SessionTTL
}

func runtimeLogLevelName(level slog.Level) string {
	return strings.ToLower(level.String())
}

func cloneRuntimeConfig(value *config.Config) *config.Config {
	resolved := config.Resolve(value)

	return &resolved.Values
}
