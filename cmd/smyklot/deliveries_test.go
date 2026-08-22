package main

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

// The pipeline's own backoff and attempt budget are proven in pkg/webhook. What
// is this service's is the one error it knows will not come right.
var _ = Describe("Delivery retry policy [Unit]", func() {
	It("does not retry a repository whose configuration will not parse", func() {
		delay, again := retryDelivery(bot.ErrRepoConfigInvalid, 1)

		Expect(again).To(BeFalse())
		Expect(delay).To(BeZero())
	})

	It("leaves every other failure to the default policy", func() {
		cause := errors.New("connection reset")

		delay, again := retryDelivery(cause, 1)
		expectedDelay, expectedAgain := webhook.DefaultRetry(cause, 1)

		Expect(again).To(Equal(expectedAgain))
		Expect(delay).To(Equal(expectedDelay))
	})
})
