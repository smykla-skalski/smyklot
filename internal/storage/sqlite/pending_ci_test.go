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
		stored, err := store.Get(ctx, first.Request.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(stored).To(Equal(first.Request))
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
		Expect(lease.Request.ID).To(Equal(first.Request.ID))
		Expect(lease.Request.Lifecycle).To(Equal(pendingci.LifecycleSuperseded))
		Expect(lease.Request.CleanupPending).To(BeTrue())
		_, err = store.CompleteCleanup(ctx, pendingci.CompleteCleanupRequest{
			ID: lease.Request.ID, ExpectedRevision: lease.Request.Revision,
			CompletedAt: secondArm.RequestedAt,
		})
		Expect(err).NotTo(HaveOccurred())

		lease, err = store.LeaseDue(ctx, secondArm.RequestedAt, now.Add(3*time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(lease.Request).NotTo(BeNil())
		Expect(lease.Request.ID).To(Equal(second.Request.ID))
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
		_, err = store.ClaimMerge(ctx, pendingci.ClaimMergeRequest{
			ID: lease.Request.ID, ExpectedRevision: lease.Request.Revision,
			ClaimedAt: now.Add(2 * time.Minute),
		})
		Expect(errors.Is(err, storage.ErrConflict)).To(BeTrue())

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
		claimed, err := store.ClaimMerge(ctx, pendingci.ClaimMergeRequest{
			ID: lease.Request.ID, ExpectedRevision: lease.Request.Revision,
			ClaimedAt: now.Add(2*time.Minute + time.Second),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed.Revision).To(Equal(lease.Request.Revision + 1))
		deferred, err := store.Reschedule(ctx, pendingci.RescheduleRequest{
			ID:                claimed.ID,
			ExpectedRevision:  claimed.Revision,
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

		checked, err := store.CheckNow(ctx, pendingci.CheckNowRequest{
			ID: deferred.ID, ExpectedRevision: deferred.Revision,
			EventKey: "panel:check-now", OccurredAt: now.Add(4 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(checked.Schedule).To(Equal(pendingci.ScheduleActive))
		Expect(checked.NextCheckAt).To(Equal(now.Add(4 * time.Minute)))
		Expect(checked.LeaseExpiresAt).To(BeNil())

		deferred, err = store.Reschedule(ctx, pendingci.RescheduleRequest{
			ID: checked.ID, ExpectedRevision: checked.Revision,
			Schedule: pendingci.ScheduleDeferred, HeadSHA: checked.HeadSHA,
			NextCheckAt: now.Add(8 * time.Hour), LastProgressAt: checked.LastProgressAt,
			LastObservedState: "failing", LastFingerprint: "checks:v2:red",
			CheckedAt: now.Add(5 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())

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
			RepositoryID:   secondArm.RepositoryID,
			PullRequest:    secondArm.PullRequest,
			CommentID:      999,
			SourceRevision: now.Add(4 * time.Minute).Format(time.RFC3339Nano),
			SourceSequence: 2,
			SourceOrder:    1,
			Reason:         "source comment changed",
			CancelledAt:    now.Add(4 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(cancelled).To(BeNil())

		cancelled, err = store.CancelBySource(ctx, pendingci.CancelRequest{
			RepositoryID:   secondArm.RepositoryID,
			PullRequest:    secondArm.PullRequest,
			CommentID:      secondArm.SourceCommentID,
			SourceRevision: now.Add(4 * time.Minute).Format(time.RFC3339Nano),
			SourceSequence: 2,
			SourceOrder:    1,
			Reason:         "source comment changed",
			CancelledAt:    now.Add(4 * time.Minute),
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

	It("persists terminal cleanup retries across service restarts", func() {
		armed, err := store.Arm(ctx, pendingCITestArm(now, 101, "cleanup-sha"))
		Expect(err).NotTo(HaveOccurred())
		lease, err := store.LeaseDue(ctx, now, now.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())

		finishedAt := now.Add(time.Second)
		finished, err := store.Finish(ctx, pendingci.FinishRequest{
			ID: armed.Request.ID, ExpectedRevision: lease.Request.Revision,
			Lifecycle: pendingci.LifecycleCancelled,
			Reason:    "source command removed", FinishedAt: finishedAt,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(finished.CleanupPending).To(BeTrue())
		Expect(finished.NextCheckAt).To(Equal(finishedAt))

		cleanupLease, err := store.LeaseDue(ctx, finishedAt, now.Add(2*time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(cleanupLease.Request).NotTo(BeNil())
		Expect(cleanupLease.Request.ID).To(Equal(finished.ID))
		Expect(cleanupLease.Request.CleanupPending).To(BeTrue())

		retryAt := now.Add(10 * time.Second)
		retried, err := store.RetryCleanup(ctx, pendingci.RetryCleanupRequest{
			ID: cleanupLease.Request.ID, ExpectedRevision: cleanupLease.Request.Revision,
			NextAttemptAt: retryAt, FailedAt: finishedAt,
			Error: "GitHub temporarily unavailable",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(retried.CleanupPending).To(BeTrue())
		Expect(retried.CleanupAttempts).To(Equal(1))
		Expect(retried.CleanupError).To(Equal("GitHub temporarily unavailable"))
		Expect(retried.LeaseExpiresAt).To(BeNil())

		Expect(store.Close()).To(Succeed())
		store, err = storagesqlite.Open(ctx, path)
		Expect(err).NotTo(HaveOccurred())
		notDue, err := store.LeaseDue(ctx, retryAt.Add(-time.Nanosecond), now.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(notDue.Request).To(BeNil())
		Expect(notDue.AvailableAt).To(HaveValue(Equal(retryAt)))

		cleanupLease, err = store.LeaseDue(ctx, retryAt, retryAt.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(cleanupLease.Request).NotTo(BeNil())
		Expect(cleanupLease.Request.CleanupAttempts).To(Equal(1))
		completed, err := store.CompleteCleanup(ctx, pendingci.CompleteCleanupRequest{
			ID: cleanupLease.Request.ID, ExpectedRevision: cleanupLease.Request.Revision,
			CompletedAt: retryAt,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(completed.CleanupPending).To(BeFalse())
		Expect(completed.CleanupError).To(BeEmpty())

		done, err := store.LeaseDue(ctx, retryAt.Add(time.Hour), retryAt.Add(2*time.Hour))
		Expect(err).NotTo(HaveOccurred())
		Expect(done.Request).To(BeNil())
		Expect(done.AvailableAt).To(BeNil())
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

	It("orders mutable source deliveries and admits retries of the same event", func() {
		claim := pendingci.SourceRevisionRequest{
			RepositoryID: "9001", PullRequest: 198, CommentID: 101,
			Revision: now.Format(time.RFC3339Nano), Sequence: 1,
			EventKey: "issue_comment:created:101", ObservedAt: now,
		}
		result, err := store.ClaimSourceRevision(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Accepted).To(BeTrue())
		Expect(result.SourceOrder).To(Equal(int64(1)))
		result, err = store.ClaimSourceRevision(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Accepted).To(BeTrue())
		Expect(result.SourceOrder).To(Equal(int64(1)))

		older := claim
		older.Revision = now.Add(-time.Second).Format(time.RFC3339Nano)
		older.EventKey = "issue_comment:created:old"
		result, err = store.ClaimSourceRevision(ctx, older)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Accepted).To(BeFalse())

		edited := claim
		edited.Sequence = 2
		edited.EventKey = "issue_comment:edited:101"
		result, err = store.ClaimSourceRevision(ctx, edited)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Accepted).To(BeTrue())
		Expect(result.SourceOrder).To(Equal(int64(2)))

		editedAgain := edited
		editedAgain.EventKey = "issue_comment:edited:101:different-body"
		result, err = store.ClaimSourceRevision(ctx, editedAgain)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Accepted).To(BeTrue())
		Expect(result.SourceOrder).To(Equal(int64(3)))

		result, err = store.ClaimSourceRevision(ctx, edited)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Accepted).To(BeFalse())
		Expect(result.SourceOrder).To(Equal(int64(2)))
	})

	It("uses durable order to break same-timestamp command ties", func() {
		firstArm := pendingCITestArm(now, 101, "first-head")
		firstArm.SourceOrder = 1
		firstArm.MergeMethod = pendingci.MergeMethodMerge
		first, err := store.Arm(ctx, firstArm)
		Expect(err).NotTo(HaveOccurred())

		lastArm := pendingCITestArm(now, 202, "last-head")
		lastArm.SourceOrder = 2
		lastArm.MergeMethod = pendingci.MergeMethodSquash
		last, err := store.Arm(ctx, lastArm)
		Expect(err).NotTo(HaveOccurred())
		Expect(last.Superseded).NotTo(BeNil())
		Expect(last.Superseded.ID).To(Equal(first.Request.ID))

		current, err := store.GetArmed(ctx, lastArm.RepositoryID, lastArm.PullRequest)
		Expect(err).NotTo(HaveOccurred())
		Expect(current.ID).To(Equal(last.Request.ID))
		Expect(current.MergeMethod).To(Equal(pendingci.MergeMethodSquash))

		_, err = store.Arm(ctx, firstArm)
		Expect(errors.Is(err, pendingci.ErrStaleSourceRevision)).To(BeTrue())
	})

	It("rejects delayed commands and source cancellations", func() {
		newer := pendingCITestArm(now.Add(time.Minute), 202, "new-head")
		armed, err := store.Arm(ctx, newer)
		Expect(err).NotTo(HaveOccurred())

		_, err = store.Arm(ctx, pendingCITestArm(now, 101, "old-head"))
		Expect(errors.Is(err, pendingci.ErrStaleSourceRevision)).To(BeTrue())
		current, err := store.GetArmed(ctx, newer.RepositoryID, newer.PullRequest)
		Expect(err).NotTo(HaveOccurred())
		Expect(current.ID).To(Equal(armed.Request.ID))

		cancelled, err := store.CancelBySource(ctx, pendingci.CancelRequest{
			RepositoryID: newer.RepositoryID, PullRequest: newer.PullRequest,
			CommentID:      newer.SourceCommentID,
			SourceRevision: now.Format(time.RFC3339Nano), SourceSequence: 2,
			SourceOrder: 1,
			Reason:      "delayed edit", CancelledAt: now.Add(2 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(cancelled).To(BeNil())
		_, err = store.GetArmed(ctx, newer.RepositoryID, newer.PullRequest)
		Expect(err).NotTo(HaveOccurred())
	})

	It("drains each pre-durable label once without arming an unknown head", func() {
		drain := pendingci.LegacyDrainRequest{
			TargetID: "installation:77", InstallationID: 77,
			RepositoryID: "9001", RepositoryFullName: "smykla-skalski/smyklot",
			PullRequest: 198, HeadSHA: "observed-head", BaseBranch: "main",
			Labels: []pendingci.LegacyPendingCILabel{
				{MergeMethod: pendingci.MergeMethodSquash, Label: "smyklot:pending-ci:squash"},
				{MergeMethod: pendingci.MergeMethodRebase, Label: "smyklot:pending-ci:rebase"},
			},
			DrainedAt: now,
		}
		first, err := store.DrainLegacy(ctx, drain)
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Requests).To(HaveLen(2))
		Expect(first.Requests).To(ConsistOf(
			HaveField("Label", "smyklot:pending-ci:squash"),
			HaveField("Label", "smyklot:pending-ci:rebase"),
		))
		for _, request := range first.Requests {
			Expect(request.Lifecycle).To(Equal(pendingci.LifecycleCancelled))
			Expect(request.CleanupPending).To(BeTrue())
			Expect(request.SourceCommentID).To(BeZero())
		}
		_, err = store.GetArmed(ctx, drain.RepositoryID, drain.PullRequest)
		Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())

		second, err := store.DrainLegacy(ctx, drain)
		Expect(err).NotTo(HaveOccurred())
		Expect(second.Requests).To(BeEmpty())
	})

	It("cancels every armed request when a repository leaves the service", func() {
		first := pendingCITestArm(now, 101, "first-head")
		_, err := store.Arm(ctx, first)
		Expect(err).NotTo(HaveOccurred())
		second := pendingCITestArm(now.Add(time.Minute), 202, "second-head")
		second.PullRequest = 199
		_, err = store.Arm(ctx, second)
		Expect(err).NotTo(HaveOccurred())

		cancelled, err := store.CancelRepository(ctx, pendingci.CancelRepositoryRequest{
			RepositoryID: first.RepositoryID,
			Reason:       "repository switched to the GitHub Action runner",
			CancelledAt:  now.Add(2 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(cancelled).To(HaveLen(2))
		for _, request := range cancelled {
			Expect(request.Lifecycle).To(Equal(pendingci.LifecycleCancelled))
			Expect(request.CleanupPending).To(BeTrue())
			Expect(request.LeaseExpiresAt).To(BeNil())
		}
		_, err = store.GetArmed(ctx, first.RepositoryID, first.PullRequest)
		Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())
		_, err = store.GetArmed(ctx, second.RepositoryID, second.PullRequest)
		Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())

		cancelled, err = store.CancelRepository(ctx, pendingci.CancelRepositoryRequest{
			RepositoryID: first.RepositoryID,
			Reason:       "repository switched to the GitHub Action runner",
			CancelledAt:  now.Add(3 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(cancelled).To(BeEmpty())
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
		SourceSequence:     1,
		SourceOrder:        commentID,
		Label:              "smyklot:pending:ci:squash:required",
		RequestedAt:        requestedAt,
	}
}
