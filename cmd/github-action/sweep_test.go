package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/githubtest"
	adminpanel "github.com/smykla-skalski/smyklot/internal/panel"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

var _ = Describe("Reaction sweep [Unit]", func() {
	var (
		stub     *githubStub
		endpoint *httptest.Server
		service  *server
	)

	start := func() {
		GinkgoHelper()

		endpoint = httptest.NewServer(stub)
		DeferCleanup(endpoint.Close)

		srv, err := newServer(&serveConfig{
			webhookPath:   defaultWebhookPath,
			webhookSecret: []byte(testSecret),
			apiBaseURL:    endpoint.URL,
			botUsername:   defaultBotUsername,
			appClientID:   "Iv1.test",
			appPrivateKey: githubtest.AppPrivateKey(),
			botConfig:     config.Default(),
			logWriter:     io.Discard,
		})
		Expect(err).NotTo(HaveOccurred())

		service = srv
	}

	BeforeEach(func() {
		stub = newGitHubStub()
	})

	// GitHub sends no webhook for a reaction, so this sweep is the only way
	// reaction commands are ever found
	It("should poll every repository of every installation", func() {
		stub.installations = `[
			{"id": 111, "account": {"login": "smykla-skalski"}},
			{"id": 222, "account": {"login": "someone-else"}}
		]`
		stub.repos = `{
			"total_count": 2,
			"repositories": [
				{"name": "smyklot", "owner": {"login": "smykla-skalski"}},
				{"name": "sai", "owner": {"login": "smykla-skalski"}}
			]
		}`
		start()

		Expect(service.sweep(GinkgoT().Context())).To(Succeed())

		Expect(stub.countCalls(http.MethodGet, "/app/installations")).To(Equal(1))
		Expect(stub.countCalls(http.MethodGet, "/installation/repositories")).To(Equal(2))
		Expect(stub.countCalls(http.MethodGet, "/repos/smykla-skalski/smyklot/pulls")).To(Equal(2))
		Expect(stub.countCalls(http.MethodGet, "/repos/smykla-skalski/sai/pulls")).To(Equal(2))
	})

	// The installation list comes from GitHub, so a repository installed while
	// the process runs is picked up without a restart or any configuration
	It("should pick up an installation added between sweeps", func() {
		start()

		Expect(service.sweep(GinkgoT().Context())).To(Succeed())
		Expect(stub.countCalls(http.MethodGet, "/installation/repositories")).To(BeZero())

		stub.installations = `[{"id": 111, "account": {"login": "smykla-skalski"}}]`

		Expect(service.sweep(GinkgoT().Context())).To(Succeed())
		Expect(stub.countCalls(http.MethodGet, "/installation/repositories")).To(Equal(1))
	})

	It("should apply a new reaction sweep interval without restarting", func() {
		start()
		ctx, cancel := context.WithCancel(GinkgoT().Context())
		stopped := make(chan struct{})
		go func() {
			defer close(stopped)
			service.pollLoop(ctx)
		}()

		Consistently(func() int {
			return stub.countCalls(http.MethodGet, "/app/installations")
		}, 30*time.Millisecond).Should(BeZero())

		service.ApplyRuntimeSettings(adminpanel.RuntimeValues{
			BotConfig: config.Default(), LogLevel: slog.LevelInfo,
			PollInterval: 10 * time.Millisecond, SessionTTL: time.Hour,
		})
		Eventually(func() int {
			return stub.countCalls(http.MethodGet, "/app/installations")
		}, time.Second).Should(BeNumerically(">", 0))

		cancel()
		Eventually(stopped).Should(BeClosed())
	})

	It("should process reactions on the open PRs it finds", func() {
		stub.installations = `[{"id": 111, "account": {"login": "smykla-skalski"}}]`
		stub.repos = `{
			"total_count": 1,
			"repositories": [{"name": "smyklot", "owner": {"login": "smykla-skalski"}}]
		}`
		stub.openPRs = `[{
			"number": 42,
			"state": "open",
			"title": "a change",
			"user": {"login": "author"},
			"labels": []
		}]`
		start()

		Expect(service.sweep(GinkgoT().Context())).To(Succeed())

		Expect(stub.countCalls(http.MethodGet, "/repos/smykla-skalski/smyklot/issues/42/reactions")).
			To(Equal(1))
	})

	// Without caching these two files, a sweep re-reads them for every
	// repository on every tick, forever, for content that changes far less
	// often than it is looked at
	It("should read a repository's CODEOWNERS and config once across sweeps", func() {
		stub.installations = `[{"id": 111, "account": {"login": "smykla-skalski"}}]`
		stub.repos = `{
			"total_count": 1,
			"repositories": [{"name": "smyklot", "owner": {"login": "smykla-skalski"}}]
		}`
		start()

		for range 3 {
			Expect(service.sweep(GinkgoT().Context())).To(Succeed())
		}

		Expect(stub.countCalls(http.MethodGet, "/contents/.github/CODEOWNERS")).To(Equal(1))
		Expect(stub.countCalls(http.MethodGet, "/contents/.github/smyklot.yaml")).To(Equal(1))

		// The pull request list is what a sweep is actually for, so it is read
		// every time
		Expect(stub.countCalls(http.MethodGet, "/repos/smykla-skalski/smyklot/pulls")).To(Equal(3))
	})

	// One installation revoking access must not silence every other one
	It("should keep sweeping after a repository fails", func() {
		stub.installations = `[{"id": 111, "account": {"login": "smykla-skalski"}}]`
		stub.repos = `{
			"total_count": 2,
			"repositories": [
				{"name": "gone", "owner": {"login": "smykla-skalski"}},
				{"name": "smyklot", "owner": {"login": "smykla-skalski"}}
			]
		}`
		stub.brokenRepo = "gone"
		start()

		Expect(service.sweep(GinkgoT().Context())).To(Succeed())

		Expect(stub.countCalls(http.MethodGet, "/repos/smykla-skalski/smyklot/pulls")).To(Equal(1))
	})

	It("should report a failure to list installations", func() {
		failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// The token store still needs to mint, or the sweep fails earlier
			// than the case under test
			if strings.HasSuffix(r.URL.Path, "/access_tokens") {
				w.WriteHeader(http.StatusCreated)
				_, _ = fmt.Fprintf(w, `{"token": "t", "expires_at": %q}`,
					time.Now().Add(time.Hour).UTC().Format(time.RFC3339))

				return
			}

			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message": "Bad credentials"}`))
		}))
		DeferCleanup(failing.Close)

		srv, err := newServer(&serveConfig{
			webhookSecret: []byte(testSecret),
			apiBaseURL:    failing.URL,
			botUsername:   defaultBotUsername,
			appClientID:   "Iv1.test",
			appPrivateKey: githubtest.AppPrivateKey(),
			botConfig:     config.Default(),
			logWriter:     io.Discard,
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(srv.sweep(GinkgoT().Context())).To(MatchError(ErrListInstallations))
	})
})
