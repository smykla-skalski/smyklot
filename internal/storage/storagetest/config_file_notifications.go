package storagetest

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func declareConfigFileNotificationSpecs(runtime queueRuntime) {
	declareConfigFileSaveNotificationSpec(runtime)
	declareConfigFileDeferredNotificationSpecs(runtime)
	It("configuration file notifications expedite checks and retain changes during a running check", func() {
		ctx, store, now := runtime()
		account, installation := seedInstallationSettingsBatch(ctx, store, now)
		enableConfigFileConnections(ctx, store, account, installation.TargetID, now)
		count, err := store.DispatchConfigFileNotifications(ctx, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(2), "activation saves notify both enabled scopes")
		finishNotifiedConfigFiles(ctx, store, now)
		now = now.Add(time.Minute)
		for range 3 {
			Expect(store.NotifyConfigFileChange(ctx, installation.TargetID, "repo-1", now)).To(Succeed())
		}
		count, err = store.DispatchConfigFileNotifications(ctx, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(1), "coalesce repeated source events")
		item, claimed, err := store.ClaimNextRecurringWork(ctx, workqueue.RecurringLease{Now: now, LeaseDuration: time.Minute})
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).To(BeTrue())
		Expect(*item.RepositoryID).To(Equal("repo-1"))
		Expect(item.Immediate).To(BeFalse())
		Expect(item.WindowMode).To(Equal(workqueue.WindowRespect))
		Expect(store.NotifyConfigFileChange(ctx, installation.TargetID, "repo-1", now)).To(Succeed())
		count, err = store.DispatchConfigFileNotifications(ctx, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(BeZero(), "a running check cannot consume a newer notification")
		_, err = store.FinishRecurringWork(ctx, item.ID, workqueue.RecurringCompletion{}, now)
		Expect(err).NotTo(HaveOccurred())
		page, err := store.ListWorkQueue(ctx, workqueue.Filter{
			Kinds:  []workqueue.Kind{workqueue.KindConfigFileSync},
			States: []workqueue.State{workqueue.StateScheduled}, RepositoryID: new("repo-1"),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(page.Items).To(HaveLen(1))
		Expect(page.Items[0].NotBefore).To(Equal(now.Add(15*time.Minute)), "the fallback starts from the expedited check")
		count, err = store.DispatchConfigFileNotifications(ctx, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(1))
		next, claimed, err := store.ClaimNextRecurringWork(ctx, workqueue.RecurringLease{Now: now, LeaseDuration: time.Minute})
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).To(BeTrue())
		Expect(next.ID).NotTo(Equal(item.ID))
		Expect(*next.RepositoryID).To(Equal("repo-1"))
	})
	It("configuration file notifications respect disabled policies and active hours", func() {
		ctx, store, now := runtime()
		account, installation := seedInstallationSettingsBatch(ctx, store, now)
		enableConfigFileConnections(ctx, store, account, installation.TargetID, now)
		policy, err := store.GetEffectiveQueuePolicy(ctx, workqueue.KindConfigFileSync, nil)
		Expect(err).NotTo(HaveOccurred())
		policy.Enabled = false
		policy, err = store.SaveQueuePolicy(ctx, policyChange(policy, account.ID, now))
		Expect(err).NotTo(HaveOccurred())
		count, err := store.DispatchConfigFileNotifications(ctx, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(BeZero())
		now = time.Date(2026, time.September, 7, 12, 0, 0, 0, time.UTC)
		profile, err := store.SaveScheduleProfile(ctx, workqueue.ProfileChange{
			ID: "config-notifications-afternoon", Name: "Monday afternoon", Timezone: "UTC",
			Windows: []workqueue.Window{{Weekday: time.Monday, Start: 13 * 60, End: 14 * 60}},
			ActorID: account.ID, ChangedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		policy.Enabled, policy.ProfileID = true, profile.ID
		_, err = store.SaveQueuePolicy(ctx, policyChange(policy, account.ID, now))
		Expect(err).NotTo(HaveOccurred())
		count, err = store.DispatchConfigFileNotifications(ctx, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(2), "paused notifications survive until enabled")
		page, err := store.ListWorkQueue(ctx, workqueue.Filter{Kinds: []workqueue.Kind{workqueue.KindConfigFileSync}})
		Expect(err).NotTo(HaveOccurred())
		Expect(page.Items).To(HaveLen(2))
		for _, item := range page.Items {
			Expect(item.EligibleAt).To(Equal(now.Add(time.Hour)))
			Expect(item.Immediate).To(BeFalse())
			Expect(item.WindowMode).To(Equal(workqueue.WindowRespect))
		}
	})
	It("configuration file notifications ignore disabled connections and reject foreign repositories", func() {
		ctx, store, now := runtime()
		_, installation := seedInstallationSettingsBatch(ctx, store, now)
		Expect(store.NotifyConfigFileChange(ctx, installation.TargetID, "repo-1", now)).To(Succeed())
		count, err := store.DispatchConfigFileNotifications(ctx, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(BeZero())
		Expect(store.NotifyConfigFileChange(ctx, installation.TargetID, "not-owned", now)).To(MatchError(storage.ErrNotFound))
		Expect(store.NotifyConfigFileChange(ctx, "", "repo-1", now)).NotTo(Succeed())
		Expect(store.NotifyConfigFileChange(ctx, installation.TargetID, "repo-1", time.Time{})).NotTo(Succeed())
		_, err = store.DispatchConfigFileNotifications(ctx, time.Time{})
		Expect(err).To(HaveOccurred())
	})
}

func declareConfigFileDeferredNotificationSpecs(runtime queueRuntime) {
	It("configuration file notifications preserve an operator's delay after the original cadence elapsed", func() {
		ctx, store, now := runtime()
		account, installation := seedInstallationSettingsBatch(ctx, store, now)
		enableConfigFileConnections(ctx, store, account, installation.TargetID, now)
		_, err := store.DispatchConfigFileNotifications(ctx, now)
		Expect(err).NotTo(HaveOccurred())
		item := onlyQueueItem(ctx, store, workqueue.Filter{
			Kinds: []workqueue.Kind{workqueue.KindConfigFileSync}, RepositoryID: new("repo-1"),
		})
		delayed := now.Add(6 * time.Hour)
		item, err = store.ApplyQueueAction(ctx, item.ID, workqueue.ItemAction{
			Type: workqueue.ActionScheduleAt, At: delayed, ExpectedRevision: item.Revision,
			ActorID: account.ID, ChangedAt: now.Add(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		now = now.Add(16 * time.Minute)
		Expect(store.NotifyConfigFileChange(ctx, installation.TargetID, "repo-1", now)).To(Succeed())
		count, err := store.DispatchConfigFileNotifications(ctx, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(1))
		unchanged, err := store.GetQueueItem(ctx, item.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(unchanged.NotBefore).To(Equal(delayed))
		Expect(unchanged.EligibleAt).To(Equal(delayed))
		Expect(unchanged.Revision).To(Equal(item.Revision))
		_, claimed, err := store.ClaimRecurringWork(ctx, workqueue.RecurringClaim{
			Kind: workqueue.KindConfigFileSync, TargetID: &installation.TargetID,
			RepositoryID: new("repo-1"), Title: "Sync repository configuration",
			Now: now, LeaseDuration: time.Minute,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).To(BeFalse(), "fallback reconciliation must also retain the operator's delay")
	})
}

func declareConfigFileSaveNotificationSpec(runtime queueRuntime) {
	It("configuration file notifications commit with Sync-only saves and restores but not no-ops or rejected writes", func() {
		ctx, store, now := runtime()
		account, installation := seedInstallationSettingsBatch(ctx, store, now)
		enableConfigFileConnections(ctx, store, account, installation.TargetID, now)
		_, err := store.DispatchConfigFileNotifications(ctx, now)
		Expect(err).NotTo(HaveOccurred())
		finishNotifiedConfigFiles(ctx, store, now)
		now = now.Add(time.Minute)
		request := storage.SaveInstallationSettingsRequest{
			TargetID: installation.TargetID, ActorAccountID: account.ID, ChangedAt: now,
			SyncConfigs: []storage.InstallationSyncConfigChange{{
				Kind: orgsync.KindSettings, Enabled: true, Document: []byte(`{"has_issues":true}`),
			}},
			SyncOverrides: []storage.InstallationSyncOverrideChange{{
				RepositoryID: "repo-1", Kind: orgsync.KindSettings, Enabled: new(false),
			}},
		}
		saved, err := store.SaveInstallationSettings(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(saved.CatalogSettingsChanged).To(BeFalse())
		Expect(saved.CheckpointID).NotTo(BeNil())
		count, err := store.DispatchConfigFileNotifications(ctx, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(2))
		request.SyncConfigs[0].ExpectedRevision, request.SyncOverrides[0].ExpectedRevision = 1, 1
		unchanged, err := store.SaveInstallationSettings(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(unchanged.CheckpointID).To(BeNil())
		count, err = store.DispatchConfigFileNotifications(ctx, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(BeZero())
		inspection, err := store.InspectInstallationSettingsCheckpoint(ctx,
			installationCheckpointRef(installation.TargetID, *saved.CheckpointID))
		Expect(err).NotTo(HaveOccurred())
		_, err = store.RestoreInstallationSettings(ctx, installationSideRestoreRequest(
			installation.TargetID, *saved.CheckpointID, account.ID, now,
			storage.SettingsCheckpointRestoreBefore, inspection))
		Expect(err).NotTo(HaveOccurred())
		count, err = store.DispatchConfigFileNotifications(ctx, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(2))
		request.SyncConfigs[0].ExpectedRevision = 99
		_, err = store.SaveInstallationSettings(ctx, request)
		Expect(err).To(MatchError(storage.ErrConflict))
		count, err = store.DispatchConfigFileNotifications(ctx, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(BeZero(), "the rejected save must not leave a wake-up behind")
	})
}

func finishNotifiedConfigFiles(ctx context.Context, store storage.Store, now time.Time) {
	for range 2 {
		item, claimed, err := store.ClaimNextRecurringWork(ctx, workqueue.RecurringLease{Now: now, LeaseDuration: time.Minute})
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).To(BeTrue())
		Expect(item.Kind).To(Equal(workqueue.KindConfigFileSync))
		_, err = store.FinishRecurringWork(ctx, item.ID, workqueue.RecurringCompletion{}, now)
		Expect(err).NotTo(HaveOccurred())
	}
}
