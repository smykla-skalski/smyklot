package storagetest

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

func declareInstallationSyncSettingsSpecs(
	runtime func() (context.Context, storage.Store, time.Time),
) {
	declareMixedInstallationSyncSettingsSpec(runtime)
	declareInstallationSyncNoopSpec(runtime)
	declareInstallationSyncRevisionSpec(runtime)
}

func declareMixedInstallationSyncSettingsSpec(
	runtime func() (context.Context, storage.Store, time.Time),
) {
	It("saves catalog and Sync resources as one deterministic checkpoint", func() {
		ctx, store, now := runtime()
		account, target := seedInstallationSettingsBatch(ctx, store, now)
		seedInstallationSettingsPlan(ctx, store, account, target.TargetID, now)
		enabled := true
		labelsDocument := []byte("{\n  \"labels\": []\n}")
		result, err := store.SaveInstallationSettings(ctx, mixedInstallationSyncRequest(
			target.TargetID, account.ID, now.Add(time.Minute), enabled, labelsDocument,
		))
		Expect(err).NotTo(HaveOccurred())
		assertMixedInstallationSyncResult(result)

		checkpoint := readInstallationCheckpoint(
			ctx, store, *result.CheckpointID, target.TargetID,
		)
		assertMixedInstallationSyncCheckpoint(checkpoint, labelsDocument, enabled)
		assertInstallationSettingsAudit(
			ctx, store, target.TargetID, *result.CheckpointID, 1,
		)
		audit, err := store.ListAudit(ctx, target.TargetID, storage.AuditPageRequest{
			HistoryPageRequest: storage.HistoryPageRequest{Limit: 10},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(audit.Items[0].Action).To(Equal("installation.settings.saved"))
		syncAudit, err := store.ListAudit(ctx, target.TargetID, storage.AuditPageRequest{
			HistoryPageRequest: storage.HistoryPageRequest{Limit: 10},
			Change:             storage.AuditChangeSync,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(syncAudit.Items).To(ConsistOf(HaveField(
			"SettingsCheckpointID", HaveValue(Equal(*result.CheckpointID)),
		)))
		assertInstallationSettingsPlanState(ctx, store, target.TargetID, orgsync.PlanStale)
	})
}

func declareInstallationSyncNoopSpec(
	runtime func() (context.Context, storage.Store, time.Time),
) {
	It("creates absent Sync rows and suppresses exact no-ops", func() {
		ctx, store, now := runtime()
		account, target := seedInstallationSettingsBatch(ctx, store, now)
		enabled := false
		request := storage.SaveInstallationSettingsRequest{
			TargetID: target.TargetID, ActorAccountID: account.ID, ChangedAt: now,
			SyncConfigs: []storage.InstallationSyncConfigChange{{
				Kind: orgsync.KindSettings, Enabled: true,
				Document: []byte(`{"has_issues":true}`),
			}},
			SyncOverrides: []storage.InstallationSyncOverrideChange{{
				RepositoryID: "repo-1", Kind: orgsync.KindSettings, Enabled: &enabled,
			}},
		}
		created, err := store.SaveInstallationSettings(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(created.CheckpointID).NotTo(BeNil())
		Expect(created.CatalogSettingsChanged).To(BeFalse())
		Expect(created.TargetChanged).To(BeFalse())
		Expect(created.ChangedRepositoryIDs).To(BeEmpty())
		Expect(created.SyncChanged).To(BeTrue())
		Expect(created.SyncConfigs).To(ConsistOf(HaveField("Revision", int64(1))))
		Expect(created.SyncOverrides).To(ConsistOf(And(
			HaveField("Revision", int64(1)), HaveField("Document", []byte(`{}`)),
		)))

		seedInstallationSettingsPlan(ctx, store, account, target.TargetID, now.Add(time.Minute))
		request.SyncConfigs[0].ExpectedRevision = 1
		request.SyncOverrides[0].ExpectedRevision = 1
		request.ChangedAt = now.Add(2 * time.Minute)
		unchanged, err := store.SaveInstallationSettings(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(unchanged.CheckpointID).To(BeNil())
		Expect(unchanged.CatalogSettingsChanged).To(BeFalse())
		Expect(unchanged.SyncChanged).To(BeFalse())
		Expect(unchanged.SyncConfigs).To(ConsistOf(HaveField("Revision", int64(1))))
		Expect(unchanged.SyncOverrides).To(ConsistOf(HaveField("Revision", int64(1))))
		assertInstallationSettingsAudit(ctx, store, target.TargetID, *created.CheckpointID, 1)
		assertInstallationSettingsPlanState(ctx, store, target.TargetID, orgsync.PlanComputed)

		request.SyncConfigs[0].ExpectedRevision = 0
		_, err = store.SaveInstallationSettings(ctx, request)
		Expect(errors.Is(err, storage.ErrConflict)).To(BeTrue())
	})
}

func declareInstallationSyncRevisionSpec(
	runtime func() (context.Context, storage.Store, time.Time),
) {
	It("continues revisions retained by canonical deletion checkpoints", func() {
		ctx, store, now := runtime()
		account, target := seedInstallationSettingsBatch(ctx, store, now)
		created, err := store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: target.TargetID, ActorAccountID: account.ID, ChangedAt: now,
			SyncConfigs: []storage.InstallationSyncConfigChange{{
				Kind: orgsync.KindFiles, Enabled: true, Document: []byte(`{"files":[]}`),
			}},
			SyncOverrides: []storage.InstallationSyncOverrideChange{{
				RepositoryID: "repo-1", Kind: orgsync.KindFiles,
			}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(created.SyncConfigs).To(ConsistOf(HaveField("Revision", int64(1))))
		Expect(created.SyncOverrides).To(ConsistOf(HaveField("Revision", int64(1))))
		baseline, err := store.InspectInstallationSettingsBaseline(ctx, target.TargetID)
		Expect(err).NotTo(HaveOccurred())
		selections := installationRestoreSelectionsForSide(
			storage.SettingsCheckpointRestoreAfter, baseline,
		)
		Expect(selections).To(HaveLen(2))
		deleted, err := store.RestoreInstallationSettings(ctx,
			storage.RestoreInstallationSettingsRequest{
				TargetID: target.TargetID, CheckpointID: baseline.Checkpoint.ID,
				ActorAccountID: account.ID, ChangedAt: now.Add(time.Minute),
				Side: storage.SettingsCheckpointRestoreAfter, Selections: selections,
			},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(deleted.CheckpointID).NotTo(BeNil())
		deletionCheckpoint := readInstallationCheckpoint(
			ctx, store, *deleted.CheckpointID, target.TargetID,
		)

		result, err := store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: target.TargetID, ActorAccountID: account.ID,
			ChangedAt: now.Add(2 * time.Minute),
			SyncConfigs: []storage.InstallationSyncConfigChange{{
				Kind: orgsync.KindFiles, Enabled: true, Document: []byte(`{"files":[]}`),
			}},
			SyncOverrides: []storage.InstallationSyncOverrideChange{{
				RepositoryID: "repo-1", Kind: orgsync.KindFiles,
			}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.SyncConfigs).To(ConsistOf(HaveField("Revision", int64(2))))
		Expect(result.SyncOverrides).To(ConsistOf(HaveField("Revision", int64(2))))
		Expect(readInstallationCheckpoint(
			ctx, store, *deleted.CheckpointID, target.TargetID,
		)).To(Equal(deletionCheckpoint))
	})
}

func mixedInstallationSyncRequest(
	targetID, actorID string,
	changedAt time.Time,
	enabled bool,
	labelsDocument []byte,
) storage.SaveInstallationSettingsRequest {
	return storage.SaveInstallationSettingsRequest{
		TargetID: targetID, ActorAccountID: actorID, ChangedAt: changedAt,
		Target: &storage.InstallationTargetSettingsChange{
			RepositoryDefaultEnabled: true, ExpectedRevision: 1,
		},
		Repositories: []storage.InstallationRepositorySettingsChange{{
			RepositoryID: "repo-2", IgnoreRepositoryFile: true, ExpectedRevision: 1,
		}},
		SyncConfigs: []storage.InstallationSyncConfigChange{
			{Kind: orgsync.KindLabels, Enabled: true, Document: labelsDocument},
			{Kind: orgsync.KindFiles, Enabled: false, Document: []byte(`{"files":[]}`)},
		},
		SyncOverrides: []storage.InstallationSyncOverrideChange{
			{RepositoryID: "repo-2", Kind: orgsync.KindRulesets},
			{RepositoryID: "repo-1", Kind: orgsync.KindLabels, Enabled: &enabled},
		},
	}
}

func assertMixedInstallationSyncResult(result storage.SaveInstallationSettingsResult) {
	GinkgoHelper()
	Expect(result.CheckpointID).NotTo(BeNil())
	Expect(result.CatalogSettingsChanged).To(BeTrue())
	Expect(result.TargetChanged).To(BeTrue())
	Expect(result.ChangedRepositoryIDs).To(Equal([]string{"repo-2"}))
	Expect(result.SyncChanged).To(BeTrue())
	Expect(result.Target).To(HaveField("Revision", int64(2)))
	Expect(repositoryIDs(result.Repositories)).To(Equal([]string{"repo-2"}))
	Expect(result.Repositories[0].Revision).To(Equal(int64(2)))
	Expect(syncConfigKinds(result.SyncConfigs)).To(Equal([]orgsync.Kind{
		orgsync.KindFiles, orgsync.KindLabels,
	}))
	Expect(syncOverrideKeys(result.SyncOverrides)).To(Equal([]string{
		"repo-1/labels", "repo-2/rulesets",
	}))
	for _, config := range result.SyncConfigs {
		Expect(config.Revision).To(Equal(int64(1)))
	}
	for _, override := range result.SyncOverrides {
		Expect(override.Revision).To(Equal(int64(1)))
	}
	Expect(result.SyncOverrides[1].Document).To(Equal([]byte(`{}`)))
}

func assertMixedInstallationSyncCheckpoint(
	checkpoint storage.SettingsCheckpoint,
	labelsDocument []byte,
	enabled bool,
) {
	GinkgoHelper()
	Expect(checkpoint.Items).To(HaveLen(7))
	Expect(checkpointItemKinds(checkpoint.Items)).To(Equal([]storage.SettingsCheckpointItemKind{
		storage.SettingsCheckpointItemRepository,
		storage.SettingsCheckpointItemRepository,
		storage.SettingsCheckpointItemSyncConfig,
		storage.SettingsCheckpointItemSyncConfig,
		storage.SettingsCheckpointItemSyncOverride,
		storage.SettingsCheckpointItemSyncOverride,
		storage.SettingsCheckpointItemTarget,
	}))
	Expect(checkpoint.Items[0].RepositoryID).To(Equal("repo-1"))
	Expect(checkpoint.Items[0].Before.Digest).To(Equal(checkpoint.Items[0].After.Digest))
	Expect(checkpoint.Items[2].SyncKind).To(Equal(orgsync.KindFiles))
	Expect(checkpoint.Items[3].SyncKind).To(Equal(orgsync.KindLabels))
	Expect(checkpoint.Items[4]).To(And(
		HaveField("RepositoryID", "repo-1"), HaveField("SyncKind", orgsync.KindLabels),
	))
	Expect(checkpoint.Items[5]).To(And(
		HaveField("RepositoryID", "repo-2"), HaveField("SyncKind", orgsync.KindRulesets),
	))
	for _, index := range []int{2, 3, 4, 5} {
		Expect(checkpoint.Items[index].Before).To(BeNil())
		Expect(checkpoint.Items[index].After).NotTo(BeNil())
		Expect(checkpoint.Items[index].After.Revision).To(Equal(int64(1)))
	}
	labels := decodeSettingsDocument[storage.SyncConfigSettingsDocument](
		checkpoint.Items[3].After.Document,
	)
	Expect(labels).To(Equal(storage.SyncConfigSettingsDocument{
		Enabled: true, Document: string(labelsDocument),
	}))
	labelOverride := decodeSettingsDocument[storage.SyncOverrideSettingsDocument](
		checkpoint.Items[4].After.Document,
	)
	Expect(labelOverride).To(Equal(storage.SyncOverrideSettingsDocument{
		Enabled: &enabled, Document: `{}`,
	}))
	rulesetOverride := decodeSettingsDocument[storage.SyncOverrideSettingsDocument](
		checkpoint.Items[5].After.Document,
	)
	Expect(rulesetOverride.Document).To(Equal(`{}`))
}

func syncConfigKinds(configs []orgsync.Config) []orgsync.Kind {
	kinds := make([]orgsync.Kind, 0, len(configs))
	for _, config := range configs {
		kinds = append(kinds, config.Kind)
	}

	return kinds
}

func syncOverrideKeys(overrides []orgsync.RepositoryOverride) []string {
	keys := make([]string, 0, len(overrides))
	for _, override := range overrides {
		keys = append(keys, override.RepositoryID+"/"+string(override.Kind))
	}

	return keys
}
