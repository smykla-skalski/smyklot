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
)

// unreachableHeartbeat is a port nothing listens on, so Do fails before it
// ever gets a status to read.
const unreachableHeartbeat = "http://127.0.0.1:1/heartbeat"

var _ = Describe("Heartbeat [Unit]", func() {
	var (
		logs    *syncBuffer
		hits    atomic.Int64
		status  atomic.Int64
		monitor *httptest.Server
	)

	BeforeEach(func() {
		hits.Store(0)
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
	Entry("a scheme nothing here can send", "ftp://uptime.example.com/beat", false),
	Entry("not a URL at all", "http://[::1", false),
)
