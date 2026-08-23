package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

const syncCheckpointColumns = `
    id, target_id, actor_account_id, action, restored_from_id, created_at`

// SetSyncConfig keeps the original one-kind port while using the same audited
// transaction as a panel save that changes several kinds.
func (s *Store) SetSyncConfig(
	ctx context.Context,
	change orgsync.ConfigChange,
) (orgsync.Config, error) {
	written, err := s.SetSyncConfigs(ctx, orgsync.ConfigBatchChange{
		TargetID: change.TargetID,
		ActorID:  change.ActorID,
		Now:      change.Now,
		Changes: []orgsync.ConfigPatch{{
			Kind: change.Kind, Enabled: change.Enabled, Document: change.Document,
			Revision: change.Revision,
		}},
	})
	if err != nil {
		return orgsync.Config{}, err
	}
	for _, config := range written.Configs {
		if config.Kind == change.Kind {
			return config, nil
		}
	}

	return orgsync.Config{}, storage.ErrNotFound
}

// SetSyncConfigs writes every changed kind, one checkpoint, and one audit event
// in the same transaction.
func (s *Store) SetSyncConfigs(
	ctx context.Context,
	change orgsync.ConfigBatchChange,
) (orgsync.ConfigWrite, error) {
	patches, err := prepareSyncPatches(change.Changes)
	if err != nil {
		return orgsync.ConfigWrite{}, err
	}
	if len(patches) == 0 {
		configs, listErr := s.ListSyncConfigs(ctx, change.TargetID)
		return orgsync.ConfigWrite{Configs: configs}, listErr
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return orgsync.ConfigWrite{}, fmt.Errorf("begin sync config write: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	elevation, err := s.elevatedWrite(
		ctx, tx, change.ElevationID, change.SessionTokenHash,
		change.ActorID, change.TargetID, change.Now,
	)
	if err != nil {
		return orgsync.ConfigWrite{}, err
	}
	if err := s.lockSyncTarget(ctx, tx, change.TargetID); err != nil {
		return orgsync.ConfigWrite{}, err
	}
	if err := s.ensureSyncBaseline(ctx, tx, change.TargetID, change.ActorID, change.Now); err != nil {
		return orgsync.ConfigWrite{}, err
	}

	changed := make([]orgsync.Kind, 0, len(patches))
	for _, patch := range patches {
		wrote, writeErr := s.writeSyncPatch(ctx, tx, change, patch)
		if writeErr != nil {
			return orgsync.ConfigWrite{}, writeErr
		}
		if wrote {
			changed = append(changed, patch.Kind)
		}
	}

	return s.finishSyncConfigWrite(ctx, tx, syncWriteFinish{
		TargetID: change.TargetID, ActorID: change.ActorID, Now: change.Now,
		ElevationID: change.ElevationID, Elevation: elevation,
		Action: orgsync.CheckpointSaved, Changed: changed,
	})
}

type syncWriteFinish struct {
	TargetID     string
	ActorID      string
	ElevationID  *string
	Elevation    *storage.Elevation
	Now          time.Time
	Action       orgsync.CheckpointAction
	RestoredFrom *int64
	Changed      []orgsync.Kind
}

func (s *Store) finishSyncConfigWrite(
	ctx context.Context,
	tx *transaction,
	finish syncWriteFinish,
) (orgsync.ConfigWrite, error) {
	configs, err := listSyncConfigs(ctx, tx, finish.TargetID)
	if err != nil {
		return orgsync.ConfigWrite{}, err
	}
	if len(finish.Changed) == 0 {
		if err := tx.Commit(); err != nil {
			return orgsync.ConfigWrite{}, fmt.Errorf("commit sync config no-op: %w", err)
		}
		return orgsync.ConfigWrite{Configs: configs}, nil
	}

	if err := invalidateLivePlans(ctx, tx, finish.TargetID, finish.Now); err != nil {
		return orgsync.ConfigWrite{}, err
	}
	checkpointID, err := s.createSyncCheckpoint(
		ctx, tx, finish.TargetID, finish.ActorID, finish.Action, finish.RestoredFrom,
		finish.Now, configs,
	)
	if err != nil {
		return orgsync.ConfigWrite{}, err
	}
	auditEventID, err := recordSyncConfigAudit(ctx, tx, finish, checkpointID)
	if err != nil {
		return orgsync.ConfigWrite{}, err
	}
	if finish.Elevation != nil {
		action := "sync.config.saved"
		if finish.Action == orgsync.CheckpointRestored {
			action = "sync.config.restored"
		}
		if err := insertElevatedNotifications(
			ctx, tx, *finish.Elevation, auditEventID, action, finish.Now,
		); err != nil {
			return orgsync.ConfigWrite{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return orgsync.ConfigWrite{}, fmt.Errorf("commit sync config write: %w", err)
	}

	return orgsync.ConfigWrite{Configs: configs, CheckpointID: &checkpointID}, nil
}

func prepareSyncPatches(patches []orgsync.ConfigPatch) ([]orgsync.ConfigPatch, error) {
	held := append([]orgsync.ConfigPatch(nil), patches...)
	seen := make(map[orgsync.Kind]bool, len(held))
	for _, patch := range held {
		if !patch.Kind.Valid() {
			return nil, fmt.Errorf("%w: unknown kind %q", storage.ErrNotFound, patch.Kind)
		}
		if seen[patch.Kind] {
			return nil, fmt.Errorf("duplicate sync kind %q", patch.Kind)
		}
		seen[patch.Kind] = true
	}
	sort.Slice(held, func(i, j int) bool { return held[i].Kind < held[j].Kind })

	return held, nil
}

func (s *Store) lockSyncTarget(ctx context.Context, tx *transaction, targetID string) error {
	var held string
	err := tx.QueryRowContext(ctx,
		"SELECT id FROM targets WHERE id = ?"+s.dialect.RowLock(), targetID,
	).Scan(&held)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock sync target: %w", err)
	}

	return nil
}

func (s *Store) writeSyncPatch(
	ctx context.Context,
	tx *transaction,
	change orgsync.ConfigBatchChange,
	patch orgsync.ConfigPatch,
) (bool, error) {
	current, err := syncConfigForUpdate(ctx, tx, s.dialect, change.TargetID, patch.Kind)
	if errors.Is(err, storage.ErrNotFound) {
		if patch.Revision != 0 {
			return false, storage.ErrConflict
		}
		revision, revisionErr := nextSyncConfigRevision(
			ctx, tx, change.TargetID, patch.Kind,
		)
		if revisionErr != nil {
			return false, revisionErr
		}
		digest := orgsync.DigestConfig(patch.Enabled, patch.Document)
		_, err = tx.ExecContext(ctx, `
INSERT INTO sync_configs (
    target_id, kind, enabled, document, digest, revision, updated_by, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			change.TargetID, patch.Kind, patch.Enabled, patch.Document,
			digest, revision, change.ActorID, change.Now,
		)
		if err != nil {
			return false, fmt.Errorf("insert sync config: %w", err)
		}
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if patch.Revision != current.Revision {
		return false, storage.ErrConflict
	}

	digest := orgsync.DigestConfig(patch.Enabled, patch.Document)
	if digest == current.Digest {
		return false, nil
	}
	_, err = tx.ExecContext(ctx, `
UPDATE sync_configs SET
    enabled = ?, document = ?, digest = ?, revision = revision + 1,
    updated_by = ?, updated_at = ?
WHERE target_id = ? AND kind = ?`,
		patch.Enabled, patch.Document, digest, change.ActorID, change.Now,
		change.TargetID, patch.Kind,
	)
	if err != nil {
		return false, fmt.Errorf("update sync config: %w", err)
	}

	return true, nil
}

// nextSyncConfigRevision prevents an absent kind from resetting its optimistic
// revision to one. Restore can deliberately remove a kind, but its immutable
// checkpoints still retain the highest revision that existed before removal.
func nextSyncConfigRevision(
	ctx context.Context,
	queryer rowQuerier,
	targetID string,
	kind orgsync.Kind,
) (int64, error) {
	var revision int64
	err := queryer.QueryRowContext(ctx, `
SELECT COALESCE(MAX(item.revision), 0) + 1
FROM sync_config_checkpoint_items item
JOIN sync_config_checkpoints checkpoint ON checkpoint.id = item.checkpoint_id
WHERE checkpoint.target_id = ? AND item.kind = ?`, targetID, kind).Scan(&revision)
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

func (s *Store) ensureSyncBaseline(
	ctx context.Context,
	tx *transaction,
	targetID, actorID string,
	now time.Time,
) error {
	var checkpointID int64
	err := tx.QueryRowContext(ctx, `
SELECT id FROM sync_config_checkpoints
WHERE target_id = ?
ORDER BY id DESC
LIMIT 1`, targetID).Scan(&checkpointID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read sync config baseline: %w", err)
	}
	configs, err := listSyncConfigs(ctx, tx, targetID)
	if err != nil {
		return err
	}
	_, err = s.createSyncCheckpoint(
		ctx, tx, targetID, actorID, orgsync.CheckpointBaseline, nil, now, configs,
	)

	return err
}

func (s *Store) createSyncCheckpoint(
	ctx context.Context,
	tx *transaction,
	targetID, actorID string,
	action orgsync.CheckpointAction,
	restoredFrom *int64,
	now time.Time,
	configs []orgsync.Config,
) (int64, error) {
	var checkpointID int64
	err := tx.QueryRowContext(ctx, `
INSERT INTO sync_config_checkpoints (
    target_id, actor_account_id, action, restored_from_id, created_at
) VALUES (?, ?, ?, ?, ?)
RETURNING id`, targetID, actorID, action, restoredFrom, now).Scan(&checkpointID)
	if err != nil {
		return 0, fmt.Errorf("insert sync config checkpoint: %w", err)
	}
	for _, config := range configs {
		_, err = tx.ExecContext(ctx, `
INSERT INTO sync_config_checkpoint_items (
    checkpoint_id, kind, enabled, document, digest, revision
) VALUES (?, ?, ?, ?, ?, ?)`, checkpointID, config.Kind, config.Enabled,
			config.Document, config.Digest, config.Revision)
		if err != nil {
			return 0, fmt.Errorf("insert sync config checkpoint item: %w", err)
		}
	}

	return checkpointID, nil
}

func recordSyncConfigAudit(
	ctx context.Context,
	tx *transaction,
	finish syncWriteFinish,
	checkpointID int64,
) (int64, error) {
	action := "sync.config.saved"
	verb := "Saved"
	if finish.Action == orgsync.CheckpointRestored {
		action = "sync.config.restored"
		verb = "Restored"
	}
	sourceKind := "sync_config_checkpoint"
	auditEventID, err := insertAudit(ctx, tx, auditInsert{
		TargetID: finish.TargetID, SyncConfigCheckpointID: &checkpointID,
		ActorAccountID: finish.ActorID, ElevationID: finish.ElevationID,
		SourceKind: &sourceKind, SourceID: &checkpointID,
		Action: action, Summary: syncConfigSummary(verb, finish.Changed), CreatedAt: finish.Now,
	})
	if err != nil {
		return 0, fmt.Errorf("insert sync config audit: %w", err)
	}

	return auditEventID, nil
}

func syncConfigSummary(verb string, kinds []orgsync.Kind) string {
	names := make([]string, len(kinds))
	for index, kind := range kinds {
		names[index] = string(kind)
	}
	return fmt.Sprintf("%s %s sync configuration", verb, humanList(names))
}

func humanList(values []string) string {
	switch len(values) {
	case 0:
		return ""
	case 1:
		return values[0]
	case 2:
		return values[0] + " and " + values[1]
	default:
		return strings.Join(values[:len(values)-1], ", ") + ", and " + values[len(values)-1]
	}
}
