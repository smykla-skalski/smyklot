package storagetest

import (
	"context"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

// contenders is how many callers race for the same row. It only has to be
// larger than one connection, so that an engine with a real pool actually
// overlaps the work rather than serializing it.
const contenders = 8

// declareConcurrencySpecs declares the invariants that hold only because the
// store makes each of these decisions once.
//
// A store backed by a single connection gets these for free: nothing overlaps,
// so nothing can interleave. A store backed by a pool does not, and these are
// the three places where losing that would corrupt real state - a delivery
// acted on twice, a settings change written over a newer one, and a session
// cap that lets an account keep more sessions than it is allowed.
//
// The store comes from the enclosing suite rather than being opened here, so
// that a spec opens exactly one.
func declareConcurrencySpecs(current func() (context.Context, storage.Store, time.Time)) {
	It("accepts one delivery claim when many arrive for the same event", func() {
		ctx, store, now := current()
		_, target := seedInstallation(ctx, store, now)
		claim := storage.DeliveryClaim{
			ClaimKey:           "issue_comment:created:repo:comment:revision",
			DeliveryID:         "delivery-contended",
			TargetID:           target.TargetID,
			RepositoryFullName: "smykla-skalski/smyklot",
			Event:              "issue_comment",
			ClaimedAt:          now,
		}

		results := race(func(int) (storage.DeliveryClaimResult, error) {
			return store.ClaimDelivery(ctx, claim)
		})

		accepted := 0
		for _, result := range results {
			Expect(result.err).NotTo(HaveOccurred())
			if result.value.Disposition == storage.DeliveryClaimAccepted {
				accepted++
				Expect(result.value.ID).NotTo(BeZero())

				continue
			}
			// Anything not accepted must say so, or the caller would run the
			// same comment twice.
			Expect(result.value.Disposition).To(Equal(storage.DeliveryClaimInProgress))
		}
		Expect(accepted).To(Equal(1))
	})

	It("applies one settings change when many carry the same revision", func() {
		ctx, store, now := current()
		account, initial := seedInstallation(ctx, store, now)

		results := race(func(index int) (storage.Target, error) {
			return store.UpdateTargetSettings(ctx, storage.TargetSettingsChange{
				TargetID:                 initial.TargetID,
				ActorAccountID:           account.ID,
				RepositoryDefaultEnabled: index%2 == 0,
				ExpectedRevision:         1,
				ChangedAt:                now.Add(time.Duration(index) * time.Second),
			})
		})

		applied := 0
		for _, result := range results {
			if result.err == nil {
				applied++
				Expect(result.value.Revision).To(Equal(int64(2)))

				continue
			}
			// A losing writer must be told its revision is stale rather than
			// silently overwriting the change that won.
			Expect(result.err).To(MatchError(storage.ErrConflict))
		}
		Expect(applied).To(Equal(1))

		target, err := store.GetTarget(ctx, initial.TargetID)
		Expect(err).NotTo(HaveOccurred())
		Expect(target.Revision).To(Equal(int64(2)))
	})

	It("never keeps more sessions than the cap allows", func() {
		const cap = 3

		ctx, store, now := current()
		account := testAccount(now)
		Expect(store.UpsertAccount(ctx, account)).To(Succeed())

		results := race(func(index int) (struct{}, error) {
			return struct{}{}, store.CreateSession(ctx, storage.Session{
				TokenHash: fmt.Sprintf("token-%d", index),
				AccountID: account.ID,
				CreatedAt: now.Add(time.Duration(index) * time.Second),
				ExpiresAt: now.Add(time.Hour),
			}, cap)
		})
		for _, result := range results {
			Expect(result.err).NotTo(HaveOccurred())
		}

		live := 0
		for index := range contenders {
			_, err := store.GetSession(ctx, fmt.Sprintf("token-%d", index), now)
			if err == nil {
				live++

				continue
			}
			Expect(err).To(SatisfyAny(
				MatchError(storage.ErrNotFound),
				MatchError(storage.ErrRevoked),
			))
		}
		Expect(live).To(BeNumerically("<=", cap))
		Expect(live).To(BeNumerically(">", 0))
	})
}

// outcome is what one racing caller returned.
type outcome[T any] struct {
	value T
	err   error
}

// race runs work concurrently and returns every outcome. Each caller is handed
// its own index so it can vary what it writes.
func race[T any](work func(index int) (T, error)) []outcome[T] {
	results := make([]outcome[T], contenders)

	var ready sync.WaitGroup
	var done sync.WaitGroup
	start := make(chan struct{})

	ready.Add(contenders)
	done.Add(contenders)
	for index := range contenders {
		go func() {
			defer GinkgoRecover()
			defer done.Done()

			ready.Done()
			<-start

			value, err := work(index)
			results[index] = outcome[T]{value: value, err: err}
		}()
	}

	// Releasing every caller at once is what makes the calls overlap on an
	// engine that can run them at the same time.
	ready.Wait()
	close(start)
	done.Wait()

	return results
}
