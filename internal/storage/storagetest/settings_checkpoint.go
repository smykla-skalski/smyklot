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

func declareSettingsCheckpointSpecs(
	runtime func() (context.Context, storage.Store, time.Time),
) {
	declareInstallationCheckpointSpec(runtime)
	declareRootCheckpointSpec(runtime)
	declareInvalidCheckpointSpec(runtime)
}

func declareInstallationCheckpointSpec(
	runtime func() (context.Context, storage.Store, time.Time),
) {
	It("keeps sparse installation setting states and typed identities", func() {
		ctx, store, now := runtime()
		account, targetID := seedSettingsCheckpointInstallation(ctx, store, now)
		repositoryID := "github:repository:1"
		items := []storage.SettingsCheckpointItem{
			settingsCheckpointItem(storage.SettingsCheckpointItemSyncOverride,
				repositoryID, "smykla-skalski/one", orgsync.KindLabels,
				`{"enabled":null}`, 2, `{"enabled":false}`, 3),
			settingsCheckpointItem(storage.SettingsCheckpointItemTarget,
				"", "", "", `{"repositoryDefaultEnabled":true}`, 4,
				`{"repositoryDefaultEnabled":false}`, 5),
			settingsCheckpointItem(storage.SettingsCheckpointItemSyncConfig,
				"", "", orgsync.KindFiles, `{"enabled":false,"document":{}}`, 1,
				`{"enabled":true,"document":{"paths":[]}}`, 2),
			settingsCheckpointItem(storage.SettingsCheckpointItemRepository,
				repositoryID, "smykla-skalski/one", "", `{"enabledOverride":null}`, 7,
				`{"enabledOverride":true}`, 8),
		}

		created, err := store.CreateSettingsCheckpoint(ctx, storage.SettingsCheckpointCreate{
			Scope: storage.SettingsCheckpointScopeInstallation, TargetID: targetID,
			ActorAccountID: account.ID, Action: storage.SettingsCheckpointActionSave,
			CreatedAt: now.Add(time.Minute), Items: items,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(created.ID).To(BeNumerically(">", 0))
		Expect(created.Scope).To(Equal(storage.SettingsCheckpointScopeInstallation))
		Expect(created.TargetID).To(Equal(targetID))
		Expect(created.ActorAccountID).To(Equal(account.ID))
		Expect(created.Items).To(HaveLen(4))
		Expect(checkpointItemKinds(created.Items)).To(Equal([]storage.SettingsCheckpointItemKind{
			storage.SettingsCheckpointItemRepository,
			storage.SettingsCheckpointItemSyncConfig,
			storage.SettingsCheckpointItemSyncOverride,
			storage.SettingsCheckpointItemTarget,
		}))

		items[0].After.Document[0] = '['
		read, err := store.GetSettingsCheckpoint(ctx, storage.SettingsCheckpointRef{
			ID: created.ID, Scope: created.Scope, TargetID: targetID,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(read).To(Equal(created))
	})
}

func declareRootCheckpointSpec(
	runtime func() (context.Context, storage.Store, time.Time),
) {
	It("links restores only to a checkpoint in the same settings scope", func() {
		ctx, store, now := runtime()
		account, targetID := seedSettingsCheckpointInstallation(ctx, store, now)
		original := settingsCheckpointItem(storage.SettingsCheckpointItemRuntime,
			"", "", "", "", 0, `{"logLevel":"debug"}`, 1)
		original.Before = nil
		saved, err := store.CreateSettingsCheckpoint(ctx, storage.SettingsCheckpointCreate{
			Scope: storage.SettingsCheckpointScopeRoot, ActorAccountID: account.ID,
			Action: storage.SettingsCheckpointActionSave, CreatedAt: now, Items: []storage.SettingsCheckpointItem{original},
		})
		Expect(err).NotTo(HaveOccurred())

		restoreItem := settingsCheckpointItem(storage.SettingsCheckpointItemRuntime,
			"", "", "", `{"logLevel":"info"}`, 2, `{"logLevel":"debug"}`, 3)
		restored, err := store.CreateSettingsCheckpoint(ctx, storage.SettingsCheckpointCreate{
			Scope: storage.SettingsCheckpointScopeRoot, ActorAccountID: account.ID,
			Action: storage.SettingsCheckpointActionRestore, RestoredFromID: &saved.ID,
			CreatedAt: now.Add(time.Minute), Items: []storage.SettingsCheckpointItem{restoreItem},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(restored.RestoredFromID).To(HaveValue(Equal(saved.ID)))

		_, err = store.GetSettingsCheckpoint(ctx, storage.SettingsCheckpointRef{
			ID: saved.ID, Scope: storage.SettingsCheckpointScopeInstallation, TargetID: targetID,
		})
		Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())
		_, err = store.CreateSettingsCheckpoint(ctx, storage.SettingsCheckpointCreate{
			Scope: storage.SettingsCheckpointScopeInstallation, TargetID: targetID,
			ActorAccountID: account.ID, Action: storage.SettingsCheckpointActionRestore,
			RestoredFromID: &saved.ID, CreatedAt: now.Add(2 * time.Minute),
			Items: []storage.SettingsCheckpointItem{settingsCheckpointItem(
				storage.SettingsCheckpointItemTarget, "", "", "", `{"enabled":true}`, 1,
				`{"enabled":false}`, 2,
			)},
		})
		Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())
	})
}

func declareInvalidCheckpointSpec(
	runtime func() (context.Context, storage.Store, time.Time),
) {
	It("rejects malformed checkpoint items before inserting a header", func() {
		ctx, store, now := runtime()
		account, _ := seedSettingsCheckpointInstallation(ctx, store, now)
		item := settingsCheckpointItem(storage.SettingsCheckpointItemRuntime,
			"", "", "", `{"logLevel":"info"}`, 1, `{"logLevel":"debug"}`, 2)
		item.After.Digest = "not-the-document-digest"

		_, err := store.CreateSettingsCheckpoint(ctx, storage.SettingsCheckpointCreate{
			Scope: storage.SettingsCheckpointScopeRoot, ActorAccountID: account.ID,
			Action: storage.SettingsCheckpointActionSave, CreatedAt: now,
			Items: []storage.SettingsCheckpointItem{item},
		})
		Expect(err).To(MatchError(ContainSubstring("digest does not match")))

		item.After = checkpointState(`{"logLevel":"debug"}`, 2)
		created, err := store.CreateSettingsCheckpoint(ctx, storage.SettingsCheckpointCreate{
			Scope: storage.SettingsCheckpointScopeRoot, ActorAccountID: account.ID,
			Action: storage.SettingsCheckpointActionSave, CreatedAt: now,
			Items: []storage.SettingsCheckpointItem{item},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(created.ID).To(Equal(int64(1)))
	})
}

func seedSettingsCheckpointInstallation(
	ctx context.Context,
	store storage.Store,
	now time.Time,
) (storage.Account, string) {
	GinkgoHelper()

	account := testAccount(now)
	installation := testInstallation(account, now, []storage.RepositorySnapshot{
		testRepository("github:repository:1", "smykla-skalski/one", false),
	})
	Expect(store.UpsertAccount(ctx, account)).To(Succeed())
	Expect(store.ReconcileCatalog(ctx, []storage.InstallationSnapshot{installation})).To(Succeed())

	return account, installation.TargetID
}

func settingsCheckpointItem(
	kind storage.SettingsCheckpointItemKind,
	repositoryID, repositoryFullName string,
	syncKind orgsync.Kind,
	before string,
	beforeRevision int64,
	after string,
	afterRevision int64,
) storage.SettingsCheckpointItem {
	item := storage.SettingsCheckpointItem{
		Kind: kind, RepositoryID: repositoryID, RepositoryFullName: repositoryFullName,
		SyncKind: syncKind, DocumentVersion: storage.SettingsCheckpointDocumentVersion,
	}
	if before != "" {
		item.Before = checkpointState(before, beforeRevision)
	}
	if after != "" {
		item.After = checkpointState(after, afterRevision)
	}

	return item
}

func checkpointState(document string, revision int64) *storage.SettingsCheckpointState {
	state := storage.NewSettingsCheckpointState([]byte(document), revision)

	return &state
}

func checkpointItemKinds(items []storage.SettingsCheckpointItem) []storage.SettingsCheckpointItemKind {
	kinds := make([]storage.SettingsCheckpointItemKind, len(items))
	for index, item := range items {
		kinds[index] = item.Kind
	}

	return kinds
}
