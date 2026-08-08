package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Delivery persistence [Unit]", func() {
	It("finalizes a delivery after its execution context expires", func() {
		type contextKey string
		const loggerKey contextKey = "logger"

		executionContext, cancelExecution := context.WithCancel(
			context.WithValue(GinkgoT().Context(), loggerKey, "delivery logger"),
		)
		cancelExecution()

		finalizationContext, cancelFinalization := deliveryFinalizationContext(executionContext)
		DeferCleanup(cancelFinalization)

		Expect(finalizationContext.Err()).NotTo(HaveOccurred())
		Expect(finalizationContext.Value(loggerKey)).To(Equal("delivery logger"))
		_, hasDeadline := finalizationContext.Deadline()
		Expect(hasDeadline).To(BeTrue())
	})

	It("retries a delivery outcome after a transient persistence failure", func() {
		retryContext, cancelRetry := context.WithCancel(GinkgoT().Context())
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		service := &server{
			logger:              logger,
			deliveryRetryCtx:    retryContext,
			cancelDeliveryRetry: cancelRetry,
		}
		DeferCleanup(service.stopDeliveryFinalizationRetries)

		var attempts atomic.Int32
		persisted := make(chan struct{})
		service.retryDeliveryFinalization(job{logger: logger}, "failure", func(context.Context) error {
			if attempts.Add(1) == 1 {
				return errors.New("database is busy")
			}
			close(persisted)

			return nil
		})

		Eventually(persisted).Within(2 * time.Second).Should(BeClosed())
		Expect(attempts.Load()).To(Equal(int32(2)))
	})
})
