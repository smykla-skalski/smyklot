package metrics_test

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/smykla-skalski/smyklot/pkg/metrics"
)

var _ = Describe("Metrics [Unit]", func() {
	var (
		reg *prometheus.Registry
		met *metrics.Metrics
	)

	BeforeEach(func() {
		reg = prometheus.NewRegistry()
		met = metrics.New(reg)
	})

	// scrape returns what a Prometheus scrape of reg would read
	scrape := func() string {
		GinkgoHelper()

		rec := httptest.NewRecorder()
		metrics.Handler(reg).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

		Expect(rec.Code).To(Equal(http.StatusOK))

		return rec.Body.String()
	}

	It("reports a delivery outcome under its labels", func() {
		met.WebhookRequests.WithLabelValues("issue_comment", metrics.OutcomeAccepted).Inc()

		Expect(scrape()).To(ContainSubstring(
			`smyklot_webhook_requests_total{event="issue_comment",outcome="accepted"} 1`))
	})

	It("keeps a rejected delivery apart from an accepted one", func() {
		met.WebhookRequests.WithLabelValues("issue_comment", metrics.OutcomeUnsigned).Inc()
		met.WebhookRequests.WithLabelValues("issue_comment", metrics.OutcomeAccepted).Inc()
		met.WebhookRequests.WithLabelValues("issue_comment", metrics.OutcomeAccepted).Inc()

		Expect(scrape()).To(ContainSubstring(`outcome="unsigned"} 1`))
		Expect(scrape()).To(ContainSubstring(`outcome="accepted"} 2`))
	})

	It("counts a failed delivery apart from a successful one", func() {
		met.Deliveries.WithLabelValues("created", metrics.ResultFailure).Inc()

		Expect(scrape()).To(ContainSubstring(
			`smyklot_deliveries_total{action="created",result="failure"} 1`))
	})

	It("reports delivery duration as a histogram", func() {
		met.DeliveryDuration.WithLabelValues("created").Observe(0.25)

		Expect(scrape()).To(ContainSubstring(`smyklot_delivery_duration_seconds_count{action="created"} 1`))
		Expect(scrape()).To(ContainSubstring(`smyklot_delivery_duration_seconds_bucket{action="created",le="0.4"} 1`))
	})

	// Read at scrape time so the metric cannot drift from the endpoint that
	// serves the same fact
	It("reads readiness at scrape time", func() {
		ready := false
		metrics.RegisterReadiness(reg, func() bool { return ready })

		Expect(scrape()).To(ContainSubstring("smyklot_ready 0"))

		ready = true

		Expect(scrape()).To(ContainSubstring("smyklot_ready 1"))
	})

	It("reads the queue length at scrape time", func() {
		depth := 0.0
		metrics.RegisterQueue(reg, func() float64 { return depth }, 256)

		Expect(scrape()).To(ContainSubstring("smyklot_queue_depth 0"))
		Expect(scrape()).To(ContainSubstring("smyklot_queue_capacity 256"))

		depth = 7

		Expect(scrape()).To(ContainSubstring("smyklot_queue_depth 7"))
	})

	It("includes Go runtime series so a leak is visible", func() {
		Expect(metrics.Handler(metrics.NewRegistry())).ToNot(BeNil())

		rec := httptest.NewRecorder()
		metrics.Handler(metrics.NewRegistry()).ServeHTTP(
			rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

		Expect(rec.Body.String()).To(ContainSubstring("go_goroutines"))
	})
})
