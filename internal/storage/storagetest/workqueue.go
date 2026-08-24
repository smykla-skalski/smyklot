package storagetest

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func declareWorkQueueSpecs(runtime func() (context.Context, storage.Store, time.Time)) {
	It("seeds the always-open profile and current workload policies", func() {
		ctx, store, _ := runtime()

		profiles, err := store.ListScheduleProfiles(ctx, false)
		Expect(err).NotTo(HaveOccurred())
		alwaysOpen := profileByID(profiles, workqueue.AlwaysOpenProfileID)
		Expect(alwaysOpen.ID).To(Equal(workqueue.AlwaysOpenProfileID))
		Expect(alwaysOpen.System).To(BeTrue())

		policies, err := store.ListQueuePolicies(ctx, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(policies).To(HaveLen(len(workqueue.Kinds()) - 1))
		syncPolicy := policyByKind(policies, workqueue.KindSyncScan)
		Expect(syncPolicy.Cadence).To(Equal(6 * time.Hour))
		Expect(syncPolicy.ProfileID).To(Equal(workqueue.AlwaysOpenProfileID))
	})

	It("lists target work without leaking another installation", func() {
		ctx, store, now := runtime()
		account, first := seedInstallation(ctx, store, now)
		second := first
		second.TargetID = "github:installation:200"
		second.InstallationID = "200"
		second.Repositories = []storage.RepositorySnapshot{
			testRepository("repo-2", "smykla-skalski/other", false),
		}
		Expect(store.ReconcileInstallation(ctx, second)).To(Succeed())

		firstID := createQueueFixture(ctx, store, account.ID, first.TargetID, "repo-1", now)
		createQueueFixture(ctx, store, account.ID, second.TargetID, "repo-2", now)

		page, err := store.ListWorkQueue(ctx, workqueue.Filter{TargetID: &first.TargetID})
		Expect(err).NotTo(HaveOccurred())
		Expect(page.Total).To(Equal(1))
		Expect(page.Items).To(HaveLen(1))
		Expect(page.Items[0].ID).To(Equal(firstID))
		Expect(page.Items[0].WorkAhead).To(BeZero())
		Expect(page.Facets.Targets).To(ConsistOf(first.TargetID))
		Expect(page.Facets.Repositories).To(ConsistOf("repo-1"))
		Expect(page.Facets.Profiles).To(ConsistOf(workqueue.AlwaysOpenProfileID))

		after, before := now.Add(-time.Minute), now.Add(time.Minute)
		profileID, repositoryID := workqueue.AlwaysOpenProfileID, "repo-1"
		page, err = store.ListWorkQueue(ctx, workqueue.Filter{
			TargetID: &first.TargetID, RepositoryID: &repositoryID, ProfileID: &profileID,
			States:       []workqueue.State{workqueue.StateScheduled},
			Kinds:        []workqueue.Kind{workqueue.KindReactionScan},
			Priorities:   []workqueue.Priority{workqueue.PriorityNormal},
			CreatedAfter: &after, CreatedBefore: &before,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(page.Items).To(HaveLen(1))
		Expect(page.Items[0].ID).To(Equal(firstID))
	})

	It("applies audited optimistic queue controls", func() {
		ctx, store, now := runtime()
		account, target := seedInstallation(ctx, store, now)
		itemID := createQueueFixture(ctx, store, account.ID, target.TargetID, "repo-1", now)

		high, err := store.ApplyQueueAction(ctx, itemID, workqueue.ItemAction{
			Type: workqueue.ActionSetPriority, Priority: workqueue.PriorityHigh,
			ExpectedRevision: 1, ActorID: account.ID, ChangedAt: now.Add(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(high.Priority).To(Equal(workqueue.PriorityHigh))
		Expect(high.PriorityOverride).To(BeTrue())
		Expect(high.Revision).To(Equal(int64(2)))

		policy, err := store.GetEffectiveQueuePolicy(ctx, workqueue.KindReactionScan, nil)
		Expect(err).NotTo(HaveOccurred())
		_, err = store.SaveQueuePolicy(ctx, workqueue.PolicyChange{
			Kind: workqueue.KindReactionScan, Enabled: policy.Enabled,
			Cadence: policy.Cadence, ProfileID: policy.ProfileID,
			DefaultPriority: workqueue.PriorityLow, RetryDelay: policy.RetryDelay,
			Retention: policy.Retention, ApprovalTTL: policy.ApprovalTTL,
			Configuration: policy.Configuration, ExpectedRevision: policy.Revision,
			ActorID: account.ID, ChangedAt: now.Add(90 * time.Second),
		})
		Expect(err).NotTo(HaveOccurred())
		high, err = store.GetQueueItem(ctx, itemID)
		Expect(err).NotTo(HaveOccurred())
		Expect(high.Priority).To(Equal(workqueue.PriorityHigh))
		Expect(high.PriorityOverride).To(BeTrue())

		reason := "production repair"
		immediate, err := store.ApplyQueueAction(ctx, itemID, workqueue.ItemAction{
			Type: workqueue.ActionRunNow, ExpectedRevision: high.Revision,
			ActorID: account.ID, Reason: reason, ChangedAt: now.Add(2 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(immediate.WindowMode).To(Equal(workqueue.WindowBypass))
		Expect(immediate.State).To(Equal(workqueue.StateReady))
		Expect(immediate.EligibleAt).To(Equal(now.Add(2 * time.Minute)))

		_, err = store.ApplyQueueAction(ctx, itemID, workqueue.ItemAction{
			Type: workqueue.ActionCancel, ExpectedRevision: high.Revision,
			ActorID: account.ID, ChangedAt: now.Add(3 * time.Minute),
		})
		Expect(errors.Is(err, storage.ErrConflict)).To(BeTrue())

		events, err := store.ListQueueEvents(ctx, itemID, 20)
		Expect(err).NotTo(HaveOccurred())
		Expect(events).To(HaveLen(4))
		Expect(events[2].Kind).To(Equal("schedule.recomputed"))
		Expect(events[2].Actor).To(Equal(account.ID))
		Expect(events[3].Summary).To(Equal("Run now requested: " + reason))
	})

	It("stores profiles, effective overrides, and Root-approved requests", func() {
		ctx, store, now := runtime()
		account, target := seedInstallation(ctx, store, now)
		profile, err := store.SaveScheduleProfile(ctx, workqueue.ProfileChange{
			ID: "weekday", Name: "Weekday", Timezone: "Europe/Warsaw",
			Windows: []workqueue.Window{{Weekday: time.Monday, Start: 9 * 60, End: 17 * 60}},
			ActorID: account.ID, ChangedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(profile.Revision).To(Equal(int64(1)))

		global, err := store.GetEffectiveQueuePolicy(ctx, workqueue.KindSyncScan, nil)
		Expect(err).NotTo(HaveOccurred())
		request, err := store.CreateScheduleRequest(ctx, workqueue.ScheduleRequestCreate{
			ID: "schedule-request-1", TargetID: target.TargetID,
			Kind: workqueue.KindSyncScan, BaseRevision: global.Revision,
			ProfileID: &profile.ID, Cadence: 2 * time.Hour,
			DefaultPriority: workqueue.PriorityHigh,
			Reason:          "finish before the workday", RequestedBy: account.ID, CreatedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(request.State).To(Equal(workqueue.RequestPending))

		approved, err := store.DecideScheduleRequest(ctx, request.ID, workqueue.ScheduleDecision{
			Approve: true, ExpectedRevision: request.Revision,
			ReviewerID: account.ID, ReviewedAt: now.Add(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(approved.State).To(Equal(workqueue.RequestApproved))

		effective, err := store.GetEffectiveQueuePolicy(
			ctx, workqueue.KindSyncScan, &target.TargetID,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(effective.TargetID).To(HaveValue(Equal(target.TargetID)))
		Expect(effective.Cadence).To(Equal(2 * time.Hour))
		Expect(effective.ProfileID).To(Equal(profile.ID))
	})

	It("marks a request stale when its effective policy source changes", func() {
		ctx, store, now := runtime()
		account, target := seedInstallation(ctx, store, now)
		global, err := store.GetEffectiveQueuePolicy(ctx, workqueue.KindSyncScan, nil)
		Expect(err).NotTo(HaveOccurred())
		profileID := workqueue.AlwaysOpenProfileID
		request, err := store.CreateScheduleRequest(ctx, workqueue.ScheduleRequestCreate{
			ID: "schedule-request-source", TargetID: target.TargetID,
			Kind: workqueue.KindSyncScan, BaseRevision: global.Revision,
			ProfileID: &profileID, Cadence: global.Cadence,
			DefaultPriority: global.DefaultPriority, Configuration: global.Configuration,
			Reason: "use the installation window", RequestedBy: account.ID, CreatedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(request.BaseTargetID).To(BeNil())

		_, err = store.SaveQueuePolicy(ctx, workqueue.PolicyChange{
			Kind: workqueue.KindSyncScan, TargetID: &target.TargetID,
			Enabled: global.Enabled, Cadence: global.Cadence, ProfileID: global.ProfileID,
			DefaultPriority: global.DefaultPriority, RetryDelay: global.RetryDelay,
			Retention: global.Retention, ApprovalTTL: global.ApprovalTTL,
			Configuration: global.Configuration, ExpectedRevision: 0,
			ActorID: account.ID, ChangedAt: now.Add(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())

		stale, err := store.DecideScheduleRequest(ctx, request.ID, workqueue.ScheduleDecision{
			Approve: true, ExpectedRevision: request.Revision,
			ReviewerID: account.ID, ReviewedAt: now.Add(2 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(stale.State).To(Equal(workqueue.RequestStale))
	})

	It("leases, retries, and coalesces recurring occurrences", func() {
		ctx, store, now := runtime()
		claim := workqueue.RecurringClaim{
			Kind: workqueue.KindCatalogRefresh, Title: "Refresh installation catalog",
			Now: now, LeaseDuration: time.Minute,
		}
		item, claimed, err := store.ClaimRecurringWork(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).To(BeTrue())
		Expect(item.State).To(Equal(workqueue.StateRunning))
		Expect(item.Attempt).To(Equal(1))

		retrying, err := store.FinishRecurringWork(
			ctx, item.ID, "GitHub unavailable", "", now.Add(time.Minute),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(retrying.State).To(Equal(workqueue.StateRetrying))
		Expect(retrying.EligibleAt).To(Equal(now.Add(90 * time.Second)))

		claim.Now = now.Add(2 * time.Minute)
		item, claimed, err = store.ClaimRecurringWork(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).To(BeTrue())
		Expect(item.Attempt).To(Equal(2))
		_, err = store.FinishRecurringWork(ctx, item.ID, "", "", now.Add(3*time.Minute))
		Expect(err).NotTo(HaveOccurred())
		next, err := store.NextQueueAvailability(
			ctx, workqueue.LaneMaintenance, now.Add(3*time.Minute),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(next).To(HaveValue(Equal(now.Add(5 * time.Minute))))

		claim.Now = now.Add(23 * time.Minute)
		item, claimed, err = store.ClaimRecurringWork(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).To(BeTrue())
		Expect(item.NotBefore).To(Equal(now.Add(20 * time.Minute)))
	})

	It("defers expired work that missed its execution window", func() {
		ctx, store, now := runtime()
		account, target := seedInstallation(ctx, store, now)
		profile, err := store.SaveScheduleProfile(ctx, workqueue.ProfileChange{
			ID: "monday-morning", Name: "Monday morning", Timezone: "UTC",
			Windows: []workqueue.Window{{
				Weekday: time.Monday, Start: 9 * 60, End: 10 * 60,
			}},
			ActorID: account.ID, ChangedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())

		windowStart := time.Date(2026, time.August, 24, 9, 0, 0, 0, time.UTC)
		dispatchAt := windowStart.Add(3 * time.Hour)
		leaseExpiredAt := windowStart.Add(90 * time.Minute)
		itemID := "pending-ci:missed-window"
		_, err = store.CreateQueueItem(ctx, workqueue.Item{
			ID: itemID, Kind: workqueue.KindPendingCI, Lane: workqueue.LanePendingCI,
			TargetID: &target.TargetID, SourceKind: "pending_ci", SourceID: "missing",
			Title: "Pending CI check", State: workqueue.StateRunning,
			Priority: workqueue.PriorityNormal, WindowMode: workqueue.WindowRespect,
			ProfileID: &profile.ID, NotBefore: windowStart, EligibleAt: windowStart,
			LeaseExpiresAt: &leaseExpiredAt, StartedAt: &windowStart,
			CreatedAt: windowStart, UpdatedAt: windowStart,
		})
		Expect(err).NotTo(HaveOccurred())

		lease, err := store.LeaseDue(ctx, dispatchAt, dispatchAt.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(lease.Request).To(BeNil())
		nextWindow := time.Date(2026, time.August, 31, 9, 0, 0, 0, time.UTC)
		Expect(lease.AvailableAt).To(HaveValue(Equal(nextWindow)))

		deferred, err := store.GetQueueItem(ctx, itemID)
		Expect(err).NotTo(HaveOccurred())
		Expect(deferred.State).To(Equal(workqueue.StateScheduled))
		Expect(deferred.EligibleAt).To(Equal(nextWindow))
		Expect(deferred.LeaseExpiresAt).To(BeNil())
		Expect(deferred.BlockedReason).To(Equal("Waiting for the Monday morning window"))
		Expect(deferred.Revision).To(Equal(int64(2)))

		events, err := store.ListQueueEvents(ctx, itemID, 20)
		Expect(err).NotTo(HaveOccurred())
		Expect(events).To(HaveLen(2))
		Expect(events[1].Kind).To(Equal("window_missed"))
		Expect(events[1].State).To(Equal(workqueue.StateScheduled))
	})
}

func createQueueFixture(
	ctx context.Context,
	store storage.Store,
	actorID, targetID, repositoryID string,
	now time.Time,
) string {
	GinkgoHelper()

	itemID := "queue:" + targetID
	item, err := store.CreateQueueItem(ctx, workqueue.Item{
		ID: itemID, Kind: workqueue.KindReactionScan, Lane: workqueue.LaneMaintenance,
		TargetID: &targetID, RepositoryID: &repositoryID,
		Title: "Reaction scan", State: workqueue.StateScheduled,
		Priority: workqueue.PriorityNormal, WindowMode: workqueue.WindowRespect,
		ProfileID: pointer(workqueue.AlwaysOpenProfileID),
		NotBefore: now.Add(time.Hour), EligibleAt: now.Add(time.Hour),
		RequestedBy: &actorID, CreatedAt: now, UpdatedAt: now,
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(item.Revision).To(Equal(int64(1)))

	return itemID
}

func pointer[T any](value T) *T { return &value }

func profileByID(profiles []workqueue.Profile, id string) workqueue.Profile {
	for _, profile := range profiles {
		if profile.ID == id {
			return profile
		}
	}

	return workqueue.Profile{}
}

func policyByKind(policies []workqueue.Policy, kind workqueue.Kind) workqueue.Policy {
	for _, policy := range policies {
		if policy.Kind == kind {
			return policy
		}
	}

	return workqueue.Policy{}
}
