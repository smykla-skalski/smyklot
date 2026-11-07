package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"
)

// heartbeatTimeout bounds one ping.
//
// Short on purpose. The ping runs between maintenance ticks, so a provider
// that has stopped answering must not be able to hold the lane that answers
// reactions.
const heartbeatTimeout = 10 * time.Second

// defaultHeartbeatInterval is how often the lane beats.
//
// Independent of the poll interval on purpose. The beat answers "is this loop
// still turning", which stays a question worth asking when reaction polling is
// switched off and when background work is paused. Set the monitor's grace
// period to a few multiples of this.
const defaultHeartbeatInterval = time.Minute

// heartbeatBeats returns the channel pollLoop beats on and a function to stop
// it.
//
// With no heartbeat configured the channel is nil, and a receive on a nil
// channel blocks for ever - so the select case is inert rather than something
// every arm of the loop has to ask about.
func (s *server) heartbeatBeats() (<-chan time.Time, func()) {
	if s.cfg.heartbeatURL == "" {
		return nil, func() {}
	}

	ticker := time.NewTicker(s.heartbeatInterval)

	return ticker.C, ticker.Stop
}

// heartbeatClient follows no redirect.
//
// The token is in the URL, so a 302 would hand it to whatever host the
// response names. Refusing to follow keeps the credential on the one host the
// operator configured, and turns a redirect into a status this logs about
// rather than a silent leak.
var heartbeatClient = &http.Client{
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// heartbeatCause is a transport error with the address it was dialling
// stripped off.
//
// A *url.Error prints the URL it failed on, and here the URL is the token. The
// redactor catches the configured string exactly, which a re-serialised URL
// need not match, so the address is removed at the source rather than trusted
// to be masked downstream. What is left is the part worth reading: connection
// refused, no such host, deadline exceeded.
func heartbeatCause(err error) error {
	var urlErr *url.Error
	for errors.As(err, &urlErr) {
		err = urlErr.Err
	}

	return err
}

// pingHeartbeat tells an external monitor that the maintenance loop is turning.
//
// This is liveness, not correctness. The alert fires when nothing arrives
// inside the monitor's grace period, so the beat must survive everything that
// is not an outage: no work to do, background work paused, reaction polling
// switched off, and a job that keeps failing. What the lane actually did is in
// the metrics and the log instead.
//
// That is why pollLoop beats on its own ticker rather than at the end of a
// dispatch. Coupled to the dispatch it would have gone quiet whenever the lane
// had nothing to do, and under the panel a single failing job - whose error
// travels all the way up - would have silenced it while the loop kept retrying.
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

	reqCtx, cancel := context.WithTimeout(ctx, heartbeatTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, s.cfg.heartbeatURL, nil)
	if err != nil {
		s.logger.Warn("could not build the heartbeat request", "error", heartbeatCause(err))

		return
	}

	resp, err := heartbeatClient.Do(req)
	if err != nil {
		// A stop cancels the request already in flight, which the check above
		// cannot catch and which is not a missed beat. Read from the parent
		// rather than reqCtx, because reqCtx's own deadline expiring means a
		// monitor that stopped answering, and that is worth saying
		if ctx.Err() != nil {
			return
		}

		s.logger.Warn("could not send the heartbeat", "error", heartbeatCause(err))

		return
	}
	defer func() {
		// Drained so the connection is reusable rather than torn down every
		// tick for a body nothing reads
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	// Only 2xx counts. A redirect is not followed, so it arrives here as a 3xx
	// and means the URL is wrong rather than that the beat landed
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		s.logger.Warn("heartbeat refused", "status", resp.StatusCode)

		return
	}

	s.logger.Debug("heartbeat sent", "status", resp.StatusCode)
}
