package main

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/metrics"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

type deliveryPauseStore struct {
	storage.DeliveryStore
	leases int
}

func (store *deliveryPauseStore) LeaseDelivery(
	context.Context,
	time.Time,
	time.Time,
) (storage.DeliveryLeaseResult, error) {
	store.leases++

	return storage.DeliveryLeaseResult{
		Work: &storage.DeliveryWork{ID: 7, DeliveryID: "delivery-7"},
	}, nil
}

var _ = Describe("Delivery pause [Unit]", func() {
	It("keeps durable work unleased until automatic work resumes", func(ctx SpecContext) {
		paused := true
		store := &deliveryPauseStore{}
		inbox := deliveryInbox{store: store, paused: func() bool { return paused }}

		lease, err := inbox.Lease(ctx, time.Now(), time.Now().Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(lease.Work).To(BeNil())
		Expect(store.leases).To(BeZero())

		paused = false
		lease, err = inbox.Lease(ctx, time.Now(), time.Now().Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(lease.Work).NotTo(BeNil())
		Expect(store.leases).To(Equal(1))
	})
})

var _ = Describe("Delivery retry policy [Unit]", func() {
	It("should not retry a repository whose configuration will not parse", func() {
		delay, again := retryDelivery(bot.ErrRepoConfigInvalid, 1)

		Expect(again).To(BeFalse())
		Expect(delay).To(BeZero())
	})

	It("should leave every other failure to the default policy", func() {
		cause := errors.New("connection reset")

		delay, again := retryDelivery(cause, 1)
		expectedDelay, expectedAgain := webhook.DefaultRetry(cause, 1)

		Expect(again).To(Equal(expectedAgain))
		Expect(delay).To(Equal(expectedDelay))
	})
})

var _ = Describe("Delivery failure log [Unit]", func() {
	var service *server

	BeforeEach(func() {
		registry := metrics.NewRegistry()
		service = &server{
			metrics:  metrics.New(registry),
			failures: newFailureLog(maxRecordedFailures),
			redactor: nil,
		}
	})

	executed := func(cause error, attempt int) {
		GinkgoHelper()
		service.deliveryObserver().Executed(webhook.Delivery{
			Event:   webhook.EventIssueComment,
			ID:      "d1",
			Attempt: attempt,
			Payload: commandDelivery("/approve"),
		}, time.Millisecond, cause)
	}

	// The log is a bounded ring an operator reads to find what went wrong. A
	// transient failure is going to be tried again, and recording every one of
	// them pushes the terminal failures out of it.
	It("should record a failure that will not be retried", func() {
		executed(bot.ErrRepoConfigInvalid, 1)

		Expect(service.failures.Snapshot()).To(HaveLen(1))
	})

	It("should not record a failure that will be retried", func() {
		executed(errors.New("connection reset"), 1)

		Expect(service.failures.Snapshot()).To(BeEmpty())
	})

	It("should record it once the attempt budget runs out", func() {
		executed(errors.New("connection reset"), 8)

		Expect(service.failures.Snapshot()).To(HaveLen(1))
	})
})
