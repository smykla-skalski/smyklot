package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

// GetSyncConfigCheckpoint reads one immutable state, scoped to its target.
func (s *Store) GetSyncConfigCheckpoint(
	ctx context.Context,
	targetID string,
	checkpointID int64,
) (orgsync.ConfigCheckpoint, error) {
	return getSyncConfigCheckpoint(ctx, s.db, targetID, checkpointID)
}

func getSyncConfigCheckpoint(
	ctx context.Context,
	queryer runner,
	targetID string,
	checkpointID int64,
) (orgsync.ConfigCheckpoint, error) {
	checkpoint, err := scanSyncConfigCheckpoint(queryer.QueryRowContext(ctx, `
SELECT`+syncCheckpointColumns+`
FROM sync_config_checkpoints
WHERE target_id = ? AND id = ?`, targetID, checkpointID))
	if errors.Is(err, sql.ErrNoRows) {
		return orgsync.ConfigCheckpoint{}, storage.ErrNotFound
	}
	if err != nil {
		return orgsync.ConfigCheckpoint{}, err
	}

	rows, err := queryer.QueryContext(ctx, `
SELECT kind, enabled, document, digest, revision
FROM sync_config_checkpoint_items
WHERE checkpoint_id = ?
ORDER BY kind`, checkpointID)
	if err != nil {
		return orgsync.ConfigCheckpoint{}, fmt.Errorf("list sync config checkpoint items: %w", err)
	}
	checkpoint.Items, err = collectRows(rows, scanSyncConfigCheckpointItem)
	if err != nil {
		return orgsync.ConfigCheckpoint{}, fmt.Errorf("read sync config checkpoint items: %w", err)
	}
	checkpoint.PreviousItems, err = previousSyncConfigCheckpointItems(
		ctx, queryer, targetID, checkpointID,
	)
	if err != nil {
		return orgsync.ConfigCheckpoint{}, err
	}

	return checkpoint, nil
}

func previousSyncConfigCheckpointItems(
	ctx context.Context,
	queryer runner,
	targetID string,
	checkpointID int64,
) ([]orgsync.ConfigCheckpointItem, error) {
	var previousID int64
	err := queryer.QueryRowContext(ctx, `
SELECT id
FROM sync_config_checkpoints
WHERE target_id = ? AND id < ?
ORDER BY id DESC
LIMIT 1`, targetID, checkpointID).Scan(&previousID)
	if errors.Is(err, sql.ErrNoRows) {
		return []orgsync.ConfigCheckpointItem{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read previous sync config checkpoint: %w", err)
	}

	rows, err := queryer.QueryContext(ctx, `
SELECT kind, enabled, document, digest, revision
FROM sync_config_checkpoint_items
WHERE checkpoint_id = ?
ORDER BY kind`, previousID)
	if err != nil {
		return nil, fmt.Errorf("list previous sync config checkpoint items: %w", err)
	}
	items, err := collectRows(rows, scanSyncConfigCheckpointItem)
	if err != nil {
		return nil, fmt.Errorf("read previous sync config checkpoint items: %w", err)
	}

	return items, nil
}

func scanSyncConfigCheckpoint(scanner rowScanner) (orgsync.ConfigCheckpoint, error) {
	var checkpoint orgsync.ConfigCheckpoint
	var restoredFrom sql.NullInt64
	var createdAt StoredTime
	if err := scanner.Scan(
		&checkpoint.ID, &checkpoint.TargetID, &checkpoint.ActorID, &checkpoint.Action,
		&restoredFrom, &createdAt,
	); err != nil {
		return orgsync.ConfigCheckpoint{}, fmt.Errorf("scan sync config checkpoint: %w", err)
	}
	if restoredFrom.Valid {
		checkpoint.RestoredFromID = &restoredFrom.Int64
	}
	checkpoint.CreatedAt = createdAt.Time()

	return checkpoint, nil
}

func scanSyncConfigCheckpointItem(scanner rowScanner) (orgsync.ConfigCheckpointItem, error) {
	var item orgsync.ConfigCheckpointItem
	if err := scanner.Scan(
		&item.Kind, &item.Enabled, &item.Document, &item.Digest, &item.Revision,
	); err != nil {
		return orgsync.ConfigCheckpointItem{}, err
	}

	return item, nil
}

// RestoreSyncConfigCheckpoint replaces selected kinds and records the resulting
// state without changing the checkpoint being restored.
func (s *Store) RestoreSyncConfigCheckpoint(
	ctx context.Context,
	restore orgsync.ConfigRestore,
) (orgsync.ConfigWrite, error) {
	kinds, err := prepareRestoreKinds(restore)
	if err != nil {
		return orgsync.ConfigWrite{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return orgsync.ConfigWrite{}, fmt.Errorf("begin sync config restore: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	elevation, err := s.elevatedWrite(
		ctx, tx, restore.ElevationID, restore.SessionTokenHash,
		restore.ActorID, restore.TargetID, restore.Now,
	)
	if err != nil {
		return orgsync.ConfigWrite{}, err
	}
	if err := s.lockSyncTarget(ctx, tx, restore.TargetID); err != nil {
		return orgsync.ConfigWrite{}, err
	}
	checkpoint, err := getSyncConfigCheckpoint(
		ctx, tx, restore.TargetID, restore.CheckpointID,
	)
	if err != nil {
		return orgsync.ConfigWrite{}, err
	}
	items := make(map[orgsync.Kind]orgsync.ConfigCheckpointItem, len(checkpoint.Items))
	for _, item := range checkpoint.Items {
		items[item.Kind] = item
	}

	changed := make([]orgsync.Kind, 0, len(kinds))
	for _, kind := range kinds {
		wrote, restoreErr := s.restoreSyncKind(ctx, tx, restore, kind, items)
		if restoreErr != nil {
			return orgsync.ConfigWrite{}, restoreErr
		}
		if wrote {
			changed = append(changed, kind)
		}
	}

	return s.finishSyncConfigWrite(ctx, tx, syncWriteFinish{
		TargetID: restore.TargetID, ActorID: restore.ActorID, Now: restore.Now,
		ElevationID: restore.ElevationID, Elevation: elevation,
		Action: orgsync.CheckpointRestored, RestoredFrom: &restore.CheckpointID,
		Changed: changed,
	})
}

func prepareRestoreKinds(restore orgsync.ConfigRestore) ([]orgsync.Kind, error) {
	if len(restore.Kinds) == 0 {
		return nil, fmt.Errorf("restore needs at least one sync kind")
	}
	kinds := append([]orgsync.Kind(nil), restore.Kinds...)
	seen := make(map[orgsync.Kind]bool, len(kinds))
	for _, kind := range kinds {
		if !kind.Valid() {
			return nil, fmt.Errorf("%w: unknown kind %q", storage.ErrNotFound, kind)
		}
		if seen[kind] {
			return nil, fmt.Errorf("duplicate sync kind %q", kind)
		}
		if _, ok := restore.Revisions[kind]; !ok {
			return nil, fmt.Errorf("restore needs the current %s revision", kind)
		}
		seen[kind] = true
	}
	sort.Slice(kinds, func(i, j int) bool { return kinds[i] < kinds[j] })

	return kinds, nil
}

func (s *Store) restoreSyncKind(
	ctx context.Context,
	tx *transaction,
	restore orgsync.ConfigRestore,
	kind orgsync.Kind,
	items map[orgsync.Kind]orgsync.ConfigCheckpointItem,
) (bool, error) {
	current, err := syncConfigForUpdate(ctx, tx, s.dialect, restore.TargetID, kind)
	missing := errors.Is(err, storage.ErrNotFound)
	if err != nil && !missing {
		return false, err
	}
	expected := restore.Revisions[kind]
	if (missing && expected != 0) || (!missing && current.Revision != expected) {
		return false, storage.ErrConflict
	}

	item, present := items[kind]
	if !present {
		if missing {
			return false, nil
		}
		_, err = tx.ExecContext(ctx,
			"DELETE FROM sync_configs WHERE target_id = ? AND kind = ?",
			restore.TargetID, kind,
		)
		if err != nil {
			return false, fmt.Errorf("remove restored sync config: %w", err)
		}
		return true, nil
	}
	if orgsync.DigestConfig(item.Enabled, item.Document) != item.Digest {
		return false, fmt.Errorf("sync config checkpoint %d has an invalid %s digest",
			restore.CheckpointID, kind)
	}
	if !missing && current.Digest == item.Digest {
		return false, nil
	}

	return s.writeRestoredSyncKind(ctx, tx, restore, item, missing, current.Revision)
}

func (s *Store) writeRestoredSyncKind(
	ctx context.Context,
	tx *transaction,
	restore orgsync.ConfigRestore,
	item orgsync.ConfigCheckpointItem,
	missing bool,
	currentRevision int64,
) (bool, error) {
	if missing {
		revision, err := nextSyncConfigRevision(ctx, tx, restore.TargetID, item.Kind)
		if err != nil {
			return false, err
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO sync_configs (
    target_id, kind, enabled, document, digest, revision, updated_by, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, restore.TargetID, item.Kind, item.Enabled,
			item.Document, item.Digest, revision, restore.ActorID, restore.Now)
		if err != nil {
			return false, fmt.Errorf("insert restored sync config: %w", err)
		}
		return true, nil
	}

	_, err := tx.ExecContext(ctx, `
UPDATE sync_configs SET
    enabled = ?, document = ?, digest = ?, revision = ?, updated_by = ?, updated_at = ?
WHERE target_id = ? AND kind = ?`, item.Enabled, item.Document, item.Digest,
		currentRevision+1, restore.ActorID, restore.Now, restore.TargetID, item.Kind)
	if err != nil {
		return false, fmt.Errorf("update restored sync config: %w", err)
	}

	return true, nil
}
