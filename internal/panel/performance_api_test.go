package panel

import (
	"slices"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

func TestPerformanceWindowDefaultsToADayAndStopsAtNinety(t *testing.T) {
	for _, expected := range []struct {
		raw    string
		window time.Duration
	}{
		{raw: "", window: 24 * time.Hour},
		{raw: "not a number", window: 24 * time.Hour},
		{raw: "0", window: 24 * time.Hour},
		{raw: "-6", window: 24 * time.Hour},
		{raw: "1", window: time.Hour},
		{raw: "168", window: 168 * time.Hour},
		{raw: "2160", window: 2160 * time.Hour},
		{raw: "999999", window: 2160 * time.Hour},
	} {
		if window := performanceWindow(expected.raw); window != expected.window {
			t.Errorf("%q asked for %v, want %v", expected.raw, window, expected.window)
		}
	}
}

func TestPerformanceSeriesRanksTheLoudestFirst(t *testing.T) {
	hour := time.Date(2026, 9, 5, 16, 0, 0, 0, time.UTC)
	timing := func(label string, at time.Time, worst time.Duration) storage.ServiceSample {
		return storage.ServiceSample{
			Metric: storage.SampleQuery, Label: label, SampledAt: at,
			Observations: 4, Total: 8 * time.Millisecond, Max: worst,
		}
	}

	series := performanceSeries([]storage.ServiceSample{
		timing("Store.Quiet", hour, 5*time.Millisecond),
		{
			Metric: storage.SampleDatabase, Label: "size_bytes",
			SampledAt: hour, Value: 20,
		},
		timing("Store.Loud", hour, 50*time.Millisecond),
		timing("Store.Quiet", hour.Add(time.Hour), 3*time.Millisecond),
	})

	labels := make([]string, 0, len(series))
	for _, one := range series {
		labels = append(labels, one.Label)
	}
	if want := []string{"Store.Loud", "size_bytes", "Store.Quiet"}; !slices.Equal(labels, want) {
		t.Fatalf("ranked %v, want %v", labels, want)
	}

	quiet := series[2]
	if len(quiet.Points) != 2 {
		t.Fatalf("a label measured twice kept %d points, want 2", len(quiet.Points))
	}
	if !quiet.Points[0].At.Equal(hour) {
		t.Errorf("the first point is at %v, want %v", quiet.Points[0].At, hour)
	}
	if quiet.Points[0].MeanMillis != 2 {
		t.Errorf("4 calls in 8ms averaged %v ms, want 2", quiet.Points[0].MeanMillis)
	}
	if quiet.Points[0].MaxMillis != 5 {
		t.Errorf("the worst call was %v ms, want 5", quiet.Points[0].MaxMillis)
	}
}

func TestPerformanceSeriesRanksAnEqualPeakByName(t *testing.T) {
	hour := time.Date(2026, 9, 5, 16, 0, 0, 0, time.UTC)
	alike := func(label string) storage.ServiceSample {
		return storage.ServiceSample{
			Metric: storage.SampleLedger, Label: label, SampledAt: hour, Value: 7,
		}
	}

	series := performanceSeries([]storage.ServiceSample{
		alike("sync_scan"), alike("auth_cleanup"), alike("reaction_scan"),
	})

	labels := make([]string, 0, len(series))
	for _, one := range series {
		labels = append(labels, one.Label)
	}
	if want := []string{"auth_cleanup", "reaction_scan", "sync_scan"}; !slices.Equal(labels, want) {
		t.Fatalf("ranked %v, want %v", labels, want)
	}
}
