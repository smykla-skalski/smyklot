package sqlstore

import (
	"context"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

func (s *Store) buildInstallationSyncItems(
	ctx context.Context,
	tx *transaction,
	targetID string,
	work installationSettingsWork,
) (installationSettingsWork, error) {
	work, err := s.buildInstallationSyncConfigItems(ctx, tx, targetID, work)
	if err != nil {
		return installationSettingsWork{}, err
	}

	return s.buildInstallationSyncOverrideItems(ctx, tx, targetID, work)
}

func (s *Store) buildInstallationSyncConfigItems(
	ctx context.Context,
	tx *transaction,
	targetID string,
	work installationSettingsWork,
) (installationSettingsWork, error) {
	for index := range work.syncConfigs {
		entry := &work.syncConfigs[index]
		item, err := s.buildInstallationSyncConfigItem(ctx, tx, targetID, entry)
		if err != nil {
			return installationSettingsWork{}, err
		}
		if item != nil {
			work.items = append(work.items, *item)
			work.syncChanged = true
		}
	}

	return work, nil
}

func (s *Store) buildInstallationSyncConfigItem(
	ctx context.Context,
	tx *transaction,
	targetID string,
	entry *syncConfigSettingsWork,
) (*storage.SettingsCheckpointItem, error) {
	if entry.prepared.remove {
		entry.changed = entry.current != nil
	} else {
		entry.changed = entry.current == nil || entry.current.Digest != entry.prepared.digest
	}
	if !entry.changed {
		if entry.current != nil {
			entry.afterRevision = entry.current.Revision
		}

		return nil, nil
	}
	if entry.current == nil {
		revision, err := nextSyncConfigRevision(ctx, tx, targetID, entry.prepared.change.Kind)
		if err != nil {
			return nil, err
		}
		entry.afterRevision = revision
	} else {
		entry.afterRevision = entry.current.Revision + 1
	}
	item, err := syncConfigSettingsCheckpointItem(*entry)
	if err != nil {
		return nil, err
	}

	return &item, nil
}

func (s *Store) buildInstallationSyncOverrideItems(
	ctx context.Context,
	tx *transaction,
	targetID string,
	work installationSettingsWork,
) (installationSettingsWork, error) {
	for index := range work.syncOverrides {
		entry := &work.syncOverrides[index]
		item, err := s.buildInstallationSyncOverrideItem(ctx, tx, targetID, entry)
		if err != nil {
			return installationSettingsWork{}, err
		}
		if item != nil {
			work.items = append(work.items, *item)
			work.syncChanged = true
		}
	}

	return work, nil
}

func (s *Store) buildInstallationSyncOverrideItem(
	ctx context.Context,
	tx *transaction,
	targetID string,
	entry *syncOverrideSettingsWork,
) (*storage.SettingsCheckpointItem, error) {
	if entry.prepared.remove {
		entry.changed = entry.current != nil
	} else {
		entry.changed = entry.current == nil || !sameSyncOverride(*entry.current, entry.prepared)
	}
	if !entry.changed {
		if entry.current != nil {
			entry.afterRevision = entry.current.Revision
		}

		return nil, nil
	}
	if entry.current == nil {
		revision, err := nextSyncOverrideRevision(
			ctx, tx, targetID, entry.repository.ID, entry.prepared.change.Kind,
		)
		if err != nil {
			return nil, err
		}
		entry.afterRevision = revision
	} else {
		entry.afterRevision = entry.current.Revision + 1
	}
	item, err := syncOverrideSettingsCheckpointItem(*entry)
	if err != nil {
		return nil, err
	}

	return &item, nil
}

func syncConfigSettingsCheckpointItem(
	work syncConfigSettingsWork,
) (storage.SettingsCheckpointItem, error) {
	var before *storage.SettingsCheckpointState
	if work.current != nil {
		state, err := syncConfigSettingsState(
			syncConfigSettingsDocument(*work.current), work.current.Revision,
		)
		if err != nil {
			return storage.SettingsCheckpointItem{}, err
		}
		before = state
	}
	if work.prepared.remove {
		return storage.SettingsCheckpointItem{
			Kind: storage.SettingsCheckpointItemSyncConfig, SyncKind: work.prepared.change.Kind,
			DocumentVersion: storage.SettingsCheckpointDocumentVersion,
			Before:          before,
		}, nil
	}
	after, err := syncConfigSettingsState(storage.SyncConfigSettingsDocument{
		Enabled: work.prepared.change.Enabled, Document: string(work.prepared.document),
	}, work.afterRevision)
	if err != nil {
		return storage.SettingsCheckpointItem{}, err
	}

	return storage.SettingsCheckpointItem{
		Kind: storage.SettingsCheckpointItemSyncConfig, SyncKind: work.prepared.change.Kind,
		DocumentVersion: storage.SettingsCheckpointDocumentVersion,
		Before:          before, After: after,
	}, nil
}

func syncOverrideSettingsCheckpointItem(
	work syncOverrideSettingsWork,
) (storage.SettingsCheckpointItem, error) {
	var before *storage.SettingsCheckpointState
	if work.current != nil {
		state, err := syncOverrideSettingsState(
			syncOverrideSettingsDocument(*work.current), work.current.Revision,
		)
		if err != nil {
			return storage.SettingsCheckpointItem{}, err
		}
		before = state
	}
	if work.prepared.remove {
		return storage.SettingsCheckpointItem{
			Kind:         storage.SettingsCheckpointItemSyncOverride,
			RepositoryID: work.repository.ID, RepositoryFullName: work.repository.FullName,
			SyncKind:        work.prepared.change.Kind,
			DocumentVersion: storage.SettingsCheckpointDocumentVersion,
			Before:          before,
		}, nil
	}
	after, err := syncOverrideSettingsState(storage.SyncOverrideSettingsDocument{
		Enabled: work.prepared.change.Enabled, Document: string(work.prepared.document),
	}, work.afterRevision)
	if err != nil {
		return storage.SettingsCheckpointItem{}, err
	}

	return storage.SettingsCheckpointItem{
		Kind:         storage.SettingsCheckpointItemSyncOverride,
		RepositoryID: work.repository.ID, RepositoryFullName: work.repository.FullName,
		SyncKind:        work.prepared.change.Kind,
		DocumentVersion: storage.SettingsCheckpointDocumentVersion,
		Before:          before, After: after,
	}, nil
}

func syncConfigSettingsState(
	document storage.SyncConfigSettingsDocument,
	revision int64,
) (*storage.SettingsCheckpointState, error) {
	return installationSettingsState(document, revision)
}

func syncOverrideSettingsState(
	document storage.SyncOverrideSettingsDocument,
	revision int64,
) (*storage.SettingsCheckpointState, error) {
	return installationSettingsState(document, revision)
}

func syncConfigSettingsDocument(config orgsync.Config) storage.SyncConfigSettingsDocument {
	return storage.SyncConfigSettingsDocument{
		Enabled: config.Enabled, Document: string(config.Document),
	}
}

func syncOverrideSettingsDocument(
	override orgsync.RepositoryOverride,
) storage.SyncOverrideSettingsDocument {
	return storage.SyncOverrideSettingsDocument{
		Enabled: override.Enabled, Document: string(override.Document),
	}
}

func nextSyncOverrideRevision(
	ctx context.Context,
	queryer rowQuerier,
	targetID, repositoryID string,
	kind orgsync.Kind,
) (int64, error) {
	var revision int64
	err := queryer.QueryRowContext(ctx, `
SELECT COALESCE(MAX(revision), 0) + 1
FROM (
    SELECT item.before_revision AS revision
    FROM settings_checkpoint_items item
    JOIN settings_checkpoints checkpoint ON checkpoint.id = item.checkpoint_id
    WHERE checkpoint.target_id = ? AND item.item_kind = 'sync_override'
      AND item.repository_id = ? AND item.sync_kind = ?
    UNION ALL
    SELECT item.after_revision AS revision
    FROM settings_checkpoint_items item
    JOIN settings_checkpoints checkpoint ON checkpoint.id = item.checkpoint_id
    WHERE checkpoint.target_id = ? AND item.item_kind = 'sync_override'
      AND item.repository_id = ? AND item.sync_kind = ?
) revisions`, targetID, repositoryID, kind, targetID, repositoryID, kind).Scan(&revision)
	if err != nil {
		return 0, fmt.Errorf("read next sync override revision: %w", err)
	}

	return revision, nil
}
