package storagetest

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

func declareDeliveryOrderSpecs(runtime func() (context.Context, storage.Store, time.Time)) {
	It("keeps delivery outcomes immutable after redelivery and late finalization", func() {
		ctx, store, now := runtime()
		_, target := seedInstallation(ctx, store, now)
		claim := storage.DeliveryClaim{ClaimKey: "immutable-outcome", DeliveryID: "immutable-delivery", TargetID: target.TargetID, RepositoryFullName: "smykla-skalski/smyklot", Event: "issue_comment", ClaimedAt: now}
		original, err := store.ClaimDelivery(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		failure := storage.DeliveryFailureChange{ClaimID: original.ID, Stage: "execute", Reason: "original reason", Retryable: true, FailedAt: now}
		Expect(store.FailDelivery(ctx, failure)).To(Succeed())
		itemID := fmt.Sprintf("delivery:%d", original.ID)
		before, err := store.GetQueueItem(ctx, itemID)
		Expect(err).NotTo(HaveOccurred())
		events, err := store.ListQueueEvents(ctx, itemID, 100)
		Expect(err).NotTo(HaveOccurred())
		successor, err := store.ClaimDelivery(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		failure.Reason = "late reason"
		failure.Retryable = false
		failure.FailedAt = now.Add(time.Hour)
		Expect(store.FailDelivery(ctx, failure)).To(Succeed())
		after, err := store.GetQueueItem(ctx, itemID)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))
		afterEvents, err := store.ListQueueEvents(ctx, itemID, 100)
		Expect(err).NotTo(HaveOccurred())
		Expect(afterEvents).To(Equal(events))
		failures, err := store.ListFailures(ctx, target.TargetID, storage.FailurePageRequest{})
		Expect(err).NotTo(HaveOccurred())
		Expect(failures.Items).To(HaveLen(1))
		Expect(failures.Items[0].Reason).To(Equal("original reason"))
		Expect(failures.Items[0].Retryable).To(BeTrue())
		Expect(failures.Items[0].OccurredAt).To(BeTemporally("==", now))
		Expect(store.CompleteDelivery(ctx, original.ID, now)).To(MatchError(storage.ErrConflict))
		Expect(store.CompleteDelivery(ctx, successor.ID, now)).To(Succeed())
		successorID := fmt.Sprintf("delivery:%d", successor.ID)
		succeeded, err := store.GetQueueItem(ctx, successorID)
		Expect(err).NotTo(HaveOccurred())
		Expect(store.CompleteDelivery(ctx, successor.ID, now.Add(time.Hour))).To(Succeed())
		repeated, err := store.GetQueueItem(ctx, successorID)
		Expect(err).NotTo(HaveOccurred())
		Expect(repeated).To(Equal(succeeded))
		failure.ClaimID = successor.ID
		Expect(store.FailDelivery(ctx, failure)).To(MatchError(storage.ErrConflict))
		Expect(store.PruneDeliveries(ctx, now.Add(time.Second))).To(Succeed())
		Expect(store.CompleteDelivery(ctx, successor.ID, now)).To(MatchError(storage.ErrNotFound))
		Expect(store.FailDelivery(ctx, failure)).To(MatchError(storage.ErrNotFound))
	})

	It("preserves delivery source order across redelivery and pruning", func() {
		ctx, store, now := runtime()
		_, target := seedInstallation(ctx, store, now)
		claim := storage.DeliveryClaim{ClaimKey: "original-command", DeliveryID: "original-delivery", TargetID: target.TargetID, RepositoryFullName: "smykla-skalski/smyklot", Event: "issue_comment", Payload: []byte(`{"action":"created"}`), ClaimedAt: now}
		original, err := store.ClaimDelivery(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		first, err := store.LeaseDelivery(ctx, now, now.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Work).NotTo(BeNil())
		Expect(first.Work.SourceOrder).To(Equal(original.ID))
		Expect(store.FailDelivery(ctx, storage.DeliveryFailureChange{ClaimID: original.ID, Stage: "execute", Reason: "temporary failure", Retryable: true, FailedAt: now})).To(Succeed())
		abandoned, err := store.ClaimDelivery(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		Expect(store.AbandonDelivery(ctx, abandoned.ID)).To(Succeed())
		later := claim
		later.ClaimKey = "later-command"
		later.DeliveryID = "later-delivery"
		newer, err := store.ClaimDelivery(ctx, later)
		Expect(err).NotTo(HaveOccurred())
		Expect(store.CompleteDelivery(ctx, newer.ID, now)).To(Succeed())
		repeated, err := store.ClaimDelivery(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		Expect(repeated.ID).To(BeNumerically(">", newer.ID))
		Expect(store.PruneDeliveries(ctx, now.Add(time.Second))).To(Succeed())
		work, err := store.LeaseDelivery(ctx, now.Add(time.Second), now.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(work.Work).NotTo(BeNil())
		Expect(work.Work.ID).To(Equal(repeated.ID))
		Expect(work.Work.SourceOrder).To(Equal(original.ID))
		Expect(work.Work.SourceOrder).To(BeNumerically("<", newer.ID))
		Expect(work.Work.Attempt).To(Equal(1))
		Expect(work.Work.ClaimKey).To(Equal(claim.ClaimKey))
	})
}
