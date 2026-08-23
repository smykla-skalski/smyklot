package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

// nextSyncConfigRevision prevents an absent kind from resetting its optimistic
// revision to one. Restore can deliberately remove a kind, but its immutable
// settings checkpoints retain the highest revision that existed before removal.
func nextSyncConfigRevision(
	ctx context.Context,
	queryer rowQuerier,
	targetID string,
	kind orgsync.Kind,
) (int64, error) {
	var revision int64
	err := queryer.QueryRowContext(ctx, `
SELECT COALESCE(MAX(revision), 0) + 1
FROM (
    SELECT item.before_revision AS revision
    FROM settings_checkpoint_items item
    JOIN settings_checkpoints checkpoint ON checkpoint.id = item.checkpoint_id
    WHERE checkpoint.target_id = ? AND item.item_kind = 'sync_config'
      AND item.sync_kind = ?
    UNION ALL
    SELECT item.after_revision AS revision
    FROM settings_checkpoint_items item
    JOIN settings_checkpoints checkpoint ON checkpoint.id = item.checkpoint_id
    WHERE checkpoint.target_id = ? AND item.item_kind = 'sync_config'
      AND item.sync_kind = ?
) revisions`, targetID, kind, targetID, kind).Scan(&revision)
	if err != nil {
		return 0, fmt.Errorf("read next sync config revision: %w", err)
	}

	return revision, nil
}

func syncConfigForUpdate(
	ctx context.Context,
	tx *transaction,
	dialect Dialect,
	targetID string,
	kind orgsync.Kind,
) (orgsync.Config, error) {
	config, err := scanSyncConfig(tx.QueryRowContext(ctx, `
SELECT`+syncConfigColumns+`
FROM sync_configs
WHERE target_id = ? AND kind = ?`+dialect.RowLock(), targetID, kind))
	if errors.Is(err, sql.ErrNoRows) {
		return orgsync.Config{}, storage.ErrNotFound
	}
	if err != nil {
		return orgsync.Config{}, fmt.Errorf("read sync config for update: %w", err)
	}

	return config, nil
}

func listSyncConfigs(
	ctx context.Context,
	queryer runner,
	targetID string,
) ([]orgsync.Config, error) {
	rows, err := queryer.QueryContext(ctx, `
SELECT`+syncConfigColumns+`
FROM sync_configs
WHERE target_id = ?
ORDER BY kind`, targetID)
	if err != nil {
		return nil, fmt.Errorf("list sync configs: %w", err)
	}

	return collectRows(rows, scanSyncConfig)
}
