package storagetest

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

func declareFailureQueueSpecs(
	harness Harness,
	runtime func() (context.Context, storage.Store, time.Time),
) {
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
