package main

import (
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

func TestDatabaseSamplesChartPoolWaitsAsALevel(t *testing.T) {
	measured := &server{}
	now := time.Now().UTC()
	waits := func(lifetime int64) float64 {
		t.Helper()
		for _, sample := range measured.databaseSamples(storage.DatabaseStatus{
			Reachable:   true,
			Connections: storage.ConnectionStats{WaitCount: lifetime},
		}, now) {
			if sample.Label == "pool_waits" {
				return sample.Value
			}
		}
		t.Fatal("no pool_waits sample")

		return 0
	}

	for _, expected := range []struct {
		lifetime int64
		waits    float64
	}{
		{lifetime: 7, waits: 7},
		{lifetime: 9, waits: 2},
		{lifetime: 9, waits: 0},
		{lifetime: 1, waits: 1},
	} {
		if got := waits(expected.lifetime); got != expected.waits {
			t.Fatalf("lifetime %d: waits since the last reading = %v, want %v",
				expected.lifetime, got, expected.waits)
		}
	}
}
