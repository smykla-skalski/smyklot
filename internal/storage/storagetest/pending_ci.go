package storagetest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func declarePendingCISpecs(runtime func() (context.Context, storage.Store, time.Time)) {
	seedCheckCatalog := func(ctx context.Context, store storage.Store, now time.Time) {
		GinkgoHelper()
		Expect(store.ReconcileInstallation(ctx, storage.InstallationSnapshot{
			TargetID: "installation:77", InstallationID: "77", Kind: storage.TargetOrganization,
			Account: storage.Account{
				ID: "account:pending-ci", Provider: "github", SubjectID: "77",
				Login: "owner", UpdatedAt: now,
			},
			Repositories: []storage.RepositorySnapshot{{
				ID: "repository-20", Name: "repo", FullName: "owner/repo", DefaultBranch: "main",
			}},
			SyncedAt: now,
		})).To(Succeed())
		gate, err := store.GetPendingCIRepositoryGate(ctx, "repository-20")
		Expect(err).NotTo(HaveOccurred())
		Expect(gate.DesiredMode).To(Equal(storage.PendingCIModeChecks))
		Expect(gate.Readiness).To(Equal(storage.PendingCIProvisioning))
	}
	ensureSlot := func(
		ctx context.Context,
		store storage.Store,
		at time.Time,
		pullRequest int,
		headSHA, suffix, digest string,
	) pendingci.CheckSlot {
		GinkgoHelper()
		slot, err := store.EnsureCheckSlot(
			ctx,
			pendingCICheckSlot(at, pullRequest, headSHA, suffix, digest),
		)
		Expect(err).NotTo(HaveOccurred())

		return slot
	}
	deferRequest := func(
		ctx context.Context,
		store storage.Store,
		arm pendingci.ArmRequest,
	) pendingci.Request {
		GinkgoHelper()
		armed, err := store.Arm(ctx, arm)
		Expect(err).NotTo(HaveOccurred())
		lease, err := store.LeaseDue(ctx, arm.RequestedAt, arm.RequestedAt.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(lease.Request).NotTo(BeNil())
		deferred, err := store.Reschedule(ctx, pendingci.RescheduleRequest{
			ID: lease.Request.ID, ExpectedRevision: lease.Request.Revision,
			Schedule: pendingci.ScheduleDeferred, HeadSHA: armed.Request.HeadSHA,
			NextCheckAt:       arm.RequestedAt.Add(6 * time.Hour),
			NextCheckTrigger:  pendingci.TriggerFallback,
			LastProgressAt:    arm.RequestedAt.Add(-time.Hour),
			LastObservedState: string(pendingci.ObservedIndeterminate),
			LastFingerprint:   "deferred", CheckedAt: arm.RequestedAt,
		})
		Expect(err).NotTo(HaveOccurred())

		return deferred
	}

	It("lists repository gates within one installation", func() {
		ctx, store, now := runtime()
		seedCheckCatalog(ctx, store, now)

		gates, err := store.ListTargetPendingCIRepositoryGates(ctx, "installation:77")
		Expect(err).NotTo(HaveOccurred())
		Expect(gates).To(HaveLen(1))
		Expect(gates[0].RepositoryID).To(Equal("repository-20"))

		other, err := store.ListTargetPendingCIRepositoryGates(ctx, "installation:other")
		Expect(err).NotTo(HaveOccurred())
		Expect(other).To(BeEmpty())
	})

	/* The panel lists work in flight by what it is ABOUT, and the pull
	   request's own title is the one thing about it nothing downstream can
	   recover - the queue holds a number, and asking GitHub means a request
	   per row on a page the reader is already looking at. */
	It("keeps the pull request's own title, on the request and on its queue row", func() {
		ctx, store, now := runtime()
		seedCheckCatalog(ctx, store, now)

		arm := pendingCIArm(now, 214, 114, "head")
		armed, err := store.Arm(ctx, arm)
		Expect(err).NotTo(HaveOccurred())
		Expect(armed.Request.PullRequestTitle).To(Equal(arm.PullRequestTitle))

		reread, err := store.Get(ctx, armed.Request.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(reread.PullRequestTitle).To(Equal(arm.PullRequestTitle))

		item, err := store.GetQueueItem(ctx, "pending-ci:"+fmt.Sprint(armed.Request.ID))
		Expect(err).NotTo(HaveOccurred())
		Expect(queueDetails(item)).To(HaveKeyWithValue("pull_request_title", arm.PullRequestTitle))

		/* A request armed before the column existed reads back empty, and the
		   row leaves the key out rather than carrying "" - an empty title is
		   not a title, and a reader falls back to the act. */
		untitled := pendingCIArm(now.Add(time.Minute), 215, 115, "head-2")
		untitled.PullRequestTitle = ""
		second, err := store.Arm(ctx, untitled)
		Expect(err).NotTo(HaveOccurred())
		Expect(second.Request.PullRequestTitle).To(BeEmpty())
		bare, err := store.GetQueueItem(ctx, "pending-ci:"+fmt.Sprint(second.Request.ID))
		Expect(err).NotTo(HaveOccurred())
		Expect(queueDetails(bare)).NotTo(HaveKey("pull_request_title"))
	})

	It("keeps source deadlines and disabled policy state aligned with the queue", func() {
		ctx, store, now := runtime()
		seedCheckCatalog(ctx, store, now)
		targetID := "installation:77"
		policy, err := store.GetEffectiveQueuePolicy(ctx, workqueue.KindPendingCI, &targetID)
		Expect(err).NotTo(HaveOccurred())
		policy, err = store.SaveQueuePolicy(ctx, workqueue.PolicyChange{
			Kind: policy.Kind, TargetID: &targetID, Enabled: false,
			Cadence: policy.Cadence, ProfileID: policy.ProfileID,
			DefaultPriority: policy.DefaultPriority, RetryDelay: policy.RetryDelay,
			Retention: policy.Retention, ApprovalTTL: policy.ApprovalTTL,
			Configuration: policy.Configuration, ExpectedRevision: 0,
			ActorID: "account:pending-ci", ChangedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())

		armed, err := store.Arm(ctx, pendingCIArm(now, 214, 114, "head"))
		Expect(err).NotTo(HaveOccurred())
		itemID := "pending-ci:" + fmt.Sprint(armed.Request.ID)
		item, err := store.GetQueueItem(ctx, itemID)
		Expect(err).NotTo(HaveOccurred())
		Expect(item.State).To(Equal(workqueue.StateBlocked))
		Expect(item.BlockedReason).To(Equal("Workload disabled by policy"))

		policy, err = store.SaveQueuePolicy(ctx, workqueue.PolicyChange{
			Kind: policy.Kind, TargetID: &targetID, Enabled: true,
			Cadence: policy.Cadence, ProfileID: policy.ProfileID,
			DefaultPriority: policy.DefaultPriority, RetryDelay: policy.RetryDelay,
			Retention: policy.Retention, ApprovalTTL: policy.ApprovalTTL,
			Configuration: policy.Configuration, ExpectedRevision: policy.Revision,
			ActorID: "account:pending-ci", ChangedAt: now.Add(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		item, err = store.GetQueueItem(ctx, itemID)
		Expect(err).NotTo(HaveOccurred())
		Expect(item.State).NotTo(Equal(workqueue.StateBlocked))

		scheduledAt := now.Add(4 * time.Hour)
		item, err = store.ApplyQueueAction(ctx, itemID, workqueue.ItemAction{
			Type: workqueue.ActionScheduleAt, ExpectedRevision: item.Revision,
			ActorID: "account:pending-ci", At: scheduledAt, ChangedAt: now.Add(2 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		request, err := store.GetArmed(ctx, armed.Request.RepositoryID, armed.Request.PullRequest)
		Expect(err).NotTo(HaveOccurred())
		Expect(request.NextCheckAt).To(Equal(item.EligibleAt))

		runAt := now.Add(3 * time.Minute)
		item, err = store.ApplyQueueAction(ctx, itemID, workqueue.ItemAction{
			Type: workqueue.ActionRunNow, ExpectedRevision: item.Revision,
			ActorID: "account:pending-ci", Reason: "check pending CI now", ChangedAt: runAt,
		})
		Expect(err).NotTo(HaveOccurred())
		request, err = store.GetArmed(ctx, armed.Request.RepositoryID, armed.Request.PullRequest)
		Expect(err).NotTo(HaveOccurred())
		Expect(request.NextCheckAt).To(Equal(runAt))
		Expect(request.NextCheckAt).To(Equal(item.EligibleAt))
		lease, err := store.LeaseDue(ctx, runAt, runAt.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(lease.Request).NotTo(BeNil())
		Expect(lease.Request.ID).To(Equal(armed.Request.ID))
	})

	It("persists exact check reauthorization and idempotent action replay", func() {
		ctx, store, now := runtime()
		seedCheckCatalog(ctx, store, now)
		first := ensureSlot(ctx, store, now, 198, "old-head", "old", "authorized")
		bound, err := store.BindCheckRun(ctx, pendingci.BindCheckRunRequest{
			ID: first.ID, ExpectedRevision: first.Revision, CheckRunID: 701,
			CheckURL: "https://github.example/checks/701", BoundAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		applied, err := store.ApplyCheckSlot(ctx, pendingci.ApplyCheckSlotRequest{
			ID: bound.ID, ExpectedRevision: bound.Revision, AppliedDigest: bound.DesiredDigest,
			CheckRunID: 701, CheckURL: bound.CheckURL, AppliedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(applied.State).To(Equal(pendingci.CheckSlotReady))
		Expect(applied.AppliedDigest).To(Equal("authorized"))

		arm := pendingCIArm(now, 198, 101, "old-head")
		arm.RepositoryFullName = "owner/repo"
		arm.ArtifactKind = pendingci.ArtifactCheck
		arm.Label = ""
		arm.CheckSlotID = &applied.ID
		armed, err := store.Arm(ctx, arm)
		Expect(err).NotTo(HaveOccurred())
		second := ensureSlot(ctx, store, now.Add(time.Minute), 198, "new-head", "new", "reauthorize")
		waiting, err := store.RequireReauthorization(ctx, pendingci.RequireReauthorizationRequest{
			ID: armed.Request.ID, ExpectedRevision: armed.Request.Revision,
			CandidateHeadSHA: "new-head", CandidateBase: "release",
			CandidateCheckID: second.ID, ObservedAt: now.Add(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(waiting.HeadSHA).To(Equal("old-head"))
		Expect(waiting.CandidateHeadSHA).To(Equal("new-head"))
		Expect(waiting.AuthorizationState).To(Equal(pendingci.AuthorizationReauthorizationNeeded))
		Expect(waiting.RetiredCheckSlotID).NotTo(BeNil())
		Expect(*waiting.RetiredCheckSlotID).To(Equal(first.ID))

		change := pendingci.ReauthorizeRequest{
			RepositoryID: arm.RepositoryID, PullRequest: arm.PullRequest,
			HeadSHA: "new-head", BaseBranch: "release", CheckSlotID: second.ID,
			Actor: "maintainer", EventKey: "check_run:701:requested_action:reauthorize",
			DeliveryID: "delivery-701", AuthorizedAt: now.Add(2 * time.Minute),
		}
		reauthorized, err := store.Reauthorize(ctx, change)
		Expect(err).NotTo(HaveOccurred())
		Expect(reauthorized).NotTo(BeNil())
		Expect(reauthorized.HeadSHA).To(Equal("new-head"))
		Expect(reauthorized.AuthorizedBy).To(Equal("maintainer"))
		Expect(reauthorized.AuthorizationState).To(Equal(pendingci.AuthorizationAuthorized))
		replayed, err := store.Reauthorize(ctx, change)
		Expect(err).NotTo(HaveOccurred())
		Expect(replayed).NotTo(BeNil())
		Expect(replayed.Revision).To(Equal(reauthorized.Revision))
	})

	It("swaps a returning reauthorization head without losing the displaced cleanup", func() {
		ctx, store, now := runtime()
		seedCheckCatalog(ctx, store, now)
		first := ensureSlot(ctx, store, now, 198, "head-a", "a", "authorized-a")
		arm := pendingCIArm(now, 198, 101, "head-a")
		arm.RepositoryFullName = "owner/repo"
		arm.ArtifactKind = pendingci.ArtifactCheck
		arm.Label = ""
		arm.CheckSlotID = &first.ID
		armed, err := store.Arm(ctx, arm)
		Expect(err).NotTo(HaveOccurred())
		second := ensureSlot(ctx, store, now.Add(time.Minute), 198, "head-b", "b", "reauthorize-b")
		waiting, err := store.RequireReauthorization(ctx, pendingci.RequireReauthorizationRequest{
			ID: armed.Request.ID, ExpectedRevision: armed.Request.Revision,
			CandidateHeadSHA: "head-b", CandidateBase: "main",
			CandidateCheckID: second.ID, ObservedAt: now.Add(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		returned, err := store.RequireReauthorization(ctx, pendingci.RequireReauthorizationRequest{
			ID: waiting.ID, ExpectedRevision: waiting.Revision,
			CandidateHeadSHA: "head-a", CandidateBase: "main",
			CandidateCheckID: first.ID, ObservedAt: now.Add(2 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(returned.CheckSlotID).NotTo(BeNil())
		Expect(*returned.CheckSlotID).To(Equal(first.ID))
		Expect(returned.RetiredCheckSlotID).NotTo(BeNil())
		Expect(*returned.RetiredCheckSlotID).To(Equal(second.ID))

		cleared, err := store.ClearRetiredCheckSlot(ctx, pendingci.ClearRetiredCheckSlotRequest{
			ID: returned.ID, ExpectedRevision: returned.Revision,
			CheckSlotID: second.ID, ClearedAt: now.Add(3 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(cleared.RetiredCheckSlotID).To(BeNil())
	})

	It("rejects active shared heads, reassigns retired slots, and renews durably", func() {
		ctx, store, now := runtime()
		seedCheckCatalog(ctx, store, now)
		slot := ensureSlot(ctx, store, now, 42, "shared", "original", "digest")
		_, err := store.EnsureCheckSlot(ctx, pendingCICheckSlot(now, 43, "shared", "other", "other"))
		Expect(errors.Is(err, pendingci.ErrSharedHead)).To(BeTrue())
		reassigned, err := store.ReassignCheckSlot(ctx, pendingci.ReassignCheckSlotRequest{
			ID: slot.ID, ExpectedRevision: slot.Revision,
			PullRequest: 43, ReassignedAt: now.Add(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(reassigned.PullRequest).To(Equal(43))
		slot = reassigned
		renewed, err := store.RenewCheckSlot(ctx, pendingci.RenewCheckSlotRequest{
			ID: slot.ID, ExpectedRevision: slot.Revision,
			ExternalID: "smyklot:merge-after-ci:repository-20:shared:g2",
			RenewedAt:  now.Add(13 * 24 * time.Hour),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(renewed.Generation).To(Equal(int64(2)))
		Expect(renewed.CheckRunID).To(BeNil())
		Expect(renewed.AppliedDigest).To(BeEmpty())
		Expect(renewed.State).To(Equal(pendingci.CheckSlotProvisioning))
	})

	It("refreshes check ownership after a repository transfer", func() {
		ctx, store, now := runtime()
		seedCheckCatalog(ctx, store, now)
		originalGate, err := store.GetPendingCIRepositoryGate(ctx, "repository-20")
		Expect(err).NotTo(HaveOccurred())
		original := ensureSlot(ctx, store, now, 42, "transfer-head", "transfer-head", "digest")
		arm := pendingCIArm(now, 42, 101, "transfer-head")
		arm.RepositoryFullName = "owner/repo"
		arm.ArtifactKind = pendingci.ArtifactCheck
		arm.Label = ""
		arm.CheckSlotID = &original.ID
		armed, err := store.Arm(ctx, arm)
		Expect(err).NotTo(HaveOccurred())
		Expect(store.ReconcileInstallation(ctx, storage.InstallationSnapshot{
			TargetID: "installation:88", InstallationID: "88", Kind: storage.TargetOrganization,
			Account: storage.Account{
				ID: "account:new-owner", Provider: "github", SubjectID: "88",
				Login: "new-owner", UpdatedAt: now.Add(time.Minute),
			},
			Repositories: []storage.RepositorySnapshot{{
				ID: "repository-20", Name: "repo", FullName: "new-owner/repo", DefaultBranch: "main",
			}},
			SyncedAt: now.Add(time.Minute),
		})).To(Succeed())
		refreshed, err := store.GetCheckSlot(ctx, original.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(refreshed.ID).To(Equal(original.ID))
		Expect(refreshed.TargetID).To(Equal("installation:88"))
		Expect(refreshed.InstallationID).To(Equal(int64(88)))
		Expect(refreshed.RepositoryFullName).To(Equal("new-owner/repo"))
		Expect(refreshed.Revision).To(BeNumerically(">", original.Revision))
		request, err := store.GetArmed(ctx, arm.RepositoryID, arm.PullRequest)
		Expect(err).NotTo(HaveOccurred())
		Expect(request.TargetID).To(Equal("installation:88"))
		Expect(request.InstallationID).To(Equal(int64(88)))
		Expect(request.RepositoryFullName).To(Equal("new-owner/repo"))
		Expect(request.Revision).To(BeNumerically(">", armed.Request.Revision))
		Expect(request.NextCheckAt).To(Equal(now.Add(time.Minute)))
		Expect(request.LeaseExpiresAt).To(BeNil())
		gate, err := store.GetPendingCIRepositoryGate(ctx, arm.RepositoryID)
		Expect(err).NotTo(HaveOccurred())
		Expect(gate.TargetID).To(Equal("installation:88"))
		Expect(gate.Revision).To(BeNumerically(">", originalGate.Revision))
		Expect(gate.Readiness).To(Equal(storage.PendingCIProvisioning))
	})

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
		staleLease := *lease.Request
		changedAt = changedAt.Add(100 * time.Millisecond)
		changed, err = store.RetuneQuietPeriod(ctx, pendingci.RetuneQuietPeriodRequest{
			PassingQuiet: time.Second,
			ChangedAt:    changedAt,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(Equal(int64(1)))
		_, err = store.Reschedule(ctx, pendingci.RescheduleRequest{
			ID: staleLease.ID, ExpectedRevision: staleLease.Revision,
			Schedule: pendingci.ScheduleActive, HeadSHA: staleLease.HeadSHA,
			NextCheckAt: now.Add(24 * time.Hour), NextCheckTrigger: pendingci.TriggerQuietPeriod,
			LastProgressAt: now, LastObservedState: string(pendingci.ObservedPassing),
			LastFingerprint: "stale-worker", CheckedAt: changedAt,
		})
		Expect(errors.Is(err, storage.ErrConflict)).To(BeTrue())
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

	It("wakes deferred check requests when their repository gate recovers", func() {
		ctx, store, now := runtime()
		seedCheckCatalog(ctx, store, now)
		slot := ensureSlot(ctx, store, now, 201, "gate-head", "gate", "authorized")
		arm := pendingCIArm(now, 201, 104, "gate-head")
		arm.RepositoryFullName = "owner/repo"
		arm.ArtifactKind = pendingci.ArtifactCheck
		arm.Label = ""
		arm.CheckSlotID = &slot.ID
		armed, err := store.Arm(ctx, arm)
		Expect(err).NotTo(HaveOccurred())
		lease, err := store.LeaseDue(ctx, now, now.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		_, err = store.Reschedule(ctx, pendingci.RescheduleRequest{
			ID: lease.Request.ID, ExpectedRevision: lease.Request.Revision,
			Schedule: pendingci.ScheduleDeferred, HeadSHA: armed.Request.HeadSHA,
			NextCheckAt: now.Add(6 * time.Hour), NextCheckTrigger: pendingci.TriggerFallback,
			LastProgressAt:    now.Add(-time.Hour),
			LastObservedState: string(pendingci.ObservedIndeterminate),
			LastFingerprint:   "gate:not-ready", CheckedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		gate, err := store.GetPendingCIRepositoryGate(ctx, arm.RepositoryID)
		Expect(err).NotTo(HaveOccurred())
		readyAt := now.Add(time.Second)
		appID := int64(17)
		_, err = store.UpdatePendingCIRepositoryGate(ctx, storage.PendingCIGateChange{
			RepositoryID: arm.RepositoryID, ExpectedRevision: gate.Revision,
			EffectiveMode: storage.PendingCIEffectiveChecks, Readiness: storage.PendingCIReady,
			Reason: "ready", AppID: &appID, ObservedAt: readyAt,
		})
		Expect(err).NotTo(HaveOccurred())
		lease, err = store.LeaseDue(ctx, readyAt, readyAt.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(lease.Request).NotTo(BeNil())
		Expect(lease.Request.ID).To(Equal(armed.Request.ID))
	})

	It("wakes waiting check requests when branch settings change", func() {
		ctx, store, now := runtime()
		seedCheckCatalog(ctx, store, now)
		slot := ensureSlot(ctx, store, now, 202, "settings-head", "settings", "authorized")
		arm := pendingCIArm(now, 202, 105, "settings-head")
		arm.RepositoryFullName = "owner/repo"
		arm.ArtifactKind = pendingci.ArtifactCheck
		arm.Label = ""
		arm.CheckSlotID = &slot.ID
		armed, err := store.Arm(ctx, arm)
		Expect(err).NotTo(HaveOccurred())
		lease, err := store.LeaseDue(ctx, now, now.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		_, err = store.Reschedule(ctx, pendingci.RescheduleRequest{
			ID: lease.Request.ID, ExpectedRevision: lease.Request.Revision,
			Schedule: pendingci.ScheduleDeferred, HeadSHA: armed.Request.HeadSHA,
			NextCheckAt: now.Add(6 * time.Hour), NextCheckTrigger: pendingci.TriggerFallback,
			LastProgressAt:    now.Add(-time.Hour),
			LastObservedState: string(pendingci.ObservedIndeterminate),
			LastFingerprint:   "waiting", CheckedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		repository, err := store.GetRepository(ctx, arm.TargetID, arm.RepositoryID)
		Expect(err).NotTo(HaveOccurred())
		changedAt := now.Add(time.Second)
		patterns := storage.PendingCIBranchPatterns{Include: []string{"refs/heads/release/*"}}
		_, err = store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: arm.TargetID, ActorAccountID: "account:pending-ci", ChangedAt: changedAt,
			Repositories: []storage.InstallationRepositorySettingsChange{{
				RepositoryID: arm.RepositoryID, PendingCIBranchPatternsOverride: &patterns,
				ExpectedRevision: repository.Revision,
			}},
		})
		Expect(err).NotTo(HaveOccurred())
		lease, err = store.LeaseDue(ctx, changedAt, changedAt.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(lease.Request).NotTo(BeNil())
		Expect(lease.Request.ID).To(Equal(armed.Request.ID))
	})

	It("wakes deferred label requests when a repository is disabled", func() {
		ctx, store, now := runtime()
		seedCheckCatalog(ctx, store, now)
		labelMode := storage.PendingCIModeLabels
		enabled := true
		saved, err := store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: "installation:77", ActorAccountID: "account:pending-ci",
			ChangedAt: now.Add(time.Minute),
			Repositories: []storage.InstallationRepositorySettingsChange{{
				RepositoryID: "repository-20", EnabledOverride: &enabled,
				PendingCIModeOverride: &labelMode, ExpectedRevision: 1,
			}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(saved.Repositories).To(HaveLen(1))
		repository := saved.Repositories[0]
		arm := pendingCIArm(now.Add(2*time.Minute), 203, 106, "label-repository-head")
		arm.RepositoryFullName = "owner/repo"
		deferred := deferRequest(ctx, store, arm)
		disabled := false
		changedAt := now.Add(3 * time.Minute)
		_, err = store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: arm.TargetID, ActorAccountID: "account:pending-ci", ChangedAt: changedAt,
			Repositories: []storage.InstallationRepositorySettingsChange{{
				RepositoryID: arm.RepositoryID, EnabledOverride: &disabled,
				PendingCIModeOverride: &labelMode, ExpectedRevision: repository.Revision,
			}},
		})
		Expect(err).NotTo(HaveOccurred())
		lease, err := store.LeaseDue(ctx, changedAt, changedAt.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(lease.Request).NotTo(BeNil())
		Expect(lease.Request.ID).To(Equal(deferred.ID))
	})

	It("wakes deferred label requests when an inherited target is disabled", func() {
		ctx, store, now := runtime()
		seedCheckCatalog(ctx, store, now)
		saved, err := store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: "installation:77", ActorAccountID: "account:pending-ci",
			ChangedAt: now.Add(time.Minute),
			Target: &storage.InstallationTargetSettingsChange{
				RepositoryDefaultEnabled: true, PendingCIModeDefault: storage.PendingCIModeLabels,
				ExpectedRevision: 1,
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(saved.Target).NotTo(BeNil())
		target := *saved.Target
		arm := pendingCIArm(now.Add(2*time.Minute), 204, 107, "label-target-head")
		arm.RepositoryFullName = "owner/repo"
		deferred := deferRequest(ctx, store, arm)
		changedAt := now.Add(3 * time.Minute)
		_, err = store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: arm.TargetID, ActorAccountID: "account:pending-ci", ChangedAt: changedAt,
			Target: &storage.InstallationTargetSettingsChange{
				RepositoryDefaultEnabled: false, PendingCIModeDefault: storage.PendingCIModeLabels,
				ExpectedRevision: target.Revision,
			},
		})
		Expect(err).NotTo(HaveOccurred())
		lease, err := store.LeaseDue(ctx, changedAt, changedAt.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(lease.Request).NotTo(BeNil())
		Expect(lease.Request.ID).To(Equal(deferred.ID))
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

	It("orders draft transitions and commands by GitHub source time", func() {
		ctx, store, now := runtime()
		draftedAt := now.Add(time.Minute)
		barrier, err := store.RecordDraftTransition(ctx, pendingci.DraftTransitionRequest{
			RepositoryID: "repository-20", PullRequest: 199,
			EventKey:  "pull_request:9001:199:converted_to_draft",
			DraftedAt: draftedAt, RecordedAt: now.Add(time.Hour),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(barrier.Changed).To(BeTrue())
		Expect(barrier.Finished).To(BeNil())

		delayed := pendingCIArm(now.Add(2*time.Hour), 199, 102, "delayed-head")
		delayed.SourceRevision = now.Format(time.RFC3339Nano)
		_, err = store.Arm(ctx, delayed)
		Expect(errors.Is(err, pendingci.ErrStaleSourceRevision)).To(BeTrue())

		ambiguous := pendingCIArm(now.Add(2*time.Hour), 199, 103, "ambiguous-head")
		ambiguous.SourceRevision = draftedAt.Format(time.RFC3339Nano)
		_, err = store.Arm(ctx, ambiguous)
		Expect(errors.Is(err, pendingci.ErrAmbiguousSourceRevision)).To(BeTrue())
		_, err = store.CancelByIntent(ctx, pendingci.CancelIntentRequest{
			RepositoryID: ambiguous.RepositoryID, PullRequest: ambiguous.PullRequest,
			CommentID: ambiguous.SourceCommentID, SourceRevision: ambiguous.SourceRevision,
			SourceSequence: ambiguous.SourceSequence, SourceOrder: ambiguous.SourceOrder,
			Reason: "same-second immediate merge", CancelledAt: now.Add(2 * time.Hour),
		})
		Expect(errors.Is(err, pendingci.ErrAmbiguousSourceRevision)).To(BeTrue())

		fresh := pendingCIArm(now.Add(2*time.Hour), 199, 104, "fresh-head")
		fresh.SourceRevision = draftedAt.Add(time.Second).Format(time.RFC3339Nano)
		armed, err := store.Arm(ctx, fresh)
		Expect(err).NotTo(HaveOccurred())
		Expect(armed.Request.ID).NotTo(BeZero())

		staleDraft, err := store.RecordDraftTransition(ctx, pendingci.DraftTransitionRequest{
			RepositoryID: fresh.RepositoryID, PullRequest: fresh.PullRequest,
			EventKey:  "pull_request:9001:199:older-draft",
			DraftedAt: draftedAt, RecordedAt: now.Add(3 * time.Hour),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(staleDraft.Changed).To(BeFalse())
		current, err := store.GetArmed(ctx, fresh.RepositoryID, fresh.PullRequest)
		Expect(err).NotTo(HaveOccurred())
		Expect(current.ID).To(Equal(armed.Request.ID))

		cancelled, err := store.RecordDraftTransition(ctx, pendingci.DraftTransitionRequest{
			RepositoryID: fresh.RepositoryID, PullRequest: fresh.PullRequest,
			EventKey:  "pull_request:9001:199:newer-draft",
			DraftedAt: draftedAt.Add(2 * time.Second), RecordedAt: now.Add(4 * time.Hour),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(cancelled.Changed).To(BeTrue())
		Expect(cancelled.Finished.ID).To(Equal(armed.Request.ID))
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

func pendingCICheckSlot(
	now time.Time,
	pullRequest int,
	headSHA, suffix, digest string,
) pendingci.EnsureCheckSlotRequest {
	return pendingci.EnsureCheckSlotRequest{
		TargetID: "installation:77", InstallationID: 77,
		RepositoryID: "repository-20", RepositoryFullName: "owner/repo",
		PullRequest: pullRequest, HeadSHA: headSHA, AppID: 17,
		Name:          storage.PendingCICheckName,
		ExternalID:    "smyklot:merge-after-ci:repository-20:" + suffix,
		DesiredStatus: "in_progress", DesiredTitle: "Merge authorized",
		DesiredSummary: "Waiting for CI", DesiredDigest: digest, ChangedAt: now,
	}
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
		PullRequest:      pullRequest,
		PullRequestTitle: "Update rate limits for the edge tier",
		HeadSHA:          headSHA, BaseBranch: "main",
		MergeMethod: pendingci.MergeMethodSquash, RequiredChecksOnly: true,
		Requester: "operator", SourceCommentID: commentID,
		SourceRevision: requestedAt.Format(time.RFC3339Nano),
		SourceSequence: 1, SourceOrder: commentID,
		Label: "smyklot:pending:ci:squash:required", RequestedAt: requestedAt,
	}
}

// queueDetails reads a row's details back as the map they were written from.
// The column travels as raw JSON, so a key assertion on it would be a
// substring match over a document whose key order is nobody's to promise.
func queueDetails(item workqueue.Item) map[string]any {
	GinkgoHelper()
	if len(item.Details) == 0 {
		return map[string]any{}
	}
	var details map[string]any
	Expect(json.Unmarshal(item.Details, &details)).To(Succeed())

	return details
}
