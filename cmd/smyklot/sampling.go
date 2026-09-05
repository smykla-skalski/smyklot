package main

import (
	"context"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

const (
	sampleRetention = 90 * 24 * time.Hour
	sampleInterval  = 5 * time.Minute
)

// sampleLoop measures the service on a cadence of its own until ctx is
// cancelled. The query counters are read destructively, so what a stored point
// covers is decided by how long ago the last read was, and that must not be an
// editable schedule.
//
// It reads before it waits. Most of what it stores is a level - a size, a
// depth, a count of rows - and a level is knowable the moment the service is
// up, so waiting out the first interval left the page with four empty cards
// saying nothing had been measured, which is indistinguishable from a page that
// is broken.
func (s *server) sampleLoop(ctx context.Context) {
	ticker := time.NewTicker(sampleInterval)
	defer ticker.Stop()

	for {
		if err := s.sampleServiceHealth(ctx, time.Now().UTC()); err != nil {
			s.logger.Error("service measurement failed", "error", s.redactor.Error(err))
		}

		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
		}
	}
}

func (s *server) sampleServiceHealth(ctx context.Context, now time.Time) error {
	ledger, err := s.store.LedgerSizes(ctx)
	if err != nil {
		return fmt.Errorf("read ledger sizes: %w", err)
	}
	lanes, err := s.store.LaneBacklogs(ctx, now)
	if err != nil {
		return fmt.Errorf("read lane backlogs: %w", err)
	}
	status := s.store.Status(ctx)
	samples := append(ledgerSamples(ledger, now), laneSamples(lanes, now)...)
	samples = append(samples, databaseSamples(status, s.sampledWaits, now)...)
	drained := s.drainQueryStats()
	samples = append(samples, queryLatencySamples(drained, now)...)
	if err := s.store.RecordServiceSamples(ctx, samples); err != nil {
		s.keepQueryStats(drained)

		return fmt.Errorf("record service samples: %w", err)
	}
	if status.Reachable {
		s.sampledWaits = status.Connections.WaitCount
	}
	if _, err := s.store.PruneServiceSamples(ctx, now.Add(-sampleRetention)); err != nil {
		return fmt.Errorf("prune service samples: %w", err)
	}

	return nil
}

// drainQueryStats takes the counters the store has been keeping, and whatever
// a previous reading could not store. The store's counters reset when they are
// read, so a failed write would otherwise take that window with it - and the
// write fails when the database is struggling, which is when the timings are
// worth the most.
func (s *server) drainQueryStats() []storage.QueryStats {
	drained := s.store.DrainQueryStats()
	if len(s.unstoredStats) == 0 {
		return drained
	}
	folded := make(map[string]storage.QueryStats, len(s.unstoredStats)+len(drained))
	for _, stat := range append(s.unstoredStats, drained...) {
		held := folded[stat.Name]
		held.Name = stat.Name
		held.Observations += stat.Observations
		held.Failures += stat.Failures
		held.Total += stat.Total
		held.Max = max(held.Max, stat.Max)
		folded[stat.Name] = held
	}
	s.unstoredStats = nil
	held := make([]storage.QueryStats, 0, len(folded))
	for _, stat := range folded {
		held = append(held, stat)
	}

	return held
}

func (s *server) keepQueryStats(stats []storage.QueryStats) {
	s.unstoredStats = stats
}

func queryLatencySamples(stats []storage.QueryStats, now time.Time) []storage.ServiceSample {
	samples := make([]storage.ServiceSample, 0, len(stats))
	for _, stat := range stats {
		samples = append(samples, storage.ServiceSample{
			Metric: storage.SampleQuery, Label: stat.Name, SampledAt: now,
			Observations: stat.Observations, Failures: stat.Failures,
			Total: stat.Total, Max: stat.Max,
		})
	}

	return samples
}

// ledgerSamples measures every kind of work the queue defines, and not only
// the kinds holding rows: a count is grouped away when it reaches zero, and a
// series that stops is drawn as one whose last reading still stands.
func ledgerSamples(sizes []storage.LedgerSize, now time.Time) []storage.ServiceSample {
	kept := make(map[string]int64, len(sizes))
	for _, size := range sizes {
		kept[size.Kind] = size.Finished
	}
	samples := make([]storage.ServiceSample, 0, len(workqueue.Kinds()))
	for _, kind := range workqueue.Kinds() {
		samples = append(samples, storage.ServiceSample{
			Metric: storage.SampleLedger, Label: string(kind),
			SampledAt: now, Value: float64(kept[string(kind)]),
		})
	}

	return samples
}

// laneSamples measures every lane, including the ones holding nothing, for the
// reason ledgerSamples measures every kind.
func laneSamples(lanes []storage.LaneBacklog, now time.Time) []storage.ServiceSample {
	waiting := make(map[string]storage.LaneBacklog, len(lanes))
	for _, backlog := range lanes {
		waiting[backlog.Lane] = backlog
	}
	samples := make([]storage.ServiceSample, 0, len(workqueue.Lanes()))
	for _, lane := range workqueue.Lanes() {
		backlog := waiting[string(lane)]
		samples = append(samples, storage.ServiceSample{
			Metric: storage.SampleLane, Label: string(lane), SampledAt: now,
			Observations: 1, Total: backlog.Oldest, Max: backlog.Oldest,
			Value: float64(backlog.Depth),
		})
	}

	return samples
}

// databaseSamples reads the database's own numbers, turning the pool's
// lifetime wait count into how many waits happened since the reading that was
// stored: every other series here is a level, and a counter drawn as one falls
// off a cliff at each deploy and reads as an improvement. The caller moves that
// baseline once the write has succeeded, so a refused write leaves the waits it
// could not store to the next one, and the waits are stored as a count of what
// happened so that the readings sharing a point add up rather than folding to
// the busiest few minutes among them.
//
// A database that answers a ping and then refuses to describe itself is
// reachable with an error and a size of zero, so the size is stored only where
// it was read. The rest of these are measured before that can happen: the round
// trip is the ping, and the pool is sampled from the connection that served it.
func databaseSamples(
	status storage.DatabaseStatus,
	stored int64,
	now time.Time,
) []storage.ServiceSample {
	if !status.Reachable {
		return nil
	}
	waits := status.Connections.WaitCount - stored
	if waits < 0 {
		waits = status.Connections.WaitCount
	}
	samples := make([]storage.ServiceSample, 0, 4)
	if status.Error == "" {
		samples = append(samples, storage.ServiceSample{
			Metric: storage.SampleDatabase, Label: "size_bytes",
			SampledAt: now, Value: float64(status.SizeBytes),
		})
	}

	return append(samples,
		storage.ServiceSample{
			Metric: storage.SampleDatabase, Label: "round_trip",
			SampledAt: now, Observations: 1,
			Total: status.Latency, Max: status.Latency,
		},
		storage.ServiceSample{
			Metric: storage.SampleDatabase, Label: "pool_waits",
			SampledAt: now, Value: float64(waits), Cumulative: true,
		},
		storage.ServiceSample{
			Metric: storage.SampleDatabase, Label: "pool_in_use",
			SampledAt: now, Value: float64(status.Connections.InUse),
		},
	)
}
