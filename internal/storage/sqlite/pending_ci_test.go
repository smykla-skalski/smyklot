package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	storagesqlite "github.com/smykla-skalski/smyklot/internal/storage/sqlite"
)

var _ = Describe("pending CI storage [Unit]", func() {
	var (
		ctx   context.Context
		store *storagesqlite.Store
		now   time.Time
		path  string
	)

	BeforeEach(func() {
		ctx = context.Background()
		now = time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
		path = filepath.Join(GinkgoT().TempDir(), "pending-ci.db")

		var err error
		store, err = storagesqlite.Open(ctx, path)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(store.Close()).To(Succeed())
	})

	It("keeps only the latest command armed and prevents stale workers from overwriting events", func() {
		first, err := store.Arm(ctx, pendingCITestArm(now, 101, "sha-1"))
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Superseded).To(BeNil())
		Expect(first.Request.Revision).To(Equal(int64(1)))

		secondArm := pendingCITestArm(now.Add(time.Minute), 202, "sha-2")
		secondArm.MergeMethod = pendingci.MergeMethodRebase
		second, err := store.Arm(ctx, secondArm)
		Expect(err).NotTo(HaveOccurred())
		Expect(second.Superseded).NotTo(BeNil())
		Expect(second.Superseded.ID).To(Equal(first.Request.ID))
		Expect(second.Superseded.Lifecycle).To(Equal(pendingci.LifecycleSuperseded))

		armed, err := store.GetArmed(ctx, secondArm.RepositoryID, secondArm.PullRequest)
		Expect(err).NotTo(HaveOccurred())
		Expect(armed.ID).To(Equal(second.Request.ID))
		Expect(armed.MergeMethod).To(Equal(pendingci.MergeMethodRebase))

		lease, err := store.LeaseDue(ctx, secondArm.RequestedAt, now.Add(3*time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(lease.Request).NotTo(BeNil())
		Expect(lease.Request.Revision).To(Equal(int64(2)))
		Expect(lease.Request.LeaseExpiresAt).To(HaveValue(Equal(now.Add(3 * time.Minute))))

		wake := pendingci.WakeRequest{
			RepositoryID:    secondArm.RepositoryID,
			PullRequest:     secondArm.PullRequest,
			EventKey:        "check_run:501:completed",
			ExpectedHeadSHA: secondArm.HeadSHA,
			OccurredAt:      now.Add(2 * time.Minute),
		}
		changed, err := store.Wake(ctx, wake)
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeTrue())
		changed, err = store.Wake(ctx, wake)
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeFalse())

		_, err = store.Reschedule(ctx, pendingci.RescheduleRequest{
			ID:                lease.Request.ID,
			ExpectedRevision:  lease.Request.Revision,
			Schedule:          pendingci.ScheduleDeferred,
			HeadSHA:           secondArm.HeadSHA,
			NextCheckAt:       now.Add(6 * time.Hour),
			LastProgressAt:    secondArm.RequestedAt,
			LastObservedState: "failing",
			CheckedAt:         now.Add(2 * time.Minute),
		})
		Expect(errors.Is(err, storage.ErrConflict)).To(BeTrue())

		lease, err = store.LeaseDue(ctx, wake.OccurredAt, now.Add(4*time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(lease.Request).NotTo(BeNil())
		deferred, err := store.Reschedule(ctx, pendingci.RescheduleRequest{
			ID:                lease.Request.ID,
			ExpectedRevision:  lease.Request.Revision,
			Schedule:          pendingci.ScheduleDeferred,
			HeadSHA:           "sha-3",
			NextCheckAt:       now.Add(8 * time.Hour),
			LastProgressAt:    secondArm.RequestedAt,
			LastObservedState: "failing",
			LastFingerprint:   "checks:v2:red",
			CheckedAt:         now.Add(3 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(deferred.Schedule).To(Equal(pendingci.ScheduleDeferred))
		Expect(deferred.HeadSHA).To(Equal("sha-3"))
		Expect(deferred.LeaseExpiresAt).To(BeNil())

		staleWake := wake
		staleWake.EventKey = "check_run:old-head:completed"
		changed, err = store.Wake(ctx, staleWake)
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeFalse())

		activeSchedule := pendingci.ScheduleActive
		active, err := store.ListQueue(ctx, pendingci.QueueFilter{Schedule: &activeSchedule})
		Expect(err).NotTo(HaveOccurred())
		Expect(active).To(BeEmpty())
		deferredSchedule := pendingci.ScheduleDeferred
		deferredQueue, err := store.ListQueue(ctx, pendingci.QueueFilter{Schedule: &deferredSchedule})
		Expect(err).NotTo(HaveOccurred())
		Expect(deferredQueue).To(ConsistOf(HaveField("ID", deferred.ID)))

		cancelled, err := store.CancelBySource(ctx, pendingci.CancelRequest{
			RepositoryID: secondArm.RepositoryID,
			PullRequest:  secondArm.PullRequest,
			CommentID:    999,
			Reason:       "source comment changed",
			CancelledAt:  now.Add(4 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(cancelled).To(BeNil())

		cancelled, err = store.CancelBySource(ctx, pendingci.CancelRequest{
			RepositoryID: secondArm.RepositoryID,
			PullRequest:  secondArm.PullRequest,
			CommentID:    secondArm.SourceCommentID,
			Reason:       "source comment changed",
			CancelledAt:  now.Add(4 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(cancelled.Lifecycle).To(Equal(pendingci.LifecycleCancelled))
		Expect(cancelled.FinishedAt).To(HaveValue(Equal(now.Add(4 * time.Minute))))

		wake.EventKey = "check_run:502:completed"
		changed, err = store.Wake(ctx, wake)
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeFalse())
		_, err = store.GetArmed(ctx, secondArm.RepositoryID, secondArm.PullRequest)
		Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())

		third, err := store.Arm(ctx, pendingCITestArm(now.Add(5*time.Minute), 303, "sha-3"))
		Expect(err).NotTo(HaveOccurred())
		Expect(third.Superseded).To(BeNil())
	})

	It("persists armed requests across service restarts", func() {
		armed, err := store.Arm(ctx, pendingCITestArm(now, 101, "restart-sha"))
		Expect(err).NotTo(HaveOccurred())
		Expect(store.Close()).To(Succeed())

		store, err = storagesqlite.Open(ctx, path)
		Expect(err).NotTo(HaveOccurred())
		restored, err := store.GetArmed(ctx, armed.Request.RepositoryID, armed.Request.PullRequest)
		Expect(err).NotTo(HaveOccurred())
		Expect(restored).To(Equal(armed.Request))
	})

	It("keeps finished requests terminal even when later events arrive", func() {
		armed, err := store.Arm(ctx, pendingCITestArm(now, 101, "merged-sha"))
		Expect(err).NotTo(HaveOccurred())
		lease, err := store.LeaseDue(ctx, now, now.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())

		finished, err := store.Finish(ctx, pendingci.FinishRequest{
			ID:               armed.Request.ID,
			ExpectedRevision: lease.Request.Revision,
			Lifecycle:        pendingci.LifecycleMerged,
			Reason:           "merged at the requested head SHA",
			FinishedAt:       now.Add(time.Second),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(finished.Lifecycle).To(Equal(pendingci.LifecycleMerged))

		changed, err := store.Wake(ctx, pendingci.WakeRequest{
			RepositoryID:    armed.Request.RepositoryID,
			PullRequest:     armed.Request.PullRequest,
			EventKey:        "check_suite:700:completed",
			ExpectedHeadSHA: armed.Request.HeadSHA,
			OccurredAt:      now.Add(2 * time.Second),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeFalse())
		queue, err := store.ListQueue(ctx, pendingci.QueueFilter{})
		Expect(err).NotTo(HaveOccurred())
		Expect(queue).To(BeEmpty())
	})

	It("wakes by the current head and applies terminal pull request events once", func() {
		armed, err := store.Arm(ctx, pendingCITestArm(now, 101, "head-for-status"))
		Expect(err).NotTo(HaveOccurred())
		changed, err := store.WakeByHead(ctx, pendingci.WakeHeadRequest{
			RepositoryID: armed.Request.RepositoryID,
			HeadSHA:      "stale-head",
			EventKey:     "status:stale-head:build:success",
			OccurredAt:   now.Add(time.Second),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeZero())

		wake := pendingci.WakeHeadRequest{
			RepositoryID: armed.Request.RepositoryID,
			HeadSHA:      armed.Request.HeadSHA,
			EventKey:     "status:head-for-status:build:success",
			OccurredAt:   now.Add(2 * time.Second),
		}
		changed, err = store.WakeByHead(ctx, wake)
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(Equal(int64(1)))
		changed, err = store.WakeByHead(ctx, wake)
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeZero())

		finished, err := store.FinishPR(ctx, pendingci.FinishPRRequest{
			RepositoryID: armed.Request.RepositoryID,
			PullRequest:  armed.Request.PullRequest,
			Lifecycle:    pendingci.LifecycleCancelled,
			Reason:       "pending CI label removed",
			FinishedAt:   now.Add(3 * time.Second),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(finished.Lifecycle).To(Equal(pendingci.LifecycleCancelled))
		finished, err = store.FinishPR(ctx, pendingci.FinishPRRequest{
			RepositoryID: armed.Request.RepositoryID,
			PullRequest:  armed.Request.PullRequest,
			Lifecycle:    pendingci.LifecycleMerged,
			Reason:       "late close event",
			FinishedAt:   now.Add(4 * time.Second),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(finished).To(BeNil())
	})

	It("rejects invalid transitions before they reach SQLite", func() {
		invalidArm := pendingCITestArm(now, 101, "sha-1")
		invalidArm.MergeMethod = "octopus"
		_, err := store.Arm(ctx, invalidArm)
		Expect(errors.Is(err, pendingci.ErrInvalidRequest)).To(BeTrue())

		armed, err := store.Arm(ctx, pendingCITestArm(now, 101, "sha-1"))
		Expect(err).NotTo(HaveOccurred())
		lease, err := store.LeaseDue(ctx, now, now.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())

		_, err = store.Reschedule(ctx, pendingci.RescheduleRequest{
			ID:                armed.Request.ID,
			ExpectedRevision:  lease.Request.Revision,
			Schedule:          "forgotten",
			HeadSHA:           armed.Request.HeadSHA,
			NextCheckAt:       now.Add(time.Hour),
			LastProgressAt:    now,
			LastObservedState: "pending",
			CheckedAt:         now,
		})
		Expect(errors.Is(err, pendingci.ErrInvalidRequest)).To(BeTrue())

		_, err = store.Finish(ctx, pendingci.FinishRequest{
			ID:               armed.Request.ID,
			ExpectedRevision: lease.Request.Revision,
			Lifecycle:        pendingci.LifecycleArmed,
			Reason:           "invalid terminal transition",
			FinishedAt:       now,
		})
		Expect(errors.Is(err, pendingci.ErrInvalidRequest)).To(BeTrue())
	})
})

func pendingCITestArm(requestedAt time.Time, commentID int64, headSHA string) pendingci.ArmRequest {
	return pendingci.ArmRequest{
		TargetID:           "installation:77",
		InstallationID:     77,
		RepositoryID:       "9001",
		RepositoryFullName: "smykla-skalski/smyklot",
		PullRequest:        198,
		HeadSHA:            headSHA,
		BaseBranch:         "main",
		MergeMethod:        pendingci.MergeMethodSquash,
		RequiredChecksOnly: true,
		Requester:          "bartsmykla",
		SourceCommentID:    commentID,
		SourceRevision:     requestedAt.Format(time.RFC3339Nano),
		Label:              "smyklot:pending:ci:squash-required",
		RequestedAt:        requestedAt,
	}
}
