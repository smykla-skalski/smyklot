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
	declareUnreadableInstallationSyncOverrideSpec(runtime)
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
		configured, err := store.SetSyncConfig(ctx, orgsync.ConfigChange{
			TargetID: target.TargetID, Kind: orgsync.KindFiles, Enabled: true,
			Document: []byte(installationFilesDocument), ActorID: account.ID, Now: now,
		})
		Expect(err).NotTo(HaveOccurred())
		override, err := store.SetSyncRepositoryOverride(
			ctx, orgsync.RepositoryOverrideChange{
				RepositoryID: "repo-1", Kind: orgsync.KindFiles,
				Document: []byte(installationFilesOverride), ActorID: account.ID, Now: now,
			},
		)
		Expect(err).NotTo(HaveOccurred())
		_, err = store.SetSyncConfig(ctx, orgsync.ConfigChange{
			TargetID: target.TargetID, Kind: orgsync.KindFiles, Enabled: true,
			Document: []byte(`{"files":[]}`), Revision: configured.Revision,
			ActorID: account.ID, Now: now.Add(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())

		disabled := false
		result, err := store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: target.TargetID, ActorAccountID: account.ID, ChangedAt: now.Add(2 * time.Minute),
			SyncOverrides: []storage.InstallationSyncOverrideChange{{
				RepositoryID: "repo-1", Kind: orgsync.KindFiles, Enabled: &disabled,
				Document: []byte(installationFilesOverride), ExpectedRevision: override.Revision,
			}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.SyncOverrides).To(ConsistOf(And(
			HaveField("Revision", int64(2)),
			HaveField("Enabled", HaveValue(BeFalse())),
		)))
	})
}

func declareUnreadableInstallationSyncOverrideSpec(
	runtime func() (context.Context, storage.Store, time.Time),
) {
	DescribeTable("refuses to replace an unreadable existing Sync override",
		func(kind orgsync.Kind, storedDocument []byte) {
			ctx, store, now := runtime()
			account, target := seedInstallationSettingsBatch(ctx, store, now)
			stored, err := store.SetSyncRepositoryOverride(
				ctx, orgsync.RepositoryOverrideChange{
					RepositoryID: "repo-1", Kind: kind, Document: storedDocument,
					ActorID: account.ID, Now: now,
				},
			)
			Expect(err).NotTo(HaveOccurred())
			seedInstallationSettingsPlan(ctx, store, account, target.TargetID, now.Add(time.Minute))

			enabled := true
			_, err = store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
				TargetID: target.TargetID, ActorAccountID: account.ID,
				ChangedAt: now.Add(2 * time.Minute),
				Target: &storage.InstallationTargetSettingsChange{
					RepositoryDefaultEnabled: true, ExpectedRevision: 1,
				},
				Repositories: []storage.InstallationRepositorySettingsChange{{
					RepositoryID: "repo-2", IgnoreRepositoryFile: true, ExpectedRevision: 1,
				}},
				SyncConfigs: []storage.InstallationSyncConfigChange{{
					Kind: orgsync.KindLabels, Enabled: true, Document: []byte(`{"labels":[]}`),
				}},
				SyncOverrides: []storage.InstallationSyncOverrideChange{{
					RepositoryID: "repo-1", Kind: kind, Enabled: &enabled,
					ExpectedRevision: stored.Revision,
				}},
			})
			Expect(errors.Is(err, orgsync.ErrInvalidConfig)).To(BeTrue())
			assertUnreadableInstallationSyncOverrideRollback(
				ctx, store, target.TargetID, kind, stored,
			)
		},
		Entry("when a non-files document is not empty", orgsync.KindLabels,
			[]byte(`{"unknown":true}`)),
		Entry("when a files document cannot decode", orgsync.KindFiles,
			[]byte(`{"merges":"not-a-list"}`)),
		Entry("when a files document fails intrinsic validation", orgsync.KindFiles,
			[]byte(`{"merges":[{"path":"../outside.json"}]}`)),
	)
}

func assertUnreadableInstallationSyncOverrideRollback(
	ctx context.Context,
	store storage.Store,
	targetID string,
	kind orgsync.Kind,
	stored orgsync.RepositoryOverride,
) {
	GinkgoHelper()
	assertUnchangedInstallationSettings(ctx, store, targetID)
	current, err := store.GetSyncRepositoryOverride(ctx, targetID, "repo-1", kind)
	Expect(err).NotTo(HaveOccurred())
	Expect(current).To(Equal(stored))
	_, err = store.GetSyncConfig(ctx, targetID, orgsync.KindLabels)
	Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())
	_, err = store.GetSettingsCheckpoint(ctx, storage.SettingsCheckpointRef{
		ID: 1, Scope: storage.SettingsCheckpointScopeInstallation, TargetID: targetID,
	})
	Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())
	assertInstallationSettingsAudit(ctx, store, targetID, 0, 0)
	assertInstallationSettingsPlanState(ctx, store, targetID, orgsync.PlanComputed)
}
