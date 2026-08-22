package main

import (
	"context"
	"sync"
	"time"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

const (
	// readyInterval is how often GitHub is checked. Short enough that a revoked
	// credential is noticed before the next delivery, long enough that the
	// check is invisible next to the traffic it guards
	readyInterval = 30 * time.Second

	// readyTimeout caps one check, so a hung connection cannot keep a stale
	// answer alive
	readyTimeout = 10 * time.Second

	// readyStaleAfter is how long an answer stands before it is treated as no
	// answer. Without it, a prober that died would leave the service reporting
	// ready forever on the strength of one old check
	readyStaleAfter = 3 * readyInterval

	// reasonUnchecked is the state before the first check finishes. Starting
	// unready is what keeps traffic away from a process whose credentials have
	// not been proven yet
	reasonUnchecked = "no check has completed yet"

	// reasonStale is the state when the prober has stopped reporting
	reasonStale = "no check has completed recently"
)

// readiness records whether the service can reach GitHub.
//
// Liveness and readiness answer different questions. A process can be perfectly
// alive and still unable to do anything, because its credentials expired or
// GitHub is down. An orchestrator that cannot tell those apart either restarts
// a healthy process or keeps sending work to one that will drop it.
type readiness struct {
	mu        sync.RWMutex
	reason    string
	checkedAt time.Time
}

// newReadiness returns a state that is unready until the first check succeeds.
func newReadiness() *readiness {
	return &readiness{reason: reasonUnchecked}
}

// readinessState is one answer, as the endpoint serves it.
type readinessState struct {
	Ready     bool      `json:"ready"`
	Reason    string    `json:"reason,omitempty"`
	CheckedAt time.Time `json:"checked_at,omitzero"`
}

// set records one check's result and reports whether the answer changed.
//
// The caller logs only on a change, so a service that has been unreachable for
// an hour says so once rather than a hundred and twenty times.
//
// An answer that had gone stale counts as a change even when the result is the
// same as before. A process paused past the staleness window and then resumed
// went from not-ready to ready as far as every reader is concerned, and a
// recovery nothing logged is one nobody can correlate with the dip.
func (r *readiness) set(reason string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	changed := reason != r.reason || r.stale()

	r.reason = reason
	r.checkedAt = time.Now()

	return changed
}

// state returns the current answer.
func (r *readiness) state() readinessState {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.stale() {
		return readinessState{Reason: reasonStale, CheckedAt: r.checkedAt}
	}

	return readinessState{
		Ready:     r.reason == "",
		Reason:    r.reason,
		CheckedAt: r.checkedAt,
	}
}

// stale reports whether the last answer is too old to stand for.
//
// Callers hold the lock.
func (r *readiness) stale() bool {
	return !r.checkedAt.IsZero() && time.Since(r.checkedAt) > readyStaleAfter
}

// probeLoop keeps the readiness answer current until ctx is cancelled.
func (s *server) probeLoop(ctx context.Context) {
	// Checked once up front rather than after the first tick, so a service
	// with working credentials is ready in a second rather than in thirty
	s.probe(ctx)

	ticker := time.NewTicker(readyInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			s.probe(ctx)
		}
	}
}

// probe checks GitHub once and records what it found.
func (s *server) probe(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, readyTimeout)
	defer cancel()

	err := s.pingGitHub(ctx)
	if err == nil && s.store != nil {
		err = s.store.Ping(ctx)
	}

	// A check cut short by shutdown says nothing about GitHub, so it must not
	// be the last word on record
	if parent.Err() != nil {
		return
	}

	if err != nil {
		// Logged only on a change, so a service that has been unreachable for
		// an hour says so once rather than a hundred and twenty times
		if s.readiness.set(s.redactor.Error(err)) {
			s.logger.Warn("not ready", "error", err)
		}

		return
	}

	if s.readiness.set("") {
		s.logger.Info("ready")
	}
}

// pingGitHub proves both halves of what the service needs: that GitHub answers,
// and that these credentials still work.
func (s *server) pingGitHub(ctx context.Context) error {
	token, err := s.tokens.AppToken()
	if err != nil {
		return bot.NewGitHubError(bot.ErrGitHubAppAuth, err)
	}

	client, err := github.NewAppClient(token, s.cfg.apiBaseURL)
	if err != nil {
		return bot.NewGitHubError(bot.ErrGitHubClient, err)
	}

	return client.Ping(ctx)
}
