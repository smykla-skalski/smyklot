package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

const settingsCheckpointColumns = `
    id, scope, target_id, actor_account_id, action, restored_from_id, restored_side, created_at`

const settingsCheckpointSourceKind = "settings_checkpoint"

// createSettingsCheckpoint is the transaction-level primitive the settings
// coordinator uses after it has written every changed aggregate.
func (s *Store) createSettingsCheckpoint(
	ctx context.Context,
	tx *transaction,
	create storage.SettingsCheckpointCreate,
) (int64, error) {
	if err := create.Validate(); err != nil {
		return 0, err
	}
	if err := validateSettingsRestoreSource(ctx, tx, create); err != nil {
		return 0, err
	}

	var targetID any
	if create.TargetID != "" {
		targetID = create.TargetID
	}
	var checkpointID int64
	err := tx.QueryRowContext(ctx, `
INSERT INTO settings_checkpoints (
    scope, target_id, actor_account_id, action, restored_from_id, restored_side, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING id`, create.Scope, targetID, create.ActorAccountID, create.Action,
		create.RestoredFromID, optionalRestoreSide(create.RestoredSide), create.CreatedAt).
		Scan(&checkpointID)
	if err != nil {
		return 0, fmt.Errorf("insert settings checkpoint: %w", err)
	}

	items := append([]storage.SettingsCheckpointItem(nil), create.Items...)
	sort.Slice(items, func(left, right int) bool {
		return settingsCheckpointItemKey(items[left]) < settingsCheckpointItemKey(items[right])
	})
	for _, item := range items {
		if err := insertSettingsCheckpointItem(ctx, tx, checkpointID, item); err != nil {
			return 0, err
		}
	}

	return checkpointID, nil
}

func validateSettingsRestoreSource(
	ctx context.Context,
	tx *transaction,
	create storage.SettingsCheckpointCreate,
) error {
	if create.RestoredFromID == nil {
		return nil
	}
	source, err := getSettingsCheckpointHeader(ctx, tx, storage.SettingsCheckpointRef{
		ID: *create.RestoredFromID, Scope: create.Scope, TargetID: create.TargetID,
	})
	if err != nil {
		return fmt.Errorf("read settings restore source: %w", err)
	}
	if source.Action == storage.SettingsCheckpointActionBaseline &&
		create.RestoredSide == storage.SettingsCheckpointRestoreBefore {
		return fmt.Errorf("%w: baseline has no before state", storage.ErrSettingsRestoreBlocked)
	}

	return nil
}

func optionalRestoreSide(side storage.SettingsCheckpointRestoreSide) any {
	if side == "" {
		return nil
	}

	return side
}

func insertSettingsCheckpointItem(
	ctx context.Context,
	tx *transaction,
	checkpointID int64,
	item storage.SettingsCheckpointItem,
) error {
	beforeDocument, beforeRevision, beforeDigest := settingsCheckpointStateValues(item.Before)
	afterDocument, afterRevision, afterDigest := settingsCheckpointStateValues(item.After)
	_, err := tx.ExecContext(ctx, `
INSERT INTO settings_checkpoint_items (
    checkpoint_id, item_kind, repository_id, repository_full_name, sync_kind,
    document_version,
    before_document, before_revision, before_digest,
    after_document, after_revision, after_digest
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		checkpointID, item.Kind, item.RepositoryID, item.RepositoryFullName, item.SyncKind,
		item.DocumentVersion,
		beforeDocument, beforeRevision, beforeDigest,
		afterDocument, afterRevision, afterDigest,
	)
	if err != nil {
		return fmt.Errorf("insert settings checkpoint item: %w", err)
	}

	return nil
}

func settingsCheckpointStateValues(state *storage.SettingsCheckpointState) (any, any, any) {
	if state == nil {
		return nil, nil, nil
	}

	return string(state.Document), state.Revision, state.Digest
}

func getSettingsCheckpoint(
	ctx context.Context,
	queryer runner,
	ref storage.SettingsCheckpointRef,
) (storage.SettingsCheckpoint, error) {
	checkpoint, err := getSettingsCheckpointHeader(ctx, queryer, ref)
	if err != nil {
		return storage.SettingsCheckpoint{}, err
	}

	rows, err := queryer.QueryContext(ctx, `
SELECT item_kind, repository_id, repository_full_name, sync_kind, document_version,
       before_document, before_revision, before_digest,
       after_document, after_revision, after_digest
FROM settings_checkpoint_items
WHERE checkpoint_id = ?
ORDER BY item_kind, repository_id, sync_kind`, checkpoint.ID)
	if err != nil {
		return storage.SettingsCheckpoint{}, fmt.Errorf("list settings checkpoint items: %w", err)
	}
	checkpoint.Items, err = collectRows(rows, scanSettingsCheckpointItem)
	if err != nil {
		return storage.SettingsCheckpoint{}, fmt.Errorf("read settings checkpoint items: %w", err)
	}
	if err := checkpointCreate(checkpoint).Validate(); err != nil {
		return storage.SettingsCheckpoint{}, fmt.Errorf(
			"%w: validate stored settings checkpoint %d: %v",
			storage.ErrSettingsCheckpointCorrupt, checkpoint.ID, err,
		)
	}

	return checkpoint, nil
}

func getSettingsCheckpointHeader(
	ctx context.Context,
	queryer rowQuerier,
	ref storage.SettingsCheckpointRef,
) (storage.SettingsCheckpoint, error) {
	if err := ref.Validate(); err != nil {
		return storage.SettingsCheckpoint{}, err
	}

	var row *sql.Row
	if ref.Scope == storage.SettingsCheckpointScopeRoot {
		row = queryer.QueryRowContext(ctx, `
SELECT`+settingsCheckpointColumns+`
FROM settings_checkpoints
WHERE id = ? AND scope = ? AND target_id IS NULL`, ref.ID, ref.Scope)
	} else {
		row = queryer.QueryRowContext(ctx, `
SELECT`+settingsCheckpointColumns+`
FROM settings_checkpoints
WHERE id = ? AND scope = ? AND target_id = ?`, ref.ID, ref.Scope, ref.TargetID)
	}
	checkpoint, err := scanSettingsCheckpoint(row)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.SettingsCheckpoint{}, storage.ErrNotFound
	}
	if err != nil {
		return storage.SettingsCheckpoint{}, fmt.Errorf("read settings checkpoint: %w", err)
	}

	return checkpoint, nil
}

func scanSettingsCheckpoint(scanner rowScanner) (storage.SettingsCheckpoint, error) {
	var checkpoint storage.SettingsCheckpoint
	var targetID sql.NullString
	var restoredFrom sql.NullInt64
	var restoredSide sql.NullString
	var createdAt StoredTime
	if err := scanner.Scan(
		&checkpoint.ID, &checkpoint.Scope, &targetID, &checkpoint.ActorAccountID,
		&checkpoint.Action, &restoredFrom, &restoredSide, &createdAt,
	); err != nil {
		return storage.SettingsCheckpoint{}, err
	}
	if targetID.Valid {
		checkpoint.TargetID = targetID.String
	}
	if restoredFrom.Valid {
		checkpoint.RestoredFromID = &restoredFrom.Int64
	}
	if restoredSide.Valid {
		checkpoint.RestoredSide = storage.SettingsCheckpointRestoreSide(restoredSide.String)
	}
	checkpoint.CreatedAt = createdAt.Time()

	return checkpoint, nil
}

func scanSettingsCheckpointItem(scanner rowScanner) (storage.SettingsCheckpointItem, error) {
	var item storage.SettingsCheckpointItem
	var beforeDocument, beforeDigest, afterDocument, afterDigest sql.NullString
	var beforeRevision, afterRevision sql.NullInt64
	if err := scanner.Scan(
		&item.Kind, &item.RepositoryID, &item.RepositoryFullName, &item.SyncKind,
		&item.DocumentVersion,
		&beforeDocument, &beforeRevision, &beforeDigest,
		&afterDocument, &afterRevision, &afterDigest,
	); err != nil {
		return storage.SettingsCheckpointItem{}, err
	}
	before, err := scanSettingsCheckpointState(beforeDocument, beforeRevision, beforeDigest)
	if err != nil {
		return storage.SettingsCheckpointItem{}, fmt.Errorf("scan before state: %w", err)
	}
	after, err := scanSettingsCheckpointState(afterDocument, afterRevision, afterDigest)
	if err != nil {
		return storage.SettingsCheckpointItem{}, fmt.Errorf("scan after state: %w", err)
	}
	item.Before = before
	item.After = after

	return item, nil
}

func scanSettingsCheckpointState(
	document sql.NullString,
	revision sql.NullInt64,
	digest sql.NullString,
) (*storage.SettingsCheckpointState, error) {
	if !document.Valid && !revision.Valid && !digest.Valid {
		return nil, nil
	}
	if !document.Valid || !revision.Valid || !digest.Valid {
		return nil, fmt.Errorf("%w: checkpoint state is incomplete",
			storage.ErrSettingsCheckpointCorrupt)
	}

	return &storage.SettingsCheckpointState{
		Document: []byte(document.String), Revision: revision.Int64, Digest: digest.String,
	}, nil
}

func checkpointCreate(checkpoint storage.SettingsCheckpoint) storage.SettingsCheckpointCreate {
	return storage.SettingsCheckpointCreate{
		Scope: checkpoint.Scope, TargetID: checkpoint.TargetID,
		ActorAccountID: checkpoint.ActorAccountID, Action: checkpoint.Action,
		RestoredFromID: checkpoint.RestoredFromID, RestoredSide: checkpoint.RestoredSide,
		CreatedAt: checkpoint.CreatedAt,
		Items:     checkpoint.Items,
	}
}

func settingsCheckpointItemKey(item storage.SettingsCheckpointItem) string {
	return fmt.Sprintf("%s\x00%s\x00%s", item.Kind, item.RepositoryID, item.SyncKind)
}
