package main

import (
	"time"

	adminpanel "github.com/smykla-skalski/smyklot/internal/panel"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

// ApplyRuntimeSettings swaps the process defaults as one snapshot and updates
// the shared dynamic log level. Panel clients learn about the committed change
// from the panel WebSocket resync event.
func (s *server) ApplyRuntimeSettings(values adminpanel.RuntimeValues) {
	resolved := config.Resolve(values.BotConfig)
	s.runtimeMu.Lock()
	backgroundWorkPauseChanged :=
		s.runtimeBackgroundWorkPaused != values.BackgroundWorkPaused
	pollIntervalChanged := s.runtimePollInterval != values.PollInterval
	pathIndexIntervalChanged := s.runtimePathIndexInterval != values.PathIndexInterval
	s.runtimeBotConfig = &resolved.Values
	s.runtimeBackgroundWorkPaused = values.BackgroundWorkPaused
	s.runtimePollInterval = values.PollInterval
	s.runtimePathIndexInterval = values.PathIndexInterval
	s.runtimeMu.Unlock()
	s.logLevel.Set(values.LogLevel)
	s.gate.RetuneQuietPeriod(values.PendingCIQuietPeriod)
	if pollIntervalChanged {
		select {
		case s.pollIntervalChanged <- struct{}{}:
		default:
		}
	}
	if pathIndexIntervalChanged {
		s.WakeQueue(workqueue.LaneMaintenance)
	}
	if backgroundWorkPauseChanged {
		for _, lane := range []workqueue.Lane{
			workqueue.LaneWebhook, workqueue.LanePendingCI, workqueue.LaneMaintenance,
		} {
			s.WakeQueue(lane)
		}
	}
}

func (s *server) backgroundWorkPaused() bool {
	s.runtimeMu.RLock()
	defer s.runtimeMu.RUnlock()

	return s.runtimeBackgroundWorkPaused
}

// beginBackgroundWork makes lease acquisition mutually exclusive with pausing.
// The caller releases the read lock as soon as the durable claim is complete;
// after that the item is already in flight and may finish while paused.
func (s *server) beginBackgroundWork() (func(), bool) {
	s.runtimeMu.RLock()
	if s.runtimeBackgroundWorkPaused {
		s.runtimeMu.RUnlock()

		return nil, false
	}

	return s.runtimeMu.RUnlock, true
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

// pathIndexInterval is the process's answer, which an installation or one of
// its repositories may override.
func (s *server) pathIndexInterval() time.Duration {
	s.runtimeMu.RLock()
	defer s.runtimeMu.RUnlock()

	return s.runtimePathIndexInterval
}
