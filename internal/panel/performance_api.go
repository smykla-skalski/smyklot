package panel

import (
	"cmp"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

const (
	performanceDefaultWindow = 24 * time.Hour
	performanceMaxWindow     = 90 * 24 * time.Hour
	performanceSeriesLimit   = 12
)

var performanceMetrics = map[storage.SampleMetric]int{
	storage.SampleQuery:    performanceSeriesLimit,
	storage.SampleLedger:   0,
	storage.SampleLane:     0,
	storage.SampleDatabase: 0,
}

func (s *Server) getRootPerformance(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.requireRoot(w, r); !ok {
		return
	}
	now := s.now().UTC()
	since := now.Add(-performanceWindow(r.URL.Query().Get("window")))
	response := performanceResponse{
		Since:   since,
		Until:   now,
		Metrics: map[string][]performanceSeriesResponse{},
	}
	for metric, limit := range performanceMetrics {
		samples, err := s.store.ListServiceSamples(r.Context(), storage.ServiceSampleQuery{
			Metric: metric, Since: since, Until: now, Limit: limit,
		})
		if err != nil {
			s.writeInternal(w, err)

			return
		}
		response.Metrics[string(metric)] = performanceSeries(samples)
	}
	writeJSON(w, http.StatusOK, response)
}

func performanceWindow(raw string) time.Duration {
	hours, err := strconv.Atoi(raw)
	if err != nil || hours <= 0 {
		return performanceDefaultWindow
	}
	if hours > int(performanceMaxWindow/time.Hour) {
		return performanceMaxWindow
	}

	return time.Duration(hours) * time.Hour
}

type performanceResponse struct {
	Since   time.Time                              `json:"since"`
	Until   time.Time                              `json:"until"`
	Metrics map[string][]performanceSeriesResponse `json:"metrics"`
}

type performanceSeriesResponse struct {
	Label  string                     `json:"label"`
	Points []performancePointResponse `json:"points"`
}

type performancePointResponse struct {
	At           time.Time `json:"at"`
	Observations int64     `json:"observations,omitempty"`
	Failures     int64     `json:"failures,omitempty"`
	MeanMillis   float64   `json:"mean_ms,omitempty"`
	MaxMillis    float64   `json:"max_ms,omitempty"`
	Value        float64   `json:"value,omitempty"`
}

func performanceSeries(samples []storage.ServiceSample) []performanceSeriesResponse {
	order := make([]string, 0)
	byLabel := map[string][]performancePointResponse{}
	peaks := map[string]float64{}
	for _, sample := range samples {
		if _, seen := byLabel[sample.Label]; !seen {
			order = append(order, sample.Label)
		}
		point := performancePointResponse{
			At:           sample.SampledAt,
			Observations: sample.Observations,
			Failures:     sample.Failures,
			MeanMillis:   millisecondsDTO(sample.Mean()),
			MaxMillis:    millisecondsDTO(sample.Max),
			Value:        sample.Value,
		}
		byLabel[sample.Label] = append(byLabel[sample.Label], point)
		peaks[sample.Label] = max(peaks[sample.Label], point.MaxMillis, point.Value)
	}
	slices.SortFunc(order, func(left, right string) int {
		if peaks[left] != peaks[right] {
			return cmp.Compare(peaks[right], peaks[left])
		}

		return cmp.Compare(left, right)
	})
	series := make([]performanceSeriesResponse, 0, len(order))
	for _, label := range order {
		series = append(series, performanceSeriesResponse{Label: label, Points: byLabel[label]})
	}

	return series
}
