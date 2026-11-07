package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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

	// buildWith wires a service with no panel against the GitHub handler the
	// spec wants, and points its heartbeat wherever the spec wants
	buildWith := func(heartbeatURL string, api http.Handler) *server {
		GinkgoHelper()

		endpoint := httptest.NewServer(api)
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

		// Short enough that a spec can watch several beats
		srv.heartbeatInterval = 10 * time.Millisecond

		return srv
	}

	build := func(heartbeatURL string) *server {
		GinkgoHelper()

		return buildWith(heartbeatURL, newGitHubStub())
	}

	// tick runs one maintenance dispatch and reports the retry boundary it
	// left behind, which is how the lane says whether it got through
	tick := func(srv *server) time.Time {
		GinkgoHelper()

		retryAt := time.Time{}
		srv.tryDispatchMaintenanceQueue(GinkgoT().Context(), &retryAt)

		return retryAt
	}

	// runLoop starts the real loop, which is the only place the beat lives
	runLoop := func(srv *server) {
		GinkgoHelper()

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

	// turns is how many times the lane has been round
	turns := func(srv *server) int {
		GinkgoHelper()

		const marker = "smyklot_maintenance_cycle_duration_seconds_count "

		for line := range strings.SplitSeq(scrape(srv), "\n") {
			count, found := strings.CutPrefix(line, marker)
			if !found {
				continue
			}

			parsed, err := strconv.Atoi(strings.TrimSpace(count))
			Expect(err).NotTo(HaveOccurred())

			return parsed
		}

		return 0
	}

	Describe("the beat", func() {
		It("beats while the lane has nothing to do", func() {
			runLoop(build(monitor.URL))

			Eventually(hits.Load).Should(BeNumerically(">=", 2))
		})

		It("beats while background work is paused", func() {
			// Paused is idle on purpose, not down, and a monitor cannot tell
			// either apart from silence
			srv := build(monitor.URL)
			srv.runtimeBackgroundWorkPaused = true

			runLoop(srv)

			Eventually(hits.Load).Should(BeNumerically(">=", 2))
		})

		It("beats with reaction polling switched off", func() {
			// The beat is the loop's own ticker, so it does not depend on the
			// poll interval - which an operator can set to zero at runtime,
			// long after any check at startup
			srv := build(monitor.URL)
			srv.runtimePollInterval = 0

			runLoop(srv)

			Eventually(hits.Load).Should(BeNumerically(">=", 2))
		})

		It("leaves the pending turn alone while it beats", func() {
			// The beat used to stop the timer, so the next pass armed a whole
			// fresh interval. One beat a minute against a five-minute poll
			// meant the turn never arrived and no reaction was ever swept,
			// while the monitor went on reporting the service healthy
			srv := build(monitor.URL)
			srv.runtimePollInterval = 40 * srv.heartbeatInterval

			runLoop(srv)

			Eventually(hits.Load).Should(BeNumerically(">=", 5))
			Eventually(func() int { return turns(srv) }).Should(BeNumerically(">=", 1))
		})

		It("sends nothing when no heartbeat is configured", func() {
			runLoop(build(""))

			Consistently(hits.Load, 150*time.Millisecond).Should(BeZero())
		})

		It("keeps beating when the monitor refuses the ping", func() {
			status.Store(http.StatusInternalServerError)

			runLoop(build(monitor.URL))

			Eventually(hits.Load).Should(BeNumerically(">=", 2))
			Eventually(func() map[string]any {
				return logs.find("heartbeat refused")
			}).ShouldNot(BeNil())
		})

		It("keeps the token out of the log when the ping fails", func() {
			// A *url.Error prints the URL it failed on, and the token is in
			// that URL. The redactor matches the configured string exactly,
			// which a re-serialised URL need not, so the address is stripped
			// at source
			srv := build(unreachableHeartbeat)
			srv.pingHeartbeat(GinkgoT().Context())

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

			srv := build(redirector.URL + "/api/v1/heartbeat/" + heartbeatToken)
			srv.pingHeartbeat(GinkgoT().Context())

			Expect(leaked.Load()).To(BeZero())
			Expect(logs.find("heartbeat refused")).NotTo(BeNil())
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

	Describe("the cycle metrics", func() {
		It("counts and times a turn of the maintenance lane", func() {
			srv := build("")

			tick(srv)

			body := scrape(srv)
			Expect(body).To(ContainSubstring(`smyklot_maintenance_cycles_total{result="success"} 1`))
			Expect(body).To(ContainSubstring("smyklot_maintenance_cycle_duration_seconds_count 1"))
		})

		It("counts a turn that did no work", func() {
			// Measured above the panel branch, which is why the old
			// sweeps_total read zero for ever on a deployment with a panel: a
			// count kept inside runSweep only moves where there is no panel
			srv := build("")
			srv.runtimeBackgroundWorkPaused = true

			tick(srv)

			Expect(scrape(srv)).To(ContainSubstring(
				`smyklot_maintenance_cycles_total{result="success"} 1`))
		})

		It("counts a sweep that failed as a failed turn", func() {
			// runSweep used to log its error and return nothing, so a 401 on
			// the installation list arrived here as a successful turn
			refusing := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			})

			srv := buildWith("", refusing)

			retryAt := tick(srv)

			Expect(scrape(srv)).To(ContainSubstring(
				`smyklot_maintenance_cycles_total{result="failure"} 1`))
			Expect(retryAt.IsZero()).To(BeFalse())
		})
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
