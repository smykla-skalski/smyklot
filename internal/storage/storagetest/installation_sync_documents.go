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

const (
	installationFilesDocument = `{"files":[{"path":"renovate.json","content":"{}"}]}`
	installationFilesOverride = `{"merges":[{"path":"renovate.json",` +
		`"overrides":{"timezone":"Europe/Warsaw"}}]}`
)

func declareInstallationSyncDocumentSpecs(
	runtime func() (context.Context, storage.Store, time.Time),
) {
	declareProposedInstallationFilesConfigSpec(runtime)
	declareExistingInstallationFilesOverrideSpec(runtime)
}

func declareProposedInstallationFilesConfigSpec(
	runtime func() (context.Context, storage.Store, time.Time),
) {
	It("validates files overrides against the config proposed in the same batch", func() {
		ctx, store, now := runtime()
		account, target := seedInstallationSettingsBatch(ctx, store, now)
		result, err := store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
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
		Expect(result.SyncChanged).To(BeTrue())
		Expect(result.SyncConfigs).To(ConsistOf(HaveField("Revision", int64(1))))
		Expect(result.SyncOverrides).To(ConsistOf(HaveField("Revision", int64(1))))

		_, err = store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: target.TargetID, ActorAccountID: account.ID, ChangedAt: now.Add(time.Minute),
			Target: &storage.InstallationTargetSettingsChange{
				RepositoryDefaultEnabled: true, ExpectedRevision: 1,
			},
			SyncOverrides: []storage.InstallationSyncOverrideChange{{
				RepositoryID: "repo-2", Kind: orgsync.KindFiles,
				Document: []byte(`{"merges":[{"path":"package.json"}]}`),
			}},
		})
		Expect(errors.Is(err, orgsync.ErrInvalidConfig)).To(BeTrue())
		current, err := store.GetTarget(ctx, target.TargetID)
		Expect(err).NotTo(HaveOccurred())
		Expect(current.RepositoryDefaultEnabled).To(BeFalse())
		Expect(current.Revision).To(Equal(int64(1)))
		_, err = store.GetSyncRepositoryOverride(
			ctx, target.TargetID, "repo-2", orgsync.KindFiles,
		)
		Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())
		assertInstallationSettingsAudit(
			ctx, store, target.TargetID, *result.CheckpointID, 1,
		)
	})
}

func declareExistingInstallationFilesOverrideSpec(
	runtime func() (context.Context, storage.Store, time.Time),
) {
	It("keeps an existing files adjustment valid when the template moves", func() {
		ctx, store, now := runtime()
		account, target := seedInstallationSettingsBatch(ctx, store, now)
		setup, err := store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
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
		Expect(setup.SyncConfigs).To(HaveLen(1))
		Expect(setup.SyncOverrides).To(HaveLen(1))
		_, err = store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: target.TargetID, ActorAccountID: account.ID,
			ChangedAt: now.Add(time.Minute),
			SyncConfigs: []storage.InstallationSyncConfigChange{{
				Kind: orgsync.KindFiles, Enabled: true, Document: []byte(`{"files":[]}`),
				ExpectedRevision: setup.SyncConfigs[0].Revision,
			}},
		})
		Expect(err).NotTo(HaveOccurred())

		disabled := false
		result, err := store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: target.TargetID, ActorAccountID: account.ID, ChangedAt: now.Add(2 * time.Minute),
			SyncOverrides: []storage.InstallationSyncOverrideChange{{
				RepositoryID: "repo-1", Kind: orgsync.KindFiles, Enabled: &disabled,
				Document:         []byte(installationFilesOverride),
				ExpectedRevision: setup.SyncOverrides[0].Revision,
			}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.SyncOverrides).To(ConsistOf(And(
			HaveField("Revision", int64(2)),
			HaveField("Enabled", HaveValue(BeFalse())),
		)))
	})
}
