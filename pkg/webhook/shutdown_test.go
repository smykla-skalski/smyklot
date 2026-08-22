package webhook_test

import (
	"context"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

var _ = Describe("Shutdown [Unit]", func() {
	var inbox *webhook.MemoryInbox

	BeforeEach(func() {
		inbox = webhook.NewMemoryInbox(webhook.MemoryInboxOptions{})
	})

	build := func(handle webhook.Handler, drain time.Duration) *webhook.Pipeline {
		GinkgoHelper()
		pipeline, err := webhook.New(
			[]byte(testSecret), inbox, handle,
			webhook.Options{
				Events:   []string{webhook.EventIssueComment},
				Workers:  1,
				Timeouts: webhook.Timeouts{Drain: drain},
			},
		)
		Expect(err).NotTo(HaveOccurred())

		return pipeline
	}

	post := func(pipeline *webhook.Pipeline, deliveryID string) int {
		GinkgoHelper()
		response := httptest.NewRecorder()
		pipeline.Receiver().ServeHTTP(
			response, signed(webhook.EventIssueComment, deliveryID, comment("/approve")),
		)

		return response.Code
	}

	// Cancelling Start's context stops new leases. It must not abandon a
	// delivery a worker has already been handed - GitHub was told 202.
	It("should finish work already handed to a worker after its context is cancelled", func() {
		running := make(chan struct{})
		release := make(chan struct{})
		finished := make(chan struct{})

		pipeline := build(func(ctx context.Context, _ webhook.Delivery) error {
			close(running)
			<-release
			// The handler's own context is still live: only leasing stopped.
			Expect(ctx.Err()).NotTo(HaveOccurred())
			close(finished)

			return nil
		}, 5*time.Second)

		leaseCtx, stopLeasing := context.WithCancel(context.Background())
		pipeline.Start(leaseCtx)
		Expect(post(pipeline, "d1")).To(Equal(202))

		Eventually(running).Should(BeClosed())
		stopLeasing()

		shutdown := make(chan error, 1)
		go func() { shutdown <- pipeline.Shutdown(context.Background()) }()

		close(release)
		Eventually(finished).Should(BeClosed())
		Eventually(shutdown).Should(Receive(BeNil()))
	})

	It("should report a drain that ran out of time rather than hanging", func() {
		running := make(chan struct{})
		release := make(chan struct{})
		defer close(release)

		pipeline := build(func(context.Context, webhook.Delivery) error {
			close(running)
			<-release

			return nil
		}, 50*time.Millisecond)

		pipeline.Start(GinkgoT().Context())
		Expect(post(pipeline, "d1")).To(Equal(202))

		// Waited for rather than assumed: Shutdown called before the worker
		// picked the delivery up would find an empty queue and drain cleanly,
		// which is a green spec that proves nothing.
		Eventually(running).Should(BeClosed())

		Expect(pipeline.Shutdown(context.Background())).To(MatchError(webhook.ErrDrainTimeout))
	})

	It("should be safe to shut down twice", func() {
		pipeline := build(func(context.Context, webhook.Delivery) error { return nil }, time.Second)
		pipeline.Start(GinkgoT().Context())

		Expect(pipeline.Shutdown(context.Background())).To(Succeed())
		Expect(pipeline.Shutdown(context.Background())).To(Succeed())
	})

	It("should be safe to shut down without ever starting", func() {
		pipeline := build(func(context.Context, webhook.Delivery) error { return nil }, time.Second)

		Expect(pipeline.Shutdown(context.Background())).To(Succeed())
	})

	// The claim is still written - the row is what survives a restart - but
	// nothing is queued, because the queue is closed. A send on it would panic.
	It("should still claim a delivery arriving after shutdown", func() {
		pipeline := build(func(context.Context, webhook.Delivery) error { return nil }, time.Second)
		pipeline.Start(GinkgoT().Context())
		Expect(pipeline.Shutdown(context.Background())).To(Succeed())

		Expect(post(pipeline, "d1")).To(Equal(202))
		Expect(pipeline.QueueDepth()).To(BeZero())
	})
})
