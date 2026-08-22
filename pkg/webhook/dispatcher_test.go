package webhook_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

type terminal struct{ error }

func (terminal) Retryable() bool { return false }

type countingInbox struct {
	*webhook.MemoryInbox

	mu         sync.Mutex
	completed  int
	failed     []webhook.Failure
	retried    []webhook.Retry
	refuseOnce bool
}

func (c *countingInbox) Complete(ctx context.Context, claimID int64, at time.Time) error {
	c.mu.Lock()
	if c.refuseOnce {
		c.refuseOnce = false
		c.mu.Unlock()

		return errors.New("database unavailable")
	}
	c.completed++
	c.mu.Unlock()

	return c.MemoryInbox.Complete(ctx, claimID, at)
}

func (c *countingInbox) Fail(ctx context.Context, failure webhook.Failure) error {
	c.mu.Lock()
	c.failed = append(c.failed, failure)
	c.mu.Unlock()

	return c.MemoryInbox.Fail(ctx, failure)
}

func (c *countingInbox) Retry(ctx context.Context, retry webhook.Retry) error {
	c.mu.Lock()
	c.retried = append(c.retried, retry)
	c.mu.Unlock()

	return c.MemoryInbox.Retry(ctx, retry)
}

func (c *countingInbox) outcomes() (int, []webhook.Failure, []webhook.Retry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.completed, append([]webhook.Failure(nil), c.failed...),
		append([]webhook.Retry(nil), c.retried...)
}

var _ = Describe("Dispatcher [Unit]", func() {
	var (
		inbox    *countingInbox
		handled  chan webhook.Delivery
		fail     error
		retry    webhook.Retryable
		pipeline *webhook.Pipeline
	)

	BeforeEach(func() {
		inbox = &countingInbox{MemoryInbox: webhook.NewMemoryInbox(webhook.MemoryInboxOptions{})}
		handled = make(chan webhook.Delivery, 8)
		fail = nil
		retry = nil
	})

	start := func() {
		GinkgoHelper()
		var err error
		pipeline, err = webhook.New(
			[]byte(testSecret), inbox,
			func(_ context.Context, delivery webhook.Delivery) error {
				handled <- delivery

				return fail
			},
			webhook.Options{
				Events:   []string{webhook.EventIssueComment},
				Retry:    retry,
				Workers:  1,
				Timeouts: webhook.Timeouts{Drain: time.Second},
			},
		)
		Expect(err).NotTo(HaveOccurred())
		pipeline.Start(GinkgoT().Context())
		DeferCleanup(func() {
			Expect(pipeline.Shutdown(context.Background())).To(Succeed())
		})
	}

	post := func(deliveryID, body string) {
		GinkgoHelper()
		response := httptest.NewRecorder()
		pipeline.Receiver().ServeHTTP(
			response, signed(webhook.EventIssueComment, deliveryID, comment(body)),
		)
		Expect(response.Code).To(Equal(202))
	}

	It("should lease an accepted delivery and complete it", func() {
		start()
		post("d1", "/approve")

		var delivered webhook.Delivery
		Eventually(handled).Should(Receive(&delivered))
		Expect(delivered.Attempt).To(Equal(1))
		Expect(delivered.Source.Repository.FullName).To(Equal(testOwner + "/" + testRepo))
		Expect(delivered.ClaimID).NotTo(BeZero())

		Eventually(func() int {
			completed, _, _ := inbox.outcomes()

			return completed
		}).Should(Equal(1))
	})

	It("should retry a delivery whose handler failed transiently, without failing it", func() {
		fail = errors.New("github is having a moment")
		start()
		post("d1", "/approve")

		Eventually(handled).Should(Receive())
		Eventually(func() []webhook.Retry {
			_, _, retried := inbox.outcomes()

			return retried
		}).Should(HaveLen(1))

		_, failed, retried := inbox.outcomes()
		Expect(failed).To(BeEmpty())
		Expect(retried[0].Stage).To(Equal(webhook.StageExecute))
		Expect(retried[0].At).To(BeTemporally(">", time.Now()))
	})

	It("should not retry a handler failure the policy calls terminal", func() {
		var asked atomic.Int32
		fail = terminal{errors.New("repository configuration is invalid")}
		retry = func(cause error, attempt int) (time.Duration, bool) {
			asked.Add(1)

			return webhook.DefaultRetry(cause, attempt)
		}
		start()
		post("d1", "/approve")

		Eventually(handled).Should(Receive())
		Eventually(func() []webhook.Failure {
			_, failed, _ := inbox.outcomes()

			return failed
		}).Should(HaveLen(1))

		_, failed, retried := inbox.outcomes()
		Expect(retried).To(BeEmpty())
		Expect(failed[0].Retryable).To(BeFalse())
		Expect(asked.Load()).To(Equal(int32(1)))
	})

	It("should record a spent transient failure as retryable", func() {
		fail = errors.New("connection reset")
		retry = func(_ error, attempt int) (time.Duration, bool) {
			return time.Millisecond, attempt < 2
		}
		start()
		post("d1", "/approve")

		Eventually(func() []webhook.Failure {
			_, failed, _ := inbox.outcomes()

			return failed
		}).Should(HaveLen(1))

		_, failed, retried := inbox.outcomes()
		Expect(retried).To(HaveLen(1))
		Expect(failed[0].Retryable).To(BeTrue())
	})

	It("should fail a stored payload it cannot decode, without running it", func() {
		claimed, err := inbox.Claim(GinkgoT().Context(), webhook.Claim{
			Key: "k", DeliveryID: "d1", Event: webhook.EventIssueComment,
			Payload: []byte("not json"), At: time.Now().UTC(),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed.Disposition).To(Equal(webhook.Accepted))

		start()

		Eventually(func() []webhook.Failure {
			_, failed, _ := inbox.outcomes()

			return failed
		}).Should(HaveLen(1))

		_, failed, _ := inbox.outcomes()
		Expect(failed[0].Stage).To(Equal(webhook.StageDecode))
		Expect(failed[0].Retryable).To(BeFalse())
		Expect(handled).NotTo(Receive())
	})

	It("should keep writing an outcome the inbox refused, and join it on shutdown", func() {
		inbox.refuseOnce = true
		start()
		post("d1", "/approve")

		Eventually(handled).Should(Receive())
		Eventually(func() int {
			completed, _, _ := inbox.outcomes()

			return completed
		}, "5s").Should(Equal(1))
	})
})
