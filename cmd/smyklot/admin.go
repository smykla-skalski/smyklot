package main

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/smykla-skalski/smyklot/pkg/metrics"
)

// Routes an operator and an orchestrator read.
const (
	// livePath answers whether the process is running at all. It says nothing
	// about whether the process can do anything, on purpose: a restart cannot
	// fix GitHub being down, so failing this on an outage would restart every
	// replica for nothing
	livePath = "/livez"

	// readyPath answers whether the process can reach GitHub, so an
	// orchestrator can keep traffic away from one that cannot
	readyPath = "/readyz"

	// metricsPath serves the Prometheus exposition format
	metricsPath = "/metrics"

	// failuresPath serves the recent deliveries that did not take effect
	failuresPath = "/failures"
)

// adminHandler builds the routes that describe the service.
//
// They sit on their own listener because the webhook port is reachable from the
// internet and none of this should be. Queue depth, failure reasons and Go
// runtime detail tell an operator how the service is doing, and would tell
// anyone else the same.
func (s *server) adminHandler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET "+livePath, func(w http.ResponseWriter, _ *http.Request) {
		writeText(w, http.StatusOK, "ok\n")
	})

	mux.HandleFunc("GET "+readyPath, s.handleReady)
	mux.Handle("GET "+metricsPath, metrics.Handler(s.registry))
	mux.HandleFunc("GET "+failuresPath, s.handleFailures)

	return mux
}

// handleReady reports whether the service can reach GitHub, and why not when it
// cannot.
func (s *server) handleReady(w http.ResponseWriter, _ *http.Request) {
	state := s.readiness.state()

	status := http.StatusOK
	if !state.Ready {
		status = http.StatusServiceUnavailable
	}

	writeJSON(w, status, state)
}

// handleFailures serves the deliveries that were accepted and then failed.
func (s *server) handleFailures(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.failures.Snapshot())
}

func writeText(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)

	_, _ = io.WriteString(w, body)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	_ = json.NewEncoder(w).Encode(body)
}
