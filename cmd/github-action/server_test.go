package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/githubtest"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/storage/storagetest"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

const (
	testSecret       = "a-webhook-secret"
	approveReviews   = "/pulls/42/reviews"
	prCommentsPath   = "/issues/42/comments"
	deliveryOne      = "delivery-1"
	deliveryTwo      = "delivery-2"
	deliveryThree    = "delivery-3"
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

func checkRunDelivery() []byte {
	return []byte(`{
  "action": "completed",
  "check_run": {
    "id": 501,
    "head_sha": "pending-head",
    "status": "completed",
    "conclusion": "success",
    "updated_at": "2026-08-15T12:00:00Z",
    "pull_requests": [{"number": 42}]
  },
  "repository": {
    "id": 123456,
    "name": "smyklot",
    "full_name": "smykla-skalski/smyklot",
    "owner": {"login": "smykla-skalski"}
  },
  "installation": {"id": 987}
}`)
}

func pendingLabelRemovedDelivery() []byte {
	return pendingLabelRemovedDeliveryFor("smyklot:pending:ci:squash")
}

func pendingLabelRemovedDeliveryFor(label string) []byte {
	return fmt.Appendf(nil, `{
	  "action": "unlabeled",
	  "number": 42,
	  "pull_request": {
	    "merged": false,
	    "updated_at": "2026-08-15T12:01:00Z",
	    "head": {"sha": "pending-head"}
	  },
	  "label": {"name": %q},
	  "repository": {
	    "id": 123456,
	    "name": "smyklot",
	    "full_name": "smykla-skalski/smyklot",
	    "owner": {"login": "smykla-skalski"}
	  },
	  "installation": {"id": 987}
	}`, label)
}

func pendingClosedDelivery() []byte {
	return []byte(`{
  "action": "closed",
  "number": 42,
  "pull_request": {
    "merged": false,
    "updated_at": "2026-08-15T12:01:00Z",
    "head": {"sha": "pending-head"}
  },
  "repository": {
    "id": 123456,
    "name": "smyklot",
    "full_name": "smykla-skalski/smyklot",
    "owner": {"login": "smykla-skalski"}
  },
  "installation": {"id": 987}
}`)
}

func armWebhookTestRequest(srv *server) pendingci.Request {
	return armWebhookTestRequestWithLabel(srv, "smyklot:pending:ci:squash")
}

func armWebhookTestRequestWithLabel(srv *server, label string) pendingci.Request {
	GinkgoHelper()
	requestedAt := time.Now().UTC().Add(-time.Minute)
	result, err := srv.store.Arm(GinkgoT().Context(), pendingci.ArmRequest{
		TargetID:           installationStorageID(987),
		InstallationID:     987,
		RepositoryID:       repositoryStorageID(githubtest.DefaultRepoID),
		RepositoryFullName: "smykla-skalski/smyklot",
		PullRequest:        42,
		HeadSHA:            "pending-head",
		BaseBranch:         "main",
		MergeMethod:        pendingci.MergeMethodSquash,
		RequiredChecksOnly: false,
		Requester:          "someone",
		SourceCommentID:    555,
		SourceRevision:     requestedAt.Format(time.RFC3339Nano),
		SourceSequence:     1,
		SourceOrder:        1,
		Label:              label,
		RequestedAt:        requestedAt,
	})
	Expect(err).NotTo(HaveOccurred())

	return result.Request
}

func seedPendingCISource(stub *githubStub, request pendingci.Request, body string) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.issueComments[request.SourceCommentID] = issueCommentRecord{
		exists: true, body: body, updatedAt: request.SourceRevision,
		author: request.Requester, authorType: githubtest.DefaultAuthorTypeVal,
	}
}

func startPendingCITestScheduler(srv *server) {
	GinkgoHelper()
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		srv.pendingCI.Run(ctx)
	}()
	DeferCleanup(func() {
		cancel()
		<-stopped
	})
}

// postDelivery sends a delivery to a running service, signing it unless the
// spec supplied a signature of its own
func postDelivery(
	service *httptest.Server,
	stub *githubStub,
	event, deliveryID string,
	body []byte,
	signature *string,
) *http.Response {
	GinkgoHelper()
	if event == webhook.EventIssueComment {
		stub.observeIssueComment(body)
	}

	return postDeliveryWithoutCommentUpdate(service, event, deliveryID, body, signature)
}

func postDeliveryWithoutCommentUpdate(
	service *httptest.Server,
	event, deliveryID string,
	body []byte,
	signature *string,
) *http.Response {
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

// deliverAccepted posts a delivery and insists the service took it.
//
// A refusal is a legitimate answer here, not a fault: the queue is bounded and
// a claim can be contended, and dispatch releases the claim precisely so that
// GitHub's redelivery gets a fresh try. A spec that posts once and then waits
// on the work is therefore asserting on something the service was allowed not
// to do, and when that happened the failure surfaced two seconds later against
// an unrelated expectation - "expected 1 to be >= 2" for a config read that was
// never going to happen, rather than "the delivery was refused".
//
// So this redelivers the way GitHub does, and says what went wrong when even
// that does not get the delivery taken.
func deliverAccepted(
	service *httptest.Server,
	stub *githubStub,
	event, deliveryID string,
	body []byte,
) {
	GinkgoHelper()

	Eventually(func() int {
		return postDelivery(service, stub, event, deliveryID, body, nil).StatusCode
	}, eventuallyWindow).Should(
		Equal(http.StatusAccepted),
		"the service never accepted delivery %s", deliveryID,
	)
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
			database:      storagetest.Connection(GinkgoT(), GinkgoT().TempDir()),
			listenAddress: "127.0.0.1:0",
			webhookPath:   defaultWebhookPath,
			webhookSecret: []byte(testSecret),
			apiBaseURL:    endpoint.URL,
			botUsername:   defaultBotUsername,
			appClientID:   "Iv1.test",
			appPrivateKey: githubtest.AppPrivateKey(),
			botConfig:     botConfig,
			logWriter:     io.Discard,
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

	post := func(event, deliveryID string, body []byte, signature *string) *http.Response {
		GinkgoHelper()

		return postDelivery(service, stub, event, deliveryID, body, signature)
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

			Eventually(func() int {
				return post(webhook.EventIssueComment, deliveryOne, payload, nil).StatusCode
			}, eventuallyWindow).Should(Equal(http.StatusOK))

			Consistently(func() int {
				return stub.countCalls(http.MethodPost, approveReviews)
			}, 200*time.Millisecond).Should(Equal(1))
		})

		It("should execute a same-second edit whose content cycles back", func() {
			updatedAt := "2026-08-08T10:05:00Z"
			post(
				webhook.EventIssueComment, deliveryOne,
				delivery("edited", "/approve", "User", updatedAt, true), nil,
			)
			Eventually(func() int {
				return stub.countCalls(http.MethodPost, approveReviews)
			}, eventuallyWindow).Should(Equal(1))

			post(
				webhook.EventIssueComment, deliveryTwo,
				delivery("edited", "/help", "User", updatedAt, true), nil,
			)
			post(
				webhook.EventIssueComment, deliveryThree,
				delivery("edited", "/approve", "User", updatedAt, true), nil,
			)

			Eventually(func() int {
				return stub.countCalls(http.MethodPost, approveReviews)
			}, eventuallyWindow).Should(Equal(2))
		})

		It("should ignore an out-of-order same-second edit", func() {
			updatedAt := "2026-08-08T10:05:00Z"
			squash := delivery(
				"edited", "/squash after ci", "User", updatedAt, true,
			)
			staleMerge := delivery(
				"edited", "/merge after ci", "User", updatedAt, true,
			)
			stub.observeIssueComment(squash)
			response := postDeliveryWithoutCommentUpdate(
				service, webhook.EventIssueComment, deliveryOne, squash, nil,
			)
			Expect(response.StatusCode).To(Equal(http.StatusAccepted))

			Eventually(func() string {
				request, err := srv.store.GetArmed(
					GinkgoT().Context(), repositoryStorageID(githubtest.DefaultRepoID),
					githubtest.DefaultPRNumber,
				)
				if err != nil {
					return ""
				}

				return string(request.MergeMethod)
			}, eventuallyWindow).Should(Equal(string(pendingci.MergeMethodSquash)))

			response = postDeliveryWithoutCommentUpdate(
				service, webhook.EventIssueComment, deliveryTwo, staleMerge, nil,
			)
			Expect(response.StatusCode).To(Equal(http.StatusAccepted))
			Eventually(func() int {
				return stub.countCalls(http.MethodGet, "/issues/comments/555")
			}, eventuallyWindow).Should(BeNumerically(">=", 2))
			request, err := srv.store.GetArmed(
				GinkgoT().Context(), repositoryStorageID(githubtest.DefaultRepoID),
				githubtest.DefaultPRNumber,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(request.MergeMethod).To(Equal(pendingci.MergeMethodSquash))
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

		It("should not resurrect a command from an older comment revision", func() {
			/* The newer revision carries a command with a visible effect, because
			   the older delivery must not be posted until the newer revision has
			   been recorded, and the approval is the only thing here that happens
			   after the claim.

			   Waiting on the access token waited on something that happens before
			   it: the delivery still had a comment to fetch and a revision to
			   claim, so the older delivery could overtake and claim first. Nothing
			   was then stale about it - it was the first revision recorded - and
			   the command it carried was armed, which is what the spec exists to
			   forbid. It failed about one run in four against PostgreSQL, where the
			   round trips are slower and the window is wider. */
			post(
				webhook.EventIssueComment,
				deliveryOne,
				delivery("edited", "/approve", "User", "2026-08-08T10:05:00Z", true),
				nil,
			)
			Eventually(func() int {
				return stub.countCalls(http.MethodPost, approveReviews)
			}, eventuallyWindow).Should(Equal(1))

			post(
				webhook.EventIssueComment,
				deliveryTwo,
				delivery("created", "/squash after ci", "User", "2026-08-08T10:00:00Z", true),
				nil,
			)
			Consistently(func() int {
				return stub.countCalls(http.MethodPost, approveReviews)
			}, 300*time.Millisecond).Should(Equal(1))
			_, err := srv.store.GetArmed(
				GinkgoT().Context(), repositoryStorageID(githubtest.DefaultRepoID),
				githubtest.DefaultPRNumber,
			)
			Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())
		})

		It("should execute an accepted command after a service restart", func() {
			statePath := GinkgoT().TempDir() + "/restart.sqlite3"
			endpoint = httptest.NewServer(stub)
			DeferCleanup(endpoint.Close)
			serviceConfig := func() *serveConfig {
				return &serveConfig{
					database: statePath, listenAddress: "127.0.0.1:0",
					webhookPath: defaultWebhookPath, webhookSecret: []byte(testSecret),
					apiBaseURL: endpoint.URL, botUsername: defaultBotUsername,
					appClientID: "Iv1.test", appPrivateKey: githubtest.AppPrivateKey(),
					botConfig: config.Default(), logWriter: io.Discard,
				}
			}

			first, err := newServer(serviceConfig())
			Expect(err).NotTo(HaveOccurred())
			public := httptest.NewServer(first.handler())
			response := postDelivery(
				public, stub, webhook.EventIssueComment, deliveryOne,
				commandDelivery("/approve"), nil,
			)
			Expect(response.StatusCode).To(Equal(http.StatusAccepted))
			public.Close()
			Expect(first.Close()).To(Succeed())

			second, err := newServer(serviceConfig())
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(second.Close)
			workers := second.startWorkers()
			DeferCleanup(func() {
				second.closeQueue()
				workers.Wait()
			})

			Eventually(func() int {
				return stub.countCalls(http.MethodPost, approveReviews)
			}, eventuallyWindow).Should(Equal(1))
		})
	})

	Describe("pending CI events", func() {
		BeforeEach(func() {
			stub.prHead = "pending-head"
			start(config.Default())
		})

		It("durably accepts a check event and releases a matching request lease", func() {
			armed := armWebhookTestRequest(srv)
			lease, err := srv.store.LeaseDue(
				GinkgoT().Context(),
				time.Now().UTC(),
				time.Now().UTC().Add(time.Minute),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(lease.Request.ID).To(Equal(armed.ID))

			response := post(webhook.EventCheckRun, "check-delivery", checkRunDelivery(), nil)
			Expect(response.StatusCode).To(Equal(http.StatusAccepted))

			Eventually(func(g Gomega) {
				updated, readErr := srv.store.GetArmed(
					GinkgoT().Context(),
					armed.RepositoryID,
					armed.PullRequest,
				)
				g.Expect(readErr).NotTo(HaveOccurred())
				g.Expect(updated.LastEventKey).To(ContainSubstring("check_run"))
				g.Expect(updated.LeaseExpiresAt).To(BeNil())
			}).Within(eventuallyWindow).Should(Succeed())
		})

		It("cancels an armed request when its pending label is removed", func() {
			armed := armWebhookTestRequest(srv)
			response := post(
				webhook.EventPullRequest,
				"unlabeled-delivery",
				pendingLabelRemovedDelivery(),
				nil,
			)
			Expect(response.StatusCode).To(Equal(http.StatusAccepted))
			Eventually(func(g Gomega) {
				request, err := srv.store.GetArmed(
					GinkgoT().Context(), armed.RepositoryID, armed.PullRequest,
				)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(request.LastEventKey).To(ContainSubstring("pull_request"))
			}).Within(eventuallyWindow).Should(Succeed())
			startPendingCITestScheduler(srv)

			Eventually(func() bool {
				_, err := srv.store.GetArmed(
					GinkgoT().Context(),
					armed.RepositoryID,
					armed.PullRequest,
				)

				return errors.Is(err, storage.ErrNotFound)
			}).Within(eventuallyWindow).Should(BeTrue())
		})

		It("cancels a command edit when its webhook was missed", func() {
			armed := armWebhookTestRequest(srv)
			stub.prLabels = `[{"name":"smyklot:pending:ci:squash"}]`
			seedPendingCISource(stub, armed, "do not merge")
			startPendingCITestScheduler(srv)

			Eventually(func() bool {
				_, err := srv.store.GetArmed(
					GinkgoT().Context(), armed.RepositoryID, armed.PullRequest,
				)

				return errors.Is(err, storage.ErrNotFound)
			}, eventuallyWindow).Should(BeTrue())
		})

		It("cancels a command deletion when its webhook was missed", func() {
			armed := armWebhookTestRequest(srv)
			stub.prLabels = `[{"name":"smyklot:pending:ci:squash"}]`
			stub.mu.Lock()
			stub.issueComments[armed.SourceCommentID] = issueCommentRecord{exists: false}
			stub.mu.Unlock()
			startPendingCITestScheduler(srv)

			Eventually(func() bool {
				_, err := srv.store.GetArmed(
					GinkgoT().Context(), armed.RepositoryID, armed.PullRequest,
				)

				return errors.Is(err, storage.ErrNotFound)
			}, eventuallyWindow).Should(BeTrue())
			Expect(stub.countCalls(
				http.MethodPost, "/issues/comments/555/reactions",
			)).To(BeZero())
		})

		It("completes terminal cleanup without adding an ownership label", func() {
			armed := armWebhookTestRequest(srv)
			terminal, err := srv.store.Finish(
				GinkgoT().Context(), pendingci.FinishRequest{
					ID: armed.ID, ExpectedRevision: armed.Revision,
					Lifecycle: pendingci.LifecycleCancelled,
					Reason:    "test terminal cleanup", FinishedAt: time.Now().UTC(),
				},
			)
			Expect(err).NotTo(HaveOccurred())
			stub.prLabels = `[{"name":"smyklot:pending:ci:squash"}]`
			startPendingCITestScheduler(srv)

			Eventually(func(g Gomega) {
				cleaned, readErr := srv.store.Get(GinkgoT().Context(), terminal.ID)
				g.Expect(readErr).NotTo(HaveOccurred())
				g.Expect(cleaned.CleanupPending).To(BeFalse())
			}).Within(eventuallyWindow).Should(Succeed())
			Expect(stub.countCalls(http.MethodPost, "/issues/42/labels")).To(BeZero())
			Expect(stub.countCalls(
				http.MethodDelete,
				"/issues/42/labels/smyklot:pending:ci:squash",
			)).To(Equal(1))
		})

		It("treats a superseded label event as a hint about live state", func() {
			armWebhookTestRequest(srv)
			current := armWebhookTestRequestWithLabel(srv, "smyklot:pending:ci")
			seedPendingCISource(stub, current, "/squash after ci")
			stub.prLabels = `[{"name":"smyklot:pending:ci"}]`

			response := post(
				webhook.EventPullRequest,
				"stale-unlabeled-delivery",
				pendingLabelRemovedDelivery(),
				nil,
			)
			Expect(response.StatusCode).To(Equal(http.StatusAccepted))
			Eventually(func(g Gomega) {
				armed, err := srv.store.GetArmed(
					GinkgoT().Context(), current.RepositoryID, current.PullRequest,
				)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(armed.LastEventKey).To(ContainSubstring("pull_request"))
			}).Within(eventuallyWindow).Should(Succeed())
			startPendingCITestScheduler(srv)

			Eventually(func(g Gomega) {
				armed, err := srv.store.GetArmed(
					GinkgoT().Context(), current.RepositoryID, current.PullRequest,
				)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(armed.ID).To(Equal(current.ID))
				g.Expect(armed.LastObservedState).To(Equal("no_checks"))
			}).Within(eventuallyWindow).Should(Succeed())
		})

		It("treats a delayed close event as a hint about live state", func() {
			current := armWebhookTestRequest(srv)
			seedPendingCISource(stub, current, "/squash after ci")
			stub.prLabels = `[{"name":"smyklot:pending:ci:squash"}]`

			response := post(
				webhook.EventPullRequest,
				"stale-closed-delivery",
				pendingClosedDelivery(),
				nil,
			)
			Expect(response.StatusCode).To(Equal(http.StatusAccepted))
			Eventually(func(g Gomega) {
				armed, err := srv.store.GetArmed(
					GinkgoT().Context(), current.RepositoryID, current.PullRequest,
				)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(armed.LastEventKey).To(ContainSubstring("pull_request"))
			}).Within(eventuallyWindow).Should(Succeed())
			startPendingCITestScheduler(srv)

			Eventually(func(g Gomega) {
				armed, err := srv.store.GetArmed(
					GinkgoT().Context(), current.RepositoryID, current.PullRequest,
				)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(armed.ID).To(Equal(current.ID))
				g.Expect(armed.LastObservedState).To(Equal("no_checks"))
			}).Within(eventuallyWindow).Should(Succeed())
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

	// http.Server.Shutdown can leave a verified handler running after workers
	// stop. Durable acceptance must remain safe and leave the command for the
	// next dispatcher rather than sending to a closed in-memory queue.
	Describe("a delivery that arrives during shutdown", func() {
		BeforeEach(func() {
			start(config.Default())
		})

		It("should persist it rather than panic or lose it", func() {
			srv.closeQueue()

			resp := post(webhook.EventIssueComment, deliveryOne, commandDelivery("/approve"), nil)
			Expect(resp.StatusCode).To(Equal(http.StatusAccepted))

			Consistently(stub.total, 200*time.Millisecond).Should(BeZero())
			lease, err := srv.deliveryStore.LeaseDelivery(
				GinkgoT().Context(), time.Now().UTC(), time.Now().UTC().Add(jobTimeout),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(lease.Work).NotTo(BeNil())
			Expect(lease.Work.DeliveryID).To(Equal(deliveryOne))
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
