package storage

import (
	"context"
	"time"
)

// SampleMetric names what a stored sample measures.
//
// It is a small closed set rather than a free string because the panel draws
// one chart per metric and the migration's CHECK is what stops a typo becoming
// a series nobody can find again.
type SampleMetric string

const (
	// SampleQuery is how long the store's own statements took, by the function
	// that issued them.
	SampleQuery SampleMetric = "query"

	// SampleLedger is how many rows a queue workload holds, by kind.
	SampleLedger SampleMetric = "ledger"

	// SampleLane is how long the work in one lane took to run, and how much of
	// it was waiting.
	SampleLane SampleMetric = "lane"

	// SampleDatabase is what the database itself reports: its size, the
	// latency of a round trip, and what its pool has had to wait for.
	SampleDatabase SampleMetric = "database"
)

// ServiceSample is one hour of one measured thing.
//
// The hour is the grain because a chart of the last month is what an operator
// reads and a row per observation is what fills a disk. A timing writes
// Observations, Total and Max; a gauge writes Value; nothing writes both, and
// the reader knows which by the metric.
type ServiceSample struct {
	Metric       SampleMetric
	Label        string
	SampledAt    time.Time
	Observations int64
	Failures     int64
	Total        time.Duration
	Max          time.Duration
	Value        float64
}

// Mean is the average observation, or zero when nothing was observed.
func (s ServiceSample) Mean() time.Duration {
	if s.Observations <= 0 {
		return 0
	}

	return s.Total / time.Duration(s.Observations)
}

// ServiceSampleQuery selects one metric's series over a window.
type ServiceSampleQuery struct {
	Metric SampleMetric
	Since  time.Time
	Until  time.Time

	// Labels narrows to named series. Empty means every series the metric has.
	Labels []string
}

// QueryStats is what the store measured for one of its own statements since
// the last drain.
//
// The store keeps this itself rather than taking an observer, because the
// alternative is threading one through the engine adapters and the port for a
// number the store is the only thing able to produce.
type QueryStats struct {
	Name         string
	Observations int64
	Failures     int64
	Total        time.Duration
	Max          time.Duration
}

// SampleStore persists and reads the service's own measurements.
type SampleStore interface {
	// RecordServiceSamples folds observations into their hour, adding to
	// whatever that hour already holds.
	RecordServiceSamples(context.Context, []ServiceSample) error

	// ListServiceSamples reads one metric's series, oldest first.
	ListServiceSamples(context.Context, ServiceSampleQuery) ([]ServiceSample, error)

	// PruneServiceSamples removes samples older than the cutoff.
	PruneServiceSamples(context.Context, time.Time) (int64, error)

	// DrainQueryStats returns what the store's statements have cost since the
	// last call, and resets the counters.
	DrainQueryStats() []QueryStats
}
