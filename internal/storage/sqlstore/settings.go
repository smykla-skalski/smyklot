package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

const (
	actionTargetSettings     = "target.settings.updated"
	actionRepositorySettings = "repository.settings.updated"
	actionConfigMigration    = "repository.config_migration.reset"
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

	elevation, err := s.elevatedWrite(
		ctx,
		tx,
		change.ElevationID,
		change.SessionTokenHash,
		change.ActorAccountID,
		change.TargetID,
		change.ChangedAt,
	)
	if err != nil {
		return storage.Target{}, err
	}

	result, err := tx.ExecContext(ctx, `
UPDATE targets SET
    repository_default_enabled = ?,
    config_patch = ?,
    revision = revision + 1,
    settings_updated_at = ?
WHERE id = ? AND revision = ?`,
		change.RepositoryDefaultEnabled,
		patch,
		change.ChangedAt,
		change.TargetID,
		change.ExpectedRevision,
	)
	if err != nil {
		return storage.Target{}, fmt.Errorf("update target settings: %w", err)
	}

	if err := checkTargetUpdate(ctx, tx, result, change.TargetID); err != nil {
		return storage.Target{}, err
	}

	auditEventID, err := insertAudit(ctx, tx, auditInsert{
		TargetID:       change.TargetID,
		ActorAccountID: change.ActorAccountID,
		ElevationID:    change.ElevationID,
		Action:         actionTargetSettings,
		Summary:        "Updated account defaults",
		CreatedAt:      change.ChangedAt,
	})
	if err != nil {
		return storage.Target{}, err
	}
	if elevation != nil {
		if err := insertElevatedNotifications(
			ctx, tx, *elevation, auditEventID, actionTargetSettings, change.ChangedAt,
		); err != nil {
			return storage.Target{}, err
		}
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

	elevation, err := s.elevatedWrite(
		ctx,
		tx,
		change.ElevationID,
		change.SessionTokenHash,
		change.ActorAccountID,
		change.TargetID,
		change.ChangedAt,
	)
	if err != nil {
		return storage.Repository{}, err
	}

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

	auditEventID, err := insertAudit(ctx, tx, auditInsert{
		TargetID:           change.TargetID,
		RepositoryID:       &change.RepositoryID,
		RepositoryFullName: &repository.FullName,
		ActorAccountID:     change.ActorAccountID,
		ElevationID:        change.ElevationID,
		Action:             actionRepositorySettings,
		Summary:            "Updated repository settings",
		CreatedAt:          change.ChangedAt,
	})
	if err != nil {
		return storage.Repository{}, err
	}
	if elevation != nil {
		if err := insertElevatedNotifications(
			ctx, tx, *elevation, auditEventID, actionRepositorySettings, change.ChangedAt,
		); err != nil {
			return storage.Repository{}, err
		}
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
	superseded, err := marshalPaths(state.Superseded)
	if err != nil {
		return false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin repository file state update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var currentStatus, currentPatch, currentPath, currentSuperseded string
	var currentError sql.NullString
	if err := tx.QueryRowContext(ctx, `
SELECT config_file_status, config_file_patch, config_file_error,
       config_file_path, config_file_superseded
FROM repositories
WHERE target_id = ? AND id = ?`, state.TargetID, state.RepositoryID).Scan(
		&currentStatus,
		&currentPatch,
		&currentError,
		&currentPath,
		&currentSuperseded,
	); err != nil {
		return false, fmt.Errorf("read repository file state: %w", noRows(err))
	}

	// The path and the superseded list are part of "changed" because the panel
	// is told to refresh on the strength of this. A repository that moved its
	// file from the legacy path to a TOML one changes neither its status nor
	// its patch, and a panel left un-announced would keep naming the old file
	changed := currentStatus != string(state.Status) ||
		currentPatch != patch ||
		currentPath != state.Path ||
		currentSuperseded != superseded ||
		currentError.Valid != (state.Error != nil) ||
		(state.Error != nil && currentError.String != *state.Error)

	_, err = tx.ExecContext(ctx, `
UPDATE repositories SET
    config_file_status = ?,
    config_file_patch = ?,
    config_file_error = ?,
    config_file_path = ?,
    config_file_superseded = ?,
    file_observed_at = ?
WHERE target_id = ? AND id = ?`,
		state.Status,
		patch,
		state.Error,
		state.Path,
		superseded,
		state.ObservedAt,
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

// SetRepositoryConfigMigration records how far the move to TOML has got.
//
// Outside the revision guard the panel's own settings sit behind, because this
// is not a setting anyone chose: it is what the service observed of a pull
// request it opened, and making it contend for the same revision would let a
// sweep tick fail somebody's save.
func (s *Store) SetRepositoryConfigMigration(
	ctx context.Context,
	migration storage.RepositoryConfigMigration,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin repository config migration update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var elevation *storage.Elevation
	if migration.ActorAccountID != nil {
		elevation, err = s.elevatedWrite(
			ctx,
			tx,
			migration.ElevationID,
			migration.SessionTokenHash,
			*migration.ActorAccountID,
			migration.TargetID,
			migration.ChangedAt,
		)
		if err != nil {
			return err
		}
	}

	changed, err := updateRepositoryConfigMigration(ctx, tx, migration)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}

	// Only when somebody decided. The sweep observed a pull request rather than
	// choosing anything, and audit_entries wants an account - inventing one to
	// fill the column would put a person's name on a machine's observation.
	if migration.ActorAccountID != nil {
		repository, err := getRepository(ctx, tx, migration.TargetID, migration.RepositoryID)
		if err != nil {
			return fmt.Errorf("read repository for audit: %w", err)
		}
		auditEventID, err := insertAudit(ctx, tx, auditInsert{
			TargetID:           migration.TargetID,
			RepositoryID:       &migration.RepositoryID,
			RepositoryFullName: &repository.FullName,
			ActorAccountID:     *migration.ActorAccountID,
			ElevationID:        migration.ElevationID,
			Action:             actionConfigMigration,
			Summary:            "Allowed Smyklot to propose the TOML migration again",
			CreatedAt:          migration.ChangedAt,
		})
		if err != nil {
			return err
		}
		if elevation != nil {
			if err := insertElevatedNotifications(
				ctx, tx, *elevation, auditEventID, actionConfigMigration, migration.ChangedAt,
			); err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit repository config migration update: %w", err)
	}

	return nil
}

func updateRepositoryConfigMigration(
	ctx context.Context,
	tx runner,
	migration storage.RepositoryConfigMigration,
) (bool, error) {
	query := `
UPDATE repositories SET
    config_migration = ?,
    config_migration_pr = ?
WHERE target_id = ? AND id = ?`
	arguments := []any{
		migration.State,
		migration.PullRequest,
		migration.TargetID,
		migration.RepositoryID,
	}
	if migration.ActorAccountID != nil {
		if migration.State != storage.ConfigMigrationNone || migration.PullRequest != nil {
			return false, storage.ErrConflict
		}
		query += " AND config_migration IN (?, ?)"
		arguments = append(
			arguments,
			storage.ConfigMigrationDeclined,
			storage.ConfigMigrationBlocked,
		)
	}
	result, err := tx.ExecContext(ctx, query, arguments...)
	if err != nil {
		return false, fmt.Errorf("update repository config migration: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("update repository config migration: %w", err)
	}
	if affected != 0 {
		return true, nil
	}

	var current storage.ConfigMigrationState
	err = tx.QueryRowContext(ctx, `
SELECT config_migration FROM repositories WHERE target_id = ? AND id = ?`,
		migration.TargetID,
		migration.RepositoryID,
	).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return false, storage.ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("classify repository config migration update: %w", err)
	}
	if migration.ActorAccountID != nil && current == storage.ConfigMigrationNone {
		return false, nil
	}

	return false, storage.ErrConflict
}

func updateRepositorySettings(
	ctx context.Context,
	tx runner,
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
		change.ChangedAt,
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
	tx runner,
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
	tx runner,
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
	ElevationID        *string
	Action             string
	Summary            string
	CreatedAt          time.Time
}

func insertAudit(ctx context.Context, tx runner, entry auditInsert) (int64, error) {
	var sourceID int64
	err := tx.QueryRowContext(ctx, `
INSERT INTO audit_entries (
    target_id, repository_id, repository_full_name,
    actor_account_id, action, summary, created_at
)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING id`,
		entry.TargetID,
		entry.RepositoryID,
		entry.RepositoryFullName,
		entry.ActorAccountID,
		entry.Action,
		entry.Summary,
		entry.CreatedAt,
	).Scan(&sourceID)
	if err != nil {
		return 0, fmt.Errorf("insert settings audit: %w", err)
	}
	sourceKind := "settings"
	targetID := entry.TargetID
	auditEventID, err := insertAppAudit(ctx, tx, appAuditInsert{
		Category:       "configuration",
		SourceKind:     &sourceKind,
		SourceID:       &sourceID,
		TargetID:       &targetID,
		ActorAccountID: entry.ActorAccountID,
		ElevationID:    entry.ElevationID,
		Action:         entry.Action,
		Summary:        entry.Summary,
		CreatedAt:      entry.CreatedAt,
	})
	if err != nil {
		return 0, err
	}

	return auditEventID, nil
}
