package main

import (
	"time"

	adminpanel "github.com/smykla-skalski/smyklot/internal/panel"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

// ApplyRuntimeSettings swaps the process defaults as one snapshot and updates
// the shared dynamic log level. Panel clients learn about the committed change
// from the panel WebSocket resync event.
func (s *server) ApplyRuntimeSettings(values adminpanel.RuntimeValues) {
	resolved := config.Resolve(values.BotConfig)
	s.runtimeMu.Lock()
	pollIntervalChanged := s.runtimePollInterval != values.PollInterval
	s.runtimeBotConfig = &resolved.Values
	s.runtimePollInterval = values.PollInterval
	s.runtimeMu.Unlock()
	s.logLevel.Set(values.LogLevel)
	if pollIntervalChanged {
		select {
		case s.pollIntervalChanged <- struct{}{}:
		default:
		}
	}
}

func (s *server) botConfig() *config.Config {
	s.runtimeMu.RLock()
	defer s.runtimeMu.RUnlock()
	resolved := config.Resolve(s.runtimeBotConfig)

	return &resolved.Values
}

func (s *server) pollInterval() time.Duration {
	s.runtimeMu.RLock()
	defer s.runtimeMu.RUnlock()

	return s.runtimePollInterval
}
