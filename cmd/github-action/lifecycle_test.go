package main

import (
	"context"
	"fmt"
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
		stub    *githubStub
		address string
		srv     *server
	)

	BeforeEach(func() {
		stub = newGitHubStub()

		endpoint := httptest.NewServer(stub)
		DeferCleanup(endpoint.Close)

		address = fmt.Sprintf("127.0.0.1:%d", freePort())

		var err error

		srv, err = newServer(&serveConfig{
			listenAddress: address,
			webhookPath:   defaultWebhookPath,
			webhookSecret: []byte(testSecret),
			apiBaseURL:    endpoint.URL,
			botUsername:   defaultBotUsername,
			appClientID:   "Iv1.test",
			appPrivateKey: githubtest.AppPrivateKey(),
			botConfig:     config.Default(),
			// A sweep on every tick would race the spec's own assertions
			pollInterval: time.Hour,
		})
		Expect(err).NotTo(HaveOccurred())
	})

	// A rolling update sends SIGTERM. Run must come back rather than hang, or
	// the orchestrator kills the process with deliveries still in the queue
	It("should serve until its context is cancelled and then return", func() {
		ctx, cancel := context.WithCancel(GinkgoT().Context())

		result := make(chan error, 1)
		go func() {
			defer GinkgoRecover()

			result <- srv.Run(ctx)
		}()

		Eventually(func() int {
			resp, err := http.Get("http://" + address + healthPath) //nolint:noctx // probe in a spec
			if err != nil {
				return 0
			}
			defer func() { _ = resp.Body.Close() }()

			return resp.StatusCode
		}, 5*time.Second).Should(Equal(http.StatusOK))

		cancel()

		Eventually(result, 5*time.Second).Should(Receive(BeNil()))
	})

	It("should report a listen address it cannot bind", func() {
		blocker, err := net.Listen("tcp", address)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = blocker.Close() })

		Expect(srv.Run(GinkgoT().Context())).To(HaveOccurred())
	})
})
