package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

const (
	actionTargetSettings     = "target.settings.updated"
	actionRepositorySettings = "repository.settings.updated"
)

// UpdateTargetSettings changes target defaults and appends immutable audit in
// the same transaction.
func (s *Store) UpdateTargetSettings(
	ctx context.Context,
	change storage.TargetSettingsChange,
) (storage.Target, error) {
	patch, err := marshalPatch(change.ConfigPatch)
	if err != nil {
		return storage.Target{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.Target{}, fmt.Errorf("begin target settings update: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `
UPDATE targets SET
    repository_default_enabled = ?,
    config_patch = ?,
    revision = revision + 1,
    settings_updated_at = ?
WHERE id = ? AND revision = ?`,
		change.RepositoryDefaultEnabled,
		patch,
		formatTime(change.ChangedAt),
		change.TargetID,
		change.ExpectedRevision,
	)
	if err != nil {
		return storage.Target{}, fmt.Errorf("update target settings: %w", err)
	}

	if err := checkTargetUpdate(ctx, tx, result, change.TargetID); err != nil {
		return storage.Target{}, err
	}

	if err := insertAudit(ctx, tx, auditInsert{
		TargetID:       change.TargetID,
		ActorAccountID: change.ActorAccountID,
		Action:         actionTargetSettings,
		Summary:        "Updated account defaults",
		CreatedAt:      formatTime(change.ChangedAt),
	}); err != nil {
		return storage.Target{}, err
	}

	target, err := getTarget(ctx, tx, change.TargetID)
	if err != nil {
		return storage.Target{}, fmt.Errorf("read updated target: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return storage.Target{}, fmt.Errorf("commit target settings update: %w", err)
	}

	return target, nil
}

// UpdateRepositorySettings changes local controls and appends immutable audit
// in the same transaction.
func (s *Store) UpdateRepositorySettings(
	ctx context.Context,
	change storage.RepositorySettingsChange,
) (storage.Repository, error) {
	patch, err := marshalPatch(change.ConfigPatch)
	if err != nil {
		return storage.Repository{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.Repository{}, fmt.Errorf("begin repository settings update: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	result, err := updateRepositorySettings(ctx, tx, change, patch)
	if err != nil {
		return storage.Repository{}, err
	}

	if err := checkRepositoryUpdate(ctx, tx, result, change.TargetID, change.RepositoryID); err != nil {
		return storage.Repository{}, err
	}

	repository, err := getRepository(ctx, tx, change.TargetID, change.RepositoryID)
	if err != nil {
		return storage.Repository{}, fmt.Errorf("read repository for audit: %w", err)
	}

	if err := insertAudit(ctx, tx, auditInsert{
		TargetID:           change.TargetID,
		RepositoryID:       &change.RepositoryID,
		RepositoryFullName: &repository.FullName,
		ActorAccountID:     change.ActorAccountID,
		Action:             actionRepositorySettings,
		Summary:            "Updated repository settings",
		CreatedAt:          formatTime(change.ChangedAt),
	}); err != nil {
		return storage.Repository{}, err
	}

	if err := tx.Commit(); err != nil {
		return storage.Repository{}, fmt.Errorf("commit repository settings update: %w", err)
	}

	return repository, nil
}

// UpdateRepositoryFileState records a repository-file observation without
// changing panel settings or their optimistic revision.
func (s *Store) UpdateRepositoryFileState(
	ctx context.Context,
	state storage.RepositoryFileState,
) (bool, error) {
	patch, err := marshalPatch(state.Patch)
	if err != nil {
		return false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin repository file state update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var currentStatus, currentPatch string
	var currentError sql.NullString
	if err := tx.QueryRowContext(ctx, `
SELECT config_file_status, config_file_patch, config_file_error
FROM repositories
WHERE target_id = ? AND id = ?`, state.TargetID, state.RepositoryID).Scan(
		&currentStatus,
		&currentPatch,
		&currentError,
	); err != nil {
		return false, fmt.Errorf("read repository file state: %w", noRows(err))
	}
	changed := currentStatus != string(state.Status) ||
		currentPatch != patch ||
		currentError.Valid != (state.Error != nil) ||
		(state.Error != nil && currentError.String != *state.Error)

	_, err = tx.ExecContext(ctx, `
UPDATE repositories SET
    config_file_status = ?,
    config_file_patch = ?,
    config_file_error = ?,
    file_observed_at = ?
WHERE target_id = ? AND id = ?`,
		state.Status,
		patch,
		state.Error,
		formatTime(state.ObservedAt),
		state.TargetID,
		state.RepositoryID,
	)
	if err != nil {
		return false, fmt.Errorf("update repository file state: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit repository file state update: %w", err)
	}

	return changed, nil
}

func updateRepositorySettings(
	ctx context.Context,
	tx *sql.Tx,
	change storage.RepositorySettingsChange,
	patch string,
) (sql.Result, error) {
	result, err := tx.ExecContext(ctx, `
UPDATE repositories SET
    enabled_override = ?,
    config_patch = ?,
    ignore_repository_file = ?,
    revision = revision + 1,
    settings_updated_at = ?
WHERE target_id = ? AND id = ? AND revision = ?`,
		change.EnabledOverride,
		patch,
		change.IgnoreRepositoryFile,
		formatTime(change.ChangedAt),
		change.TargetID,
		change.RepositoryID,
		change.ExpectedRevision,
	)
	if err != nil {
		return nil, fmt.Errorf("update repository settings: %w", err)
	}

	return result, nil
}

func checkTargetUpdate(
	ctx context.Context,
	tx *sql.Tx,
	result sql.Result,
	targetID string,
) error {
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read target update result: %w", err)
	}

	if changed != 0 {
		return nil
	}

	var exists int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM targets WHERE id = ?", targetID).Scan(&exists); err != nil {
		return fmt.Errorf("classify target update: %w", err)
	}

	if exists == 0 {
		return storage.ErrNotFound
	}

	return storage.ErrConflict
}

func checkRepositoryUpdate(
	ctx context.Context,
	tx *sql.Tx,
	result sql.Result,
	targetID, repositoryID string,
) error {
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read repository update result: %w", err)
	}

	if changed != 0 {
		return nil
	}

	var exists int
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM repositories WHERE target_id = ? AND id = ?`,
		targetID,
		repositoryID,
	).Scan(&exists); err != nil {
		return fmt.Errorf("classify repository update: %w", err)
	}

	if exists == 0 {
		return storage.ErrNotFound
	}

	return storage.ErrConflict
}

type auditInsert struct {
	TargetID           string
	RepositoryID       *string
	RepositoryFullName *string
	ActorAccountID     string
	Action             string
	Summary            string
	CreatedAt          string
}

func insertAudit(ctx context.Context, tx *sql.Tx, entry auditInsert) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO audit_entries (
    target_id, repository_id, repository_full_name,
    actor_account_id, action, summary, created_at
)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.TargetID,
		entry.RepositoryID,
		entry.RepositoryFullName,
		entry.ActorAccountID,
		entry.Action,
		entry.Summary,
		entry.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert settings audit: %w", err)
	}

	return nil
}
