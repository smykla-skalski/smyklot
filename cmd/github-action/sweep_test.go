package main

import (
	"context"
	"errors"
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
	"github.com/smykla-skalski/smyklot/internal/storage"
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
			database:      GinkgoT().TempDir() + "/state.sqlite3",
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

		Eventually(func() int {
			return stub.countCalls(http.MethodGet, "/app/installations")
		}, time.Second).Should(Equal(1))
		Consistently(func() int {
			return stub.countCalls(http.MethodGet, "/app/installations")
		}, 30*time.Millisecond).Should(Equal(1))

		service.ApplyRuntimeSettings(adminpanel.RuntimeValues{
			BotConfig: config.Default(), LogLevel: slog.LevelInfo,
			PollInterval: 10 * time.Millisecond, SessionTTL: time.Hour,
		})
		Eventually(func() int {
			return stub.countCalls(http.MethodGet, "/app/installations")
		}, time.Second).Should(BeNumerically(">", 1))

		cancel()
		Eventually(stopped).Should(BeClosed())
	})

	It("should drain pre-durable labels when reaction polling is disabled", func() {
		stub.installations = `[{"id":111,"account":{"login":"smykla-skalski"}}]`
		stub.repos = `{"total_count":1,"repositories":[{
			"id":123456,
			"name":"smyklot",
			"full_name":"smykla-skalski/smyklot",
			"owner":{"login":"smykla-skalski"}
		}]}`
		stub.openPRs = `[{
			"number":42,
			"state":"open",
			"head":{"sha":"legacy-head"},
			"base":{"ref":"main"},
			"labels":[{"name":"smyklot:pending-ci:squash"}]
		}]`
		start()
		ctx, cancel := context.WithCancel(GinkgoT().Context())
		stopped := make(chan struct{})
		go func() {
			defer close(stopped)
			service.pollLoop(ctx)
		}()

		Eventually(func() []int {
			return []int{
				stub.countCalls(http.MethodGet, "/app/installations"),
				stub.countCalls(http.MethodGet, "/installation/repositories"),
				stub.countCalls(http.MethodGet, "/repos/smykla-skalski/smyklot/pulls"),
				stub.countCalls(http.MethodPost, "/issues/42/comments"),
			}
		}).Within(eventuallyWindow).Should(Equal([]int{1, 1, 1, 1}))
		Consistently(func() int {
			return stub.countCalls(http.MethodGet, "/issues/42/reactions")
		}, 30*time.Millisecond).Should(BeZero())

		cancel()
		Eventually(stopped).Should(BeClosed())
	})

	It("should retry pre-durable cleanup independently of reaction polling", func() {
		stub.installations = `not-json`
		stub.repos = `{"total_count":1,"repositories":[{
			"id":123456,
			"name":"smyklot",
			"full_name":"smykla-skalski/smyklot",
			"owner":{"login":"smykla-skalski"}
		}]}`
		stub.openPRs = `[{"number":42,"state":"open",
			"head":{"sha":"legacy-head"},"base":{"ref":"main"},
			"labels":[{"name":"smyklot:pending-ci:squash"}]}]`
		start()
		service.migrationRetryDelay = 10 * time.Millisecond
		ctx, cancel := context.WithCancel(GinkgoT().Context())
		stopped := make(chan struct{})
		go func() {
			defer close(stopped)
			service.pollLoop(ctx)
		}()

		Eventually(func() int {
			return stub.countCalls(http.MethodGet, "/app/installations")
		}, time.Second).Should(Equal(1))
		stub.setInstallations(`[{"id":111,"account":{"login":"smykla-skalski"}}]`)
		Eventually(func() int {
			return stub.countCalls(http.MethodPost, "/issues/42/comments")
		}, eventuallyWindow).Should(Equal(1))
		Expect(stub.countCalls(http.MethodGet, "/app/installations")).To(BeNumerically(">", 1))

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

	It("should terminalize service requests before handing a repository to the Action", func() {
		stub.installations = `[{"id":987,"account":{"login":"smykla-skalski"}}]`
		stub.repos = `{"total_count":1,"repositories":[{
			"id":123456,
			"name":"smyklot",
			"full_name":"smykla-skalski/smyklot",
			"owner":{"login":"smykla-skalski"}
		}]}`
		stub.repoConfig = "runner: action\n"
		stub.openPRs = `[{
			"number":42,
			"state":"open",
			"head":{"sha":"command-head"},
			"base":{"ref":"main"},
			"labels":[
				{"name":"smyklot:pending:ci:service"},
				{"name":"smyklot:pending:ci:squash"}
			]
		}]`
		stub.prLabels = `[
			{"name":"smyklot:pending:ci:service"},
			{"name":"smyklot:pending:ci:squash"}
		]`
		start()
		armed := armWebhookTestRequest(service)

		Expect(service.sweep(GinkgoT().Context())).To(Succeed())
		startPendingCITestScheduler(service)
		_, err := service.store.GetArmed(
			GinkgoT().Context(), armed.RepositoryID, armed.PullRequest,
		)
		Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())
		Eventually(func() int {
			return stub.countCalls(
				http.MethodDelete, "/issues/42/labels/smyklot:pending:ci:squash",
			)
		}, eventuallyWindow).Should(Equal(1))
		Eventually(func() int {
			return stub.countCalls(
				http.MethodDelete, "/issues/42/labels/smyklot:pending:ci:service",
			)
		}, eventuallyWindow).Should(Equal(1))
		calls := stub.recordedCalls()
		Expect(indexCall(calls, "DELETE", "/issues/42/labels/smyklot:pending:ci:squash")).
			To(BeNumerically("<", indexCall(
				calls, "DELETE", "/issues/42/labels/smyklot:pending:ci:service",
			)))
	})

	It("should remove orphaned service ownership without draining it as legacy work", func() {
		stub.installations = `[{"id":111,"account":{"login":"smykla-skalski"}}]`
		stub.repos = `{"total_count":1,"repositories":[{
			"id":123456,
			"name":"smyklot",
			"full_name":"smykla-skalski/smyklot",
			"owner":{"login":"smykla-skalski"}
		}]}`
		stub.openPRs = `[{
			"number":42,
			"state":"open",
			"head":{"sha":"orphan-head"},
			"base":{"ref":"main"},
			"labels":[
				{"name":"smyklot:pending:ci:service"},
				{"name":"smyklot:pending:ci:squash"}
			]
		}]`
		start()

		Expect(service.migrationSweep(GinkgoT().Context())).To(Succeed())

		Expect(stub.countCalls(
			http.MethodDelete, "/issues/42/labels/smyklot:pending:ci:squash",
		)).To(Equal(1))
		Expect(stub.countCalls(
			http.MethodDelete, "/issues/42/labels/smyklot:pending:ci:service",
		)).To(Equal(1))
		Expect(stub.countCalls(http.MethodPost, "/issues/42/comments")).To(BeZero())
	})

	It("should preserve ownership for an armed service request", func() {
		stub.installations = `[{"id":987,"account":{"login":"smykla-skalski"}}]`
		stub.repos = `{"total_count":1,"repositories":[{
			"id":123456,
			"name":"smyklot",
			"full_name":"smykla-skalski/smyklot",
			"owner":{"login":"smykla-skalski"}
		}]}`
		stub.openPRs = `[{
			"number":42,
			"state":"open",
			"head":{"sha":"command-head"},
			"base":{"ref":"main"},
			"labels":[
				{"name":"smyklot:pending:ci:service"},
				{"name":"smyklot:pending:ci:squash"}
			]
		}]`
		start()
		_ = armWebhookTestRequest(service)

		Expect(service.migrationSweep(GinkgoT().Context())).To(Succeed())

		Expect(stub.countCalls(
			http.MethodDelete, "/issues/42/labels/smyklot:pending:ci:squash",
		)).To(BeZero())
		Expect(stub.countCalls(
			http.MethodDelete, "/issues/42/labels/smyklot:pending:ci:service",
		)).To(BeZero())
		Expect(stub.countCalls(http.MethodPost, "/issues/42/comments")).To(BeZero())
	})

	It("should safely drain pre-durable pending CI labels once", func() {
		stub.installations = `[{"id":111,"account":{"login":"smykla-skalski"}}]`
		stub.repos = `{
			"total_count": 1,
			"repositories": [{
				"id": 123456,
				"name": "smyklot",
				"full_name": "smykla-skalski/smyklot",
				"owner": {"login": "smykla-skalski"}
			}]
		}`
		stub.openPRs = `[{
			"number": 42,
			"state": "open",
			"title": "a change",
			"user": {"login": "author"},
			"head": {"sha": "unknown-authorized-head"},
			"base": {"ref": "main"},
			"labels": [
				{"name": "smyklot:pending-ci:squash"},
				{"name": "smyklot:pending-ci:rebase"}
			]
		}]`
		stub.prHead = "unknown-authorized-head"
		stub.prLabels = `[
			{"name":"smyklot:pending-ci:squash"},
			{"name":"smyklot:pending-ci:rebase"}
		]`
		start()

		Expect(service.sweep(GinkgoT().Context())).To(Succeed())
		startPendingCITestScheduler(service)
		Eventually(func() int {
			return stub.countCalls(
				http.MethodDelete, "/issues/42/labels/smyklot:pending-ci:squash",
			)
		}).Within(eventuallyWindow).Should(Equal(1))
		Eventually(func() int {
			return stub.countCalls(
				http.MethodDelete, "/issues/42/labels/smyklot:pending-ci:rebase",
			)
		}).Within(eventuallyWindow).Should(Equal(1))
		Expect(stub.countCalls(http.MethodPost, "/issues/42/comments")).To(Equal(1))

		Expect(service.sweep(GinkgoT().Context())).To(Succeed())
		Expect(stub.countCalls(http.MethodPost, "/issues/42/comments")).To(Equal(1))
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

		Expect(service.sweep(GinkgoT().Context())).To(HaveOccurred())

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
			database:      GinkgoT().TempDir() + "/state.sqlite3",
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

func indexCall(calls []string, method, pathSuffix string) int {
	for index, call := range calls {
		if strings.HasPrefix(call, method+" ") && strings.HasSuffix(call, pathSuffix) {
			return index
		}
	}

	return len(calls)
}
