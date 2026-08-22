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

	It("should finish work already handed to a worker after its context is cancelled", func() {
		running := make(chan struct{})
		release := make(chan struct{})
		finished := make(chan struct{})

		pipeline := build(func(ctx context.Context, _ webhook.Delivery) error {
			close(running)
			<-release
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

	It("should survive Start racing Shutdown", func() {
		for range 200 {
			pipeline := build(
				func(context.Context, webhook.Delivery) error { return nil }, time.Second,
			)

			started := make(chan struct{})
			go func() {
				close(started)
				pipeline.Start(context.Background())
			}()

			<-started
			Expect(pipeline.Shutdown(context.Background())).To(Succeed())
		}
	})

	It("should refuse to start once it has shut down", func() {
		handled := make(chan struct{}, 1)
		pipeline := build(func(context.Context, webhook.Delivery) error {
			handled <- struct{}{}

			return nil
		}, time.Second)

		// Given a pipeline shut down before it ever started
		Expect(pipeline.Shutdown(context.Background())).To(Succeed())

		// When it is started and given work
		pipeline.Start(GinkgoT().Context())
		Expect(post(pipeline, "d1")).To(Equal(202))

		// Then the lease loop does not run, so nothing sends on the closed
		// queue and nothing is handled
		Consistently(handled, 300*time.Millisecond).ShouldNot(Receive())
		Expect(pipeline.QueueDepth()).To(BeZero())
	})

	It("should still claim a delivery arriving after shutdown, and queue nothing", func() {
		pipeline := build(func(context.Context, webhook.Delivery) error { return nil }, time.Second)
		pipeline.Start(GinkgoT().Context())
		Expect(pipeline.Shutdown(context.Background())).To(Succeed())

		Expect(post(pipeline, "d1")).To(Equal(202))
		Expect(pipeline.QueueDepth()).To(BeZero())
	})
})
