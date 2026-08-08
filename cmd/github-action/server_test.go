package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/githubtest"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

const (
	testSecret       = "a-webhook-secret"
	approveReviews   = "/pulls/42/reviews"
	prCommentsPath   = "/issues/42/comments"
	deliveryOne      = "delivery-1"
	deliveryTwo      = "delivery-2"
	eventuallyWindow = 2 * time.Second
)

// signBody produces the header GitHub sends alongside a delivery
func signBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)

	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// delivery renders an issue_comment payload the way GitHub sends one
func delivery(action, body, authorType, updatedAt string, isPR bool) []byte {
	return githubtest.IssueCommentPayload(githubtest.IssueComment{
		Action:        action,
		Body:          body,
		AuthorType:    authorType,
		UpdatedAt:     updatedAt,
		IsPullRequest: isPR,
	})
}

// commandDelivery is the common case: a user commenting a command on a PR
func commandDelivery(body string) []byte {
	return githubtest.Command(body)
}

var _ = Describe("Webhook service [Unit]", func() {
	var (
		stub     *githubStub
		endpoint *httptest.Server
		service  *httptest.Server
		srv      *server
	)

	// start builds a service wired to the stub, with real workers but no
	// listener of its own, so specs drive the same handler production does
	start := func(botConfig *config.Config) {
		GinkgoHelper()

		endpoint = httptest.NewServer(stub)
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
			botConfig:     botConfig,
		})
		Expect(err).NotTo(HaveOccurred())

		workers := srv.startWorkers()
		DeferCleanup(func() {
			srv.closeQueue()
			workers.Wait()
		})

		service = httptest.NewServer(srv.handler())
		DeferCleanup(service.Close)
	}

	// post sends a delivery, signing it unless the spec supplied its own
	// signature
	post := func(event, deliveryID string, body []byte, signature *string) *http.Response {
		GinkgoHelper()

		req, err := http.NewRequestWithContext(
			GinkgoT().Context(),
			http.MethodPost,
			service.URL+defaultWebhookPath,
			bytes.NewReader(body),
		)
		Expect(err).NotTo(HaveOccurred())

		req.Header.Set(webhook.EventHeader, event)
		req.Header.Set(webhook.DeliveryHeader, deliveryID)

		if signature != nil {
			req.Header.Set(webhook.SignatureHeader, *signature)
		} else {
			req.Header.Set(webhook.SignatureHeader, signBody(testSecret, body))
		}

		resp, err := http.DefaultClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(resp.Body.Close)

		return resp
	}

	BeforeEach(func() {
		stub = newGitHubStub()
	})

	Describe("executing a command", func() {
		BeforeEach(func() {
			start(config.Default())
		})

		// The whole point of the issue: a comment reaches the same execution
		// the Action runs, with no workflow in between
		It("should approve the PR a signed /approve names", func() {
			resp := post(webhook.EventIssueComment, deliveryOne, commandDelivery("/approve"), nil)
			Expect(resp.StatusCode).To(Equal(http.StatusAccepted))

			Eventually(func() int {
				return stub.countCalls(http.MethodPost, approveReviews)
			}, eventuallyWindow).Should(Equal(1))
		})

		It("should mint a token for the installation the delivery names", func() {
			post(webhook.EventIssueComment, deliveryOne, commandDelivery("/approve"), nil)

			Eventually(func() int {
				return stub.countCalls(http.MethodPost, "/app/installations/987/access_tokens")
			}, eventuallyWindow).Should(Equal(1))
		})

		It("should answer help without approving anything", func() {
			post(webhook.EventIssueComment, deliveryOne, commandDelivery("/help"), nil)

			Eventually(func() int {
				return stub.countCalls(http.MethodPost, prCommentsPath)
			}, eventuallyWindow).Should(Equal(1))

			Expect(stub.countCalls(http.MethodPost, approveReviews)).To(BeZero())
		})

		// A redelivery repeats the same comment at the same revision. Acting on
		// it again would post the feedback twice
		It("should act once when the same event is delivered twice", func() {
			payload := commandDelivery("/approve")

			Expect(post(webhook.EventIssueComment, deliveryOne, payload, nil).StatusCode).
				To(Equal(http.StatusAccepted))

			Eventually(func() int {
				return stub.countCalls(http.MethodPost, approveReviews)
			}, eventuallyWindow).Should(Equal(1))

			Expect(post(webhook.EventIssueComment, deliveryTwo, payload, nil).StatusCode).
				To(Equal(http.StatusOK))

			Consistently(func() int {
				return stub.countCalls(http.MethodPost, approveReviews)
			}, 200*time.Millisecond).Should(Equal(1))
		})

		// An edit is a new instruction, not a repeat of the old one
		It("should act again when the comment was edited", func() {
			post(webhook.EventIssueComment, deliveryOne, commandDelivery("/approve"), nil)

			Eventually(func() int {
				return stub.countCalls(http.MethodPost, approveReviews)
			}, eventuallyWindow).Should(Equal(1))

			post(
				webhook.EventIssueComment,
				deliveryTwo,
				delivery("edited", "/approve", "User", "2026-08-08T10:05:00Z", true),
				nil,
			)

			Eventually(func() int {
				return stub.countCalls(http.MethodPost, approveReviews)
			}, eventuallyWindow).Should(Equal(2))
		})
	})

	Describe("rejecting a delivery", func() {
		BeforeEach(func() {
			start(config.Default())
		})

		DescribeTable("should refuse a delivery it cannot verify, and change nothing",
			func(signature string) {
				resp := post(webhook.EventIssueComment, deliveryOne, commandDelivery("/approve"), &signature)

				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
				Consistently(stub.total, 200*time.Millisecond).Should(BeZero())
			},
			Entry("no signature at all", ""),
			Entry("a signature for a different secret",
				signBody("the-wrong-secret", commandDelivery("/approve"))),
			Entry("a signature that is not hex", "sha256=zzzz"),
			Entry("a SHA-1 signature", "sha1=abcdef"),
		)

		// The body is what is signed, so a payload swapped after signing must
		// not get through
		It("should refuse a delivery whose body was changed after signing", func() {
			signature := signBody(testSecret, commandDelivery("/approve"))
			resp := post(webhook.EventIssueComment, deliveryOne, commandDelivery("/merge"), &signature)

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			Consistently(stub.total, 200*time.Millisecond).Should(BeZero())
		})

		It("should reject a malformed payload", func() {
			resp := post(webhook.EventIssueComment, deliveryOne, []byte(`not json`), nil)

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			Consistently(stub.total, 200*time.Millisecond).Should(BeZero())
		})

		It("should reject a payload with no installation to act as", func() {
			resp := post(webhook.EventIssueComment, deliveryOne, []byte(`{
				"action": "created",
				"repository": {"name": "smyklot", "owner": {"login": "smykla-skalski"}}
			}`), nil)

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		})

		It("should reject a comment past the length cap", func() {
			oversized := make([]byte, maxCommentBodyLength+1)
			for i := range oversized {
				oversized[i] = 'a'
			}

			resp := post(webhook.EventIssueComment, deliveryOne, commandDelivery(string(oversized)), nil)

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			Consistently(stub.total, 200*time.Millisecond).Should(BeZero())
		})
	})

	Describe("ignoring a delivery", func() {
		BeforeEach(func() {
			start(config.Default())
		})

		// The bot's own feedback arrives as a delivery too. Acting on it would
		// have the bot answering itself forever
		It("should ignore a comment the bot wrote", func() {
			resp := post(
				webhook.EventIssueComment,
				deliveryOne,
				delivery("created", "/approve", "Bot", "2026-08-08T10:00:00Z", true),
				nil,
			)

			Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
			Consistently(stub.total, 200*time.Millisecond).Should(BeZero())
		})

		It("should ignore a comment on an issue that is not a PR", func() {
			resp := post(
				webhook.EventIssueComment,
				deliveryOne,
				delivery("created", "/approve", "User", "2026-08-08T10:00:00Z", false),
				nil,
			)

			Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
			Consistently(stub.total, 200*time.Millisecond).Should(BeZero())
		})

		It("should acknowledge a ping without doing anything", func() {
			resp := post(webhook.EventPing, deliveryOne, []byte(`{"zen": "Keep it logically awesome."}`), nil)

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Consistently(stub.total, 200*time.Millisecond).Should(BeZero())
		})

		It("should ignore an event it does not handle", func() {
			resp := post("push", deliveryOne, []byte(`{}`), nil)

			Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
			Consistently(stub.total, 200*time.Millisecond).Should(BeZero())
		})
	})

	Describe("permissions", func() {
		// Permission checking is the Action's own code, reached through the
		// same call. This proves the service did not route around it
		It("should refuse a command from someone CODEOWNERS does not list", func() {
			stub.codeowners = "* @someone-else\n"
			start(config.Default())

			post(webhook.EventIssueComment, deliveryOne, commandDelivery("/approve"), nil)

			Eventually(func() int {
				return stub.countCalls(http.MethodPost, prCommentsPath)
			}, eventuallyWindow).Should(Equal(1))

			Expect(stub.countCalls(http.MethodPost, approveReviews)).To(BeZero())
		})

		It("should refuse to let a PR author approve their own PR", func() {
			stub.prAuthor = "someone"
			start(config.Default())

			post(webhook.EventIssueComment, deliveryOne, commandDelivery("/approve"), nil)

			Eventually(func() int {
				return stub.countCalls(http.MethodPost, prCommentsPath)
			}, eventuallyWindow).Should(Equal(1))

			Expect(stub.countCalls(http.MethodPost, approveReviews)).To(BeZero())
		})
	})

	Describe("repository configuration", func() {
		// The service cannot read a workflow's repository variables, so this
		// file is how a repository states its preferences
		It("should honour .github/smyklot.yaml", func() {
			stub.repoConfig = "quiet_success: true\n"
			start(config.Default())

			post(webhook.EventIssueComment, deliveryOne, commandDelivery("/approve"), nil)

			Eventually(func() int {
				return stub.countCalls(http.MethodPost, approveReviews)
			}, eventuallyWindow).Should(Equal(1))

			// quiet_success keeps the reaction and drops the comment
			Consistently(func() int {
				return stub.countCalls(http.MethodPost, prCommentsPath)
			}, 200*time.Millisecond).Should(BeZero())
		})

		It("should post the usual comment for a repository without the file", func() {
			start(config.Default())

			post(webhook.EventIssueComment, deliveryOne, commandDelivery("/approve"), nil)

			Eventually(func() int {
				return stub.countCalls(http.MethodPost, prCommentsPath)
			}, eventuallyWindow).Should(Equal(1))
		})

		// A broken file used to abort before any feedback, so the bot went
		// silent for the whole repository with the reason visible only in the
		// service's own logs
		Context("when the file cannot be parsed", func() {
			BeforeEach(func() {
				stub.repoConfig = "- this is a list, not a mapping\n"
			})

			It("should say so on the pull request rather than go quiet", func() {
				start(config.Default())

				post(webhook.EventIssueComment, deliveryOne, commandDelivery("/approve"), nil)

				Eventually(func() int {
					return stub.countCalls(http.MethodPost, prCommentsPath)
				}, eventuallyWindow).Should(Equal(1))
			})

			// Carrying on with defaults would restore commands the repository
			// had deliberately turned off
			It("should run no command", func() {
				start(config.Default())

				post(webhook.EventIssueComment, deliveryOne, commandDelivery("/approve"), nil)

				Eventually(func() int {
					return stub.countCalls(http.MethodPost, prCommentsPath)
				}, eventuallyWindow).Should(Equal(1))

				Expect(stub.countCalls(http.MethodPost, approveReviews)).To(BeZero())
			})

			// A broken file must not make the bot heckle every conversation in
			// the repository
			It("should stay silent on a comment that asked for nothing", func() {
				start(config.Default())

				post(webhook.EventIssueComment, deliveryOne, commandDelivery("just thinking out loud"), nil)

				Consistently(func() int {
					return stub.countCalls(http.MethodPost, prCommentsPath)
				}, 300*time.Millisecond).Should(BeZero())
			})
		})

		It("should fall back to the process-wide configuration", func() {
			quiet := config.Default()
			quiet.QuietSuccess = true
			start(quiet)

			post(webhook.EventIssueComment, deliveryOne, commandDelivery("/approve"), nil)

			Eventually(func() int {
				return stub.countCalls(http.MethodPost, approveReviews)
			}, eventuallyWindow).Should(Equal(1))

			Consistently(func() int {
				return stub.countCalls(http.MethodPost, prCommentsPath)
			}, 200*time.Millisecond).Should(BeZero())
		})
	})

	// http.Server.Shutdown leaves a handler that is still running alone once its
	// deadline passes, so a delivery can reach the queue after Run has closed
	// it. A bare send would panic there, and the panic would skip the claim
	// release, losing the command and blocking its redelivery for an hour
	Describe("a delivery that arrives during shutdown", func() {
		BeforeEach(func() {
			start(config.Default())
		})

		It("should refuse it rather than panic, and leave it retryable", func() {
			srv.closeQueue()

			resp := post(webhook.EventIssueComment, deliveryOne, commandDelivery("/approve"), nil)
			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable))

			Consistently(stub.total, 200*time.Millisecond).Should(BeZero())

			// The claim was released, so GitHub redelivering after the restart
			// gets a fresh attempt rather than "already handled"
			Expect(srv.deduper.Begin(
				"issue_comment:created:smykla-skalski/smyklot:555:2026-08-08T10:00:00Z",
			)).To(BeTrue())
		})

		It("should be safe to close the queue more than once", func() {
			srv.closeQueue()
			Expect(srv.closeQueue).NotTo(Panic())
		})
	})

	Describe("health", func() {
		BeforeEach(func() {
			start(config.Default())
		})

		It("should answer a liveness probe without a signature", func() {
			req, err := http.NewRequestWithContext(
				GinkgoT().Context(), http.MethodGet, service.URL+healthPath, nil)
			Expect(err).NotTo(HaveOccurred())

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(resp.Body.Close)

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})
	})
})
