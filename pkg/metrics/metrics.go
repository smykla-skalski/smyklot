// Package metrics counts what the service does, in the form Prometheus scrapes.
//
// The questions it answers are the ones a log cannot: how many deliveries
// arrived in the last hour, what share of them failed, how long they took, and
// whether the queue is filling up.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// namespace prefixes every metric, so a shared Prometheus can tell this
// service's series from anything else on the same cluster.
const namespace = "smyklot"

// What happened to a webhook request, as the outcome label.
const (
	// OutcomeAccepted means the delivery was claimed and queued
	OutcomeAccepted = "accepted"

	// OutcomeDuplicate means the delivery repeated one already claimed
	OutcomeDuplicate = "duplicate"

	// OutcomeRefused means the queue was full. A rising count here is the
	// signal to raise the worker count or the queue depth
	OutcomeRefused = "refused"

	// OutcomeIgnored means the delivery carried nothing to execute
	OutcomeIgnored = "ignored"

	// OutcomeInvalid means the payload could not be parsed or used
	OutcomeInvalid = "invalid"

	// OutcomeUnsigned means the signature was missing or wrong. Anything but
	// zero here is either a misconfigured secret or someone probing the port
	OutcomeUnsigned = "unsigned"
)

// How a piece of work ended, as the result label.
const (
	// ResultSuccess means the work ran to completion
	ResultSuccess = "success"

	// ResultFailure means the work returned an error
	ResultFailure = "failure"
)

// Metrics holds every series the service reports.
type Metrics struct {
	// WebhookRequests counts deliveries by event and by what was done with
	// them, which is how a service that quietly rejects everything shows up
	WebhookRequests *prometheus.CounterVec

	// Deliveries counts executed deliveries by comment action and outcome
	Deliveries *prometheus.CounterVec

	// DeliveryDuration measures how long executing one delivery took
	DeliveryDuration *prometheus.HistogramVec

	// DeliveriesInFlight is how many deliveries are executing right now
	DeliveriesInFlight prometheus.Gauge

	// Sweeps counts reaction sweeps by outcome
	Sweeps *prometheus.CounterVec

	// SweepDuration measures how long one sweep took. A sweep that outgrows
	// the interval delays the next one, and this is where that shows
	SweepDuration prometheus.Histogram
}

// New builds the metrics and registers them.
//
// Taking a registerer rather than using the default one keeps a test's counts
// to itself, and lets the service serve only its own series.
func New(reg prometheus.Registerer) *Metrics {
	factory := promauto.With(reg)

	return &Metrics{
		WebhookRequests: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "webhook_requests_total",
			Help:      "Webhook deliveries received, by event and what was done with them.",
		}, []string{"event", "outcome"}),

		Deliveries: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "deliveries_total",
			Help:      "Deliveries executed, by comment action and result.",
		}, []string{"action", "result"}),

		DeliveryDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "delivery_duration_seconds",
			Help:      "Time spent executing one delivery.",
			Buckets:   prometheus.ExponentialBuckets(0.1, 2, 12),
		}, []string{"action"}),

		DeliveriesInFlight: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "deliveries_in_flight",
			Help:      "Deliveries executing right now.",
		}),

		Sweeps: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "sweeps_total",
			Help:      "Reaction sweeps run, by result.",
		}, []string{"result"}),

		SweepDuration: factory.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "sweep_duration_seconds",
			Help:      "Time spent on one reaction sweep.",
			Buckets:   prometheus.ExponentialBuckets(0.5, 2, 10),
		}),
	}
}

// RegisterReadiness reports whether the service can reach GitHub.
//
// Read at scrape time from whatever already knows the answer, so the metric
// cannot drift from the endpoint that serves the same fact.
func RegisterReadiness(reg prometheus.Registerer, ready func() bool) {
	promauto.With(reg).NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "ready",
		Help:      "1 while the GitHub API is reachable, 0 otherwise.",
	}, func() float64 {
		if ready() {
			return 1
		}

		return 0
	})
}

// RegisterQueue reports the work waiting for a worker.
//
// Depth is read at scrape time rather than tracked on every send, so the queue
// stays the one source of truth for its own length.
func RegisterQueue(reg prometheus.Registerer, depth func() float64, capacity int) {
	factory := promauto.With(reg)

	factory.NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "queue_depth",
		Help:      "Deliveries waiting for a worker.",
	}, depth)

	factory.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "queue_capacity",
		Help:      "Deliveries the queue holds before it refuses more.",
	}).Set(float64(capacity))
}

// NewRegistry builds a registry holding this process's own series plus the Go
// runtime's.
//
// The runtime metrics are what answer "is it wedged": a goroutine count that
// only climbs is a leak, and a heap that only climbs is the next outage.
func NewRegistry() *prometheus.Registry {
	reg := prometheus.NewRegistry()

	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	return reg
}

// Handler serves the registry in the exposition format Prometheus reads.
func Handler(gatherer prometheus.Gatherer) http.Handler {
	return promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{
		// A collector that fails must not blank the whole scrape, or one broken
		// series hides every healthy one
		ErrorHandling: promhttp.ContinueOnError,
	})
}
