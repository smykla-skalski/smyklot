package main

import (
	"context"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

const sampleRetention = 90 * 24 * time.Hour

func (s *server) sampleServiceHealth(ctx context.Context, now time.Time) error {
	ledger, err := s.store.LedgerSizes(ctx)
	if err != nil {
		return fmt.Errorf("read ledger sizes: %w", err)
	}
	lanes, err := s.store.LaneBacklogs(ctx, now)
	if err != nil {
		return fmt.Errorf("read lane backlogs: %w", err)
	}
	samples := append(ledgerSamples(ledger, now), laneSamples(lanes, now)...)
	samples = append(samples, databaseSamples(s.store.Status(ctx), now)...)
	samples = append(samples, s.queryLatencySamples(now)...)
	if err := s.store.RecordServiceSamples(ctx, samples); err != nil {
		return fmt.Errorf("record service samples: %w", err)
	}
	if _, err := s.store.PruneServiceSamples(ctx, now.Add(-sampleRetention)); err != nil {
		return fmt.Errorf("prune service samples: %w", err)
	}

	return nil
}

func (s *server) queryLatencySamples(now time.Time) []storage.ServiceSample {
	stats := s.store.DrainQueryStats()
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

func ledgerSamples(sizes []storage.LedgerSize, now time.Time) []storage.ServiceSample {
	samples := make([]storage.ServiceSample, 0, len(sizes))
	for _, size := range sizes {
		samples = append(samples, storage.ServiceSample{
			Metric: storage.SampleLedger, Label: size.Kind,
			SampledAt: now, Value: float64(size.Finished),
		})
	}

	return samples
}

func laneSamples(lanes []storage.LaneBacklog, now time.Time) []storage.ServiceSample {
	samples := make([]storage.ServiceSample, 0, len(lanes))
	for _, backlog := range lanes {
		samples = append(samples, storage.ServiceSample{
			Metric: storage.SampleLane, Label: backlog.Lane, SampledAt: now,
			Observations: 1, Total: backlog.Oldest, Max: backlog.Oldest,
			Value: float64(backlog.Depth),
		})
	}

	return samples
}

func databaseSamples(status storage.DatabaseStatus, now time.Time) []storage.ServiceSample {
	if !status.Reachable {
		return nil
	}

	return []storage.ServiceSample{
		{
			Metric: storage.SampleDatabase, Label: "size_bytes",
			SampledAt: now, Value: float64(status.SizeBytes),
		},
		{
			Metric: storage.SampleDatabase, Label: "round_trip",
			SampledAt: now, Observations: 1,
			Total: status.Latency, Max: status.Latency,
		},
		{
			Metric: storage.SampleDatabase, Label: "pool_waits",
			SampledAt: now, Value: float64(status.Connections.WaitCount),
		},
		{
			Metric: storage.SampleDatabase, Label: "pool_in_use",
			SampledAt: now, Value: float64(status.Connections.InUse),
		},
	}
}
