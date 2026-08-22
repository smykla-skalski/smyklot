package main

import (
	"sync"
	"time"
)

// maxRecordedFailures is how many recent failures are kept.
//
// Enough to show a pattern rather than a single symptom, small enough that the
// buffer costs nothing. This is a window on the recent past, not an archive:
// the log is where a delivery from last week is found.
const maxRecordedFailures = 50

// deliveryFailure is one delivery that did not take effect.
type deliveryFailure struct {
	Time        time.Time `json:"time"`
	DeliveryID  string    `json:"delivery_id"`
	Repository  string    `json:"repository"`
	PullRequest int       `json:"pull_request"`
	Action      string    `json:"action"`
	Reason      string    `json:"reason"`
}

// failureLog holds the most recent delivery failures.
//
// A service answers 202 and executes later, so a delivery that fails afterwards
// leaves GitHub's own log showing a success. Without this, the only trace is a
// log line, which is no help on a host with no log collector.
type failureLog struct {
	mu      sync.Mutex
	max     int
	entries []deliveryFailure
}

func newFailureLog(max int) *failureLog {
	return &failureLog{max: max}
}

// Record adds a failure, dropping the oldest once the buffer is full.
func (l *failureLog) Record(failure deliveryFailure) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.entries = append(l.entries, failure)

	if len(l.entries) > l.max {
		l.entries = l.entries[len(l.entries)-l.max:]
	}
}

// Snapshot returns the recorded failures, newest first.
//
// Newest first because the question is always "what just broke".
func (l *failureLog) Snapshot() []deliveryFailure {
	l.mu.Lock()
	defer l.mu.Unlock()

	out := make([]deliveryFailure, 0, len(l.entries))
	for i := len(l.entries) - 1; i >= 0; i-- {
		out = append(out, l.entries[i])
	}

	return out
}
