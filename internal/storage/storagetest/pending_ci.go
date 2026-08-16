package storagetest

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

func declarePendingCISpecs(runtime func() (context.Context, storage.Store, time.Time)) {
	It("retunes durable passing deadlines when the quiet period changes", func() {
		ctx, store, now := runtime()
		armed, err := store.Arm(ctx, pendingCIArm(now, 196, 99, "retune-head"))
		Expect(err).NotTo(HaveOccurred())
		lease, err := store.LeaseDue(ctx, now, now.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		_, err = store.Reschedule(ctx, pendingci.RescheduleRequest{
			ID: lease.Request.ID, ExpectedRevision: lease.Request.Revision,
			Schedule: pendingci.ScheduleActive, HeadSHA: armed.Request.HeadSHA,
			NextCheckAt: now.Add(24 * time.Hour), NextCheckTrigger: pendingci.TriggerQuietPeriod,
			LastProgressAt: now, LastObservedState: string(pendingci.ObservedPassing),
			LastFingerprint: "passing:1:1", CheckedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())

		changedAt := now.Add(2 * time.Second)
		changed, err := store.RetuneQuietPeriod(ctx, pendingci.RetuneQuietPeriodRequest{
			PassingQuiet: time.Second,
			ChangedAt:    changedAt,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(Equal(int64(1)))
		lease, err = store.LeaseDue(ctx, changedAt, changedAt.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(lease.Request).NotTo(BeNil())
		Expect(lease.Request.ID).To(Equal(armed.Request.ID))

		progressAt := changedAt
		_, err = store.Reschedule(ctx, pendingci.RescheduleRequest{
			ID: lease.Request.ID, ExpectedRevision: lease.Request.Revision,
			Schedule: pendingci.ScheduleActive, HeadSHA: armed.Request.HeadSHA,
			NextCheckAt:      progressAt.Add(time.Second),
			NextCheckTrigger: pendingci.TriggerQuietPeriod,
			LastProgressAt:   progressAt, LastObservedState: string(pendingci.ObservedPassing),
			LastFingerprint: "passing:1:1", CheckedAt: progressAt,
		})
		Expect(err).NotTo(HaveOccurred())
		changedAt = progressAt.Add(500 * time.Millisecond)
		changed, err = store.RetuneQuietPeriod(ctx, pendingci.RetuneQuietPeriodRequest{
			PassingQuiet: time.Hour,
			ChangedAt:    changedAt,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(Equal(int64(1)))
		notDue, err := store.LeaseDue(ctx, progressAt.Add(time.Minute), changedAt.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(notDue.Request).To(BeNil())
		Expect(notDue.AvailableAt).To(HaveValue(Equal(progressAt.Add(time.Hour))))
	})

	It("records webhook causality and completed pending CI history", func() {
		ctx, store, now := runtime()
		armed, err := store.Arm(ctx, pendingCIArm(now, 197, 100, "audit-head"))
		Expect(err).NotTo(HaveOccurred())
		wakeAt := now.Add(time.Second)
		changed, err := store.Wake(ctx, pendingci.WakeRequest{
			RepositoryID: armed.Request.RepositoryID, PullRequest: armed.Request.PullRequest,
			EventName: "check_run", EventKey: "check_run:501:completed",
			DeliveryID: "delivery-501", ExpectedHeadSHA: armed.Request.HeadSHA,
			OccurredAt: wakeAt,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeTrue())

		lease, err := store.LeaseDue(ctx, wakeAt, wakeAt.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		quietAt := wakeAt.Add(30 * time.Second)
		rescheduled, err := store.Reschedule(ctx, pendingci.RescheduleRequest{
			ID: lease.Request.ID, ExpectedRevision: lease.Request.Revision,
			Schedule: pendingci.ScheduleActive, HeadSHA: armed.Request.HeadSHA,
			NextCheckAt: quietAt, NextCheckTrigger: pendingci.TriggerQuietPeriod,
			LastProgressAt: wakeAt, LastObservedState: string(pendingci.ObservedPassing),
			LastFingerprint: "passing:2:2", ObservationSummary: "2/2 checks passing",
			CheckedAt: wakeAt,
		})
		Expect(err).NotTo(HaveOccurred())
		lease, err = store.LeaseDue(ctx, quietAt, quietAt.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(lease.Request.ID).To(Equal(rescheduled.ID))
		claimed, err := store.ClaimMerge(ctx, pendingci.ClaimMergeRequest{
			ID: lease.Request.ID, ExpectedRevision: lease.Request.Revision,
			Observation: pendingci.Observation{
				State: pendingci.ObservedPassing, Summary: "2/2 checks passing",
			},
			ClaimedAt: quietAt,
		})
		Expect(err).NotTo(HaveOccurred())
		finished, err := store.Finish(ctx, pendingci.FinishRequest{
			ID: claimed.ID, ExpectedRevision: claimed.Revision,
			Lifecycle: pendingci.LifecycleMerged, Trigger: pendingci.TriggerQuietPeriod,
			Reason: "CI passed and pull request merged", FinishedAt: quietAt.Add(time.Second),
		})
		Expect(err).NotTo(HaveOccurred())

		events, err := store.ListEvents(ctx, pendingci.EventFilter{RequestID: finished.ID})
		Expect(err).NotTo(HaveOccurred())
		Expect(events).To(HaveLen(8))
		Expect(events[1].Kind).To(Equal(pendingci.EventWakeReceived))
		Expect(events[1].Trigger).To(Equal(pendingci.TriggerWebhook))
		Expect(events[1].EventName).To(Equal("check_run"))
		Expect(events[1].DeliveryID).To(Equal("delivery-501"))
		Expect(events[3].Kind).To(Equal(pendingci.EventChecksObserved))
		Expect(events[3].Summary).To(Equal("2/2 checks passing"))
		Expect(events[4].Trigger).To(Equal(pendingci.TriggerQuietPeriod))
		Expect(events[6].Kind).To(Equal(pendingci.EventMergeStarted))
		Expect(events[7].Kind).To(Equal(pendingci.EventFinished))

		history, err := store.ListHistory(ctx, pendingci.HistoryFilter{Limit: 5})
		Expect(err).NotTo(HaveOccurred())
		Expect(history).To(HaveLen(1))
		Expect(history[0].ID).To(Equal(finished.ID))
	})

	It("records the true cause of terminal pending CI events", func() {
		ctx, store, now := runtime()
		fallbackArm := pendingCIArm(now, 199, 102, "fallback-head")
		fallback, err := store.Arm(ctx, fallbackArm)
		Expect(err).NotTo(HaveOccurred())
		finished, err := store.FinishPR(ctx, pendingci.FinishPRRequest{
			RepositoryID: fallbackArm.RepositoryID, PullRequest: fallbackArm.PullRequest,
			Lifecycle: pendingci.LifecycleCancelled, Trigger: pendingci.TriggerFallback,
			Reason: "cleanup reaction found by sweep", FinishedAt: now.Add(time.Second),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(finished.ID).To(Equal(fallback.Request.ID))
		events, err := store.ListEvents(ctx, pendingci.EventFilter{RequestID: finished.ID})
		Expect(err).NotTo(HaveOccurred())
		Expect(events).To(HaveLen(2))
		Expect(events[1].Trigger).To(Equal(pendingci.TriggerFallback))

		webhookArm := pendingCIArm(now.Add(time.Minute), 200, 103, "webhook-head")
		webhookRequest, err := store.Arm(ctx, webhookArm)
		Expect(err).NotTo(HaveOccurred())
		cancelled, err := store.CancelBySource(ctx, pendingci.CancelRequest{
			RepositoryID: webhookArm.RepositoryID, PullRequest: webhookArm.PullRequest,
			CommentID:      webhookArm.SourceCommentID,
			SourceRevision: now.Add(2 * time.Minute).Format(time.RFC3339Nano),
			SourceSequence: 2, SourceOrder: webhookArm.SourceOrder + 1,
			Trigger: pendingci.TriggerWebhook,
			Reason:  "source comment edited", CancelledAt: now.Add(2 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(cancelled.ID).To(Equal(webhookRequest.Request.ID))
		events, err = store.ListEvents(ctx, pendingci.EventFilter{RequestID: cancelled.ID})
		Expect(err).NotTo(HaveOccurred())
		Expect(events).To(HaveLen(2))
		Expect(events[1].Trigger).To(Equal(pendingci.TriggerWebhook))
	})

	It("persists the pending CI lifecycle and its crash-safe cleanup phases", func() {
		ctx, store, now := runtime()
		arm := pendingCIArm(now, 198, 101, "cleanup-head")
		err := store.CheckArm(ctx, arm)
		Expect(err).NotTo(HaveOccurred())
		_, err = store.GetArmed(ctx, arm.RepositoryID, arm.PullRequest)
		Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())

		armed, err := store.Arm(ctx, arm)
		Expect(err).NotTo(HaveOccurred())

		lease, err := store.LeaseDue(ctx, now, now.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(lease.Request).NotTo(BeNil())
		finishedAt := now.Add(time.Second)
		finished, err := store.Finish(ctx, pendingci.FinishRequest{
			ID: armed.Request.ID, ExpectedRevision: lease.Request.Revision,
			Lifecycle: pendingci.LifecycleCancelled,
			Reason:    "source command removed", FinishedAt: finishedAt,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(finished.CleanupPending).To(BeTrue())
		Expect(finished.CleanupArtifactsDone).To(BeFalse())

		_, err = store.CompleteCleanup(ctx, pendingci.CompleteCleanupRequest{
			ID: finished.ID, ExpectedRevision: finished.Revision, CompletedAt: finishedAt,
		})
		Expect(errors.Is(err, storage.ErrConflict)).To(BeTrue())
		artifactsDone, err := store.MarkCleanupArtifactsDone(
			ctx,
			pendingci.MarkCleanupArtifactsDoneRequest{
				ID: finished.ID, ExpectedRevision: finished.Revision, MarkedAt: finishedAt,
			},
		)
		Expect(err).NotTo(HaveOccurred())
		pending, err := store.HasPendingCleanup(ctx, pendingci.CleanupFilter{
			RepositoryID: armed.Request.RepositoryID,
			PullRequest:  armed.Request.PullRequest,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(pending).To(BeTrue())

		completed, err := store.CompleteCleanup(ctx, pendingci.CompleteCleanupRequest{
			ID: artifactsDone.ID, ExpectedRevision: artifactsDone.Revision,
			CompletedAt: finishedAt,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(completed.CleanupPending).To(BeFalse())
	})

	It("orders pending CI cleanup and arm intents across source comments", func() {
		ctx, store, now := runtime()
		newer := pendingCIArm(now.Add(time.Minute), 198, 202, "new-head")
		armed, err := store.Arm(ctx, newer)
		Expect(err).NotTo(HaveOccurred())

		stale, err := store.CancelByIntent(ctx, pendingci.CancelIntentRequest{
			RepositoryID: newer.RepositoryID, PullRequest: newer.PullRequest,
			CommentID: 101, SourceRevision: now.Format(time.RFC3339Nano),
			SourceSequence: 1, SourceOrder: 101,
			Reason: "delayed cleanup", CancelledAt: now.Add(2 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(stale.Accepted).To(BeFalse())
		current, err := store.GetArmed(ctx, newer.RepositoryID, newer.PullRequest)
		Expect(err).NotTo(HaveOccurred())
		Expect(current.ID).To(Equal(armed.Request.ID))

		cancelledAt := now.Add(2 * time.Minute)
		cancelled, err := store.CancelByIntent(ctx, pendingci.CancelIntentRequest{
			RepositoryID: newer.RepositoryID, PullRequest: newer.PullRequest,
			CommentID: 303, SourceRevision: cancelledAt.Format(time.RFC3339Nano),
			SourceSequence: 1, SourceOrder: 303,
			Reason: "new cleanup", CancelledAt: cancelledAt,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(cancelled.Accepted).To(BeTrue())
		Expect(cancelled.Request.ID).To(Equal(armed.Request.ID))

		delayed := pendingCIArm(now.Add(90*time.Second), 198, 404, "delayed-head")
		err = store.CheckArm(ctx, delayed)
		Expect(errors.Is(err, pendingci.ErrStaleSourceRevision)).To(BeTrue())
		_, err = store.Arm(ctx, delayed)
		Expect(errors.Is(err, pendingci.ErrStaleSourceRevision)).To(BeTrue())
	})

	It("rejects a stale same-comment source delivery", func() {
		ctx, store, now := runtime()
		latest := pendingci.SourceRevisionRequest{
			RepositoryID: "repository-20", PullRequest: 198, CommentID: 101,
			Revision: now.Format(time.RFC3339Nano), Sequence: 2, SourceOrder: 2,
			EventKey: "delivery:latest", ObservedAt: now,
		}
		result, err := store.ClaimSourceRevision(ctx, latest)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Accepted).To(BeTrue())

		stale := latest
		stale.SourceOrder = 1
		stale.EventKey = "delivery:stale"
		result, err = store.ClaimSourceRevision(ctx, stale)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Accepted).To(BeFalse())
	})
}

func pendingCIArm(
	requestedAt time.Time,
	pullRequest int,
	commentID int64,
	headSHA string,
) pendingci.ArmRequest {
	return pendingci.ArmRequest{
		TargetID: "installation:77", InstallationID: 77,
		RepositoryID: "repository-20", RepositoryFullName: "smykla-skalski/smyklot",
		PullRequest: pullRequest, HeadSHA: headSHA, BaseBranch: "main",
		MergeMethod: pendingci.MergeMethodSquash, RequiredChecksOnly: true,
		Requester: "operator", SourceCommentID: commentID,
		SourceRevision: requestedAt.Format(time.RFC3339Nano),
		SourceSequence: 1, SourceOrder: commentID,
		Label: "smyklot:pending:ci:squash:required", RequestedAt: requestedAt,
	}
}
