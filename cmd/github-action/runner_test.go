package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/spf13/cobra"

	"github.com/smykla-skalski/smyklot/internal/githubtest"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

// codeownersPath is the read a sweep makes only once it has decided to act
const codeownersPath = "/contents/.github/CODEOWNERS"

var _ = Describe("Choosing an entry point [Unit]", func() {
	Describe("the Action", func() {
		// A deleted /approve posts exactly one notice when the Action acts, so
		// an empty recorder is the whole assertion: it did nothing at all
		standsDown := func(repoConfig string, env map[string]string) bool {
			GinkgoHelper()

			recorder := &commentRecorder{repoConfig: repoConfig}

			_, err := runCommentOn(recorder, "deleted", "/approve", env)
			Expect(err).NotTo(HaveOccurred())

			return len(recorder.posted()) == 0
		}

		// The whole cutover: a repository with a workflow file and nothing else
		// is served by the service, and the Action leaves the comment alone
		It("should stand down for a repository that says nothing", func() {
			Expect(standsDown("", map[string]string{envRunner: ""})).To(BeTrue())
		})

		It("should act for a repository that names it", func() {
			Expect(standsDown("runner: action\n", map[string]string{envRunner: ""})).To(BeFalse())
		})

		// The rollback direction. A repository pins itself to the Action in its
		// own file, and no workflow variable is needed to make that stick
		It("should let the repository's file beat the environment", func() {
			Expect(standsDown("runner: service\n", map[string]string{
				envRunner: string(config.RunnerAction),
			})).To(BeTrue())

			Expect(standsDown("runner: action\n", map[string]string{
				envRunner: string(config.RunnerService),
			})).To(BeFalse())
		})

		// A repository that goes silent needs somewhere to look. The pull
		// request stays clean, because the service has already reacted on it
		It("should leave the reason in the job summary", func() {
			summary := filepath.Join(GinkgoT().TempDir(), "summary.md")

			Expect(standsDown("", map[string]string{
				envRunner:      "",
				envStepSummary: summary,
			})).To(BeTrue())

			written, err := os.ReadFile(summary) //nolint:gosec // path built by the spec
			Expect(err).NotTo(HaveOccurred())
			Expect(string(written)).To(ContainSubstring("Stood down"))
			Expect(string(written)).To(ContainSubstring("service"))
		})

		// Silence is what standing down looks like, so a misspelled runner must
		// not be able to imitate it
		It("should report a runner it does not know", func() {
			recorder := &commentRecorder{repoConfig: "runner: workflow\n"}

			_, err := runCommentOn(recorder, "deleted", "/approve", nil)
			Expect(err).To(MatchError(ErrRepoConfigInvalid))

			Expect(recorder.posted()).To(HaveLen(1))
			Expect(recorder.posted()[0].Body).To(ContainSubstring("smyklot.yaml"))
		})

		// The environment is the other way a runner is set, and a repository
		// without the file never reaches the code that reads one
		It("should refuse to start on a runner the environment does not name", func() {
			_, err := runComment("deleted", "/approve", map[string]string{
				envRunner: "workflow",
			})

			Expect(err).To(MatchError(ErrConfigLoad))
			Expect(err).To(MatchError(ContainSubstring("workflow")))
		})
	})

	// The cron sweep reads the repository's file for the first time in this
	// change, so what it does with a file it cannot use is new behaviour
	Describe("the Action's sweep", func() {
		// runSweep drives runPoll end to end against a stub repository
		runSweep := func(stub *githubStub, env map[string]string) error {
			GinkgoHelper()

			endpoint := httptest.NewServer(stub)
			DeferCleanup(endpoint.Close)

			for _, name := range runEnv {
				GinkgoT().Setenv(name, "")
			}

			settings := map[string]string{
				envGitHubToken: "test-token",
				envAPIBaseURL:  endpoint.URL,
				envRepoOwner:   "smykla-skalski",
				envRepoName:    "smyklot",
				envRunner:      string(config.RunnerAction),
			}
			for key, value := range env {
				settings[key] = value
			}

			for key, value := range settings {
				GinkgoT().Setenv(key, value)
			}

			cmd := &cobra.Command{}
			cmd.Flags().StringP(flagPollRepo, "r", "", descPollRepo)
			cmd.Flags().StringP(flagPollToken, "t", "", descPollToken)
			cmd.SetContext(context.Background())

			return runPoll(cmd, nil)
		}

		// Falling back to defaults would restore commands the repository had
		// turned off, so the sweep stops. Failing rather than exiting quietly
		// is what stops a repository losing reaction commands unnoticed
		It("should stop on a file it cannot use", func() {
			stub := newGitHubStub()
			stub.repoConfig = "- a list, not a mapping\n"

			Expect(runSweep(stub, nil)).To(MatchError(ErrRepoConfigInvalid))
			Expect(stub.countCalls(http.MethodGet, codeownersPath)).To(BeZero())
		})

		It("should say why in the job summary", func() {
			summary := filepath.Join(GinkgoT().TempDir(), "summary.md")

			stub := newGitHubStub()
			stub.repoConfig = "- a list, not a mapping\n"

			Expect(runSweep(stub, map[string]string{envStepSummary: summary})).To(HaveOccurred())

			written, err := os.ReadFile(summary) //nolint:gosec // path built by the spec
			Expect(err).NotTo(HaveOccurred())
			Expect(string(written)).To(ContainSubstring("smyklot.yaml"))
		})

		It("should sweep a repository whose file is fine", func() {
			stub := newGitHubStub()

			Expect(runSweep(stub, nil)).To(Succeed())
			Expect(stub.countCalls(http.MethodGet, codeownersPath)).To(Equal(1))
		})
	})

	Describe("the service", func() {
		var (
			stub    *githubStub
			service *httptest.Server
			srv     *server
		)

		// configTTL is how long the built service trusts a repository's file.
		// A spec that changes the file mid-run shortens it, because the real
		// value is measured in seconds and a spec cannot wait that long
		configTTL := repoConfigTTL

		start := func() {
			GinkgoHelper()

			endpoint := httptest.NewServer(stub)
			DeferCleanup(endpoint.Close)

			var err error

			srv, err = newServer(&serveConfig{
				listenAddress: "127.0.0.1:0",
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

			srv.configs = newRepoCache(configTTL, fetchRepositoryConfig)

			workers := srv.startWorkers()
			DeferCleanup(func() {
				srv.closeQueue()
				workers.Wait()
			})

			service = httptest.NewServer(srv.handler())
			DeferCleanup(service.Close)
		}

		BeforeEach(func() {
			stub = newGitHubStub()
			configTTL = repoConfigTTL
		})

		It("should leave a comment alone in a repository pinned to the Action", func() {
			stub.repoConfig = "runner: action\n"
			start()

			deliverAccepted(service, webhook.EventIssueComment, deliveryOne,
				commandDelivery("/approve"))

			// The delivery still has to be read and the config still has to be
			// fetched, so waiting on the config read is what proves the decision
			// was reached rather than merely not reached yet
			Eventually(func() int {
				return stub.countCalls(http.MethodGet, "/contents/.github/smyklot.yaml")
			}, eventuallyWindow).Should(Equal(1))

			Consistently(func() int {
				return stub.countCalls(http.MethodPost, approveReviews)
			}).Should(BeZero())
		})

		It("should still approve in a repository that says nothing", func() {
			start()

			deliverAccepted(service, webhook.EventIssueComment, deliveryOne,
				commandDelivery("/approve"))

			Eventually(func() int {
				return stub.countCalls(http.MethodPost, approveReviews)
			}, eventuallyWindow).Should(Equal(1))
		})

		// The rollback the documentation promises. The Action reads the file on
		// its very next run, because a workflow is a fresh process; if this
		// process kept serving a cached answer, both would act on the same
		// comment for as long as the entry lived - the one thing the setting
		// exists to stop
		It("should stop acting once the repository rolls back to the Action", func() {
			configTTL = time.Millisecond
			start()

			deliverAccepted(service, webhook.EventIssueComment, deliveryOne,
				commandDelivery("/approve"))

			Eventually(func() int {
				return stub.countCalls(http.MethodPost, approveReviews)
			}, eventuallyWindow).Should(Equal(1))

			stub.setRepoConfig("runner: action\n")

			// A later updated_at, because the deduper keys on the comment and
			// its revision rather than the delivery identifier. Sending the
			// same comment again would be refused as a repeat and the spec
			// would pass without the file ever being read a second time
			edited := delivery("edited", "/approve", "User", "2026-01-01T00:00:01Z", true)

			deliverAccepted(service, webhook.EventIssueComment, deliveryTwo, edited)

			Eventually(func() int {
				return stub.countCalls(http.MethodGet, "/contents/.github/smyklot.yaml")
			}, eventuallyWindow).Should(BeNumerically(">=", 2))

			Consistently(func() int {
				return stub.countCalls(http.MethodPost, approveReviews)
			}).Should(Equal(1))
		})

		// Guards the constant rather than the mechanism. Reusing the
		// CODEOWNERS lifetime here, as one shared value once did, is what put a
		// rolled-back repository an hour behind the Action
		It("should not trust the file for longer than a sweep", func() {
			Expect(repoConfigTTL).To(BeNumerically("<", defaultPollInterval))
			Expect(repoConfigTTL).To(BeNumerically("<", codeownersTTL))
		})

		// The sweep is the other half. A repository left on the Action would
		// otherwise keep having its reactions acted on by both
		It("should skip a repository pinned to the Action while sweeping", func() {
			stub.installations = `[{"id": 987, "account": {"login": "smykla-skalski"}}]`
			stub.repos = `{"total_count": 1, "repositories": [
				{"name": "smyklot", "owner": {"login": "smykla-skalski"}}
			]}`
			stub.repoConfig = "runner: action\n"

			start()

			Expect(srv.sweep(context.Background())).To(Succeed())

			Expect(stub.countCalls(http.MethodGet, codeownersPath)).To(BeZero())
		})

		It("should sweep a repository that says nothing", func() {
			stub.installations = `[{"id": 987, "account": {"login": "smykla-skalski"}}]`
			stub.repos = `{"total_count": 1, "repositories": [
				{"name": "smyklot", "owner": {"login": "smykla-skalski"}}
			]}`

			start()

			Expect(srv.sweep(context.Background())).To(Succeed())

			Expect(stub.countCalls(http.MethodGet, codeownersPath)).To(Equal(1))
		})
	})
})
