package storagetest

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func declareInstallationSettingsSpecs(
	harness Harness,
	runtime func() (context.Context, storage.Store, time.Time),
) {
	declareAtomicInstallationSettingsSpec(runtime)
	declareInstallationSettingsRollbackSpec(runtime)
	declareInstallationSettingsFailureSpec(harness, runtime)
	declareInstallationSettingsNoopSpec(runtime)
	declareInstallationSyncSettingsSpecs(runtime)
	declareInstallationSyncValidationSpecs(runtime)
	declareInstallationSyncDocumentSpecs(runtime)
	declareInstallationSettingsRestoreSpecs(harness, runtime)
}

func declareAtomicInstallationSettingsSpec(
	runtime func() (context.Context, storage.Store, time.Time),
) {
	It("saves target and repository settings as one immutable checkpoint", func() {
		ctx, store, now := runtime()
		account, target := seedInstallationSettingsBatch(ctx, store, now)
		seedInstallationSettingsPlan(ctx, store, account, target.TargetID, now)
		beforeTarget, err := store.GetTarget(ctx, target.TargetID)
		Expect(err).NotTo(HaveOccurred())
		beforeOne, err := store.GetRepository(ctx, target.TargetID, "repo-1")
		Expect(err).NotTo(HaveOccurred())

		quietSuccess := true
		enabled := true
		prefix := "/batch"
		result, err := store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: target.TargetID, ActorAccountID: account.ID, ChangedAt: now.Add(time.Minute),
			Target: &storage.InstallationTargetSettingsChange{
				RepositoryDefaultEnabled: true, ConfigPatch: config.Patch{QuietSuccess: &quietSuccess},
				ExpectedRevision: 1,
			},
			Repositories: []storage.InstallationRepositorySettingsChange{
				{RepositoryID: "repo-2", ConfigPatch: config.Patch{CommandPrefix: &prefix}, ExpectedRevision: 1},
				{RepositoryID: "repo-1", EnabledOverride: &enabled, ExpectedRevision: 1},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.CheckpointID).NotTo(BeNil())
		Expect(result.Target).To(HaveField("Revision", int64(2)))
		Expect(repositoryIDs(result.Repositories)).To(Equal([]string{"repo-1", "repo-2"}))
		for _, repository := range result.Repositories {
			Expect(repository.Revision).To(Equal(int64(2)))
		}

		checkpoint := readInstallationCheckpoint(ctx, store, *result.CheckpointID, target.TargetID)
		Expect(checkpoint.Items).To(HaveLen(3))
		Expect(checkpointItemKinds(checkpoint.Items)).To(Equal([]storage.SettingsCheckpointItemKind{
			storage.SettingsCheckpointItemRepository,
			storage.SettingsCheckpointItemRepository,
			storage.SettingsCheckpointItemTarget,
		}))
		Expect(checkpoint.Items[0].RepositoryID).To(Equal("repo-1"))
		Expect(checkpoint.Items[1].RepositoryID).To(Equal("repo-2"))
		assertSettingsCheckpointState(checkpoint.Items[0], beforeOne.Revision, int64(2))
		assertSettingsCheckpointState(checkpoint.Items[2], beforeTarget.Revision, int64(2))
		beforeRepositoryDocument := installationRepositoryDocument(beforeOne)
		afterRepositoryDocument := beforeRepositoryDocument
		afterRepositoryDocument.EnabledOverride = &enabled
		Expect(decodeSettingsDocument[storage.RepositorySettingsDocument](
			checkpoint.Items[0].Before.Document,
		)).To(Equal(beforeRepositoryDocument))
		Expect(decodeSettingsDocument[storage.RepositorySettingsDocument](
			checkpoint.Items[0].After.Document,
		)).To(Equal(afterRepositoryDocument))
		beforeTargetDocument := installationTargetDocument(beforeTarget)
		afterTargetDocument := beforeTargetDocument
		afterTargetDocument.RepositoryDefaultEnabled = true
		afterTargetDocument.ConfigPatch = config.Patch{QuietSuccess: &quietSuccess}
		Expect(decodeSettingsDocument[storage.TargetSettingsDocument](
			checkpoint.Items[2].Before.Document,
		)).To(Equal(beforeTargetDocument))
		Expect(decodeSettingsDocument[storage.TargetSettingsDocument](
			checkpoint.Items[2].After.Document,
		)).To(Equal(afterTargetDocument))
		assertInstallationSettingsAudit(ctx, store, target.TargetID, *result.CheckpointID, 1)
		assertInstallationSettingsPlanState(ctx, store, target.TargetID, orgsync.PlanStale)

		original := checkpoint
		_, err = store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: target.TargetID, ActorAccountID: account.ID, ChangedAt: now.Add(2 * time.Minute),
			Target: &storage.InstallationTargetSettingsChange{
				RepositoryDefaultEnabled: false, ExpectedRevision: 2,
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(readInstallationCheckpoint(
			ctx, store, *result.CheckpointID, target.TargetID,
		)).To(Equal(original))
	})
}

func declareInstallationSettingsFailureSpec(
	harness Harness,
	runtime func() (context.Context, storage.Store, time.Time),
) {
	It("rolls back every batch effect when its elevated notification fails", func() {
		ctx, store, now := runtime()
		root, owner, target, session := seedElevationScenario(ctx, store, now)
		target.Repositories = append(target.Repositories,
			testRepository("repo-2", "smykla-skalski/platform", true),
		)
		Expect(store.ReconcileInstallation(ctx, target)).To(Succeed())
		elevation, err := store.BeginElevation(ctx, storage.ElevationGrant{
			ID: "installation-settings-rollback", SessionTokenHash: session.TokenHash,
			RootAccountID: root.ID, TargetID: target.TargetID, StartedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		seedInstallationSettingsPlan(ctx, store, root, target.TargetID, now)
		harness.RejectSecurityNotifications(ctx)

		enabled := true
		_, err = store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: target.TargetID, ActorAccountID: root.ID,
			ElevationID: &elevation.ID, SessionTokenHash: session.TokenHash,
			ChangedAt: now.Add(time.Minute),
			Target: &storage.InstallationTargetSettingsChange{
				RepositoryDefaultEnabled: true, ExpectedRevision: 1,
			},
			Repositories: []storage.InstallationRepositorySettingsChange{
				{RepositoryID: "repo-1", EnabledOverride: &enabled, ExpectedRevision: 1},
				{RepositoryID: "repo-2", IgnoreRepositoryFile: true, ExpectedRevision: 1},
			},
			SyncConfigs: []storage.InstallationSyncConfigChange{{
				Kind: orgsync.KindLabels, Enabled: true, Document: []byte(`{"labels":[]}`),
			}},
			SyncOverrides: []storage.InstallationSyncOverrideChange{{
				RepositoryID: "repo-1", Kind: orgsync.KindLabels, Enabled: &enabled,
			}},
		})
		Expect(err).To(HaveOccurred())
		assertUnchangedInstallationSettings(ctx, store, target.TargetID)
		assertInstallationSettingsAudit(ctx, store, target.TargetID, 0, 0)
		assertInstallationSettingsPlanState(ctx, store, target.TargetID, orgsync.PlanComputed)
		_, err = store.GetSyncConfig(ctx, target.TargetID, orgsync.KindLabels)
		Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())
		_, err = store.GetSyncRepositoryOverride(
			ctx, target.TargetID, "repo-1", orgsync.KindLabels,
		)
		Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())
		notifications, err := store.ListSecurityNotifications(
			ctx, owner.ID, storage.NotificationPageRequest{Limit: 10},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(notifications.Total).To(BeZero())
	})
}

func declareInstallationSettingsRollbackSpec(
	runtime func() (context.Context, storage.Store, time.Time),
) {
	It("rolls back the whole settings batch on validation or revision conflict", func() {
		ctx, store, now := runtime()
		account, target := seedInstallationSettingsBatch(ctx, store, now)
		seedInstallationSettingsPlan(ctx, store, account, target.TargetID, now)
		enabled := true
		request := storage.SaveInstallationSettingsRequest{
			TargetID: target.TargetID, ActorAccountID: account.ID, ChangedAt: now.Add(time.Minute),
			Target: &storage.InstallationTargetSettingsChange{
				RepositoryDefaultEnabled: true, ExpectedRevision: 1,
			},
			Repositories: []storage.InstallationRepositorySettingsChange{
				{RepositoryID: "repo-1", EnabledOverride: &enabled, ExpectedRevision: 1},
				{RepositoryID: "repo-2", IgnoreRepositoryFile: true, ExpectedRevision: 0},
			},
		}
		_, err := store.SaveInstallationSettings(ctx, request)
		Expect(errors.Is(err, storage.ErrConflict)).To(BeTrue())
		assertUnchangedInstallationSettings(ctx, store, target.TargetID)
		assertInstallationSettingsAudit(ctx, store, target.TargetID, 0, 0)
		assertInstallationSettingsPlanState(ctx, store, target.TargetID, orgsync.PlanComputed)

		badMode := storage.PendingCIMode("not-a-mode")
		request.Repositories[1].ExpectedRevision = 1
		request.Repositories[1].PendingCIModeOverride = &badMode
		_, err = store.SaveInstallationSettings(ctx, request)
		Expect(err).To(HaveOccurred())
		assertUnchangedInstallationSettings(ctx, store, target.TargetID)
	})
}

func declareInstallationSettingsNoopSpec(
	runtime func() (context.Context, storage.Store, time.Time),
) {
	It("suppresses no-ops and keeps live plans for non-inclusion settings", func() {
		ctx, store, now := runtime()
		account, target := seedInstallationSettingsBatch(ctx, store, now)
		seedInstallationSettingsPlan(ctx, store, account, target.TargetID, now)
		current, err := store.GetTarget(ctx, target.TargetID)
		Expect(err).NotTo(HaveOccurred())
		currentRepository, err := store.GetRepository(ctx, target.TargetID, "repo-1")
		Expect(err).NotTo(HaveOccurred())

		request := storage.SaveInstallationSettingsRequest{
			TargetID: target.TargetID, ActorAccountID: account.ID, ChangedAt: now.Add(time.Minute),
			Target:       installationTargetChange(current),
			Repositories: []storage.InstallationRepositorySettingsChange{installationRepositoryChange(currentRepository)},
		}
		result, err := store.SaveInstallationSettings(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.CheckpointID).To(BeNil())
		Expect(result.Target).To(HaveField("Revision", int64(1)))
		Expect(result.Repositories).To(ConsistOf(HaveField("Revision", int64(1))))
		assertInstallationSettingsAudit(ctx, store, target.TargetID, 0, 0)
		assertInstallationSettingsPlanState(ctx, store, target.TargetID, orgsync.PlanComputed)

		quietSuccess := true
		request.Target.ConfigPatch = config.Patch{QuietSuccess: &quietSuccess}
		result, err = store.SaveInstallationSettings(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.CheckpointID).NotTo(BeNil())
		Expect(result.Target).To(HaveField("Revision", int64(2)))
		assertInstallationSettingsAudit(ctx, store, target.TargetID, *result.CheckpointID, 1)
		accountAudit, err := store.ListAudit(ctx, target.TargetID, storage.AuditPageRequest{
			HistoryPageRequest: storage.HistoryPageRequest{Limit: 10},
			Change:             storage.AuditChangeAccount,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(accountAudit.Items).To(HaveLen(1))
		for _, change := range []storage.AuditChange{
			storage.AuditChangeRepository,
			storage.AuditChangeSync,
		} {
			filtered, listErr := store.ListAudit(ctx, target.TargetID, storage.AuditPageRequest{
				HistoryPageRequest: storage.HistoryPageRequest{Limit: 10},
				Change:             change,
			})
			Expect(listErr).NotTo(HaveOccurred())
			Expect(filtered.Items).To(BeEmpty())
		}
		assertInstallationSettingsPlanState(ctx, store, target.TargetID, orgsync.PlanComputed)

		request.Target.ExpectedRevision = 2
		request.ChangedAt = now.Add(2 * time.Minute)
		repeated, err := store.SaveInstallationSettings(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(repeated.CheckpointID).To(BeNil())
		assertInstallationSettingsAudit(ctx, store, target.TargetID, *result.CheckpointID, 1)
		request.Target.ExpectedRevision = 1
		_, err = store.SaveInstallationSettings(ctx, request)
		Expect(errors.Is(err, storage.ErrConflict)).To(BeTrue())
	})
}

func seedInstallationSettingsBatch(
	ctx context.Context,
	store storage.Store,
	now time.Time,
) (storage.Account, storage.InstallationSnapshot) {
	GinkgoHelper()
	account := testAccount(now)
	target := testInstallation(account, now, []storage.RepositorySnapshot{
		testRepository("repo-1", "smykla-skalski/smyklot", false),
		testRepository("repo-2", "smykla-skalski/platform", true),
	})
	Expect(store.ReconcileInstallation(ctx, target)).To(Succeed())

	return account, target
}

func seedInstallationSettingsPlan(
	ctx context.Context,
	store storage.Store,
	account storage.Account,
	targetID string,
	now time.Time,
) {
	GinkgoHelper()
	_, err := store.CreateSyncPlan(ctx, orgsync.PlanCreate{
		ID: "installation-settings-plan", TargetID: targetID,
		Trigger: orgsync.TriggerManual, ActorID: account.ID, Digest: "plan-digest",
		Now: now, ExpiresAt: now.Add(time.Hour),
	})
	Expect(err).NotTo(HaveOccurred())
}

func readInstallationCheckpoint(
	ctx context.Context,
	store storage.Store,
	checkpointID int64,
	targetID string,
) storage.SettingsCheckpoint {
	GinkgoHelper()
	inspection, err := store.InspectInstallationSettingsCheckpoint(ctx, storage.SettingsCheckpointRef{
		ID: checkpointID, Scope: storage.SettingsCheckpointScopeInstallation, TargetID: targetID,
	})
	Expect(err).NotTo(HaveOccurred())

	return inspection.Checkpoint
}

func checkpointItemKinds(
	items []storage.SettingsCheckpointItem,
) []storage.SettingsCheckpointItemKind {
	kinds := make([]storage.SettingsCheckpointItemKind, len(items))
	for index, item := range items {
		kinds[index] = item.Kind
	}

	return kinds
}

func assertSettingsCheckpointState(
	item storage.SettingsCheckpointItem,
	beforeRevision, afterRevision int64,
) {
	GinkgoHelper()
	Expect(item.Before).NotTo(BeNil())
	Expect(item.After).NotTo(BeNil())
	Expect(item.Before.Revision).To(Equal(beforeRevision))
	Expect(item.After.Revision).To(Equal(afterRevision))
	Expect(json.Valid(item.Before.Document)).To(BeTrue())
	Expect(json.Valid(item.After.Document)).To(BeTrue())
	Expect(item.Before.Digest).To(Equal(
		storage.DigestSettingsCheckpointDocument(item.Before.Document),
	))
	Expect(item.After.Digest).To(Equal(
		storage.DigestSettingsCheckpointDocument(item.After.Document),
	))
}

func assertInstallationSettingsAudit(
	ctx context.Context,
	store storage.Store,
	targetID string,
	checkpointID int64,
	want int,
) {
	GinkgoHelper()
	audit, err := store.ListAudit(ctx, targetID, storage.AuditPageRequest{
		HistoryPageRequest: storage.HistoryPageRequest{Limit: 10},
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(audit.Items).To(HaveLen(want))
	root, err := store.ListRootAudit(ctx, storage.RootAuditPageRequest{
		HistoryPageRequest: storage.HistoryPageRequest{Limit: 10},
		Categories:         []storage.AuditCategory{storage.AuditCategoryConfiguration},
		TargetID:           &targetID,
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(root.Items).To(HaveLen(want))
	if want > 0 {
		Expect(audit.Items[0].SettingsCheckpointID).To(HaveValue(Equal(checkpointID)))
		Expect(root.Items[0].SettingsCheckpointID).To(HaveValue(Equal(checkpointID)))
	}
}

func assertInstallationSettingsPlanState(
	ctx context.Context,
	store storage.Store,
	targetID string,
	want orgsync.PlanState,
) {
	GinkgoHelper()
	plan, _, err := store.GetSyncPlan(ctx, targetID, "installation-settings-plan")
	Expect(err).NotTo(HaveOccurred())
	Expect(plan.State).To(Equal(want))
}

func assertUnchangedInstallationSettings(
	ctx context.Context,
	store storage.Store,
	targetID string,
) {
	GinkgoHelper()
	target, err := store.GetTarget(ctx, targetID)
	Expect(err).NotTo(HaveOccurred())
	Expect(target.Revision).To(Equal(int64(1)))
	Expect(target.RepositoryDefaultEnabled).To(BeFalse())
	for _, repositoryID := range []string{"repo-1", "repo-2"} {
		repository, err := store.GetRepository(ctx, targetID, repositoryID)
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.Revision).To(Equal(int64(1)))
		Expect(repository.EnabledOverride).To(BeNil())
		Expect(repository.IgnoreRepositoryFile).To(BeFalse())
	}
}

func installationTargetChange(target storage.Target) *storage.InstallationTargetSettingsChange {
	return &storage.InstallationTargetSettingsChange{
		RepositoryDefaultEnabled:       target.RepositoryDefaultEnabled,
		PendingCIModeDefault:           target.PendingCIModeDefault,
		PendingCIBranchPatternsDefault: target.PendingCIBranchPatternsDefault,
		PendingCIQuietPeriodOverride:   target.PendingCIQuietPeriodOverride,
		PathIndexIntervalOverride:      target.PathIndexIntervalOverride,
		ConfigPatch:                    target.ConfigPatch, ExpectedRevision: target.Revision,
	}
}

func installationRepositoryChange(
	repository storage.Repository,
) storage.InstallationRepositorySettingsChange {
	return storage.InstallationRepositorySettingsChange{
		RepositoryID: repository.ID, EnabledOverride: repository.EnabledOverride,
		PendingCIModeOverride:           repository.PendingCIModeOverride,
		PendingCIBranchPatternsOverride: repository.PendingCIBranchPatternsOverride,
		PendingCIQuietPeriodOverride:    repository.PendingCIQuietPeriodOverride,
		PathIndexIntervalOverride:       repository.PathIndexIntervalOverride,
		ConfigPatch:                     repository.ConfigPatch, IgnoreRepositoryFile: repository.IgnoreRepositoryFile,
		ExpectedRevision: repository.Revision,
	}
}

func installationTargetDocument(target storage.Target) storage.TargetSettingsDocument {
	return storage.TargetSettingsDocument{
		RepositoryDefaultEnabled:       target.RepositoryDefaultEnabled,
		PendingCIModeDefault:           target.PendingCIModeDefault,
		PendingCIBranchPatternsDefault: target.PendingCIBranchPatternsDefault,
		PendingCIQuietPeriodOverride:   target.PendingCIQuietPeriodOverride,
		PathIndexIntervalOverride:      target.PathIndexIntervalOverride,
		ConfigPatch:                    target.ConfigPatch,
	}
}

func installationRepositoryDocument(
	repository storage.Repository,
) storage.RepositorySettingsDocument {
	return storage.RepositorySettingsDocument{
		EnabledOverride:                 repository.EnabledOverride,
		PendingCIModeOverride:           repository.PendingCIModeOverride,
		PendingCIBranchPatternsOverride: repository.PendingCIBranchPatternsOverride,
		PendingCIQuietPeriodOverride:    repository.PendingCIQuietPeriodOverride,
		PathIndexIntervalOverride:       repository.PathIndexIntervalOverride,
		ConfigPatch:                     repository.ConfigPatch,
		IgnoreRepositoryFile:            repository.IgnoreRepositoryFile,
	}
}

func decodeSettingsDocument[Document any](encoded []byte) Document {
	GinkgoHelper()
	var document Document
	Expect(json.Unmarshal(encoded, &document)).To(Succeed())

	return document
}

func repositoryIDs(repositories []storage.Repository) []string {
	ids := make([]string, 0, len(repositories))
	for _, repository := range repositories {
		ids = append(ids, repository.ID)
	}

	return ids
}
