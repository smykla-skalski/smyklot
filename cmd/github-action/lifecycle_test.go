package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/githubtest"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

// freePort asks the kernel for a port nothing is using, so a spec that binds a
// real listener cannot collide with another
func freePort() int {
	GinkgoHelper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).NotTo(HaveOccurred())

	port := listener.Addr().(*net.TCPAddr).Port
	Expect(listener.Close()).To(Succeed())

	return port
}

var _ = Describe("Service lifecycle [Unit]", func() {
	var (
		stub         *githubStub
		address      string
		adminAddress string
		srv          *server
	)

	BeforeEach(func() {
		stub = newGitHubStub()

		endpoint := httptest.NewServer(stub)
		DeferCleanup(endpoint.Close)

		address = fmt.Sprintf("127.0.0.1:%d", freePort())
		adminAddress = fmt.Sprintf("127.0.0.1:%d", freePort())

		var err error

		srv, err = newServer(&serveConfig{
			database:      GinkgoT().TempDir() + "/state.sqlite3",
			listenAddress: address,
			adminAddress:  adminAddress,
			webhookPath:   defaultWebhookPath,
			webhookSecret: []byte(testSecret),
			apiBaseURL:    endpoint.URL,
			botUsername:   defaultBotUsername,
			appClientID:   "Iv1.test",
			appPrivateKey: githubtest.AppPrivateKey(),
			botConfig:     config.Default(),
			logWriter:     io.Discard,
			// A sweep on every tick would race the spec's own assertions
			pollInterval: time.Hour,
		})
		Expect(err).NotTo(HaveOccurred())
	})

	// get fetches one path and reports the status, or 0 when nothing answered
	get := func(base, path string) int {
		resp, err := http.Get("http://" + base + path) //nolint:noctx // probe in a spec
		if err != nil {
			return 0
		}
		defer func() { _ = resp.Body.Close() }()

		return resp.StatusCode
	}

	// A rolling update sends SIGTERM. Run must come back rather than hang, or
	// the orchestrator kills the process with deliveries still in the queue
	It("should serve until its context is cancelled and then return", func() {
		ctx, cancel := context.WithCancel(GinkgoT().Context())

		result := make(chan error, 1)
		go func() {
			defer GinkgoRecover()

			result <- srv.Run(ctx)
		}()

		Eventually(func() int { return get(address, healthPath) }, 5*time.Second).
			Should(Equal(http.StatusOK))

		cancel()

		Eventually(result, 5*time.Second).Should(Receive(BeNil()))
	})

	It("should serve the admin routes on their own port", func() {
		ctx, cancel := context.WithCancel(GinkgoT().Context())
		DeferCleanup(cancel)

		result := make(chan error, 1)
		go func() {
			defer GinkgoRecover()

			result <- srv.Run(ctx)
		}()

		Eventually(func() int { return get(adminAddress, livePath) }, 5*time.Second).
			Should(Equal(http.StatusOK))

		Expect(get(adminAddress, metricsPath)).To(Equal(http.StatusOK))
		Expect(get(adminAddress, failuresPath)).To(Equal(http.StatusOK))

		// The stub answers /app, so the first probe finds GitHub
		// reachable and readiness turns from unready to ready
		Eventually(func() int { return get(adminAddress, readyPath) }, 5*time.Second).
			Should(Equal(http.StatusOK))

		// None of it is on the port GitHub talks to
		Expect(get(address, metricsPath)).To(Equal(http.StatusNotFound))
		Expect(get(address, failuresPath)).To(Equal(http.StatusNotFound))
		Expect(get(address, readyPath)).To(Equal(http.StatusNotFound))

		cancel()

		Eventually(result, 5*time.Second).Should(Receive(BeNil()))
	})

	It("should report a listen address it cannot bind", func() {
		blocker, err := net.Listen("tcp", address)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = blocker.Close() })

		Expect(srv.Run(GinkgoT().Context())).To(HaveOccurred())
	})

	// One listener dying must take the other with it, or the service keeps
	// accepting deliveries with nothing left to report how it is doing
	It("should stop serving webhooks when the admin listener cannot bind", func() {
		blocker, err := net.Listen("tcp", adminAddress)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = blocker.Close() })

		Expect(srv.Run(GinkgoT().Context())).To(HaveOccurred())
		// Another parallel suite may claim the released port. What matters is
		// that this server's unconditional health response is gone.
		Expect(get(address, healthPath)).NotTo(Equal(http.StatusOK))
	})
})
