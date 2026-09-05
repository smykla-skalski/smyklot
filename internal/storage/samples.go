package storage

import (
	"context"
	"time"
)

type SampleMetric string

const (
	SampleQuery    SampleMetric = "query"
	SampleLedger   SampleMetric = "ledger"
	SampleLane     SampleMetric = "lane"
	SampleDatabase SampleMetric = "database"
)

type ServiceSample struct {
	Metric       SampleMetric
	Label        string
	SampledAt    time.Time
	Observations int64
	Failures     int64
	Total        time.Duration
	Max          time.Duration
	Value        float64

	// Cumulative says Value counts what happened since the last reading
	// rather than describing the service now, so the readings that share a
	// point are added rather than folded to their highest. A reading is the
	// default because most of these are one: a size, a depth, a count of
	// rows. A count of events measured every few minutes and folded to its
	// highest would report the busiest few minutes as the whole point.
	Cumulative bool
}

func (s ServiceSample) Mean() time.Duration {
	if s.Observations <= 0 {
		return 0
	}

	return s.Total / time.Duration(s.Observations)
}

type ServiceSampleQuery struct {
	Metric SampleMetric
	Since  time.Time
	Until  time.Time
	Limit  int
}

type QueryStats struct {
	Name         string
	Observations int64
	Failures     int64
	Total        time.Duration
	Max          time.Duration
}

type LedgerSize struct {
	Kind     string
	Finished int64
}

type LaneBacklog struct {
	Lane   string
	Depth  int64
	Oldest time.Duration
}

type SampleStore interface {
	LedgerSizes(context.Context) ([]LedgerSize, error)
	LaneBacklogs(context.Context, time.Time) ([]LaneBacklog, error)
	RecordServiceSamples(context.Context, []ServiceSample) error
	ListServiceSamples(context.Context, ServiceSampleQuery) ([]ServiceSample, error)
	PruneServiceSamples(context.Context, time.Time) (int64, error)
	DrainQueryStats() []QueryStats
}
