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

// queueRuntime is what every queue spec asks for: a live context, the store
// under test, and the instant the suite calls now.
type queueRuntime = func() (context.Context, storage.Store, time.Time)

// declareWorkQueueSpecs runs the queue's conformance suite on one engine, in
// four groups: what the policies say, what a reader can list, what a schedule
// permits, and what leasing actually does.
func declareWorkQueueSpecs(runtime queueRuntime) {
	declareQueuePolicySpecs(runtime)
	declareQueueListingSpecs(runtime)
	declareQueueScheduleSpecs(runtime)
	declareQueueLeaseSpecs(runtime)
}

func declareQueuePolicySpecs(runtime queueRuntime) {
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

	It("adopts deployment timings while global policies remain pristine", func() {
		ctx, store, now := runtime()
		defaults := workqueue.DeploymentDefaults{
			PollInterval:         90 * time.Second,
			PendingCIQuietPeriod: 45 * time.Second,
			PathIndexInterval:    20 * time.Minute,
		}
		Expect(store.InitializeQueuePolicies(ctx, defaults, now)).To(Succeed())
		reaction, err := store.GetEffectiveQueuePolicy(ctx, workqueue.KindReactionScan, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(reaction.Cadence).To(Equal(90 * time.Second))
		Expect(reaction.Revision).To(Equal(int64(1)))
		Expect(reaction.UpdatedBy).To(BeNil())
		path, err := store.GetEffectiveQueuePolicy(ctx, workqueue.KindPathRefresh, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(path.Cadence).To(Equal(20 * time.Minute))
		pendingCI, err := store.GetEffectiveQueuePolicy(ctx, workqueue.KindPendingCI, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(pendingCI.Configuration)).To(ContainSubstring(`"passing_quiet_seconds":45`))

		defaults.PollInterval = 2 * time.Minute
		Expect(store.InitializeQueuePolicies(ctx, defaults, now.Add(time.Minute))).To(Succeed())
		reaction, err = store.GetEffectiveQueuePolicy(ctx, workqueue.KindReactionScan, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(reaction.Cadence).To(Equal(2 * time.Minute))
		Expect(reaction.Revision).To(Equal(int64(1)))
	})

	It("keeps Root queue policy edits over later deployment initialization", func() {
		ctx, store, now := runtime()
		account := testAccount(now)
		Expect(store.UpsertAccount(ctx, account)).To(Succeed())
		policy, err := store.GetEffectiveQueuePolicy(ctx, workqueue.KindReactionScan, nil)
		Expect(err).NotTo(HaveOccurred())
		policy.Cadence = 7 * time.Minute
		policy, err = store.SaveQueuePolicy(ctx, workqueue.PolicyChange{
			Kind: policy.Kind, Enabled: policy.Enabled, Cadence: policy.Cadence,
			ProfileID: policy.ProfileID, DefaultPriority: policy.DefaultPriority,
			RetryDelay: policy.RetryDelay, Retention: policy.Retention,
			ApprovalTTL: policy.ApprovalTTL, Configuration: policy.Configuration,
			ExpectedRevision: policy.Revision, ActorID: account.ID, ChangedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(policy.Revision).To(Equal(int64(2)))

		Expect(store.InitializeQueuePolicies(ctx, workqueue.DeploymentDefaults{
			PollInterval:         2 * time.Minute,
			PendingCIQuietPeriod: 30 * time.Second,
			PathIndexInterval:    time.Hour,
		}, now.Add(time.Minute))).To(Succeed())
		kept, err := store.GetEffectiveQueuePolicy(ctx, workqueue.KindReactionScan, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(kept.Cadence).To(Equal(7 * time.Minute))
		Expect(kept.Revision).To(Equal(int64(2)))
		Expect(kept.UpdatedBy).To(HaveValue(Equal(account.ID)))
	})

	It("maps the legacy every-sweep path interval onto a valid queue cadence", func() {
		ctx, store, now := runtime()
		account := testAccount(now)
		Expect(store.UpsertAccount(ctx, account)).To(Succeed())
		poll, every := 90*time.Second, time.Duration(0)
		result, err := store.SaveRuntimeSettings(ctx, storage.RuntimeSettingsChange{
			PollInterval: &poll, PathIndexInterval: &every,
			EffectivePollInterval:         poll,
			EffectivePendingCIQuietPeriod: 30 * time.Second,
			EffectivePathIndexInterval:    every,
			EffectiveSessionTTL:           time.Hour,
			ActorAccountID:                account.ID, ChangedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		path, err := store.GetEffectiveQueuePolicy(ctx, workqueue.KindPathRefresh, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(path.Enabled).To(BeTrue())
		Expect(path.Cadence).To(Equal(poll))

		disabled := time.Duration(0)
		_, err = store.SaveRuntimeSettings(ctx, storage.RuntimeSettingsChange{
			PollInterval: &disabled, PathIndexInterval: &every,
			EffectivePendingCIQuietPeriod: 30 * time.Second,
			EffectiveSessionTTL:           time.Hour,
			ExpectedRevision:              result.Settings.Revision,
			ActorAccountID:                account.ID, ChangedAt: now.Add(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		path, err = store.GetEffectiveQueuePolicy(ctx, workqueue.KindPathRefresh, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(path.Enabled).To(BeFalse())
		Expect(path.Cadence).To(BeZero())
	})

	It("rejects a zero cadence for enabled recurring work", func() {
		ctx, store, now := runtime()
		policy, err := store.GetEffectiveQueuePolicy(ctx, workqueue.KindCatalogRefresh, nil)
		Expect(err).NotTo(HaveOccurred())
		policy.Cadence = 0
		_, err = store.SaveQueuePolicy(ctx, workqueue.PolicyChange{
			Kind: policy.Kind, Enabled: policy.Enabled, Cadence: policy.Cadence,
			ProfileID: policy.ProfileID, DefaultPriority: policy.DefaultPriority,
			RetryDelay: policy.RetryDelay, Retention: policy.Retention,
			ApprovalTTL: policy.ApprovalTTL, Configuration: policy.Configuration,
			ExpectedRevision: policy.Revision, ActorID: "root", ChangedAt: now,
		})
		Expect(err).To(MatchError(ContainSubstring("positive cadence")))

		webhook, err := store.GetEffectiveQueuePolicy(ctx, workqueue.KindWebhookDelivery, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(webhook.Enabled).To(BeTrue())
		Expect(webhook.Cadence).To(BeZero())
	})
}

func declareQueueListingSpecs(runtime queueRuntime) {
	It("publishes every recurring workload through the shared scheduler", func() {
		ctx, store, now := runtime()
		targetID, repositoryID := "installation:scheduled", "repository:scheduled"
		claims := []workqueue.RecurringClaim{
			{Kind: workqueue.KindCatalogRefresh, Title: "Refresh the list of repositories"},
			{Kind: workqueue.KindDeliveryCleanup, Title: "Tidy finished background work"},
			{Kind: workqueue.KindAuthCleanup, Title: "Tidy expired sign-ins"},
			{Kind: workqueue.KindSyncScan, TargetID: &targetID, Title: "Check which repositories are in step"},
			{Kind: workqueue.KindPathRefresh, TargetID: &targetID, Title: "Refresh which paths are watched"},
			{
				Kind: workqueue.KindPendingCIGate, TargetID: &targetID,
				RepositoryID: &repositoryID, Title: "Hold pull requests until CI settles",
			},
			{
				Kind: workqueue.KindConfigMigration, TargetID: &targetID,
				RepositoryID: &repositoryID, Title: "Check the repository's configuration file",
			},
			{
				Kind: workqueue.KindReactionScan, TargetID: &targetID,
				RepositoryID: &repositoryID, Title: "Scan for new commands",
			},
		}
		for index := range claims {
			claims[index].Now, claims[index].LeaseDuration = now, time.Minute
			item, err := store.EnsureRecurringWork(ctx, claims[index])
			Expect(err).NotTo(HaveOccurred())
			Expect(item.Kind).To(Equal(claims[index].Kind))
			Expect(item.Lane).To(Equal(workqueue.LaneMaintenance))
			Expect(item.State).To(Equal(workqueue.StateReady))
		}
		for _, kind := range workqueue.Kinds() {
			if kind.Recurring() {
				Expect(claims).To(ContainElement(HaveField("Kind", kind)))
			}
		}
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

	It("names the repository a row is about, and finds it by its words", func() {
		ctx, store, now := runtime()
		account, target := seedInstallation(ctx, store, now)
		itemID := createQueueFixture(ctx, store, account.ID, target.TargetID, "repo-1", now)

		page, err := store.ListWorkQueue(ctx, workqueue.Filter{TargetID: &target.TargetID})
		Expect(err).NotTo(HaveOccurred())
		Expect(page.Items).To(HaveLen(1))
		Expect(page.Items[0].RepositoryName).To(Equal("smykla-skalski/smyklot"))

		item, err := store.GetQueueItem(ctx, itemID)
		Expect(err).NotTo(HaveOccurred())
		Expect(item.RepositoryName).To(Equal("smykla-skalski/smyklot"))
		// Read through the same slice the positions are written onto: a detail that
		// answered from the row as it was scanned carried neither.
		Expect(item.ProfileName).NotTo(BeEmpty())

		// Folded on both sides, because one engine matches case and the other does not.
		page, err = store.ListWorkQueue(ctx, workqueue.Filter{Search: "REACTION"})
		Expect(err).NotTo(HaveOccurred())
		Expect(page.Items).To(HaveLen(1))
		Expect(page.Items[0].ID).To(Equal(itemID))
		Expect(page.Total).To(Equal(1))

		page, err = store.ListWorkQueue(ctx, workqueue.Filter{Search: "nothing here"})
		Expect(err).NotTo(HaveOccurred())
		Expect(page.Items).To(BeEmpty())
		Expect(page.Total).To(BeZero())
	})

	It("lists what finished lately rather than what was created lately", func() {
		ctx, store, now := runtime()
		account := testAccount(now)
		Expect(store.UpsertAccount(ctx, account)).To(Succeed())
		policy, err := store.GetEffectiveQueuePolicy(ctx, workqueue.KindReactionScan, nil)
		Expect(err).NotTo(HaveOccurred())
		policy.Enabled, policy.Cadence = true, 5*time.Minute
		_, err = store.SaveQueuePolicy(ctx, policyChange(policy, account.ID, now))
		Expect(err).NotTo(HaveOccurred())
		for _, suffix := range []string{"a", "b"} {
			targetID := "finish-target-" + suffix
			_, err = store.EnsureRecurringWork(ctx, workqueue.RecurringClaim{
				Kind: workqueue.KindReactionScan, TargetID: &targetID,
				Title: "Scan for new commands", Now: now, LeaseDuration: time.Minute,
			})
			Expect(err).NotTo(HaveOccurred())
		}

		/* One finished two days ago and one a minute ago. Both were accepted at the
		   same instant, which is the whole point: a merge held for a day of checks is
		   old work that finished recently, and "what has this service done lately"
		   cannot be answered from when it was accepted. */
		finished := map[string]time.Time{}
		for _, at := range []time.Time{now.Add(-48 * time.Hour), now.Add(-time.Minute)} {
			item, claimed, claimErr := store.ClaimNextRecurringWork(
				ctx,
				workqueue.RecurringLease{Now: now, LeaseDuration: time.Minute},
			)
			Expect(claimErr).NotTo(HaveOccurred())
			Expect(claimed).To(BeTrue())
			_, err = store.FinishRecurringWork(ctx, item.ID, workqueue.RecurringCompletion{}, at)
			Expect(err).NotTo(HaveOccurred())
			finished[item.ID] = at
		}

		since := now.Add(-24 * time.Hour)
		page, err := store.ListWorkQueue(ctx, workqueue.Filter{
			States: []workqueue.State{workqueue.StateSucceeded}, FinishedAfter: &since,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(page.Items).To(HaveLen(1))
		Expect(finished[page.Items[0].ID]).To(Equal(now.Add(-time.Minute)))
	})

	It("limits the dispatch-ordered page after scheduler position", func() {
		ctx, store, now := runtime()
		account := testAccount(now)
		Expect(store.UpsertAccount(ctx, account)).To(Succeed())
		seedDispatchOrderedQueue(ctx, store, now, account.ID)

		page, err := store.ListWorkQueue(ctx, workqueue.Filter{
			States: []workqueue.State{workqueue.StateReady}, DispatchOrder: true,
			Summary: true, Limit: 3,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(page.Total).To(Equal(5))
		Expect(page.Items).To(HaveLen(3))
		Expect(page.Items[0].Kind).To(Equal(workqueue.KindCatalogRefresh))
		Expect(page.Items[0].WorkAhead).To(BeZero())
		Expect(page.StateCounts[workqueue.StateReady]).To(Equal(5))
		Expect(page.StateCounts[workqueue.StateSucceeded]).To(Equal(2))
		Expect(page.Facets).To(Equal(workqueue.Facets{
			Targets: []string{}, Repositories: []string{}, Profiles: []string{},
			States: []workqueue.State{}, Kinds: []workqueue.Kind{},
			Priorities: []workqueue.Priority{},
		}))
	})
}

func declareQueueScheduleSpecs(runtime queueRuntime) {
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

	It("rejects recurring schedule requests with a zero cadence", func() {
		ctx, store, now := runtime()
		account, target := seedInstallation(ctx, store, now)
		global, err := store.GetEffectiveQueuePolicy(ctx, workqueue.KindSyncScan, nil)
		Expect(err).NotTo(HaveOccurred())
		profileID := workqueue.AlwaysOpenProfileID

		_, err = store.CreateScheduleRequest(ctx, workqueue.ScheduleRequestCreate{
			ID: "schedule-request-zero-cadence", TargetID: target.TargetID,
			Kind: workqueue.KindSyncScan, BaseRevision: global.Revision,
			ProfileID: &profileID, Cadence: 0,
			DefaultPriority: global.DefaultPriority, Configuration: global.Configuration,
			Reason: "run continuously", RequestedBy: account.ID, CreatedAt: now,
		})
		Expect(err).To(MatchError(ContainSubstring("policy is invalid")))
	})

	It("scopes custom profiles and retains every request decision", func() {
		ctx, store, now := runtime()
		account, target := seedInstallation(ctx, store, now)
		custom := &workqueue.Profile{
			Name: "Requested night hours", Timezone: "Europe/Warsaw",
			Windows: []workqueue.Window{{Weekday: time.Tuesday, Start: 20 * 60, End: 23 * 60}},
		}

		reaction, err := store.GetEffectiveQueuePolicy(ctx, workqueue.KindReactionScan, nil)
		Expect(err).NotTo(HaveOccurred())
		request, err := store.CreateScheduleRequest(ctx, workqueue.ScheduleRequestCreate{
			ID: "schedule-request-installation-profile", TargetID: target.TargetID,
			Kind: workqueue.KindReactionScan, BaseRevision: reaction.Revision,
			CustomProfile: custom, Cadence: reaction.Cadence,
			DefaultPriority: reaction.DefaultPriority, Configuration: reaction.Configuration,
			Reason: "scan during the local night", RequestedBy: account.ID, CreatedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		installationProfileID := "profile:installation-night"
		approved, err := store.DecideScheduleRequest(ctx, request.ID, workqueue.ScheduleDecision{
			Approve: true, ExpectedRevision: request.Revision, ProfileID: &installationProfileID,
			ReviewerID: account.ID, ReviewedAt: now.Add(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(approved.State).To(Equal(workqueue.RequestApproved))
		Expect(approved.PromotedProfileID).To(HaveValue(Equal(installationProfileID)))
		installationProfile, err := store.GetScheduleProfile(ctx, installationProfileID)
		Expect(err).NotTo(HaveOccurred())
		Expect(installationProfile.TargetID).To(HaveValue(Equal(target.TargetID)))

		path, err := store.GetEffectiveQueuePolicy(ctx, workqueue.KindPathRefresh, nil)
		Expect(err).NotTo(HaveOccurred())
		request, err = store.CreateScheduleRequest(ctx, workqueue.ScheduleRequestCreate{
			ID: "schedule-request-promoted-profile", TargetID: target.TargetID,
			Kind: workqueue.KindPathRefresh, BaseRevision: path.Revision,
			CustomProfile: custom, Cadence: path.Cadence,
			DefaultPriority: path.DefaultPriority, Configuration: path.Configuration,
			Reason: "reuse these hours", RequestedBy: account.ID, CreatedAt: now.Add(2 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		globalProfileID := "profile:global-night"
		approved, err = store.DecideScheduleRequest(ctx, request.ID, workqueue.ScheduleDecision{
			Approve: true, PromoteProfile: true, ExpectedRevision: request.Revision,
			ProfileID: &globalProfileID, ReviewerID: account.ID, ReviewedAt: now.Add(3 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		globalProfile, err := store.GetScheduleProfile(ctx, globalProfileID)
		Expect(err).NotTo(HaveOccurred())
		Expect(globalProfile.TargetID).To(BeNil())

		gate, err := store.GetEffectiveQueuePolicy(ctx, workqueue.KindPendingCIGate, nil)
		Expect(err).NotTo(HaveOccurred())
		request, err = store.CreateScheduleRequest(ctx, workqueue.ScheduleRequestCreate{
			ID: "schedule-request-rejected", TargetID: target.TargetID,
			Kind: gate.Kind, BaseRevision: gate.Revision, ProfileID: &gate.ProfileID,
			Cadence: gate.Cadence, DefaultPriority: gate.DefaultPriority,
			Configuration: gate.Configuration, Reason: "change protection cadence",
			RequestedBy: account.ID, CreatedAt: now.Add(4 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		rejected, err := store.DecideScheduleRequest(ctx, request.ID, workqueue.ScheduleDecision{
			ExpectedRevision: request.Revision, ReviewerID: account.ID,
			DecisionReason: "keep the current cadence", ReviewedAt: now.Add(5 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(rejected.State).To(Equal(workqueue.RequestRejected))

		migration, err := store.GetEffectiveQueuePolicy(ctx, workqueue.KindConfigMigration, nil)
		Expect(err).NotTo(HaveOccurred())
		request, err = store.CreateScheduleRequest(ctx, workqueue.ScheduleRequestCreate{
			ID: "schedule-request-withdrawn", TargetID: target.TargetID,
			Kind: migration.Kind, BaseRevision: migration.Revision, ProfileID: &migration.ProfileID,
			Cadence: migration.Cadence, DefaultPriority: migration.DefaultPriority,
			Configuration: migration.Configuration, Reason: "change migration cadence",
			RequestedBy: account.ID, CreatedAt: now.Add(6 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		withdrawn, err := store.WithdrawScheduleRequest(
			ctx, request.ID, request.Revision, account.ID, now.Add(7*time.Minute),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(withdrawn.State).To(Equal(workqueue.RequestWithdrawn))
		for _, requestID := range []string{"schedule-request-rejected", "schedule-request-withdrawn"} {
			item, itemErr := store.GetQueueItem(ctx, "schedule-request:"+requestID)
			Expect(itemErr).NotTo(HaveOccurred())
			Expect(item.State).To(Equal(workqueue.StateCancelled))
		}
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
}

func declareQueueLeaseSpecs(runtime queueRuntime) {
	It("leases, retries, and coalesces recurring occurrences", func() {
		ctx, store, now := runtime()
		claim := workqueue.RecurringClaim{
			Kind: workqueue.KindCatalogRefresh, Title: "Refresh the list of repositories",
			Now: now, LeaseDuration: time.Minute,
		}
		item, claimed, err := store.ClaimRecurringWork(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).To(BeTrue())
		Expect(item.State).To(Equal(workqueue.StateRunning))
		Expect(item.Attempt).To(Equal(1))

		retrying, err := store.FinishRecurringWork(
			ctx, item.ID, workqueue.RecurringCompletion{
				Failure: "GitHub unavailable", Retryable: true,
			}, now.Add(time.Minute),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(retrying.State).To(Equal(workqueue.StateRetrying))
		Expect(retrying.EligibleAt).To(Equal(now.Add(90 * time.Second)))

		claim.Now = now.Add(2 * time.Minute)
		item, claimed, err = store.ClaimRecurringWork(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).To(BeTrue())
		Expect(item.Attempt).To(Equal(2))
		_, err = store.FinishRecurringWork(
			ctx, item.ID, workqueue.RecurringCompletion{}, now.Add(3*time.Minute),
		)
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

	It("holds a stable recurring blocker until the next cadence retries it", func() {
		ctx, store, now := runtime()
		claim := workqueue.RecurringClaim{
			Kind: workqueue.KindPendingCIGate, Title: "Hold pull requests until CI settles",
			Now: now, LeaseDuration: time.Minute,
		}
		item, claimed, err := store.ClaimRecurringWork(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).To(BeTrue())

		item, err = store.FinishRecurringWork(ctx, item.ID, workqueue.RecurringCompletion{
			Failure: "GitHub rulesets require GitHub Pro", Blocked: true,
		}, now.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(item.State).To(Equal(workqueue.StateBlocked))
		Expect(item.BlockedReason).To(Equal("GitHub rulesets require GitHub Pro"))
		Expect(item.FinishedAt).To(BeNil())
		next, err := store.NextQueueAvailability(
			ctx, workqueue.LaneMaintenance, now.Add(2*time.Minute),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(next).To(BeNil())

		claim.Now = now.Add(4 * time.Minute)
		held, claimed, err := store.ClaimRecurringWork(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).To(BeFalse())
		Expect(held.State).To(Equal(workqueue.StateBlocked))
		Expect(held.BlockedReason).To(Equal("GitHub rulesets require GitHub Pro"))

		claim.Now = now.Add(6 * time.Minute)
		retried, claimed, err := store.ClaimRecurringWork(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).To(BeTrue())
		Expect(retried.ID).NotTo(Equal(item.ID))
		Expect(retried.State).To(Equal(workqueue.StateRunning))
		Expect(retried.Attempt).To(Equal(1))
		Expect(retried.BlockedReason).To(BeEmpty())

		page, err := store.ListWorkQueue(ctx, workqueue.Filter{
			States: []workqueue.State{workqueue.StateSuperseded},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(page.Items).To(HaveLen(1))
		Expect(page.Items[0].ID).To(Equal(item.ID))
		Expect(page.Items[0].FinishedAt).NotTo(BeNil())
	})

	It("leases the scheduler's next recurring occurrence in one claim", func() {
		ctx, store, now := runtime()
		claims := []workqueue.RecurringClaim{
			{
				Kind:  workqueue.KindCatalogRefresh,
				Title: "Refresh the list of repositories",
				Now:   now, LeaseDuration: time.Minute,
			},
			{
				Kind:  workqueue.KindAuthCleanup,
				Title: "Tidy expired sign-ins",
				Now:   now, LeaseDuration: time.Minute,
			},
		}
		ensureRecurringClaims(ctx, store, claims)

		item, claimed, err := store.ClaimNextRecurringWork(ctx, workqueue.RecurringLease{
			Now: now, LeaseDuration: time.Minute,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).To(BeTrue())
		Expect(item.State).To(Equal(workqueue.StateRunning))
		Expect(item.Kind).To(BeElementOf(
			workqueue.KindCatalogRefresh,
			workqueue.KindAuthCleanup,
		))
		page, err := store.ListWorkQueue(ctx, workqueue.Filter{
			States: []workqueue.State{workqueue.StateRunning},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(page.Items).To(HaveLen(1))
		Expect(page.Items[0].ID).To(Equal(item.ID))
	})

	It("recomputes recurring cadence without replacing one-off schedules", func() {
		ctx, store, now := runtime()
		account, _ := seedInstallation(ctx, store, now)
		claim := workqueue.RecurringClaim{
			Kind: workqueue.KindCatalogRefresh, Title: "Refresh the list of repositories",
			Now: now, LeaseDuration: time.Minute,
		}
		first, claimed, err := store.ClaimRecurringWork(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).To(BeTrue())
		_, err = store.FinishRecurringWork(
			ctx, first.ID, workqueue.RecurringCompletion{}, now.Add(time.Minute),
		)
		Expect(err).NotTo(HaveOccurred())

		policy, err := store.GetEffectiveQueuePolicy(ctx, claim.Kind, nil)
		Expect(err).NotTo(HaveOccurred())
		policy.Cadence = 10 * time.Minute
		policy, err = store.SaveQueuePolicy(ctx, policyChange(
			policy, account.ID, now.Add(2*time.Minute),
		))
		Expect(err).NotTo(HaveOccurred())
		next := onlyQueueItem(ctx, store, workqueue.Filter{
			Kinds: []workqueue.Kind{claim.Kind}, States: []workqueue.State{workqueue.StateScheduled},
		})
		Expect(next.NotBefore).To(Equal(now.Add(10 * time.Minute)))
		Expect(next.CadenceAnchorAt).To(HaveValue(Equal(now.Add(10 * time.Minute))))

		exact := now.Add(30 * time.Minute)
		next, err = store.ApplyQueueAction(ctx, next.ID, workqueue.ItemAction{
			Type: workqueue.ActionScheduleAt, At: exact, ExpectedRevision: next.Revision,
			ActorID: account.ID, ChangedAt: now.Add(3 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		policy.Cadence = 15 * time.Minute
		_, err = store.SaveQueuePolicy(ctx, policyChange(
			policy, account.ID, now.Add(4*time.Minute),
		))
		Expect(err).NotTo(HaveOccurred())
		next, err = store.GetQueueItem(ctx, next.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(next.NotBefore).To(Equal(exact))
		Expect(next.CadenceAnchorAt).To(HaveValue(Equal(now.Add(10 * time.Minute))))
	})

	It("supersedes recurring work whose scope disappeared after its lease", func() {
		ctx, store, now := runtime()
		_, target := seedInstallation(ctx, store, now)
		repositoryID := "repo-1"
		claim := workqueue.RecurringClaim{
			Kind: workqueue.KindReactionScan, TargetID: &target.TargetID,
			RepositoryID: &repositoryID, Title: "Scan for new commands",
			Now: now, LeaseDuration: time.Minute,
		}
		scheduled, err := store.EnsureRecurringWork(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		retired, err := store.SupersedeMissingRecurringWork(ctx, nil, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(retired).To(HaveLen(1))
		Expect(retired[0].ID).To(Equal(scheduled.ID))
		Expect(retired[0].State).To(Equal(workqueue.StateSuperseded))

		claim.RepositoryID = pointer("repo-2")
		claim.Now = now.Add(time.Minute)
		running, claimed, err := store.ClaimRecurringWork(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).To(BeTrue())
		retired, err = store.SupersedeMissingRecurringWork(ctx, nil, now.Add(90*time.Second))
		Expect(err).NotTo(HaveOccurred())
		Expect(retired).To(BeEmpty())
		retired, err = store.SupersedeMissingRecurringWork(ctx, nil, now.Add(3*time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(retired).To(HaveLen(1))
		Expect(retired[0].ID).To(Equal(running.ID))
	})

	It("returns the newest queue events in chronological order", func() {
		ctx, store, now := runtime()
		account, target := seedInstallation(ctx, store, now)
		itemID := createQueueFixture(ctx, store, account.ID, target.TargetID, "repo-1", now)
		item, err := store.GetQueueItem(ctx, itemID)
		Expect(err).NotTo(HaveOccurred())
		for index := range 6 {
			priority := workqueue.PriorityHigh
			if index%2 == 1 {
				priority = workqueue.PriorityNormal
			}
			item, err = store.ApplyQueueAction(ctx, itemID, workqueue.ItemAction{
				Type: workqueue.ActionSetPriority, Priority: priority,
				ExpectedRevision: item.Revision, ActorID: account.ID,
				ChangedAt: now.Add(time.Duration(index+1) * time.Minute),
			})
			Expect(err).NotTo(HaveOccurred())
		}

		events, err := store.ListQueueEvents(ctx, itemID, 3)
		Expect(err).NotTo(HaveOccurred())
		Expect(events).To(HaveLen(3))
		Expect(events[0].ID).To(BeNumerically("<", events[1].ID))
		Expect(events[1].ID).To(BeNumerically("<", events[2].ID))
		Expect(events[2].Summary).To(Equal("Priority changed to normal"))
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

	It("applies queue controls through the persisted schedule semantics", func() {
		ctx, store, now := runtime()
		account, target := seedInstallation(ctx, store, now)
		controlAt := time.Date(2026, time.August, 24, 8, 0, 0, 0, time.UTC)
		profile, err := store.SaveScheduleProfile(ctx, workqueue.ProfileChange{
			ID: "monday-controls", Name: "Monday controls", Timezone: "UTC",
			Windows: []workqueue.Window{{
				Weekday: time.Monday, Start: 9 * 60, End: 10 * 60,
			}},
			ActorID: account.ID, ChangedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		item, err := store.CreateQueueItem(ctx, workqueue.Item{
			ID: "queue:controls", Kind: workqueue.KindReactionScan,
			Lane: workqueue.LaneMaintenance, TargetID: &target.TargetID,
			Title: "Controlled reaction scan", State: workqueue.StateScheduled,
			Priority: workqueue.PriorityNormal, WindowMode: workqueue.WindowRespect,
			ProfileID: &profile.ID, NotBefore: controlAt.Add(24 * time.Hour),
			EligibleAt: controlAt.Add(24 * time.Hour), CreatedAt: now, UpdatedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())

		next, err := store.ApplyQueueAction(ctx, item.ID, workqueue.ItemAction{
			Type: workqueue.ActionNextWindow, ExpectedRevision: item.Revision,
			ActorID: account.ID, ChangedAt: controlAt,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(next.NotBefore).To(Equal(controlAt))
		Expect(next.EligibleAt).To(Equal(controlAt.Add(time.Hour)))
		Expect(next.WindowMode).To(Equal(workqueue.WindowRespect))

		past := controlAt.Add(-2 * time.Hour)
		bypassed, err := store.ApplyQueueAction(ctx, item.ID, workqueue.ItemAction{
			Type: workqueue.ActionScheduleAt, ExpectedRevision: next.Revision,
			ActorID: account.ID, At: past, OutsideWindow: true,
			Reason: "recover an overdue scan", ChangedAt: controlAt,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(bypassed.NotBefore).To(Equal(past))
		Expect(bypassed.EligibleAt).To(Equal(past))
		Expect(bypassed.State).To(Equal(workqueue.StateReady))
		Expect(bypassed.WindowMode).To(Equal(workqueue.WindowBypass))

		_, err = store.ApplyQueueAction(ctx, item.ID, workqueue.ItemAction{
			Type: workqueue.ActionScheduleAt, ExpectedRevision: bypassed.Revision,
			ActorID: account.ID, At: controlAt, OutsideWindow: true, ChangedAt: controlAt,
		})
		Expect(err).To(MatchError(ContainSubstring("reason is required")))
	})

	It("recomputes future profile work without moving a running lease", func() {
		ctx, store, now := runtime()
		account, target := seedInstallation(ctx, store, now)
		controlAt := time.Date(2026, time.August, 24, 8, 0, 0, 0, time.UTC)
		profile, err := store.SaveScheduleProfile(ctx, workqueue.ProfileChange{
			ID: "editable-hours", Name: "Editable hours", Timezone: "UTC",
			Windows: []workqueue.Window{{
				Weekday: time.Monday, Start: 9 * 60, End: 11 * 60,
			}},
			ActorID: account.ID, ChangedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		lease := controlAt.Add(2 * time.Hour)
		for _, fixture := range []workqueue.Item{
			{
				ID: "queue:future-profile", State: workqueue.StateScheduled,
				NotBefore: controlAt, EligibleAt: controlAt.Add(time.Hour),
			},
			{
				ID: "queue:running-profile", State: workqueue.StateRunning,
				NotBefore: controlAt, EligibleAt: controlAt.Add(time.Hour),
				LeaseExpiresAt: &lease, StartedAt: &controlAt,
			},
		} {
			fixture.Kind, fixture.Lane = workqueue.KindReactionScan, workqueue.LaneMaintenance
			fixture.TargetID, fixture.Title = &target.TargetID, "Profile-controlled scan"
			fixture.Priority, fixture.WindowMode = workqueue.PriorityNormal, workqueue.WindowRespect
			fixture.ProfileID, fixture.CreatedAt, fixture.UpdatedAt = &profile.ID, now, now
			_, err = store.CreateQueueItem(ctx, fixture)
			Expect(err).NotTo(HaveOccurred())
		}

		_, err = store.SaveScheduleProfile(ctx, workqueue.ProfileChange{
			ID: profile.ID, Name: profile.Name, Timezone: profile.Timezone,
			Windows: []workqueue.Window{{
				Weekday: time.Monday, Start: 10 * 60, End: 12 * 60,
			}},
			ExpectedRevision: profile.Revision, ActorID: account.ID,
			ChangedAt: controlAt.Add(30 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		future, err := store.GetQueueItem(ctx, "queue:future-profile")
		Expect(err).NotTo(HaveOccurred())
		Expect(future.EligibleAt).To(Equal(controlAt.Add(2 * time.Hour)))
		Expect(future.Revision).To(Equal(int64(2)))
		running, err := store.GetQueueItem(ctx, "queue:running-profile")
		Expect(err).NotTo(HaveOccurred())
		Expect(running.EligibleAt).To(Equal(controlAt.Add(time.Hour)))
		Expect(running.Revision).To(Equal(int64(1)))
	})

	It("reports queue health and prunes each workload by its retention policy", func() {
		ctx, store, now := runtime()
		account, target := seedInstallation(ctx, store, now)
		finished := now.Add(-2 * time.Hour)
		lease := now.Add(time.Hour)
		started := now.Add(-2 * time.Minute)
		fixtures := []workqueue.Item{
			{
				ID: "queue:metric-waiting", State: workqueue.StateScheduled,
				TargetID:  &target.TargetID,
				NotBefore: now.Add(-10 * time.Minute), EligibleAt: now.Add(-10 * time.Minute),
			},
			{
				ID: "queue:metric-running", State: workqueue.StateRunning,
				TargetID:  &target.TargetID,
				NotBefore: now.Add(-5 * time.Minute), EligibleAt: now.Add(-5 * time.Minute),
				LeaseExpiresAt: &lease, StartedAt: &started,
			},
			{
				ID: "queue:metric-target-failed", State: workqueue.StateFailed,
				TargetID:  &target.TargetID,
				NotBefore: finished, EligibleAt: finished, FinishedAt: &finished,
			},
			{
				ID: "queue:metric-global-failed", State: workqueue.StateFailed,
				NotBefore: finished, EligibleAt: finished, FinishedAt: &finished,
			},
		}
		for _, fixture := range fixtures {
			fixture.Kind, fixture.Lane = workqueue.KindReactionScan, workqueue.LaneMaintenance
			fixture.Title = "Measured reaction scan"
			fixture.Priority, fixture.WindowMode = workqueue.PriorityNormal, workqueue.WindowRespect
			fixture.ProfileID = pointer(workqueue.AlwaysOpenProfileID)
			fixture.CreatedAt, fixture.UpdatedAt = now.Add(-3*time.Hour), now
			_, err := store.CreateQueueItem(ctx, fixture)
			Expect(err).NotTo(HaveOccurred())
		}
		_, err := store.CreateQueueItem(ctx, workqueue.Item{
			ID: "queue:metric-schedule-change", Kind: workqueue.KindScheduleChange,
			Lane: workqueue.LaneMaintenance, TargetID: &target.TargetID,
			Title: "Measured schedule change", State: workqueue.StateFailed,
			Priority: workqueue.PriorityNormal, WindowMode: workqueue.WindowRespect,
			ProfileID: pointer(workqueue.AlwaysOpenProfileID),
			NotBefore: finished, EligibleAt: finished, FinishedAt: &finished,
			CreatedAt: now.Add(-3 * time.Hour), UpdatedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())

		metrics, err := store.WorkQueueMetrics(ctx, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(metrics.Failures).To(Equal(3))
		Expect(metrics.MissedWindows).To(Equal(1))
		Expect(metrics.RunningLeases).To(Equal(1))
		Expect(metrics.Backlogs).To(ContainElement(And(
			HaveField("Lane", workqueue.LaneMaintenance),
			HaveField("ProfileID", workqueue.AlwaysOpenProfileID),
			HaveField("Depth", 1),
			HaveField("OldestAge", 3*time.Hour),
			HaveField("EligibleToStartLatency", 3*time.Minute),
		)))

		policy, err := store.GetEffectiveQueuePolicy(ctx, workqueue.KindReactionScan, nil)
		Expect(err).NotTo(HaveOccurred())
		retention := time.Hour
		_, err = store.SaveQueuePolicy(ctx, workqueue.PolicyChange{
			Kind: policy.Kind, Enabled: policy.Enabled, Cadence: policy.Cadence,
			ProfileID: policy.ProfileID, DefaultPriority: policy.DefaultPriority,
			RetryDelay: policy.RetryDelay, Retention: &retention,
			ApprovalTTL: policy.ApprovalTTL, Configuration: policy.Configuration,
			ExpectedRevision: policy.Revision, ActorID: account.ID, ChangedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		targetRetention := 3 * time.Hour
		targetPolicy, err := store.SaveQueuePolicy(ctx, workqueue.PolicyChange{
			Kind: policy.Kind, TargetID: &target.TargetID,
			Enabled: policy.Enabled, Cadence: policy.Cadence,
			ProfileID: policy.ProfileID, DefaultPriority: policy.DefaultPriority,
			RetryDelay: policy.RetryDelay, Retention: &targetRetention,
			ApprovalTTL: policy.ApprovalTTL, Configuration: policy.Configuration,
			ExpectedRevision: 0, ActorID: account.ID, ChangedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		removed, err := store.PruneWorkQueue(ctx, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(removed).To(Equal(int64(1)))
		_, err = store.GetQueueItem(ctx, "queue:metric-global-failed")
		Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())
		_, err = store.GetQueueItem(ctx, "queue:metric-target-failed")
		Expect(err).NotTo(HaveOccurred())
		_, err = store.GetQueueItem(ctx, "queue:metric-schedule-change")
		Expect(err).NotTo(HaveOccurred())

		targetRetention = time.Hour
		targetPolicy, err = store.SaveQueuePolicy(ctx, workqueue.PolicyChange{
			Kind: targetPolicy.Kind, TargetID: targetPolicy.TargetID,
			Enabled: targetPolicy.Enabled, Cadence: targetPolicy.Cadence,
			ProfileID:       targetPolicy.ProfileID,
			DefaultPriority: targetPolicy.DefaultPriority,
			RetryDelay:      targetPolicy.RetryDelay, Retention: &targetRetention,
			ApprovalTTL:      targetPolicy.ApprovalTTL,
			Configuration:    targetPolicy.Configuration,
			ExpectedRevision: targetPolicy.Revision,
			ActorID:          account.ID, ChangedAt: now.Add(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		removed, err = store.PruneWorkQueue(ctx, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(removed).To(Equal(int64(1)))
		_, err = store.GetQueueItem(ctx, "queue:metric-target-failed")
		Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())
	})

	It("prunes a workload whose policy carries no retention of its own", func() {
		ctx, store, now := runtime()
		account, _ := seedInstallation(ctx, store, now)
		finished := now.Add(-3 * workqueue.RoutineRetention)
		_, err := store.CreateQueueItem(ctx, workqueue.Item{
			ID: "queue:unbounded", Kind: workqueue.KindReactionScan,
			Lane: workqueue.LaneMaintenance, Title: "Scan for new commands",
			State: workqueue.StateSucceeded, Priority: workqueue.PriorityNormal,
			WindowMode: workqueue.WindowRespect,
			ProfileID:  pointer(workqueue.AlwaysOpenProfileID),
			NotBefore:  finished, EligibleAt: finished, FinishedAt: &finished,
			CreatedAt: finished, UpdatedAt: finished,
		})
		Expect(err).NotTo(HaveOccurred())

		policy, err := store.GetEffectiveQueuePolicy(ctx, workqueue.KindReactionScan, nil)
		Expect(err).NotTo(HaveOccurred())
		policy, err = store.SaveQueuePolicy(ctx, workqueue.PolicyChange{
			Kind: policy.Kind, Enabled: policy.Enabled, Cadence: policy.Cadence,
			ProfileID: policy.ProfileID, DefaultPriority: policy.DefaultPriority,
			RetryDelay: policy.RetryDelay, Retention: nil,
			ApprovalTTL: policy.ApprovalTTL, Configuration: policy.Configuration,
			ExpectedRevision: policy.Revision, ActorID: account.ID, ChangedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(policy.Retention).To(BeNil())

		removed, err := store.PruneWorkQueue(ctx, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(removed).To(Equal(int64(1)))
		_, err = store.GetQueueItem(ctx, "queue:unbounded")
		Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())
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

func seedDispatchOrderedQueue(
	ctx context.Context,
	store storage.Store,
	now time.Time,
	actorID string,
) {
	GinkgoHelper()
	policy, err := store.GetEffectiveQueuePolicy(ctx, workqueue.KindReactionScan, nil)
	Expect(err).NotTo(HaveOccurred())
	policy.Enabled = true
	policy.Cadence = 5 * time.Minute
	policy.DefaultPriority = workqueue.PriorityUrgent
	_, err = store.SaveQueuePolicy(ctx, policyChange(policy, actorID, now))
	Expect(err).NotTo(HaveOccurred())
	for index := range 6 {
		targetID := "dispatch-target-" + string(rune('a'+index))
		repositoryID := "dispatch-repository-" + string(rune('a'+index))
		_, err = store.EnsureRecurringWork(ctx, workqueue.RecurringClaim{
			Kind: workqueue.KindReactionScan, TargetID: &targetID,
			RepositoryID: &repositoryID, Title: "Scan for new commands",
			Now: now, LeaseDuration: time.Minute,
		})
		Expect(err).NotTo(HaveOccurred())
	}
	_, err = store.EnsureRecurringWork(ctx, workqueue.RecurringClaim{
		Kind: workqueue.KindCatalogRefresh, Title: "Refresh the list of repositories",
		Now: now, LeaseDuration: time.Minute,
	})
	Expect(err).NotTo(HaveOccurred())
	for range 2 {
		item, claimed, claimErr := store.ClaimNextRecurringWork(
			ctx,
			workqueue.RecurringLease{Now: now, LeaseDuration: time.Minute},
		)
		Expect(claimErr).NotTo(HaveOccurred())
		Expect(claimed).To(BeTrue())
		Expect(item.Kind).To(Equal(workqueue.KindReactionScan))
		_, err = store.FinishRecurringWork(ctx, item.ID, workqueue.RecurringCompletion{}, now)
		Expect(err).NotTo(HaveOccurred())
	}
}

func ensureRecurringClaims(
	ctx context.Context,
	store storage.Store,
	claims []workqueue.RecurringClaim,
) {
	GinkgoHelper()
	for _, claim := range claims {
		_, err := store.EnsureRecurringWork(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
	}
}

func pointer[T any](value T) *T { return &value }

func policyChange(
	policy workqueue.Policy,
	actorID string,
	changedAt time.Time,
) workqueue.PolicyChange {
	return workqueue.PolicyChange{
		Kind: policy.Kind, TargetID: policy.TargetID, Enabled: policy.Enabled,
		Cadence: policy.Cadence, ProfileID: policy.ProfileID,
		DefaultPriority: policy.DefaultPriority, RetryDelay: policy.RetryDelay,
		Retention: policy.Retention, ApprovalTTL: policy.ApprovalTTL,
		Configuration: policy.Configuration, ExpectedRevision: policy.Revision,
		ActorID: actorID, ChangedAt: changedAt,
	}
}

func onlyQueueItem(
	ctx context.Context,
	store storage.Store,
	filter workqueue.Filter,
) workqueue.Item {
	GinkgoHelper()
	page, err := store.ListWorkQueue(ctx, filter)
	Expect(err).NotTo(HaveOccurred())
	Expect(page.Items).To(HaveLen(1))

	return page.Items[0]
}

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
