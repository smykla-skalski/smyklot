package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/githubtest"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

// returnBudget is how long Run gets to come back after its context is
// cancelled.
//
// Comfortably above five seconds, because http.Server.Shutdown has a five
// second floor and Run cannot get out from under it: a connection the server
// has accepted but read no request header from counts as idle only once it is
// five seconds old (net/http, golang/go#22682), and everything before then
// keeps Shutdown polling. Nothing cancels that wait - shutdownCtx is built
// from context.Background() on purpose, so the listeners drain whether the
// service was asked to stop or died on its own.
//
// A budget of five seconds sat exactly on that floor and failed in CI on a
// loaded runner. Every other step out of Run selects on ctx.Done(), so the
// shutdown is the only place seconds can be spent, and a hang there still
// fails loudly: Shutdown gives up after shutdownTimeout and Run reports the
// deadline rather than nil.
const returnBudget = 10 * time.Second

// loopbackListener keeps ownership of the kernel-selected port until the
// service starts serving it. Closing a free-port probe before Run introduces a
// race with every other process on the test host.
func loopbackListener() net.Listener {
	GinkgoHelper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { _ = listener.Close() })

	return listener
}

// useListeners lets Run own ports the spec kept bound. Returning them in call
// order also proves Run keeps the public and admin handlers on the right ports.
func useListeners(srv *server, listeners ...net.Listener) {
	GinkgoHelper()

	next := 0
	srv.listen = func(ctx context.Context, _, _ string) (net.Listener, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if next >= len(listeners) {
			return nil, fmt.Errorf("unexpected listener request %d", next+1)
		}

		listener := listeners[next]
		next++

		return listener, nil
	}
}

var _ = Describe("Service lifecycle [Unit]", func() {
	var (
		stub *githubStub
		srv  *server
	)

	BeforeEach(func() {
		stub = newGitHubStub()

		endpoint := httptest.NewServer(stub)
		DeferCleanup(endpoint.Close)

		var err error

		srv, err = newServer(&serveConfig{
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
		webhooks := loopbackListener()
		admin := loopbackListener()
		useListeners(srv, webhooks, admin)

		result := make(chan error, 1)
		go func() {
			defer GinkgoRecover()

			result <- srv.Run(ctx)
		}()

		Eventually(func() int { return get(webhooks.Addr().String(), healthPath) }, 5*time.Second).
			Should(Equal(http.StatusOK))

		cancel()

		Eventually(result, returnBudget).Should(Receive(BeNil()))
	})

	It("should cancel listener setup with its context", func() {
		ctx, cancel := context.WithCancel(GinkgoT().Context())
		started := make(chan struct{})
		result := make(chan error, 1)
		srv.listen = func(ctx context.Context, _, _ string) (net.Listener, error) {
			close(started)
			<-ctx.Done()

			return nil, ctx.Err()
		}

		go func() {
			defer GinkgoRecover()

			result <- srv.Run(ctx)
		}()

		Eventually(started).Should(BeClosed())
		cancel()

		Eventually(result, returnBudget).Should(Receive(BeNil()))
	})

	It("should cancel admin listener setup and release the webhook listener", func() {
		ctx, cancel := context.WithCancel(GinkgoT().Context())
		webhooks := loopbackListener()
		adminStarted := make(chan struct{})
		result := make(chan error, 1)
		next := 0
		srv.listen = func(ctx context.Context, _, _ string) (net.Listener, error) {
			if next == 0 {
				next++

				return webhooks, nil
			}

			close(adminStarted)
			<-ctx.Done()

			return nil, ctx.Err()
		}

		go func() {
			defer GinkgoRecover()

			result <- srv.Run(ctx)
		}()

		Eventually(adminStarted).Should(BeClosed())
		cancel()

		Eventually(result, returnBudget).Should(Receive(BeNil()))
		Expect(errors.Is(webhooks.Close(), net.ErrClosed)).To(BeTrue())
	})

	It("should own ephemeral listeners until its context is cancelled", func() {
		ctx, cancel := context.WithCancel(GinkgoT().Context())
		result := make(chan error, 1)

		go func() {
			defer GinkgoRecover()

			result <- srv.Run(ctx)
		}()

		Consistently(result, 200*time.Millisecond).ShouldNot(Receive())
		cancel()

		Eventually(result, returnBudget).Should(Receive(BeNil()))
	})

	It("should serve the admin routes on their own port", func() {
		ctx, cancel := context.WithCancel(GinkgoT().Context())
		DeferCleanup(cancel)
		webhooks := loopbackListener()
		admin := loopbackListener()
		useListeners(srv, webhooks, admin)

		result := make(chan error, 1)
		go func() {
			defer GinkgoRecover()

			result <- srv.Run(ctx)
		}()

		Eventually(func() int { return get(admin.Addr().String(), livePath) }, 5*time.Second).
			Should(Equal(http.StatusOK))

		Expect(get(admin.Addr().String(), metricsPath)).To(Equal(http.StatusOK))
		Expect(get(admin.Addr().String(), failuresPath)).To(Equal(http.StatusOK))

		// The stub answers /app, so the first probe finds GitHub
		// reachable and readiness turns from unready to ready
		Eventually(func() int { return get(admin.Addr().String(), readyPath) }, 5*time.Second).
			Should(Equal(http.StatusOK))

		// None of it is on the port GitHub talks to
		Expect(get(webhooks.Addr().String(), metricsPath)).To(Equal(http.StatusNotFound))
		Expect(get(webhooks.Addr().String(), failuresPath)).To(Equal(http.StatusNotFound))
		Expect(get(webhooks.Addr().String(), readyPath)).To(Equal(http.StatusNotFound))

		cancel()

		Eventually(result, returnBudget).Should(Receive(BeNil()))
	})

	It("should report a listen address it cannot bind", func() {
		blocker := loopbackListener()
		srv.cfg.listenAddress = blocker.Addr().String()

		Expect(srv.Run(GinkgoT().Context())).To(MatchError(ContainSubstring("listen for webhooks")))
	})

	It("should report an admin address it cannot bind", func() {
		blocker := loopbackListener()
		srv.cfg.adminAddress = blocker.Addr().String()

		Expect(srv.Run(GinkgoT().Context())).To(MatchError(ContainSubstring("listen for admin")))
	})

	// One listener dying must take the other with it, or the service keeps
	// accepting deliveries with nothing left to report how it is doing.
	It("should stop serving webhooks when the admin listener dies", func() {
		ctx, cancel := context.WithCancel(GinkgoT().Context())
		DeferCleanup(cancel)
		webhooks := loopbackListener()
		admin := loopbackListener()
		result := make(chan error, 1)

		go func() {
			defer GinkgoRecover()

			result <- srv.runWithListeners(ctx, webhooks, admin)
		}()

		Eventually(func() int { return get(webhooks.Addr().String(), healthPath) }, 5*time.Second).
			Should(Equal(http.StatusOK))

		Expect(admin.Close()).To(Succeed())
		Eventually(result, returnBudget).Should(Receive(HaveOccurred()))

		Eventually(func() int { return get(webhooks.Addr().String(), healthPath) }).
			ShouldNot(Equal(http.StatusOK))
	})
})
