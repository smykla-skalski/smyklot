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
