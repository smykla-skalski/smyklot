package storagetest

import (
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func declarePendingCIConcurrencySpecs(harness Harness, runtime queueRuntime) {
	if harness.OverlapPendingCIWake == nil {
		return
	}
	DescribeTable("pending CI source lock order", func(queueAction bool) {
		ctx, store, now := runtime()
		account, target := seedInstallation(ctx, store, now)
		arm := pendingCIArm(now, 7, 100, "head")
		arm.TargetID, arm.RepositoryID = target.TargetID, "repo-1"
		var err error
		arm.InstallationID, err = strconv.ParseInt(target.InstallationID, 10, 64)
		Expect(err).NotTo(HaveOccurred())
		armed, err := store.Arm(ctx, arm)
		Expect(err).NotTo(HaveOccurred())
		item, err := store.GetQueueItem(ctx, "pending-ci:"+strconv.FormatInt(armed.Request.ID, 10))
		Expect(err).NotTo(HaveOccurred())
		wakeErr, leaseErr := harness.OverlapPendingCIWake(ctx, func() error {
			_, wakeErr := store.Wake(ctx, pendingci.WakeRequest{
				RepositoryID: arm.RepositoryID, PullRequest: arm.PullRequest,
				EventName: "check_run", EventKey: "wake-concurrent", DeliveryID: "wake-delivery",
				OccurredAt: now,
			})
			return wakeErr
		}, func() error {
			if queueAction {
				_, actionErr := store.ApplyQueueAction(ctx, item.ID, workqueue.ItemAction{
					Type: workqueue.ActionRunNow, ExpectedRevision: item.Revision,
					ActorID: account.ID, ChangedAt: now,
				})
				return actionErr
			}
			_, leaseErr := store.LeaseDue(ctx, now, now.Add(time.Minute))
			return leaseErr
		})
		Expect(wakeErr).NotTo(HaveOccurred())
		Expect(leaseErr).To(SatisfyAny(BeNil(), MatchError(storage.ErrConflict)))
		current, err := store.Get(ctx, armed.Request.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(current.LastEventKey).To(Equal("wake-concurrent"))
		Expect(current.Lifecycle).To(Equal(pendingci.LifecycleArmed))
	},
		Entry("leases without deadlock against a concurrent webhook wake", false),
		Entry("changes a queue schedule without deadlock against a concurrent webhook wake", true),
	)
}
