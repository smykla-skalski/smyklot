package storagetest

import (
	"context"
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

func declareInstallationSyncValidationSpecs(
	runtime func() (context.Context, storage.Store, time.Time),
) {
	declareInvalidInstallationSyncSpec(runtime)
	declareStaleInstallationSyncSpec(runtime)
	declareScopedInstallationSyncOverrideSpec(runtime)
	declareInstallationSyncAuditSpec(runtime)
}

func declareInvalidInstallationSyncSpec(
	runtime func() (context.Context, storage.Store, time.Time),
) {
	It("rejects malformed or duplicate Sync resources before writing", func() {
		ctx, store, now := runtime()
		account, target := seedInstallationSettingsBatch(ctx, store, now)
		for index, request := range invalidInstallationSyncRequests(
			target.TargetID, account.ID, now,
		) {
			By(fmt.Sprintf("invalid request %d", index))
			_, err := store.SaveInstallationSettings(ctx, request)
			Expect(err).To(HaveOccurred())
		}
		assertUnchangedInstallationSettings(ctx, store, target.TargetID)
		assertNoInstallationSyncRows(ctx, store, target.TargetID)
		assertInstallationSettingsAudit(ctx, store, target.TargetID, 0, 0)
	})
}

func declareStaleInstallationSyncSpec(
	runtime func() (context.Context, storage.Store, time.Time),
) {
	It("rolls a mixed batch back on a stale Sync revision", func() {
		ctx, store, now := runtime()
		account, target := seedInstallationSettingsBatch(ctx, store, now)
		created, err := store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: target.TargetID, ActorAccountID: account.ID, ChangedAt: now,
			SyncConfigs: []storage.InstallationSyncConfigChange{{
				Kind: orgsync.KindLabels, Enabled: true, Document: []byte(`{"labels":[]}`),
			}},
		})
		Expect(err).NotTo(HaveOccurred())
		seedInstallationSettingsPlan(ctx, store, account, target.TargetID, now.Add(time.Minute))
		enabled := true
		_, err = store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: target.TargetID, ActorAccountID: account.ID, ChangedAt: now.Add(2 * time.Minute),
			Target: &storage.InstallationTargetSettingsChange{
				RepositoryDefaultEnabled: true, ExpectedRevision: 1,
			},
			SyncConfigs: []storage.InstallationSyncConfigChange{{
				Kind: orgsync.KindLabels, Enabled: true,
				Document: []byte(`{"labels":[{"name":"bug"}]}`), ExpectedRevision: 0,
			}},
			SyncOverrides: []storage.InstallationSyncOverrideChange{{
				RepositoryID: "repo-1", Kind: orgsync.KindLabels, Enabled: &enabled,
			}},
		})
		Expect(errors.Is(err, storage.ErrConflict)).To(BeTrue())
		current, err := store.GetTarget(ctx, target.TargetID)
		Expect(err).NotTo(HaveOccurred())
		Expect(current.RepositoryDefaultEnabled).To(BeFalse())
		Expect(current.Revision).To(Equal(int64(1)))
		config, err := store.GetSyncConfig(ctx, target.TargetID, orgsync.KindLabels)
		Expect(err).NotTo(HaveOccurred())
		Expect(config.Revision).To(Equal(int64(1)))
		Expect(config.Document).To(Equal([]byte(`{"labels":[]}`)))
		_, err = store.GetSyncRepositoryOverride(
			ctx, target.TargetID, "repo-1", orgsync.KindLabels,
		)
		Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())
		assertInstallationSettingsAudit(ctx, store, target.TargetID, *created.CheckpointID, 1)
		assertInstallationSettingsPlanState(ctx, store, target.TargetID, orgsync.PlanComputed)
	})
}

func declareScopedInstallationSyncOverrideSpec(
	runtime func() (context.Context, storage.Store, time.Time),
) {
	It("refuses a Sync override outside the installation", func() {
		ctx, store, now := runtime()
		account, target := seedInstallationSettingsBatch(ctx, store, now)
		other := otherInstallationSettingsTarget(account, now)
		Expect(store.ReconcileInstallation(ctx, other)).To(Succeed())
		_, err := store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: target.TargetID, ActorAccountID: account.ID, ChangedAt: now.Add(time.Minute),
			Target: &storage.InstallationTargetSettingsChange{
				RepositoryDefaultEnabled: true, ExpectedRevision: 1,
			},
			SyncOverrides: []storage.InstallationSyncOverrideChange{{
				RepositoryID: "other-repo", Kind: orgsync.KindLabels,
			}},
		})
		Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())
		assertUnchangedInstallationSettings(ctx, store, target.TargetID)
		_, err = store.GetSyncRepositoryOverride(
			ctx, other.TargetID, "other-repo", orgsync.KindLabels,
		)
		Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())
		assertInstallationSettingsAudit(ctx, store, target.TargetID, 0, 0)
	})
}

func declareInstallationSyncAuditSpec(
	runtime func() (context.Context, storage.Store, time.Time),
) {
	It("records one canonical installation audit for a Sync override", func() {
		ctx, store, now := runtime()
		account, target := seedInstallationSettingsBatch(ctx, store, now)
		result, err := store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: target.TargetID, ActorAccountID: account.ID, ChangedAt: now,
			SyncOverrides: []storage.InstallationSyncOverrideChange{{
				RepositoryID: "repo-1", Kind: orgsync.KindFiles,
			}},
		})
		Expect(err).NotTo(HaveOccurred())
		audit, err := store.ListAudit(ctx, target.TargetID, storage.AuditPageRequest{
			HistoryPageRequest: storage.HistoryPageRequest{Limit: 10},
			Change:             storage.AuditChangeSync,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(audit.Items).To(ConsistOf(And(
			HaveField("Action", "installation.settings.saved"),
			HaveField("RepositoryID", BeNil()),
			HaveField("SettingsCheckpointID", HaveValue(Equal(*result.CheckpointID))),
		)))
		assertInstallationSettingsAudit(
			ctx, store, target.TargetID, *result.CheckpointID, 1,
		)
	})
}

func invalidInstallationSyncRequests(
	targetID, actorID string,
	changedAt time.Time,
) []storage.SaveInstallationSettingsRequest {
	request := func() storage.SaveInstallationSettingsRequest {
		return storage.SaveInstallationSettingsRequest{
			TargetID: targetID, ActorAccountID: actorID, ChangedAt: changedAt,
		}
	}
	duplicateConfig := request()
	duplicateConfig.SyncConfigs = []storage.InstallationSyncConfigChange{
		{Kind: orgsync.KindLabels, Document: []byte(`{}`)},
		{Kind: orgsync.KindLabels, Document: []byte(`{}`)},
	}
	duplicateOverride := request()
	duplicateOverride.SyncOverrides = []storage.InstallationSyncOverrideChange{
		{RepositoryID: "repo-1", Kind: orgsync.KindLabels},
		{RepositoryID: "repo-1", Kind: orgsync.KindLabels},
	}
	unknownKind := request()
	unknownKind.SyncConfigs = []storage.InstallationSyncConfigChange{{
		Kind: "unknown", Document: []byte(`{}`),
	}}
	malformedConfig := request()
	malformedConfig.SyncConfigs = []storage.InstallationSyncConfigChange{{
		Kind: orgsync.KindLabels, Document: []byte(`{`),
	}}
	malformedOverride := request()
	malformedOverride.SyncOverrides = []storage.InstallationSyncOverrideChange{{
		RepositoryID: "repo-1", Kind: orgsync.KindLabels, Document: []byte(`{`),
	}}
	missingRepository := request()
	missingRepository.SyncOverrides = []storage.InstallationSyncOverrideChange{{
		Kind: orgsync.KindLabels,
	}}
	negativeRevision := request()
	negativeRevision.SyncConfigs = []storage.InstallationSyncConfigChange{{
		Kind: orgsync.KindLabels, Document: []byte(`{}`), ExpectedRevision: -1,
	}}
	invalidConcreteConfig := request()
	invalidConcreteConfig.SyncConfigs = []storage.InstallationSyncConfigChange{{
		Kind:     orgsync.KindLabels,
		Document: []byte(`{"labels":[{"name":"","color":"ffffff"}]}`),
	}}
	unknownConfigField := request()
	unknownConfigField.SyncConfigs = []storage.InstallationSyncConfigChange{{
		Kind:     orgsync.KindSettings,
		Document: []byte(`{"has_issues":true,"delete_everything":true}`),
	}}
	nonFileOverrideDocument := request()
	nonFileOverrideDocument.SyncOverrides = []storage.InstallationSyncOverrideChange{{
		RepositoryID: "repo-1", Kind: orgsync.KindLabels,
		Document: []byte(`{"excludes":[]}`),
	}}
	invalidFilesOverride := request()
	invalidFilesOverride.SyncOverrides = []storage.InstallationSyncOverrideChange{{
		RepositoryID: "repo-1", Kind: orgsync.KindFiles,
		Document: []byte(`{"merges":[{"path":"missing.json"}]}`),
	}}

	return []storage.SaveInstallationSettingsRequest{
		duplicateConfig, duplicateOverride, unknownKind, malformedConfig,
		malformedOverride, missingRepository, negativeRevision, invalidConcreteConfig,
		unknownConfigField, nonFileOverrideDocument, invalidFilesOverride,
	}
}

func assertNoInstallationSyncRows(
	ctx context.Context,
	store storage.Store,
	targetID string,
) {
	GinkgoHelper()
	configs, err := store.ListSyncConfigs(ctx, targetID)
	Expect(err).NotTo(HaveOccurred())
	Expect(configs).To(BeEmpty())
	overrides, err := store.ListSyncRepositoryOverrides(ctx, targetID)
	Expect(err).NotTo(HaveOccurred())
	Expect(overrides).To(BeEmpty())
}

func otherInstallationSettingsTarget(
	account storage.Account,
	now time.Time,
) storage.InstallationSnapshot {
	account.ID = "github:2"
	account.SubjectID = "2"
	account.Login = "other"
	account.DisplayName = "Other Owner"
	target := testInstallation(account, now, []storage.RepositorySnapshot{
		testRepository("other-repo", "other/repository", false),
	})
	target.TargetID = "github:installation:200"
	target.InstallationID = "200"

	return target
}
