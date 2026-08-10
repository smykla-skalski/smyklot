package main

import (
	adminpanel "github.com/smykla-skalski/smyklot/internal/panel"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

// ApplyRuntimeSettings swaps the process defaults as one snapshot and updates
// the shared dynamic log level. Panel clients learn about the committed change
// from the panel WebSocket resync event.
func (s *server) ApplyRuntimeSettings(values adminpanel.RuntimeValues) {
	resolved := config.Resolve(values.BotConfig)
	s.runtimeMu.Lock()
	s.runtimeBotConfig = &resolved.Values
	s.runtimeMu.Unlock()
	s.logLevel.Set(values.LogLevel)
}

func (s *server) botConfig() *config.Config {
	s.runtimeMu.RLock()
	defer s.runtimeMu.RUnlock()
	resolved := config.Resolve(s.runtimeBotConfig)

	return &resolved.Values
}
