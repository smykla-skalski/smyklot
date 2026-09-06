package storagetest

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func declareConfigFileWorkloadSpecs(runtime queueRuntime) {
	declareConfigFilePauseSpecs(runtime)
	declareConfigFileWindowSpecs(runtime)
	It("keeps configuration file connections separate and retries their unfinished work", func() {
		ctx, store, now := runtime()
		targetID, repositoryID := "configuration-target", "configuration-repository"
		workspace, err := store.EnsureRecurringWork(ctx, workqueue.RecurringClaim{
			Kind: workqueue.KindConfigFileSync, TargetID: &targetID,
			Title: "Sync workspace configuration", Now: now, LeaseDuration: time.Minute,
		})
		Expect(err).NotTo(HaveOccurred())
		repository, err := store.EnsureRecurringWork(ctx, workqueue.RecurringClaim{
			Kind: workqueue.KindConfigFileSync, TargetID: &targetID, RepositoryID: &repositoryID,
			Title: "Sync repository configuration", Now: now, LeaseDuration: time.Minute,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.ID).NotTo(Equal(workspace.ID))
		Expect(repository.SourceID).NotTo(Equal(workspace.SourceID))
		for range 2 {
			item, claimed, err := store.ClaimNextRecurringWork(ctx, workqueue.RecurringLease{Now: now, LeaseDuration: time.Minute})
			Expect(err).NotTo(HaveOccurred())
			Expect(claimed).To(BeTrue())
			Expect(item.Kind).To(Equal(workqueue.KindConfigFileSync))
			retrying, err := store.FinishRecurringWork(ctx, item.ID, workqueue.RecurringCompletion{
				Failure: "GitHub is unavailable", Retryable: true,
			}, now)
			Expect(err).NotTo(HaveOccurred())
			Expect(retrying.State).To(Equal(workqueue.StateRetrying))
			Expect(retrying.EligibleAt).To(Equal(now.Add(30 * time.Second)))
		}
		for range 2 {
			item, claimed, err := store.ClaimNextRecurringWork(ctx, workqueue.RecurringLease{
				Now: now.Add(30 * time.Second), LeaseDuration: time.Minute,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(claimed).To(BeTrue())
			Expect(item.Attempt).To(Equal(2))
			_, err = store.FinishRecurringWork(ctx, item.ID, workqueue.RecurringCompletion{
				SuccessSummary: "Configuration is up to date",
			}, now.Add(time.Minute))
			Expect(err).NotTo(HaveOccurred())
		}
		page, err := store.ListWorkQueue(ctx, workqueue.Filter{
			Kinds: []workqueue.Kind{workqueue.KindConfigFileSync}, States: []workqueue.State{workqueue.StateScheduled},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(page.Items).To(HaveLen(2))
		for _, item := range page.Items {
			Expect(item.NotBefore).To(Equal(now.Add(15 * time.Minute)))
		}
	})
}

func declareConfigFileWindowSpecs(runtime queueRuntime) {
	for _, bypass := range []bool{false, true} {
		name := "keeps configuration file connections on the current retry window"
		if bypass {
			name += " with an explicit Run now exception"
		}
		It(name, func() {
			ctx, store, seededAt := runtime()
			account, target := seedInstallation(ctx, store, seededAt)
			now := time.Date(2026, time.August, 24, 12, 0, 0, 0, time.UTC)
			item, err := store.EnsureRecurringWork(ctx, workqueue.RecurringClaim{
				Kind: workqueue.KindConfigFileSync, TargetID: &target.TargetID,
				Title: "Sync configuration", Now: now, LeaseDuration: time.Minute,
			})
			Expect(err).NotTo(HaveOccurred())
			if bypass {
				_, err = store.ApplyQueueAction(ctx, item.ID, workqueue.ItemAction{
					Type: workqueue.ActionRunNow, ExpectedRevision: item.Revision,
					ActorID: account.ID, Reason: "Sync the urgent fix", ChangedAt: now,
				})
				Expect(err).NotTo(HaveOccurred())
			}
			item, claimed, err := store.ClaimNextRecurringWork(ctx, workqueue.RecurringLease{
				Now: now, LeaseDuration: time.Minute,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(claimed).To(BeTrue())
			profile, err := store.SaveScheduleProfile(ctx, workqueue.ProfileChange{
				ID: "configuration-afternoon", Name: "Monday afternoon", Timezone: "UTC",
				Windows: []workqueue.Window{{Weekday: time.Monday, Start: 13 * 60, End: 14 * 60}},
				ActorID: account.ID, ChangedAt: now,
			})
			Expect(err).NotTo(HaveOccurred())
			policy, err := store.GetEffectiveQueuePolicy(ctx, item.Kind, nil)
			Expect(err).NotTo(HaveOccurred())
			policy.ProfileID = profile.ID
			_, err = store.SaveQueuePolicy(ctx, policyChange(policy, account.ID, now))
			Expect(err).NotTo(HaveOccurred())
			retrying, err := store.FinishRecurringWork(ctx, item.ID, workqueue.RecurringCompletion{
				Failure: "GitHub is unavailable", Retryable: true,
			}, now)
			Expect(err).NotTo(HaveOccurred())
			Expect(retrying.State).To(Equal(workqueue.StateRetrying))
			expected := now.Add(time.Hour)
			if bypass {
				expected = now.Add(30 * time.Second)
			}
			Expect(retrying.EligibleAt).To(Equal(expected))
			Expect(retrying.ProfileID).NotTo(BeNil())
			Expect(*retrying.ProfileID).To(Equal(profile.ID))
			_, claimed, err = store.ClaimNextRecurringWork(ctx, workqueue.RecurringLease{
				Now: expected.Add(-time.Second), LeaseDuration: time.Minute,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(claimed).To(BeFalse())
			resumed, claimed, err := store.ClaimNextRecurringWork(ctx, workqueue.RecurringLease{
				Now: expected, LeaseDuration: time.Minute,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(claimed).To(BeTrue())
			Expect(resumed.ID).To(Equal(item.ID))
			Expect(*resumed.ProfileID).To(Equal(profile.ID))
		})
	}
}

func declareConfigFilePauseSpecs(runtime queueRuntime) {
	It("keeps configuration file connections paused when running work fails", func() {
		ctx, store, now := runtime()
		account, target := seedInstallation(ctx, store, now)
		_, err := store.EnsureRecurringWork(ctx, workqueue.RecurringClaim{
			Kind: workqueue.KindConfigFileSync, TargetID: &target.TargetID,
			Title: "Sync configuration", Now: now, LeaseDuration: time.Minute,
		})
		Expect(err).NotTo(HaveOccurred())
		item, claimed, err := store.ClaimNextRecurringWork(ctx, workqueue.RecurringLease{
			Now: now, LeaseDuration: time.Minute,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).To(BeTrue())
		policy, err := store.GetEffectiveQueuePolicy(ctx, item.Kind, nil)
		Expect(err).NotTo(HaveOccurred())
		policy.Enabled = false
		policy, err = store.SaveQueuePolicy(ctx, policyChange(policy, account.ID, now))
		Expect(err).NotTo(HaveOccurred())
		paused, err := store.FinishRecurringWork(ctx, item.ID, workqueue.RecurringCompletion{
			Failure: "GitHub is unavailable", Retryable: true,
		}, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(paused.State).To(Equal(workqueue.StateBlocked))
		Expect(paused.BlockedReason).To(Equal("Workload disabled by policy"))
		Expect(paused.FinishedAt).To(BeNil())
		_, claimed, err = store.ClaimNextRecurringWork(ctx, workqueue.RecurringLease{
			Now: now.Add(time.Hour), LeaseDuration: time.Minute,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).To(BeFalse())
		policy.Enabled = true
		_, err = store.SaveQueuePolicy(ctx, policyChange(policy, account.ID, now.Add(time.Hour)))
		Expect(err).NotTo(HaveOccurred())
		resumed, claimed, err := store.ClaimNextRecurringWork(ctx, workqueue.RecurringLease{
			Now: now.Add(time.Hour), LeaseDuration: time.Minute,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).To(BeTrue())
		Expect(resumed.ID).To(Equal(item.ID))
		Expect(resumed.Attempt).To(Equal(2))
		Expect(resumed.BlockedReason).To(BeEmpty())
	})
}
