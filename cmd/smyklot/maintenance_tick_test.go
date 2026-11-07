package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/githubtest"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/metrics"
)

// unreachableHeartbeat is a port nothing listens on, so Do fails before it
// ever gets a status to read. The path carries a token, because that is where
// a monitor puts one and it is what must not reach a log line.
const (
	unreachableHeartbeat = "http://127.0.0.1:1/api/v1/heartbeat/" + heartbeatToken
	heartbeatToken       = "s3cret-beat-token"
)

var _ = Describe("Maintenance tick [Unit]", func() {
	var (
		logs    *syncBuffer
		hits    atomic.Int64
		leaked  atomic.Int64
		status  atomic.Int64
		monitor *httptest.Server
	)

	BeforeEach(func() {
		hits.Store(0)
		leaked.Store(0)
		status.Store(http.StatusOK)

		monitor = httptest.NewServer(http.HandlerFunc(
			func(w http.ResponseWriter, _ *http.Request) {
				hits.Add(1)
				w.WriteHeader(int(status.Load()))
			},
		))
		DeferCleanup(monitor.Close)
	})

	// build wires a service and points its heartbeat wherever the spec wants
	build := func(heartbeatURL string) *server {
		GinkgoHelper()

		endpoint := httptest.NewServer(newGitHubStub())
		DeferCleanup(endpoint.Close)

		logs = &syncBuffer{}

		srv, err := newServer(&serveConfig{
			database:      GinkgoT().TempDir() + "/state.sqlite3",
			listenAddress: "127.0.0.1:0",
			adminAddress:  "127.0.0.1:0",
			webhookPath:   defaultWebhookPath,
			webhookSecret: []byte(testSecret),
			apiBaseURL:    endpoint.URL,
			botUsername:   bot.DefaultBotUsername,
			appClientID:   "Iv1.test",
			appPrivateKey: githubtest.AppPrivateKey(),
			botConfig:     config.Default(),
			logFormat:     "json",
			logWriter:     logs,
			heartbeatURL:  heartbeatURL,
		})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(srv.Close()).To(Succeed()) })

		return srv
	}

	// tick runs one maintenance dispatch and reports the retry boundary it
	// left behind, which is how the lane says whether it got through
	tick := func(srv *server) time.Time {
		GinkgoHelper()

		retryAt := time.Time{}
		srv.tryDispatchMaintenanceQueue(GinkgoT().Context(), &retryAt)

		return retryAt
	}

	// scrape returns what a Prometheus scrape of this service would read
	scrape := func(srv *server) string {
		GinkgoHelper()

		rec := httptest.NewRecorder()
		metrics.Handler(srv.registry).ServeHTTP(
			rec, httptest.NewRequest(http.MethodGet, "/metrics", nil),
		)
		Expect(rec.Code).To(Equal(http.StatusOK))

		return rec.Body.String()
	}

	It("counts and times a turn of the maintenance lane", func() {
		srv := build("")

		tick(srv)

		body := scrape(srv)
		Expect(body).To(ContainSubstring(`smyklot_maintenance_cycles_total{result="success"} 1`))
		Expect(body).To(ContainSubstring("smyklot_maintenance_cycle_duration_seconds_count 1"))
	})

	It("counts a turn that did no work", func() {
		// The same hole the heartbeat had, and the reason the old
		// sweeps_total read zero for ever on a deployment with a panel: a
		// count kept inside runSweep only moves where there is no panel
		srv := build("")
		srv.runtimeBackgroundWorkPaused = true

		tick(srv)

		Expect(scrape(srv)).To(ContainSubstring(
			`smyklot_maintenance_cycles_total{result="success"} 1`))
	})

	It("pings the monitor after a maintenance tick", func() {
		tick(build(monitor.URL))

		Expect(hits.Load()).To(BeNumerically("==", 1))
	})

	It("pings the monitor on a tick that did no work", func() {
		// The one that matters, and it says two things. The ping sits above
		// everything the lane might do, so it cannot end up on a branch that
		// a given deployment never takes - a panel sends the lane through the
		// durable queue and never reaches runSweep at all. And a paused
		// service is idle on purpose, so it has to keep beating or pausing
		// background work would page somebody
		srv := build(monitor.URL)
		srv.runtimeBackgroundWorkPaused = true

		tick(srv)

		Expect(hits.Load()).To(BeNumerically("==", 1))
	})

	It("sends nothing when no heartbeat is configured", func() {
		tick(build(""))

		Expect(hits.Load()).To(BeZero())
	})

	It("keeps the lane going when the monitor refuses the ping", func() {
		status.Store(http.StatusInternalServerError)

		retryAt := tick(build(monitor.URL))

		Expect(hits.Load()).To(BeNumerically("==", 1))
		Expect(retryAt.IsZero()).To(BeTrue())
		Expect(logs.find("heartbeat refused")).NotTo(BeNil())
	})

	It("keeps the lane going when the monitor cannot be reached", func() {
		retryAt := tick(build(unreachableHeartbeat))

		Expect(retryAt.IsZero()).To(BeTrue())
		Expect(logs.find("could not send the heartbeat")).NotTo(BeNil())
	})

	It("keeps the token out of the log when the ping fails", func() {
		// A *url.Error prints the URL it failed on, and the token is in that
		// URL. The redactor matches the configured string exactly, which a
		// re-serialised URL need not, so the address is stripped at source
		tick(build(unreachableHeartbeat))

		line := logs.find("could not send the heartbeat")
		Expect(line).NotTo(BeNil())

		text, ok := line["error"].(string)
		Expect(ok).To(BeTrue())
		Expect(text).NotTo(ContainSubstring(heartbeatToken))
		Expect(text).NotTo(ContainSubstring("/api/v1/heartbeat"))
	})

	It("does not follow a redirect, so the token stays on one host", func() {
		// A 302 would hand the token to whatever host the response names
		elsewhere := httptest.NewServer(http.HandlerFunc(
			func(w http.ResponseWriter, _ *http.Request) {
				leaked.Add(1)
				w.WriteHeader(http.StatusOK)
			},
		))
		DeferCleanup(elsewhere.Close)

		redirector := httptest.NewServer(http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, elsewhere.URL, http.StatusFound)
			},
		))
		DeferCleanup(redirector.Close)

		tick(build(redirector.URL + "/api/v1/heartbeat/" + heartbeatToken))

		Expect(leaked.Load()).To(BeZero())
		Expect(logs.find("heartbeat refused")).NotTo(BeNil())
	})

	It("keeps beating while background work is paused", func() {
		// Driven through pollLoop rather than the dispatch helper, because
		// pausing used to park the loop on a select with no timer: the lane
		// stopped turning, the beat stopped with it, and a service idle on
		// purpose was reported as an outage
		srv := build(monitor.URL)
		srv.runtimeBackgroundWorkPaused = true
		srv.runtimePollInterval = 20 * time.Millisecond

		ctx, cancel := context.WithCancel(GinkgoT().Context())
		stopped := make(chan struct{})

		go func() {
			defer close(stopped)
			srv.pollLoop(ctx)
		}()
		DeferCleanup(func() {
			cancel()
			Eventually(stopped).Should(BeClosed())
		})

		Eventually(hits.Load).Should(BeNumerically(">=", 2))
	})

	It("sends nothing once the context is cancelled", func() {
		// A stop must not log a missed beat on its way out
		srv := build(monitor.URL)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		srv.pingHeartbeat(ctx)

		Expect(hits.Load()).To(BeZero())
		Expect(logs.find("could not send the heartbeat")).To(BeNil())
	})
})

var _ = DescribeTable("Heartbeat URL validation [Unit]",
	func(raw string, usable bool) {
		Expect(heartbeatURLUsable(raw)).To(Equal(usable))
	},
	Entry("empty switches the heartbeat off", "", true),
	Entry("https with a host", "https://uptime.example.com/api/v1/heartbeat/abc", true),
	Entry("http with a host", "http://127.0.0.1:9000/beat", true),
	Entry("no scheme", "uptime.example.com/beat", false),
	Entry("no host", "https:///beat", false),
	Entry("a port with no host", "https://:443/beat", false),
	Entry("a scheme nothing here can send", "ftp://uptime.example.com/beat", false),
	Entry("not a URL at all", "http://[::1", false),
)
