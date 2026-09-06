package storagetest

import (
	"context"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func declareConfigFileSyncSpecs(runtime func() (context.Context, storage.Store, time.Time)) {
	It("checkpoints configuration file connections and rejects stale or disabled background writes", func() {
		ctx, store, now := runtime()
		account, installation := seedInstallationSettingsBatch(ctx, store, now)
		result := enableConfigFileConnections(ctx, store, account, installation.TargetID, now)
		for _, repositoryID := range []string{"", "repo-1"} {
			state, err := store.GetConfigFileState(ctx, installation.TargetID, repositoryID)
			Expect(err).NotTo(HaveOccurred())
			Expect(state.Revision).To(BeZero())
			change := configFileStateChange(installation.TargetID, repositoryID, now)
			saved, err := store.SaveConfigFileState(ctx, change)
			Expect(err).NotTo(HaveOccurred())
			Expect(saved.Revision).To(Equal(int64(1)))
			read, err := store.GetConfigFileState(ctx, installation.TargetID, repositoryID)
			Expect(err).NotTo(HaveOccurred())
			Expect(read).To(Equal(saved))
			_, err = store.SaveConfigFileState(ctx, change)
			Expect(err).To(MatchError(storage.ErrConflict))
			change.ExpectedRevision = 1
			change.OwnerRevision = 1
			_, err = store.SaveConfigFileState(ctx, change)
			Expect(err).To(MatchError(storage.ErrConflict))
		}
		inspection, err := store.InspectInstallationSettingsCheckpoint(ctx, installationCheckpointRef(installation.TargetID, *result.CheckpointID))
		Expect(err).NotTo(HaveOccurred())
		_, err = store.RestoreInstallationSettings(ctx, storage.RestoreInstallationSettingsRequest{
			TargetID: installation.TargetID, CheckpointID: *result.CheckpointID,
			Side: storage.SettingsCheckpointRestoreBefore, ActorAccountID: account.ID,
			ChangedAt: now.Add(time.Minute), Selections: bypassRestoreSelections(inspection),
		})
		Expect(err).NotTo(HaveOccurred())
		for _, repositoryID := range []string{"", "repo-1"} {
			change := configFileStateChange(installation.TargetID, repositoryID, now)
			change.ExpectedRevision, change.OwnerRevision = 1, 3
			_, err := store.SaveConfigFileState(ctx, change)
			Expect(err).To(MatchError(storage.ErrConflict), "restoring the disabled opt-in must stop writes")
		}
	})
	It("configuration file connections also compare sync revisions and exact repository identity", func() {
		ctx, store, now := runtime()
		account, installation := seedInstallationSettingsBatch(ctx, store, now)
		enableConfigFileConnections(ctx, store, account, installation.TargetID, now)
		for _, repositoryID := range []string{"", "repo-1"} {
			change := configFileStateChange(installation.TargetID, repositoryID, now)
			change.SyncRevisions[orgsync.KindFiles] = 1
			_, err := store.SaveConfigFileState(ctx, change)
			Expect(err).To(MatchError(storage.ErrConflict))
		}
		for _, repositoryID := range []string{"missing", "smyklot", "repo-2"} {
			change := configFileStateChange(installation.TargetID, repositoryID, now)
			_, err := store.SaveConfigFileState(ctx, change)
			Expect(err).To(HaveOccurred(), "an absent, named, or disabled repository cannot receive a connection")
		}
	})
	It("configuration file imports commit settings, checkpoint provenance and comparison state together", func() {
		ctx, store, now := runtime()
		account, installation := seedInstallationSettingsBatch(ctx, store, now)
		enableConfigFileConnections(ctx, store, account, installation.TargetID, now)
		request := repositoryConfigFileImport(installation.TargetID, now)
		result, err := store.SaveInstallationSettings(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.CheckpointID).NotTo(BeNil())
		Expect(result.Repositories[0].Revision).To(Equal(int64(3)))
		Expect(result.Repositories[0].ConfigFileSyncEnabled).To(BeTrue())
		Expect(result.Repositories[0].ConfigPatch.CommandPrefix).To(Equal(new("/file ")))
		state, err := store.GetConfigFileState(ctx, installation.TargetID, "repo-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(state.Revision).To(Equal(int64(1)))
		checkpoint := readInstallationCheckpoint(ctx, store, *result.CheckpointID, installation.TargetID)
		Expect(checkpoint.ActorAccountID).To(Equal("smyklot:system"))
		audit, err := store.ListAudit(ctx, installation.TargetID, storage.AuditPageRequest{Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(audit.Items).To(ContainElement(And(
			HaveField("Action", "configuration_file.imported"),
			HaveField("Summary", ContainSubstring(".smyklot.toml at "+strings.Repeat("a", 40))),
		)))
		request.Repositories[0].ExpectedRevision = 3
		request.ConfigFileImport.State.OwnerRevision = 3
		request.ConfigFileImport.State.ExpectedRevision = 1
		request.ConfigFileImport.State.Document = []byte(`{"state":"settled"}`)
		repeated, err := store.SaveInstallationSettings(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(repeated.CheckpointID).To(BeNil())
		Expect(repeated.Repositories[0].Revision).To(Equal(int64(3)))
		state, err = store.GetConfigFileState(ctx, installation.TargetID, "repo-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(state.Revision).To(Equal(int64(2)), "a no-op import still commits its comparison state")
	})
	It("configuration file imports reject invalid settings and stale decisions without partial writes", func() {
		ctx, store, now := runtime()
		account, installation := seedInstallationSettingsBatch(ctx, store, now)
		enableConfigFileConnections(ctx, store, account, installation.TargetID, now)
		for _, mutate := range []func(*storage.SaveInstallationSettingsRequest){
			func(r *storage.SaveInstallationSettingsRequest) {
				r.Repositories[0].ConfigPatch.Runner = new(config.RunnerAction)
			},
			func(r *storage.SaveInstallationSettingsRequest) {
				r.Repositories[0].PathIndexIntervalOverride = new(8 * 24 * time.Hour)
			},
			func(r *storage.SaveInstallationSettingsRequest) { r.Repositories[0].IgnoreRepositoryFile = true },
			func(r *storage.SaveInstallationSettingsRequest) { r.ConfigFileImport.State.OwnerRevision = 1 },
			func(r *storage.SaveInstallationSettingsRequest) { r.ConfigFileImport.Path = "../config.toml" },
			func(r *storage.SaveInstallationSettingsRequest) { r.SyncOverrides[0].RepositoryID = "repo-2" },
		} {
			request := repositoryConfigFileImport(installation.TargetID, now)
			mutate(&request)
			_, err := store.SaveInstallationSettings(ctx, request)
			Expect(err).To(HaveOccurred())
			repository, err := store.GetRepository(ctx, installation.TargetID, "repo-1")
			Expect(err).NotTo(HaveOccurred())
			Expect(repository.Revision).To(Equal(int64(2)))
			Expect(repository.ConfigPatch.CommandPrefix).To(BeNil())
			state, err := store.GetConfigFileState(ctx, installation.TargetID, "repo-1")
			Expect(err).NotTo(HaveOccurred())
			Expect(state.Revision).To(BeZero())
		}
	})
}

func enableConfigFileConnections(ctx context.Context, store storage.Store, account storage.Account, targetID string, now time.Time) storage.SaveInstallationSettingsResult {
	GinkgoHelper()
	result, err := store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
		TargetID: targetID, ActorAccountID: account.ID, ChangedAt: now,
		Target: &storage.InstallationTargetSettingsChange{ConfigFileSyncEnabled: true, ExpectedRevision: 1},
		Repositories: []storage.InstallationRepositorySettingsChange{{
			RepositoryID: "repo-1", ConfigFileSyncEnabled: true, ExpectedRevision: 1,
		}},
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(result.CheckpointID).NotTo(BeNil())
	Expect(result.Target.ConfigFileSyncEnabled).To(BeTrue())
	Expect(result.Repositories[0].ConfigFileSyncEnabled).To(BeTrue())
	return result
}

func configFileStateChange(targetID, repositoryID string, now time.Time) storage.ConfigFileStateChange {
	revisions := make(map[orgsync.Kind]int64)
	for _, kind := range orgsync.Kinds() {
		revisions[kind] = 0
	}
	return storage.ConfigFileStateChange{
		TargetID: targetID, RepositoryID: repositoryID, OwnerRevision: 2,
		SyncRevisions: revisions, Document: []byte(`{"state":"checked"}`), ChangedAt: now,
	}
}

func repositoryConfigFileImport(targetID string, now time.Time) storage.SaveInstallationSettingsRequest {
	request := storage.SaveInstallationSettingsRequest{
		TargetID: targetID, ChangedAt: now,
		Repositories: []storage.InstallationRepositorySettingsChange{{
			RepositoryID: "repo-1", ConfigFileSyncEnabled: true, ExpectedRevision: 2,
			ConfigPatch: config.Patch{CommandPrefix: new("/file ")},
		}},
		ConfigFileImport: &storage.ConfigFileImport{
			State: configFileStateChange(targetID, "repo-1", now),
			Path:  ".smyklot.toml", HeadSHA: strings.Repeat("a", 40),
		},
	}
	for _, kind := range orgsync.Kinds() {
		request.SyncOverrides = append(request.SyncOverrides, storage.InstallationSyncOverrideChange{
			RepositoryID: "repo-1", Kind: kind, Remove: true,
		})
	}
	return request
}
