package storagetest

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

func declareBypassPolicySpecs(runtime func() (context.Context, storage.Store, time.Time)) {
	It("checkpoints, restores and inherits bypass exceptions atomically", func() {
		ctx, store, now := runtime()
		account, target := seedInstallationSettingsBatch(ctx, store, now)
		allow := &storage.PendingCIBypassPolicy{Allow: true, Actors: []orgsync.RulesetBypassActor{
			{ActorID: 1197525, ActorType: "Integration", Mode: "always"},
		}}
		deny := &storage.PendingCIBypassPolicy{Allow: false, Actors: allow.Actors}
		result, err := store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: target.TargetID, ActorAccountID: account.ID, ChangedAt: now.Add(time.Minute),
			Target:       &storage.InstallationTargetSettingsChange{PendingCIBypassPolicyDefault: allow, ExpectedRevision: 1},
			Repositories: []storage.InstallationRepositorySettingsChange{{RepositoryID: "repo-1", PendingCIBypassPolicyOverride: deny, ExpectedRevision: 1}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.CatalogSettingsChanged).To(BeTrue())
		Expect(result.CheckpointID).NotTo(BeNil())
		workspace, err := store.GetTarget(ctx, target.TargetID)
		Expect(err).NotTo(HaveOccurred())
		repository, err := store.GetRepository(ctx, target.TargetID, "repo-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(workspace.PendingCIBypassPolicyDefault).To(Equal(allow))
		Expect(repository.PendingCIBypassPolicyOverride).To(Equal(deny))
		Expect(storage.EffectivePendingCIBypassPolicy(workspace, repository)).To(Equal(deny))
		inherited, err := store.GetRepository(ctx, target.TargetID, "repo-2")
		Expect(err).NotTo(HaveOccurred())
		Expect(storage.EffectivePendingCIBypassPolicy(workspace, inherited)).To(Equal(allow))

		inspection, err := store.InspectInstallationSettingsCheckpoint(ctx, installationCheckpointRef(target.TargetID, *result.CheckpointID))
		Expect(err).NotTo(HaveOccurred())
		_, err = store.RestoreInstallationSettings(ctx, storage.RestoreInstallationSettingsRequest{
			TargetID: target.TargetID, CheckpointID: *result.CheckpointID, Side: storage.SettingsCheckpointRestoreBefore,
			ActorAccountID: account.ID, ChangedAt: now.Add(2 * time.Minute), Selections: bypassRestoreSelections(inspection),
		})
		Expect(err).NotTo(HaveOccurred())
		workspace, err = store.GetTarget(ctx, target.TargetID)
		Expect(err).NotTo(HaveOccurred())
		Expect(workspace.PendingCIBypassPolicyDefault).To(BeNil())
		repository, err = store.GetRepository(ctx, target.TargetID, "repo-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.PendingCIBypassPolicyOverride).To(BeNil())
		inspection, err = store.InspectInstallationSettingsCheckpoint(ctx, installationCheckpointRef(target.TargetID, *result.CheckpointID))
		Expect(err).NotTo(HaveOccurred())
		_, err = store.RestoreInstallationSettings(ctx, storage.RestoreInstallationSettingsRequest{
			TargetID: target.TargetID, CheckpointID: *result.CheckpointID, Side: storage.SettingsCheckpointRestoreAfter,
			ActorAccountID: account.ID, ChangedAt: now.Add(3 * time.Minute), Selections: bypassRestoreSelections(inspection),
		})
		Expect(err).NotTo(HaveOccurred())
		workspace, err = store.GetTarget(ctx, target.TargetID)
		Expect(err).NotTo(HaveOccurred())
		Expect(workspace.PendingCIBypassPolicyDefault).To(Equal(allow))
		repository, err = store.GetRepository(ctx, target.TargetID, "repo-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.PendingCIBypassPolicyOverride).To(Equal(deny))
	})
	It("rejects invalid and duplicate bypass actors before changing settings", func() {
		ctx, store, now := runtime()
		account, target := seedInstallationSettingsBatch(ctx, store, now)
		actor := orgsync.RulesetBypassActor{ActorID: 17, ActorType: "Integration", Mode: "always"}
		for _, actors := range [][]orgsync.RulesetBypassActor{
			{actor, actor},
			{{ActorID: 17, ActorType: "Integration", Mode: "typo"}},
			{{ActorID: 0, ActorType: "Integration", Mode: "always"}},
			{{ActorType: "DeployKey", Mode: "pull_request"}},
		} {
			_, err := store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
				TargetID: target.TargetID, ActorAccountID: account.ID, ChangedAt: now.Add(time.Minute),
				Target: &storage.InstallationTargetSettingsChange{PendingCIBypassPolicyDefault: &storage.PendingCIBypassPolicy{Allow: true, Actors: actors}, ExpectedRevision: 1},
			})
			Expect(err).To(HaveOccurred())
		}
		workspace, err := store.GetTarget(ctx, target.TargetID)
		Expect(err).NotTo(HaveOccurred())
		Expect(workspace.Revision).To(Equal(int64(1)))
		Expect(workspace.PendingCIBypassPolicyDefault).To(BeNil())
	})
}

func bypassRestoreSelections(inspection storage.SettingsCheckpointInspection) []storage.SettingsCheckpointRestoreSelection {
	var selections []storage.SettingsCheckpointRestoreSelection
	for _, item := range inspection.Items {
		Expect(item.Current).NotTo(BeNil())
		selections = append(selections, storage.SettingsCheckpointRestoreSelection{Identity: item.Identity, ExpectedRevision: item.Current.Revision})
	}
	return selections
}
