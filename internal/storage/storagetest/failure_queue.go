package storagetest

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func declareFailureQueueSpecs(
	harness Harness,
	runtime func() (context.Context, storage.Store, time.Time),
) {
	It("projects the current delivery run from retained failure history", func() {
		ctx, store, now := runtime()
		_, target := seedInstallation(ctx, store, now)
		claim := storage.DeliveryClaim{ClaimKey: "operation-projection", DeliveryID: "operation-delivery", TargetID: target.TargetID, Event: "issue_comment", Payload: []byte(`{"action":"created"}`), ClaimedAt: now}
		original, err := store.ClaimDelivery(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		first, err := store.GetDeliveryOperation(ctx, target.TargetID, original.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(first.SourceOrder).To(Equal(original.ID))
		Expect(first.Current).NotTo(BeNil())
		Expect(first.Current.PayloadAvailable).To(BeTrue())
		Expect(first.Current.Queue).NotTo(BeNil())
		Expect(first.Current.Queue.State).To(Equal(workqueue.StateScheduled))
		Expect(first.Current.Queue.EligibleAt).To(BeTemporally("==", now))
		Expect(store.FailDelivery(ctx, storage.DeliveryFailureChange{ClaimID: original.ID, Reason: "temporary", Retryable: true, FailedAt: now})).To(Succeed())
		next, err := store.ClaimDelivery(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		Expect(store.AbandonDelivery(ctx, next.ID)).To(Succeed())
		abandoned, err := store.GetDeliveryOperation(ctx, target.TargetID, original.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(abandoned.Current).To(BeNil())
		Expect(abandoned.Revision).To(BeZero())
		Expect(abandoned.SourceOrder).To(Equal(original.ID))
		next, err = store.ClaimDelivery(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		leased, err := store.LeaseDelivery(ctx, now, now.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(leased.Work).NotTo(BeNil())
		Expect(leased.Work.ID).To(Equal(next.ID))
		current, err := store.GetDeliveryOperation(ctx, target.TargetID, original.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(current.Revision).To(BeNumerically(">", 0))
		Expect(current.SourceOrder).To(Equal(original.ID))
		Expect(current.Current.ID).To(Equal(next.ID))
		Expect(current.Current.Queue.State).To(Equal(workqueue.StateRunning))
		retryAt := now.Add(time.Minute)
		Expect(store.RetryDelivery(ctx, storage.DeliveryRetryChange{ClaimID: next.ID, Reason: "temporary", RetryAt: retryAt})).To(Succeed())
		waiting, err := store.GetDeliveryOperation(ctx, target.TargetID, original.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(waiting.Current.Queue.State).To(Equal(workqueue.StateRetrying))
		Expect(waiting.Current.Queue.EligibleAt).To(BeTemporally("==", retryAt))
		Expect(store.CompleteDelivery(ctx, next.ID, now)).To(Succeed())
		completed, err := store.GetDeliveryOperation(ctx, target.TargetID, original.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(completed.Current.Status).To(Equal(storage.DeliverySucceeded))
		Expect(completed.Current.Queue.State).To(Equal(workqueue.StateSucceeded))
		harness.RemoveQueueItem(ctx, completed.Current.Queue.ID)
		retained, err := store.GetDeliveryOperation(ctx, target.TargetID, original.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(retained.Current.Status).To(Equal(storage.DeliverySucceeded))
		Expect(retained.Current.Queue).To(BeNil())
		_, err = store.GetDeliveryOperation(ctx, "another-target", original.ID)
		Expect(err).To(MatchError(storage.ErrNotFound))
		Expect(store.PruneDeliveries(ctx, now.Add(time.Second))).To(Succeed())
		_, err = store.GetDeliveryOperation(ctx, target.TargetID, original.ID)
		Expect(err).To(MatchError(storage.ErrNotFound))
	})

	DescribeTable("failure queue references",
		func(retained bool) {
			ctx, store, now := runtime()
			account, target := seedInstallation(ctx, store, now)
			claim, err := store.ClaimDelivery(ctx, storage.DeliveryClaim{
				ClaimKey: "failure-reference", DeliveryID: "github-failure-reference",
				TargetID: target.TargetID, RepositoryFullName: "smykla-skalski/smyklot",
				Event: "issue_comment", ClaimedAt: now,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(store.FailDelivery(ctx, storage.DeliveryFailureChange{
				ClaimID: claim.ID, Stage: "execute", Reason: "connection reset",
				Retryable: true, FailedAt: now.Add(time.Minute),
			})).To(Succeed())
			itemID := fmt.Sprintf("delivery:%d", claim.ID)
			var expected *string
			if retained {
				expected = &itemID
			} else {
				harness.RemoveQueueItem(ctx, itemID)
			}

			// Exercise counts, joined-column searches, sorting and time filtering,
			// not just the unfiltered SELECT where ambiguous columns can hide.
			request := storage.FailurePageRequest{
				HistoryPageRequest: storage.HistoryPageRequest{
					Limit: 1, Query: "connection reset", Order: storage.HistoryRepositoryAscending,
				},
				Since: &now,
			}
			workspace, err := store.ListFailures(ctx, target.TargetID, request)
			Expect(err).NotTo(HaveOccurred())
			Expect(workspace.Total).To(Equal(1))
			Expect(workspace.Items).To(HaveLen(1))
			Expect(workspace.Items[0].QueueItemID).To(Equal(expected))
			root, err := store.ListRootFailures(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			Expect(root.Total).To(Equal(1))
			Expect(root.Items).To(HaveLen(1))
			Expect(root.Items[0].Failure.QueueItemID).To(Equal(expected))
			overview, err := store.GetRootOverview(ctx, account.ID, now.Add(2*time.Minute))
			Expect(err).NotTo(HaveOccurred())
			Expect(overview.RecentFailures).To(HaveLen(1))
			Expect(overview.RecentFailures[0].Failure.QueueItemID).To(Equal(expected))
		},
		Entry("identify the retained record in every failure view", true),
		Entry("keep failures readable without inventing missing queue records", false),
	)
}
