package panel

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

func TestRootPerformanceReadsTheWindowAndCapsTheStatements(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	measured := make([]storage.ServiceSample, 0, 30)
	for series := range 14 {
		measured = append(measured,
			storage.ServiceSample{
				Metric: storage.SampleQuery, Label: fmt.Sprintf("Store.Read%02d", series),
				SampledAt: harness.now, Observations: 1,
				Total: time.Duration(series+1) * time.Millisecond,
				Max:   time.Duration(series+1) * time.Millisecond,
			},
			storage.ServiceSample{
				Metric: storage.SampleLedger, Label: fmt.Sprintf("kind_%02d", series),
				SampledAt: harness.now, Value: float64(series + 1),
			},
		)
	}
	measured = append(measured, storage.ServiceSample{
		Metric: storage.SampleLedger, Label: "long_ago",
		SampledAt: harness.now.Add(-5 * time.Hour), Value: 4000,
	})
	if err := harness.store.RecordServiceSamples(t.Context(), measured); err != nil {
		t.Fatal(err)
	}

	read := func(query string) performanceResponse {
		t.Helper()
		response := harness.request(
			t, http.MethodGet, "/panel/api/v1/root/performance"+query, nil, session,
		)
		if response.Code != http.StatusOK {
			t.Fatalf("%q answered %d: %s", query, response.Code, response.Body.String())
		}
		var body performanceResponse
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}

		return body
	}

	day := read("")
	if got := len(day.Metrics["query"]); got != performanceSeriesLimit {
		t.Errorf("14 statements came back as %d series, want %d", got, performanceSeriesLimit)
	}
	if got := len(day.Metrics["ledger"]); got != 15 {
		t.Errorf("the ledger came back as %d series, want all 15", got)
	}
	if !day.Since.Equal(day.Until.Add(-24 * time.Hour)) {
		t.Errorf("the default window ran %v to %v, want a day", day.Since, day.Until)
	}

	hour := read("?window=1")
	if got := len(hour.Metrics["ledger"]); got != 14 {
		t.Errorf("an hour came back as %d ledger series, want the 14 inside it", got)
	}
	for _, one := range hour.Metrics["ledger"] {
		if one.Label == "long_ago" {
			t.Error("a reading from five hours ago came back inside a one-hour window")
		}
	}
}

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
