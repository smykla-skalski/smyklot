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

func declareInstallationSettingsRestoreSpecs(
	harness Harness,
	runtime func() (context.Context, storage.Store, time.Time),
) {
	declareInstallationSettingsRestoreSpec(runtime)
	declareInstallationSettingsRestoreConflictSpec(runtime)
	declareInstallationSettingsInspectionCompatibilitySpec(harness, runtime)
	declareInstallationSettingsRestoreValidationSpec(harness, runtime)
	declareInstallationSettingsBaselineAbsenceSpec(runtime)
	declareInstallationSettingsRestoreSideSpecs(runtime)
}

func declareInstallationSettingsRestoreSpec(
	runtime func() (context.Context, storage.Store, time.Time),
) {
	It("inspects and restores selected settings without mutating the source", func() {
		ctx, store, now := runtime()
		account, target, sourceID := seedInstallationSettingsRestoreHistory(ctx, store, now)
		original := readInstallationCheckpoint(ctx, store, sourceID, target.TargetID)
		inspection, err := store.InspectInstallationSettingsCheckpoint(
			ctx, installationCheckpointRef(target.TargetID, sourceID),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(inspection.Items).To(HaveLen(5))
		changed := 0
		for _, item := range inspection.Items {
			Expect(item.After.Restorable).To(BeTrue())
			Expect(item.After.Incompatibility).To(BeNil())
			Expect(item.Current).NotTo(BeNil())
			if item.Changed {
				changed++
			}
		}
		Expect(changed).To(Equal(4))

		seedInstallationSettingsPlan(ctx, store, account, target.TargetID, now.Add(3*time.Minute))
		result, err := store.RestoreInstallationSettings(ctx,
			storage.RestoreInstallationSettingsRequest{
				TargetID: target.TargetID, CheckpointID: sourceID,
				Side:           storage.SettingsCheckpointRestoreAfter,
				ActorAccountID: account.ID, ChangedAt: now.Add(4 * time.Minute),
				Selections: restoreSelections(inspection),
			},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.CheckpointID).NotTo(BeNil())
		Expect(result.Target).To(And(
			HaveField("Revision", int64(4)),
			HaveField("RepositoryDefaultEnabled", true),
		))
		Expect(result.Repositories).To(ConsistOf(And(
			HaveField("Revision", int64(4)),
			HaveField("EnabledOverride", HaveValue(BeTrue())),
		)))
		Expect(result.SyncConfigs).To(ConsistOf(And(
			HaveField("Revision", int64(3)),
			HaveField("Document", []byte(`{"labels":[]}`)),
		)))
		Expect(result.SyncOverrides).To(ConsistOf(And(
			HaveField("Revision", int64(3)),
			HaveField("Enabled", HaveValue(BeTrue())),
		)))

		restored := readInstallationCheckpoint(
			ctx, store, *result.CheckpointID, target.TargetID,
		)
		Expect(restored.Action).To(Equal(storage.SettingsCheckpointActionRestore))
		Expect(restored.RestoredFromID).To(HaveValue(Equal(sourceID)))
		Expect(restored.RestoredSide).To(Equal(storage.SettingsCheckpointRestoreAfter))
		Expect(restored.Items).To(HaveLen(5))
		for _, item := range restored.Items {
			Expect(item.Before).NotTo(BeNil())
			Expect(item.After).NotTo(BeNil())
		}
		Expect(readInstallationCheckpoint(ctx, store, sourceID, target.TargetID)).To(Equal(original))
		assertInstallationSettingsAudit(
			ctx, store, target.TargetID, *result.CheckpointID, 3,
		)
		assertInstallationSettingsPlanState(ctx, store, target.TargetID, orgsync.PlanStale)

		_, err = store.RestoreInstallationSettings(ctx,
			storage.RestoreInstallationSettingsRequest{
				TargetID: target.TargetID, CheckpointID: sourceID,
				Side:           storage.SettingsCheckpointRestoreAfter,
				ActorAccountID: account.ID, ChangedAt: now.Add(5 * time.Minute),
				Selections: []storage.SettingsCheckpointRestoreSelection{{
					Identity: storage.SettingsCheckpointItemIdentity{
						Kind: storage.SettingsCheckpointItemTarget,
					},
					ExpectedRevision: 4,
				}},
			},
		)
		Expect(errors.Is(err, storage.ErrSettingsRestoreNoop)).To(BeTrue())
		assertInstallationSettingsAudit(
			ctx, store, target.TargetID, *result.CheckpointID, 3,
		)
	})
}

func declareInstallationSettingsRestoreConflictSpec(
	runtime func() (context.Context, storage.Store, time.Time),
) {
	It("rejects one stale selection before writing any restored state", func() {
		ctx, store, now := runtime()
		account, target, sourceID := seedInstallationSettingsRestoreHistory(ctx, store, now)
		inspection, err := store.InspectInstallationSettingsCheckpoint(
			ctx, installationCheckpointRef(target.TargetID, sourceID),
		)
		Expect(err).NotTo(HaveOccurred())
		selections := restoreSelections(inspection)
		selections[0].ExpectedRevision--

		_, err = store.RestoreInstallationSettings(ctx,
			storage.RestoreInstallationSettingsRequest{
				TargetID: target.TargetID, CheckpointID: sourceID,
				Side:           storage.SettingsCheckpointRestoreAfter,
				ActorAccountID: account.ID, ChangedAt: now.Add(4 * time.Minute),
				Selections: selections,
			},
		)
		Expect(errors.Is(err, storage.ErrConflict)).To(BeTrue())
		current, err := store.GetTarget(ctx, target.TargetID)
		Expect(err).NotTo(HaveOccurred())
		Expect(current).To(And(
			HaveField("Revision", int64(3)),
			HaveField("RepositoryDefaultEnabled", false),
		))
		assertInstallationSettingsAudit(ctx, store, target.TargetID, sourceID+1, 2)
	})
}

func declareInstallationSettingsInspectionCompatibilitySpec(
	harness Harness,
	runtime func() (context.Context, storage.Store, time.Time),
) {
	It("keeps incompatible history visible but not selectable", func() {
		ctx, store, now := runtime()
		account, target := seedInstallationSettingsBatch(ctx, store, now)
		currentTarget, err := store.GetTarget(ctx, target.TargetID)
		Expect(err).NotTo(HaveOccurred())
		currentRepository, err := store.GetRepository(ctx, target.TargetID, "repo-1")
		Expect(err).NotTo(HaveOccurred())
		saved, err := store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: target.TargetID, ActorAccountID: account.ID, ChangedAt: now,
			Target: &storage.InstallationTargetSettingsChange{
				RepositoryDefaultEnabled: true, ExpectedRevision: currentTarget.Revision,
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(saved.CheckpointID).NotTo(BeNil())
		Expect(saved.Target).NotTo(BeNil())

		badTarget := installationTargetDocument(*saved.Target)
		badTarget.PendingCIModeDefault = "unsupported"
		rewriteCheckpointItem(harness, ctx, *saved.CheckpointID,
			storage.SettingsCheckpointItemIdentity{Kind: storage.SettingsCheckpointItemTarget},
			storage.SettingsCheckpointDocumentVersion, badTarget,
		)
		repositoryIdentity := storage.SettingsCheckpointItemIdentity{
			Kind: storage.SettingsCheckpointItemRepository, RepositoryID: currentRepository.ID,
		}
		rewriteCheckpointItem(harness, ctx, *saved.CheckpointID, repositoryIdentity,
			storage.SettingsCheckpointDocumentVersion+1,
			installationRepositoryDocument(currentRepository),
		)
		inspection, err := store.InspectInstallationSettingsCheckpoint(
			ctx, installationCheckpointRef(target.TargetID, *saved.CheckpointID),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(inspection.Items).To(HaveLen(3))
		repositoryItem := inspectedCheckpointItem(inspection, repositoryIdentity)
		Expect(repositoryItem).To(And(
			HaveField("After.Restorable", false),
			HaveField("After.Incompatibility.Code", "unsupported_document_version"),
			HaveField("After.State.Document", Not(BeEmpty())),
		))
		targetItem := inspectedCheckpointItem(inspection, storage.SettingsCheckpointItemIdentity{
			Kind: storage.SettingsCheckpointItemTarget,
		})
		Expect(targetItem).To(And(
			HaveField("After.Restorable", false),
			HaveField("After.Incompatibility.Code", "snapshot_incompatible"),
			HaveField("After.State.Document", Not(BeEmpty())),
		))

		_, err = store.RestoreInstallationSettings(ctx,
			storage.RestoreInstallationSettingsRequest{
				TargetID: target.TargetID, CheckpointID: *saved.CheckpointID,
				Side:           storage.SettingsCheckpointRestoreAfter,
				ActorAccountID: account.ID, ChangedAt: now.Add(time.Minute),
				Selections: []storage.SettingsCheckpointRestoreSelection{{
					Identity: targetItem.Identity, ExpectedRevision: saved.Target.Revision,
				}},
			},
		)
		Expect(errors.Is(err, storage.ErrSettingsRestoreBlocked)).To(BeTrue())
		assertInstallationSettingsAudit(ctx, store, target.TargetID, *saved.CheckpointID, 1)

		Expect(store.ReconcileInstallation(ctx, testInstallation(
			account, now.Add(2*time.Minute), []storage.RepositorySnapshot{
				testRepository("repo-2", "smykla-skalski/platform", true),
			},
		))).To(Succeed())
		inspection, err = store.InspectInstallationSettingsCheckpoint(
			ctx, installationCheckpointRef(target.TargetID, *saved.CheckpointID),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(inspectedCheckpointItem(inspection, repositoryIdentity).After.Incompatibility.Code).
			To(Equal("repository_unavailable"))
	})
}

func declareInstallationSettingsRestoreValidationSpec(
	harness Harness,
	runtime func() (context.Context, storage.Store, time.Time),
) {
	It("cross-validates every selected state before writing any of them", func() {
		ctx, store, now := runtime()
		account, target := seedInstallationSettingsBatch(ctx, store, now)
		saved, err := store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: target.TargetID, ActorAccountID: account.ID, ChangedAt: now,
			SyncConfigs: []storage.InstallationSyncConfigChange{{
				Kind: orgsync.KindFiles, Enabled: true,
				Document: []byte(installationFilesDocument),
			}},
			SyncOverrides: []storage.InstallationSyncOverrideChange{{
				RepositoryID: "repo-1", Kind: orgsync.KindFiles,
				Document: []byte(installationFilesOverride),
			}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(saved.CheckpointID).NotTo(BeNil())
		configIdentity := storage.SettingsCheckpointItemIdentity{
			Kind: storage.SettingsCheckpointItemSyncConfig, SyncKind: orgsync.KindFiles,
		}
		overrideIdentity := storage.SettingsCheckpointItemIdentity{
			Kind:         storage.SettingsCheckpointItemSyncOverride,
			RepositoryID: "repo-1", SyncKind: orgsync.KindFiles,
		}
		rewriteCheckpointItem(harness, ctx, *saved.CheckpointID, configIdentity,
			storage.SettingsCheckpointDocumentVersion,
			storage.SyncConfigSettingsDocument{Enabled: true, Document: `{"files":[]}`},
		)
		rewriteCheckpointItem(harness, ctx, *saved.CheckpointID, overrideIdentity,
			storage.SettingsCheckpointDocumentVersion,
			storage.SyncOverrideSettingsDocument{
				Document: `{"merges":[{"path":"package.json",` +
					`"overrides":{"timezone":"UTC"}}]}`,
			},
		)
		inspection, err := store.InspectInstallationSettingsCheckpoint(
			ctx, installationCheckpointRef(target.TargetID, *saved.CheckpointID),
		)
		Expect(err).NotTo(HaveOccurred())
		for _, item := range inspection.Items {
			Expect(item.After.Restorable).To(BeTrue())
		}
		selections := restoreSelectionsAllowingAbsence(inspection)
		Expect(selections).To(HaveLen(2))

		_, err = store.RestoreInstallationSettings(ctx,
			storage.RestoreInstallationSettingsRequest{
				TargetID: target.TargetID, CheckpointID: *saved.CheckpointID,
				Side:           storage.SettingsCheckpointRestoreAfter,
				ActorAccountID: account.ID, ChangedAt: now.Add(time.Minute),
				Selections: selections,
			},
		)
		Expect(errors.Is(err, orgsync.ErrInvalidConfig)).To(BeTrue())
		currentTarget, err := store.GetTarget(ctx, target.TargetID)
		Expect(err).NotTo(HaveOccurred())
		Expect(currentTarget).To(And(
			HaveField("Revision", int64(1)),
			HaveField("RepositoryDefaultEnabled", false),
		))
		config, err := store.GetSyncConfig(ctx, target.TargetID, orgsync.KindFiles)
		Expect(err).NotTo(HaveOccurred())
		Expect(config).To(And(
			HaveField("Revision", int64(1)),
			HaveField("Document", []byte(installationFilesDocument)),
		))
		override, err := store.GetSyncRepositoryOverride(
			ctx, target.TargetID, "repo-1", orgsync.KindFiles,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(override).To(And(
			HaveField("Revision", int64(1)),
			HaveField("Document", []byte(installationFilesOverride)),
		))
		assertInstallationSettingsAudit(ctx, store, target.TargetID, *saved.CheckpointID, 1)
	})
}

func declareInstallationSettingsBaselineAbsenceSpec(
	runtime func() (context.Context, storage.Store, time.Time),
) {
	It("restores Sync absence from a complete baseline with monotonic revisions", func() {
		ctx, store, now := runtime()
		account, target := seedInstallationSettingsBatch(ctx, store, now)
		baselineInspection, err := store.InspectInstallationSettingsBaseline(ctx, target.TargetID)
		Expect(err).NotTo(HaveOccurred())
		baseline := baselineInspection.Checkpoint
		Expect(baseline.Action).To(Equal(storage.SettingsCheckpointActionBaseline))
		enabled := true
		saved, err := store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: target.TargetID, ActorAccountID: account.ID, ChangedAt: now.Add(time.Minute),
			SyncConfigs: []storage.InstallationSyncConfigChange{{
				Kind: orgsync.KindLabels, Enabled: true, Document: []byte(`{"labels":[]}`),
			}},
			SyncOverrides: []storage.InstallationSyncOverrideChange{{
				RepositoryID: "repo-1", Kind: orgsync.KindLabels, Enabled: &enabled,
			}},
		})
		Expect(err).NotTo(HaveOccurred())
		inspection, err := store.InspectInstallationSettingsCheckpoint(
			ctx, installationCheckpointRef(target.TargetID, baseline.ID),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(inspection.Items).To(HaveLen(5))
		selections := []storage.SettingsCheckpointRestoreSelection{}
		for _, item := range inspection.Items {
			if item.Identity.Kind == storage.SettingsCheckpointItemSyncConfig ||
				item.Identity.Kind == storage.SettingsCheckpointItemSyncOverride {
				Expect(item.After.State).To(BeNil())
				Expect(item.Current).NotTo(BeNil())
				Expect(item.After.Differs).To(BeTrue())
				Expect(item.After.Restorable).To(BeTrue())
				selections = append(selections, storage.SettingsCheckpointRestoreSelection{
					Identity: item.Identity, ExpectedRevision: item.Current.Revision,
				})
			}
		}

		restored, err := store.RestoreInstallationSettings(ctx,
			storage.RestoreInstallationSettingsRequest{
				TargetID: target.TargetID, CheckpointID: baseline.ID,
				Side:           storage.SettingsCheckpointRestoreAfter,
				ActorAccountID: account.ID, ChangedAt: now.Add(2 * time.Minute),
				Selections: selections,
			},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(restored.SyncConfigs).To(BeEmpty())
		Expect(restored.SyncOverrides).To(BeEmpty())
		checkpoint := readInstallationCheckpoint(
			ctx, store, *restored.CheckpointID, target.TargetID,
		)
		Expect(checkpoint.Items).To(HaveLen(5))
		changed := 0
		for _, item := range checkpoint.Items {
			Expect(item.Before).NotTo(BeNil())
			if item.After == nil {
				changed++
			} else {
				Expect(item.After.Digest).To(Equal(item.Before.Digest))
			}
		}
		Expect(changed).To(Equal(2))
		_, err = store.GetSyncConfig(ctx, target.TargetID, orgsync.KindLabels)
		Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())
		_, err = store.GetSyncRepositoryOverride(
			ctx, target.TargetID, "repo-1", orgsync.KindLabels,
		)
		Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())

		recreated, err := store.SaveInstallationSettings(ctx,
			storage.SaveInstallationSettingsRequest{
				TargetID: target.TargetID, ActorAccountID: account.ID,
				ChangedAt: now.Add(3 * time.Minute),
				SyncConfigs: []storage.InstallationSyncConfigChange{{
					Kind: orgsync.KindLabels, Enabled: true, Document: []byte(`{"labels":[]}`),
				}},
			},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(recreated.SyncConfigs).To(ConsistOf(HaveField("Revision", int64(2))))
		Expect(readInstallationCheckpoint(ctx, store, baseline.ID, target.TargetID)).To(Equal(baseline))
		Expect(saved.CheckpointID).NotTo(BeNil())
	})
}

func seedInstallationSettingsRestoreHistory(
	ctx context.Context,
	store storage.Store,
	now time.Time,
) (storage.Account, storage.InstallationSnapshot, int64) {
	GinkgoHelper()
	account, target := seedInstallationSettingsBatch(ctx, store, now)
	enabled := true
	source, err := store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
		TargetID: target.TargetID, ActorAccountID: account.ID, ChangedAt: now.Add(time.Minute),
		Target: &storage.InstallationTargetSettingsChange{
			RepositoryDefaultEnabled: true, ExpectedRevision: 1,
		},
		Repositories: []storage.InstallationRepositorySettingsChange{{
			RepositoryID: "repo-1", EnabledOverride: &enabled, ExpectedRevision: 1,
		}},
		SyncConfigs: []storage.InstallationSyncConfigChange{{
			Kind: orgsync.KindLabels, Enabled: true, Document: []byte(`{"labels":[]}`),
		}},
		SyncOverrides: []storage.InstallationSyncOverrideChange{{
			RepositoryID: "repo-1", Kind: orgsync.KindLabels, Enabled: &enabled,
		}},
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(source.CheckpointID).NotTo(BeNil())
	disabled := false
	_, err = store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
		TargetID: target.TargetID, ActorAccountID: account.ID, ChangedAt: now.Add(2 * time.Minute),
		Target: &storage.InstallationTargetSettingsChange{
			RepositoryDefaultEnabled: false, ExpectedRevision: 2,
		},
		Repositories: []storage.InstallationRepositorySettingsChange{{
			RepositoryID: "repo-1", EnabledOverride: &disabled, ExpectedRevision: 2,
		}},
		SyncConfigs: []storage.InstallationSyncConfigChange{{
			Kind: orgsync.KindLabels, Enabled: false,
			Document:         []byte(`{"labels":[{"name":"bug","color":"d73a4a"}]}`),
			ExpectedRevision: 1,
		}},
		SyncOverrides: []storage.InstallationSyncOverrideChange{{
			RepositoryID: "repo-1", Kind: orgsync.KindLabels,
			Enabled: &disabled, ExpectedRevision: 1,
		}},
	})
	Expect(err).NotTo(HaveOccurred())

	return account, target, *source.CheckpointID
}

func restoreSelections(
	inspection storage.SettingsCheckpointInspection,
) []storage.SettingsCheckpointRestoreSelection {
	GinkgoHelper()
	selections := make([]storage.SettingsCheckpointRestoreSelection, 0, len(inspection.Items))
	for _, item := range inspection.Items {
		if !item.After.Differs || !item.After.Restorable {
			continue
		}
		Expect(item.Current).NotTo(BeNil())
		selections = append(selections, storage.SettingsCheckpointRestoreSelection{
			Identity: item.Identity, ExpectedRevision: item.Current.Revision,
		})
	}

	return selections
}

func restoreSelectionsAllowingAbsence(
	inspection storage.SettingsCheckpointInspection,
) []storage.SettingsCheckpointRestoreSelection {
	selections := make([]storage.SettingsCheckpointRestoreSelection, 0, len(inspection.Items))
	for _, item := range inspection.Items {
		if !item.After.Differs || !item.After.Restorable {
			continue
		}
		revision := int64(0)
		if item.Current != nil {
			revision = item.Current.Revision
		}
		selections = append(selections, storage.SettingsCheckpointRestoreSelection{
			Identity: item.Identity, ExpectedRevision: revision,
		})
	}

	return selections
}

func installationCheckpointRef(targetID string, checkpointID int64) storage.SettingsCheckpointRef {
	return storage.SettingsCheckpointRef{
		ID: checkpointID, Scope: storage.SettingsCheckpointScopeInstallation, TargetID: targetID,
	}
}
