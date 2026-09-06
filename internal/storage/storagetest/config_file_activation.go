package storagetest

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

func declareConfigFileActivationSpecs(runtime func() (context.Context, storage.Store, time.Time)) {
	for _, restore := range []bool{false, true} {
		It("configuration file connections require fresh initialization after saving or restoring opt-in", func() {
			ctx, store, now := runtime()
			account, installation := seedInstallationSettingsBatch(ctx, store, now)
			first := enableConfigFileConnections(ctx, store, account, installation.TargetID, now)
			for _, repositoryID := range []string{"", "repo-1"} {
				state, err := store.GetConfigFileState(ctx, installation.TargetID, repositoryID)
				Expect(err).NotTo(HaveOccurred())
				Expect(state.InitializationRequired).To(BeTrue())
				change := configFileStateChange(installation.TargetID, repositoryID, now)
				change.Initialized = true
				saved, err := store.SaveConfigFileState(ctx, change)
				Expect(err).NotTo(HaveOccurred())
				Expect(saved.InitializationRequired).To(BeFalse())
			}
			for index, enabled := range []bool{false, true} {
				if restore {
					inspection, err := store.InspectInstallationSettingsCheckpoint(ctx,
						installationCheckpointRef(installation.TargetID, *first.CheckpointID))
					Expect(err).NotTo(HaveOccurred())
					side := storage.SettingsCheckpointRestoreBefore
					if enabled {
						side = storage.SettingsCheckpointRestoreAfter
					}
					_, err = store.RestoreInstallationSettings(ctx, installationSideRestoreRequest(
						installation.TargetID, *first.CheckpointID, account.ID, now.Add(time.Minute), side, inspection))
					Expect(err).NotTo(HaveOccurred())
				} else {
					_, err := store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
						TargetID: installation.TargetID, ActorAccountID: account.ID, ChangedAt: now.Add(time.Minute),
						Target: &storage.InstallationTargetSettingsChange{
							ConfigFileSyncEnabled: enabled, ExpectedRevision: int64(index + 2),
						},
						Repositories: []storage.InstallationRepositorySettingsChange{{
							RepositoryID: "repo-1", ConfigFileSyncEnabled: enabled, ExpectedRevision: int64(index + 2),
						}},
					})
					Expect(err).NotTo(HaveOccurred())
				}
			}
			for _, repositoryID := range []string{"", "repo-1"} {
				state, err := store.GetConfigFileState(ctx, installation.TargetID, repositoryID)
				Expect(err).NotTo(HaveOccurred())
				Expect(state.InitializationRequired).To(BeTrue())
				Expect(state.Revision).To(Equal(int64(2)))
				Expect(string(state.Document)).To(Equal(`{"state":"checked"}`), "retain baseline and proposal history")
				change := configFileStateChange(installation.TargetID, repositoryID, now)
				change.ExpectedRevision, change.OwnerRevision, change.Initialized = 1, 4, true
				_, err = store.SaveConfigFileState(ctx, change)
				Expect(err).To(MatchError(storage.ErrConflict), "pre-activation work cannot acknowledge readiness")
				change.ExpectedRevision, change.Initialized = state.Revision, false
				blocked, err := store.SaveConfigFileState(ctx, change)
				Expect(err).NotTo(HaveOccurred())
				Expect(blocked.InitializationRequired).To(BeTrue(), "a failed check must leave initialization pending")
				change.ExpectedRevision, change.Initialized = blocked.Revision, true
				ready, err := store.SaveConfigFileState(ctx, change)
				Expect(err).NotTo(HaveOccurred())
				Expect(ready.InitializationRequired).To(BeFalse())
				change.ExpectedRevision, change.Initialized = ready.Revision, false
				later, err := store.SaveConfigFileState(ctx, change)
				Expect(err).NotTo(HaveOccurred())
				Expect(later.InitializationRequired).To(BeFalse(), "subsequent proposals keep the established activation")
			}
		})
	}
}
