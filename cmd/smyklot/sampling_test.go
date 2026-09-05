package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func TestDatabaseSamplesChartPoolWaitsAsALevel(t *testing.T) {
	store := &samplingStore{}
	measured := &server{store: store}
	reachable := func(lifetime int64) storage.DatabaseStatus {
		return storage.DatabaseStatus{
			Reachable:   true,
			Connections: storage.ConnectionStats{WaitCount: lifetime},
		}
	}

	for _, expected := range []struct {
		name   string
		status storage.DatabaseStatus
		refuse bool
		waits  float64
		sample bool
	}{
		{name: "first reading", status: reachable(7), waits: 7, sample: true},
		{name: "two more", status: reachable(9), waits: 2, sample: true},
		{name: "none since", status: reachable(9), waits: 0, sample: true},
		{name: "unreachable", status: storage.DatabaseStatus{
			Connections: storage.ConnectionStats{WaitCount: 400},
		}},
		{name: "waited through the outage", status: reachable(30), waits: 21, sample: true},
		{name: "a refused write", status: reachable(110), refuse: true},
		{name: "the refused waits are still owed", status: reachable(112), waits: 82, sample: true},
		{name: "pool replaced", status: reachable(1), waits: 1, sample: true},
	} {
		store.status = expected.status
		store.recorded = nil
		store.refuse = nil
		if expected.refuse {
			store.refuse = errors.New("the database is restarting")
		}

		err := measured.sampleServiceHealth(context.Background(), time.Now().UTC())
		if (err != nil) != expected.refuse {
			t.Fatalf("%s: measurement error = %v", expected.name, err)
		}

		got, sampled := 0.0, false
		for _, sample := range store.recorded {
			if sample.Label == "pool_waits" {
				got, sampled = sample.Value, true
			}
		}
		if sampled != expected.sample {
			t.Fatalf("%s: stored a wait count = %v, want %v", expected.name, sampled, expected.sample)
		}
		if got != expected.waits {
			t.Fatalf("%s: waits since the reading that was stored = %v, want %v",
				expected.name, got, expected.waits)
		}
	}
}

func TestLedgerAndLaneSamplesMeasureWhatIsEmpty(t *testing.T) {
	now := time.Now().UTC()
	labels := func(samples []storage.ServiceSample) map[string]float64 {
		by := map[string]float64{}
		for _, sample := range samples {
			by[sample.Label] = sample.Value
		}

		return by
	}

	kept := labels(ledgerSamples([]storage.LedgerSize{
		{Kind: string(workqueue.KindReactionScan), Finished: 12},
	}, now))
	if len(kept) != len(workqueue.Kinds()) {
		t.Fatalf("measured %d kinds, want %d", len(kept), len(workqueue.Kinds()))
	}
	if kept[string(workqueue.KindReactionScan)] != 12 {
		t.Fatalf("reaction scan kept %v rows, want 12", kept[string(workqueue.KindReactionScan)])
	}
	if kept[string(workqueue.KindSyncApply)] != 0 {
		t.Fatalf("a kind holding nothing was measured as %v, want 0",
			kept[string(workqueue.KindSyncApply)])
	}

	waiting := labels(laneSamples([]storage.LaneBacklog{
		{Lane: string(workqueue.LaneWebhook), Depth: 3, Oldest: time.Minute},
	}, now))
	if len(waiting) != len(workqueue.Lanes()) {
		t.Fatalf("measured %d lanes, want %d", len(waiting), len(workqueue.Lanes()))
	}
	if waiting[string(workqueue.LaneWebhook)] != 3 {
		t.Fatalf("webhook lane held %v items, want 3", waiting[string(workqueue.LaneWebhook)])
	}
	if waiting[string(workqueue.LanePendingCI)] != 0 {
		t.Fatalf("an empty lane was measured as %v, want 0",
			waiting[string(workqueue.LanePendingCI)])
	}
}

func TestQueryStatsSurviveAWriteThatFailed(t *testing.T) {
	store := &samplingStore{stats: []storage.QueryStats{
		{Name: "Store.ListWorkQueue", Observations: 4, Total: 8 * time.Millisecond, Max: 3 * time.Millisecond},
	}}
	measured := &server{store: store}
	store.refuse = errors.New("the database is restarting")

	if err := measured.sampleServiceHealth(context.Background(), time.Now().UTC()); err == nil {
		t.Fatal("a refused write was reported as a measurement")
	}

	store.refuse = nil
	store.stats = []storage.QueryStats{
		{Name: "Store.ListWorkQueue", Observations: 1, Total: time.Millisecond, Max: 5 * time.Millisecond},
	}
	if err := measured.sampleServiceHealth(context.Background(), time.Now().UTC()); err != nil {
		t.Fatalf("second measurement: %v", err)
	}

	written := store.lastQuerySample(t)
	if written.Observations != 5 {
		t.Fatalf("stored %d observations, want the 4 the failed write held plus 1", written.Observations)
	}
	if written.Total != 9*time.Millisecond {
		t.Fatalf("stored %v, want 9ms", written.Total)
	}
	if written.Max != 5*time.Millisecond {
		t.Fatalf("stored a worst call of %v, want 5ms", written.Max)
	}

	// What a write stored is not held a second time.
	store.stats = []storage.QueryStats{
		{Name: "Store.ListWorkQueue", Observations: 2, Total: 2 * time.Millisecond, Max: time.Millisecond},
	}
	if err := measured.sampleServiceHealth(context.Background(), time.Now().UTC()); err != nil {
		t.Fatalf("third measurement: %v", err)
	}
	if again := store.lastQuerySample(t); again.Observations != 2 {
		t.Fatalf("stored %d observations, want the 2 measured since the write that succeeded",
			again.Observations)
	}
}

type samplingStore struct {
	storage.Store
	stats    []storage.QueryStats
	status   storage.DatabaseStatus
	refuse   error
	recorded []storage.ServiceSample
}

func (s *samplingStore) lastQuerySample(t *testing.T) storage.ServiceSample {
	t.Helper()
	for index := len(s.recorded) - 1; index >= 0; index-- {
		if s.recorded[index].Metric == storage.SampleQuery {
			return s.recorded[index]
		}
	}
	t.Fatal("nothing was stored for the statements that ran")

	return storage.ServiceSample{}
}

func (s *samplingStore) LedgerSizes(context.Context) ([]storage.LedgerSize, error) {
	return nil, nil
}

func (s *samplingStore) LaneBacklogs(
	context.Context, time.Time,
) ([]storage.LaneBacklog, error) {
	return nil, nil
}

func (s *samplingStore) Status(context.Context) storage.DatabaseStatus {
	return s.status
}

func (s *samplingStore) DrainQueryStats() []storage.QueryStats {
	drained := s.stats
	s.stats = nil

	return drained
}

func (s *samplingStore) RecordServiceSamples(
	_ context.Context, samples []storage.ServiceSample,
) error {
	if s.refuse != nil {
		return s.refuse
	}
	s.recorded = append(s.recorded, samples...)

	return nil
}

func (s *samplingStore) PruneServiceSamples(context.Context, time.Time) (int64, error) {
	return 0, nil
}
