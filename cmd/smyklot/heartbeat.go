package main

import (
	"context"
	"io"
	"net/http"
	"time"
)

// heartbeatTimeout bounds one ping.
//
// Short on purpose. The ping runs between maintenance ticks, so a provider
// that has stopped answering must not be able to hold the lane that answers
// reactions.
const heartbeatTimeout = 10 * time.Second

// pingHeartbeat tells an external monitor that the maintenance lane turned.
//
// This is liveness, not correctness: the alert fires when nothing arrives
// inside the monitor's grace period, so a tick that found no work, or one
// taken while background work is paused, still has to ping. What a sweep
// actually did is recorded in the metrics and the log beside it.
//
// It hangs off tryDispatchMaintenanceQueue rather than runSweep because
// runSweep only runs where there is no panel. A deployment with a panel takes
// the durable queue instead, so a heartbeat there would never have fired - and
// a heartbeat that is never sent reads as an outage.
//
// A failed ping is logged and nothing else. The monitor's grace period is what
// tolerates one, and a service that stopped answering comments because its
// heartbeat provider was down would be the fault this exists to find.
func (s *server) pingHeartbeat(ctx context.Context) {
	if s.cfg.heartbeatURL == "" {
		return
	}

	// A cancelled context is a shutdown in progress, not a missed beat. Left
	// unchecked, every stop logged a heartbeat failure on its way out
	if ctx.Err() != nil {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, heartbeatTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.heartbeatURL, nil)
	if err != nil {
		s.logger.Warn("could not build the heartbeat request", "error", err)

		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.logger.Warn("could not send the heartbeat", "error", err)

		return
	}
	defer func() {
		// Drained so the connection is reusable rather than torn down every
		// tick for a body nothing reads
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= http.StatusBadRequest {
		s.logger.Warn("heartbeat refused", "status", resp.StatusCode)

		return
	}

	s.logger.Debug("heartbeat sent", "status", resp.StatusCode)
}
