package webhook_test

import (
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

var _ = Describe("Deduper [Unit]", func() {
	It("should claim a key the first time and refuse it after", func() {
		d := webhook.NewDeduper(time.Hour, 100, nil)

		Expect(d.Begin("key")).To(BeTrue())
		Expect(d.Begin("key")).To(BeFalse())
		Expect(d.Begin("key")).To(BeFalse())
	})

	It("should treat different keys independently", func() {
		d := webhook.NewDeduper(time.Hour, 100, nil)

		Expect(d.Begin("one")).To(BeTrue())
		Expect(d.Begin("two")).To(BeTrue())
	})

	// A delivery that fails must stay retryable, or one transient GitHub error
	// turns into permanent silence for that comment
	It("should let an abandoned key be claimed again", func() {
		d := webhook.NewDeduper(time.Hour, 100, nil)

		Expect(d.Begin("key")).To(BeTrue())
		d.Abandon("key")
		Expect(d.Begin("key")).To(BeTrue())
	})

	It("should ignore abandoning a key it never held", func() {
		d := webhook.NewDeduper(time.Hour, 100, nil)

		Expect(func() { d.Abandon("never-seen") }).NotTo(Panic())
		Expect(d.Begin("never-seen")).To(BeTrue())
	})

	// Claiming before the work runs is what makes two simultaneous copies of
	// one delivery safe; claiming after would let both through
	It("should let exactly one of many concurrent claims through", func() {
		d := webhook.NewDeduper(time.Hour, 100, nil)

		var (
			claimed atomic.Int32
			wg      sync.WaitGroup
			start   = make(chan struct{})
		)

		for range 50 {
			wg.Add(1)

			go func() {
				defer wg.Done()
				defer GinkgoRecover()

				<-start

				if d.Begin("same-delivery") {
					claimed.Add(1)
				}
			}()
		}

		close(start)
		wg.Wait()

		Expect(claimed.Load()).To(Equal(int32(1)))
	})

	Context("expiry", func() {
		It("should forget a key once its TTL has passed", func() {
			now := time.Now()
			d := webhook.NewDeduper(time.Minute, 100, func() time.Time { return now })

			Expect(d.Begin("key")).To(BeTrue())

			now = now.Add(30 * time.Second)
			Expect(d.Begin("key")).To(BeFalse())

			now = now.Add(31 * time.Second)
			Expect(d.Begin("key")).To(BeTrue())
		})
	})

	Context("capacity", func() {
		It("should keep accepting new keys once full", func() {
			now := time.Now()
			d := webhook.NewDeduper(time.Hour, 2, func() time.Time { return now })

			Expect(d.Begin("one")).To(BeTrue())

			now = now.Add(time.Second)
			Expect(d.Begin("two")).To(BeTrue())

			now = now.Add(time.Second)
			Expect(d.Begin("three")).To(BeTrue())

			// The newest claims survive; the oldest was dropped to make room
			Expect(d.Begin("three")).To(BeFalse())
			Expect(d.Begin("two")).To(BeFalse())
			Expect(d.Begin("one")).To(BeTrue())
		})

		// Dropping expired claims first means a steady trickle of deliveries
		// never evicts a claim that is still doing its job
		It("should drop expired keys before evicting a live one", func() {
			now := time.Now()
			d := webhook.NewDeduper(time.Minute, 2, func() time.Time { return now })

			Expect(d.Begin("stale")).To(BeTrue())

			now = now.Add(2 * time.Minute)
			Expect(d.Begin("live")).To(BeTrue())
			Expect(d.Begin("newcomer")).To(BeTrue())

			Expect(d.Begin("live")).To(BeFalse())
			Expect(d.Begin("newcomer")).To(BeFalse())
		})
	})

	Context("defaults", func() {
		DescribeTable("should fall back for a non-positive setting",
			func(ttl time.Duration, max int) {
				d := webhook.NewDeduper(ttl, max, nil)

				Expect(d.Begin("key")).To(BeTrue())
				Expect(d.Begin("key")).To(BeFalse())
			},
			Entry("zero ttl", time.Duration(0), 100),
			Entry("negative ttl", -time.Hour, 100),
			Entry("zero max", time.Hour, 0),
			Entry("negative max", time.Hour, -1),
		)
	})
})
