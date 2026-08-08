package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/githubtest"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

// syncBuffer collects log output a worker writes while the spec reads it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

// lines returns every log line decoded from JSON.
func (b *syncBuffer) lines() []map[string]any {
	GinkgoHelper()

	var out []map[string]any

	for _, raw := range strings.Split(strings.TrimSpace(b.String()), "\n") {
		if raw == "" {
			continue
		}

		var decoded map[string]any
		Expect(json.Unmarshal([]byte(raw), &decoded)).To(Succeed())

		out = append(out, decoded)
	}

	return out
}

// find returns the first line whose message matches.
func (b *syncBuffer) find(message string) map[string]any {
	for _, line := range b.lines() {
		if line["msg"] == message {
			return line
		}
	}

	return nil
}

var _ = Describe("Service observability [Unit]", func() {
	var (
		stub    *githubStub
		logs    *syncBuffer
		srv     *server
		service *httptest.Server
		admin   *httptest.Server
	)

	// start wires a service to the stub with its logs captured and both
	// handlers served, so specs read exactly what an operator would
	start := func() {
		GinkgoHelper()

		endpoint := httptest.NewServer(stub)
		DeferCleanup(endpoint.Close)

		logs = &syncBuffer{}

		var err error

		srv, err = newServer(&serveConfig{
			listenAddress: "127.0.0.1:0",
			adminAddress:  "127.0.0.1:0",
			webhookPath:   defaultWebhookPath,
			webhookSecret: []byte(testSecret),
			apiBaseURL:    endpoint.URL,
			botUsername:   defaultBotUsername,
			appClientID:   "Iv1.test",
			appPrivateKey: githubtest.AppPrivateKey(),
			botConfig:     config.Default(),
			logFormat:     "json",
			logWriter:     logs,
		})
		Expect(err).NotTo(HaveOccurred())

		workers := srv.startWorkers()
		DeferCleanup(func() {
			srv.closeQueue()
			workers.Wait()
		})

		service = httptest.NewServer(srv.handler())
		DeferCleanup(service.Close)

		admin = httptest.NewServer(srv.adminHandler())
		DeferCleanup(admin.Close)
	}

	post := func(event, deliveryID string, body []byte, signature *string) *http.Response {
		GinkgoHelper()

		return postDelivery(service, event, deliveryID, body, signature)
	}

	// read fetches an admin route and returns its status and body
	read := func(path string) (int, string) {
		GinkgoHelper()

		req, err := http.NewRequestWithContext(GinkgoT().Context(), http.MethodGet, admin.URL+path, nil)
		Expect(err).NotTo(HaveOccurred())

		resp, err := http.DefaultClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(resp.Body.Close)

		body := &bytes.Buffer{}
		_, err = body.ReadFrom(resp.Body)
		Expect(err).NotTo(HaveOccurred())

		return resp.StatusCode, body.String()
	}

	BeforeEach(func() {
		stub = newGitHubStub()
	})

	Describe("tracing one delivery", func() {
		BeforeEach(func() { start() })

		// The point of the whole logging change: everything one webhook caused
		// can be found by its identifier alone
		It("should name the delivery, repository and pull request on the lines it produces", func() {
			post(webhook.EventIssueComment, deliveryOne, commandDelivery("/approve"), nil)

			Eventually(func() map[string]any {
				return logs.find("delivery executed")
			}, eventuallyWindow).ShouldNot(BeNil())

			executed := logs.find("delivery executed")
			Expect(executed).To(HaveKeyWithValue("delivery_id", deliveryOne))
			Expect(executed).To(HaveKeyWithValue("repo", "smykla-skalski/smyklot"))
			Expect(executed).To(HaveKeyWithValue("pr", float64(42)))
			Expect(executed).To(HaveKeyWithValue("action", "created"))
		})

		// The identifier arrives on a header the signature does not cover, so
		// whatever the sender put there reaches the log unless it is cleaned
		It("should log only the safe part of the identifier", func() {
			post(webhook.EventIssueComment, `abc "injected"`, commandDelivery("/approve"), nil)

			Eventually(func() map[string]any {
				return logs.find("delivery executed")
			}, eventuallyWindow).ShouldNot(BeNil())

			Expect(logs.find("delivery executed")).To(HaveKeyWithValue("delivery_id", "abcinjected"))
		})

		DescribeTable("should reduce an identifier to what cannot forge a log line",
			func(raw, expected string) {
				Expect(safeDeliveryID(raw)).To(Equal(expected))
			},
			Entry("a real GitHub identifier survives untouched",
				"7d1b0f00-1234-11ef-9f6b-0242ac120002", "7d1b0f00-1234-11ef-9f6b-0242ac120002"),
			Entry("a newline is removed", "abc\ninjected", "abcinjected"),
			Entry("a carriage return is removed", "abc\r\nlevel=ERROR", "abclevelERROR"),
			Entry("an empty identifier becomes a placeholder", "", "unknown"),
			Entry("nothing usable becomes a placeholder", "\n\t", "unknown"),
			Entry("an overlong identifier is cut", strings.Repeat("a", 200), strings.Repeat("a", 64)),
		)
	})

	Describe("readiness", func() {
		It("should report unready until the first check finishes", func() {
			start()

			status, body := read(readyPath)
			Expect(status).To(Equal(http.StatusServiceUnavailable))
			Expect(body).To(ContainSubstring(reasonUnchecked))
		})

		It("should report ready once GitHub answers", func() {
			start()
			srv.probe(GinkgoT().Context())

			status, body := read(readyPath)
			Expect(status).To(Equal(http.StatusOK))
			Expect(body).To(ContainSubstring(`"ready":true`))
		})

		// A restart cannot fix GitHub being down, so liveness must keep passing
		// while readiness fails
		It("should stay alive but report unready when GitHub cannot be reached", func() {
			stub.probeStatus = http.StatusInternalServerError
			start()

			srv.probe(GinkgoT().Context())

			status, body := read(readyPath)
			Expect(status).To(Equal(http.StatusServiceUnavailable))
			Expect(body).To(ContainSubstring("500"))

			liveStatus, _ := read(livePath)
			Expect(liveStatus).To(Equal(http.StatusOK))
		})

		It("should say so once rather than on every check", func() {
			stub.probeStatus = http.StatusInternalServerError
			start()

			srv.probe(GinkgoT().Context())
			srv.probe(GinkgoT().Context())

			Expect(strings.Count(logs.String(), `"msg":"not ready"`)).To(Equal(1))
		})

		// A prober that died would otherwise leave the last answer standing
		// forever, and the service would look healthy while it checked nothing
		It("should treat an answer that stopped being refreshed as no answer", func() {
			start()

			srv.readiness.set("")
			Expect(srv.readiness.state().Ready).To(BeTrue())

			srv.readiness.checkedAt = time.Now().Add(-readyStaleAfter - time.Second)

			Expect(srv.readiness.state().Ready).To(BeFalse())
			Expect(srv.readiness.state().Reason).To(Equal(reasonStale))
		})

		// A process paused past the staleness window and then resumed went from
		// not-ready to ready as far as every reader is concerned, even though
		// GitHub never stopped answering. A recovery nothing logged is one
		// nobody can line up against the dip in the metric
		It("should announce a recovery from staleness even when the result is unchanged", func() {
			start()

			Expect(srv.readiness.set("")).To(BeTrue())
			Expect(srv.readiness.set("")).To(BeFalse())

			srv.readiness.checkedAt = time.Now().Add(-readyStaleAfter - time.Second)

			Expect(srv.readiness.set("")).To(BeTrue())
			Expect(srv.readiness.state().Ready).To(BeTrue())
		})
	})

	Describe("metrics", func() {
		BeforeEach(func() { start() })

		It("should count a delivery it executed", func() {
			post(webhook.EventIssueComment, deliveryOne, commandDelivery("/approve"), nil)

			Eventually(func() string {
				_, body := read(metricsPath)

				return body
			}, eventuallyWindow).Should(ContainSubstring(
				`smyklot_deliveries_total{action="created",result="success"} 1`))

			_, body := read(metricsPath)
			Expect(body).To(ContainSubstring(
				`smyklot_webhook_requests_total{event="issue_comment",outcome="accepted"} 1`))
			Expect(body).To(ContainSubstring(`smyklot_delivery_duration_seconds_count{action="created"} 1`))
		})

		// A service quietly rejecting everything because its secret was rotated
		// looks exactly like a service nobody is using
		It("should count a delivery it could not verify", func() {
			signature := "sha256=deadbeef"
			post(webhook.EventIssueComment, deliveryOne, commandDelivery("/approve"), &signature)

			_, body := read(metricsPath)
			Expect(body).To(ContainSubstring(
				`smyklot_webhook_requests_total{event="issue_comment",outcome="unsigned"} 1`))
		})

		It("should count a redelivery as a duplicate rather than as work", func() {
			payload := commandDelivery("/approve")

			post(webhook.EventIssueComment, deliveryOne, payload, nil)

			Eventually(func() string {
				_, body := read(metricsPath)

				return body
			}, eventuallyWindow).Should(ContainSubstring(`outcome="accepted"} 1`))

			post(webhook.EventIssueComment, deliveryTwo, payload, nil)

			_, body := read(metricsPath)
			Expect(body).To(ContainSubstring(`outcome="duplicate"} 1`))
		})

		// The event header is not covered by the signature, so passing it
		// through would let anyone mint a time series per request
		It("should not create a series per event name", func() {
			post("a-made-up-event", deliveryOne, []byte(`{}`), nil)

			_, body := read(metricsPath)
			Expect(body).To(ContainSubstring(`event="other"`))
			Expect(body).ToNot(ContainSubstring("a-made-up-event"))
		})

		It("should report the queue and the Go runtime", func() {
			_, body := read(metricsPath)

			Expect(body).To(ContainSubstring("smyklot_queue_depth 0"))
			Expect(body).To(ContainSubstring("smyklot_queue_capacity 256"))
			Expect(body).To(ContainSubstring("go_goroutines"))
		})
	})

	Describe("recent failures", func() {
		// GitHub's own delivery log shows a 202, because the work happens after
		// the answer. Without this, a failure leaves no trace an operator can
		// reach
		It("should record a delivery that failed, with its repository and reason", func() {
			stub.brokenRepo = "smyklot"
			start()

			post(webhook.EventIssueComment, deliveryOne, commandDelivery("/approve"), nil)

			Eventually(func() string {
				_, body := read(failuresPath)

				return body
			}, eventuallyWindow).Should(ContainSubstring(deliveryOne))

			_, body := read(failuresPath)

			var failures []deliveryFailure
			Expect(json.Unmarshal([]byte(body), &failures)).To(Succeed())
			Expect(failures).To(HaveLen(1))

			Expect(failures[0].Repository).To(Equal("smykla-skalski/smyklot"))
			Expect(failures[0].PullRequest).To(Equal(42))
			Expect(failures[0].Action).To(Equal("created"))
			Expect(failures[0].Reason).ToNot(BeEmpty())
		})

		It("should serve an empty list when nothing has failed", func() {
			start()

			status, body := read(failuresPath)
			Expect(status).To(Equal(http.StatusOK))
			Expect(strings.TrimSpace(body)).To(Equal("[]"))
		})

		It("should keep the newest failures and drop the oldest", func() {
			log := newFailureLog(2)

			log.Record(deliveryFailure{DeliveryID: "one"})
			log.Record(deliveryFailure{DeliveryID: "two"})
			log.Record(deliveryFailure{DeliveryID: "three"})

			recorded := log.Snapshot()
			Expect(recorded).To(HaveLen(2))
			Expect(recorded[0].DeliveryID).To(Equal("three"))
			Expect(recorded[1].DeliveryID).To(Equal("two"))
		})
	})

	Describe("secrets", func() {
		BeforeEach(func() { start() })

		// The guard is wired from the service's own configuration, so a
		// credential that leaks into an error message is caught wherever that
		// message surfaces
		It("should keep the webhook secret out of the log", func() {
			srv.logger.Info("pretend leak", "value", testSecret)

			Expect(logs.String()).ToNot(ContainSubstring(testSecret))
			Expect(logs.String()).To(ContainSubstring("[REDACTED]"))
		})

		It("should keep the private key out of the log", func() {
			srv.logger.Error("pretend leak", "error", errors.New(string(githubtest.AppPrivateKey())))

			Expect(logs.String()).ToNot(ContainSubstring("PRIVATE KEY"))
		})

		It("should keep secrets out of a recorded failure", func() {
			Expect(srv.redactor.Error(errors.New("rejected " + testSecret))).
				To(Equal("rejected [REDACTED]"))
		})
	})
})
