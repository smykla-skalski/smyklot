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

func declareInstallationSettingsRestoreSideSpecs(
	runtime func() (context.Context, storage.Store, time.Time),
) {
	It("addresses the immutable installation baseline without its numeric ID", func() {
		ctx, store, now := runtime()
		account, target := seedInstallationSettingsBatch(ctx, store, now)
		baseline, err := store.InspectInstallationSettingsBaseline(ctx, target.TargetID)
		Expect(err).NotTo(HaveOccurred())
		Expect(baseline.Checkpoint.Action).To(Equal(storage.SettingsCheckpointActionBaseline))
		Expect(baseline.Items).To(HaveLen(3))
		for _, item := range baseline.Items {
			Expect(item.Before.Available).To(BeFalse())
			Expect(item.Before.Restorable).To(BeFalse())
			Expect(item.Before.Incompatibility.Code).To(Equal("state_unavailable"))
			Expect(item.After.Available).To(BeTrue())
			Expect(item.After.Restorable).To(BeTrue())
			Expect(item.Changed).To(BeFalse())
		}

		saved, err := store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: target.TargetID, ActorAccountID: account.ID, ChangedAt: now.Add(time.Minute),
			Target: &storage.InstallationTargetSettingsChange{
				RepositoryDefaultEnabled: true, ExpectedRevision: 1,
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(saved.Target.Revision).To(Equal(int64(2)))
		currentBaseline, err := store.InspectInstallationSettingsBaseline(ctx, target.TargetID)
		Expect(err).NotTo(HaveOccurred())
		Expect(currentBaseline.Checkpoint).To(Equal(baseline.Checkpoint))
		Expect(currentBaseline.Items[0].Current).NotTo(BeNil())

		_, err = store.RestoreInstallationSettings(ctx,
			storage.RestoreInstallationSettingsRequest{
				TargetID: target.TargetID, CheckpointID: baseline.Checkpoint.ID,
				Side:           storage.SettingsCheckpointRestoreBefore,
				ActorAccountID: account.ID, ChangedAt: now.Add(2 * time.Minute),
				Selections: []storage.SettingsCheckpointRestoreSelection{{
					Identity: storage.SettingsCheckpointItemIdentity{
						Kind: storage.SettingsCheckpointItemTarget,
					},
					ExpectedRevision: 2,
				}},
			},
		)
		Expect(errors.Is(err, storage.ErrSettingsRestoreBlocked)).To(BeTrue())
	})

	It("undoes and redoes one complete installation checkpoint", func() {
		ctx, store, now := runtime()
		account, target, sourceID := seedInstallationSettingsRestoreHistory(ctx, store, now)

		afterInspection, err := store.InspectInstallationSettingsCheckpoint(
			ctx, installationCheckpointRef(target.TargetID, sourceID),
		)
		Expect(err).NotTo(HaveOccurred())
		afterRestore, err := store.RestoreInstallationSettings(ctx,
			installationSideRestoreRequest(
				target.TargetID, sourceID, account.ID, now.Add(4*time.Minute),
				storage.SettingsCheckpointRestoreAfter, afterInspection,
			),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(afterRestore.Target.Revision).To(Equal(int64(4)))

		beforeInspection, err := store.InspectInstallationSettingsCheckpoint(
			ctx, installationCheckpointRef(target.TargetID, sourceID),
		)
		Expect(err).NotTo(HaveOccurred())
		beforeRestore, err := store.RestoreInstallationSettings(ctx,
			installationSideRestoreRequest(
				target.TargetID, sourceID, account.ID, now.Add(5*time.Minute),
				storage.SettingsCheckpointRestoreBefore, beforeInspection,
			),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(beforeRestore.Target).To(And(
			HaveField("Revision", int64(5)),
			HaveField("RepositoryDefaultEnabled", false),
		))
		Expect(beforeRestore.Repositories).To(ConsistOf(And(
			HaveField("Revision", int64(5)),
			HaveField("EnabledOverride", BeNil()),
		)))
		Expect(beforeRestore.SyncConfigs).To(BeEmpty())
		Expect(beforeRestore.SyncOverrides).To(BeEmpty())
		assertRestoreCheckpointSide(
			ctx, store, target.TargetID, *beforeRestore.CheckpointID,
			sourceID, storage.SettingsCheckpointRestoreBefore,
		)
		_, err = store.GetSyncConfig(ctx, target.TargetID, orgsync.KindLabels)
		Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())

		redoInspection, err := store.InspectInstallationSettingsCheckpoint(
			ctx, installationCheckpointRef(target.TargetID, sourceID),
		)
		Expect(err).NotTo(HaveOccurred())
		redo, err := store.RestoreInstallationSettings(ctx,
			installationSideRestoreRequest(
				target.TargetID, sourceID, account.ID, now.Add(6*time.Minute),
				storage.SettingsCheckpointRestoreAfter, redoInspection,
			),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(redo.Target.Revision).To(Equal(int64(6)))
		Expect(redo.SyncConfigs).To(ConsistOf(HaveField("Revision", int64(4))))
		Expect(redo.SyncOverrides).To(ConsistOf(HaveField("Revision", int64(4))))
		assertRestoreCheckpointSide(
			ctx, store, target.TargetID, *redo.CheckpointID,
			sourceID, storage.SettingsCheckpointRestoreAfter,
		)
	})
}

func installationSideRestoreRequest(
	targetID string,
	checkpointID int64,
	actorID string,
	changedAt time.Time,
	side storage.SettingsCheckpointRestoreSide,
	inspection storage.SettingsCheckpointInspection,
) storage.RestoreInstallationSettingsRequest {
	return storage.RestoreInstallationSettingsRequest{
		TargetID: targetID, CheckpointID: checkpointID, Side: side,
		ActorAccountID: actorID, ChangedAt: changedAt,
		Selections: installationRestoreSelectionsForSide(side, inspection),
	}
}

func installationRestoreSelectionsForSide(
	side storage.SettingsCheckpointRestoreSide,
	inspection storage.SettingsCheckpointInspection,
) []storage.SettingsCheckpointRestoreSelection {
	selections := make([]storage.SettingsCheckpointRestoreSelection, 0, len(inspection.Items))
	for _, item := range inspection.Items {
		selected := item.After
		if side == storage.SettingsCheckpointRestoreBefore {
			selected = item.Before
		}
		if !selected.Differs || !selected.Restorable {
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

func assertRestoreCheckpointSide(
	ctx context.Context,
	store storage.Store,
	targetID string,
	checkpointID, sourceID int64,
	side storage.SettingsCheckpointRestoreSide,
) {
	GinkgoHelper()
	checkpoint := readInstallationCheckpoint(ctx, store, checkpointID, targetID)
	Expect(checkpoint.Action).To(Equal(storage.SettingsCheckpointActionRestore))
	Expect(checkpoint.RestoredFromID).To(HaveValue(Equal(sourceID)))
	Expect(checkpoint.RestoredSide).To(Equal(side))
	Expect(checkpoint.Items).To(HaveLen(5))
}
