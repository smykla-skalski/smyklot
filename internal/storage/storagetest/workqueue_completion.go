package storagetest

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func declareRecurringCompletionSpecs(runtime queueRuntime) {
	It("fences recurring completion by claimed attempt and unexpired lease", func() {
		ctx, store, now := runtime()
		claim := workqueue.RecurringClaim{Kind: workqueue.KindCatalogRefresh, Title: "Refresh", Now: now, LeaseDuration: time.Minute}
		original, claimed, err := store.ClaimRecurringWork(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).To(BeTrue())
		for _, invalid := range []struct {
			attempt int
			at      time.Time
		}{
			{0, now},
			{-1, now},
			{original.Attempt + 1, now},
			{original.Attempt, now.Add(time.Minute)},
		} {
			_, err = store.FinishRecurringWork(ctx, original.ID, workqueue.RecurringCompletion{Attempt: invalid.attempt}, invalid.at)
			Expect(err).To(MatchError(storage.ErrConflict))
		}
		claim.Now = now.Add(time.Minute)
		current, claimed, err := store.ClaimRecurringWork(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).To(BeTrue())
		Expect(current.ID).To(Equal(original.ID))
		Expect(current.Attempt).To(Equal(original.Attempt + 1))
		before, err := store.GetQueueItem(ctx, current.ID)
		Expect(err).NotTo(HaveOccurred())
		events, err := store.ListQueueEvents(ctx, current.ID, 100)
		Expect(err).NotTo(HaveOccurred())
		for _, stale := range []workqueue.RecurringCompletion{
			{SuccessSummary: "Old success"},
			{Failure: "Old failure", Retryable: true},
			{Failure: "Old terminal failure"},
			{Failure: "Old blocker", Blocked: true},
		} {
			stale.Attempt = original.Attempt
			_, err = store.FinishRecurringWork(ctx, current.ID, stale, claim.Now)
			Expect(err).To(MatchError(storage.ErrConflict))
			after, err := store.GetQueueItem(ctx, current.ID)
			Expect(err).NotTo(HaveOccurred())
			// Dispatch estimates are read-time projections, not persisted occurrence state.
			after.EstimatedStartAt, before.EstimatedStartAt = nil, nil
			Expect(after).To(Equal(before))
			afterEvents, err := store.ListQueueEvents(ctx, current.ID, 100)
			Expect(err).NotTo(HaveOccurred())
			Expect(afterEvents).To(Equal(events))
		}
		page, err := store.ListWorkQueue(ctx, workqueue.Filter{Kinds: []workqueue.Kind{claim.Kind}})
		Expect(err).NotTo(HaveOccurred())
		Expect(page.Items).To(HaveLen(1))
		finished, err := store.FinishRecurringWork(ctx, current.ID, workqueue.RecurringCompletion{Attempt: current.Attempt, SuccessSummary: "Current result"}, claim.Now)
		Expect(err).NotTo(HaveOccurred())
		Expect(finished.State).To(Equal(workqueue.StateSucceeded))
		_, err = store.FinishRecurringWork(ctx, current.ID, workqueue.RecurringCompletion{Attempt: current.Attempt}, claim.Now)
		Expect(err).To(MatchError(storage.ErrConflict))
		page, err = store.ListWorkQueue(ctx, workqueue.Filter{Kinds: []workqueue.Kind{claim.Kind}})
		Expect(err).NotTo(HaveOccurred())
		Expect(page.Items).To(HaveLen(2))
	})
}
