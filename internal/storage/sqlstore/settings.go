package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

const (
	actionConfigMigration = "repository.config_migration.reset"
)

func prepareTargetSettings(
	change storage.TargetSettingsChange,
) (storage.TargetSettingsChange, string, string, error) {
	if change.PendingCIModeDefault == "" {
		change.PendingCIModeDefault = storage.PendingCIModeChecks
	}
	if len(change.PendingCIBranchPatternsDefault.Include) == 0 {
		change.PendingCIBranchPatternsDefault = storage.DefaultPendingCIBranchPatterns()
	}
	if err := storage.ValidateTargetPendingCISettings(
		change.PendingCIModeDefault,
		change.PendingCIBranchPatternsDefault,
		change.PendingCIQuietPeriodOverride,
	); err != nil {
		return storage.TargetSettingsChange{}, "", "", err
	}
	patch, err := marshalPatch(change.ConfigPatch)
	if err != nil {
		return storage.TargetSettingsChange{}, "", "", err
	}
	branchPatterns, err := marshalPendingCIBranchPatterns(change.PendingCIBranchPatternsDefault)
	if err != nil {
		return storage.TargetSettingsChange{}, "", "", err
	}

	return change, patch, branchPatterns, nil
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
	if changed {
		if err := invalidateLivePlans(ctx, tx, state.TargetID, state.ObservedAt); err != nil {
			return false, err
		}
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

func retuneTargetPendingCIQuietPeriod(
	ctx context.Context,
	tx runner,
	dialect Dialect,
	change storage.TargetSettingsChange,
) error {
	target, err := getTarget(ctx, tx, change.TargetID)
	if err != nil {
		return fmt.Errorf("read target pending CI quiet policy: %w", err)
	}
	quiet, err := inheritedPendingCIQuietPeriod(
		ctx, tx, change.DeploymentPendingCIQuietPeriod,
	)
	if err != nil {
		return err
	}
	if target.PendingCIQuietPeriodOverride != nil {
		quiet = *target.PendingCIQuietPeriodOverride
	}
	request := pendingci.RetuneQuietPeriodRequest{
		PassingQuiet: quiet, ChangedAt: change.ChangedAt,
		TargetID: change.TargetID, InheritedOnly: true,
	}
	if err := request.Validate(); err != nil {
		return err
	}
	if _, err := retuneQuietPeriod(ctx, tx, dialect, request); err != nil {
		return fmt.Errorf("retune target pending CI quiet period: %w", err)
	}

	return nil
}

func retuneRepositoryPendingCIQuietPeriod(
	ctx context.Context,
	tx runner,
	dialect Dialect,
	change storage.RepositorySettingsChange,
) error {
	target, err := getTarget(ctx, tx, change.TargetID)
	if err != nil {
		return fmt.Errorf("read repository target quiet policy: %w", err)
	}
	repository, err := getRepository(ctx, tx, change.TargetID, change.RepositoryID)
	if err != nil {
		return fmt.Errorf("read repository pending CI quiet policy: %w", err)
	}
	quiet, err := inheritedPendingCIQuietPeriod(
		ctx, tx, change.DeploymentPendingCIQuietPeriod,
	)
	if err != nil {
		return err
	}
	_, _, quiet = storage.EffectivePendingCISettings(target, repository, quiet)
	request := pendingci.RetuneQuietPeriodRequest{
		PassingQuiet: quiet, ChangedAt: change.ChangedAt,
		TargetID: change.TargetID, RepositoryID: change.RepositoryID,
	}
	if err := request.Validate(); err != nil {
		return err
	}
	if _, err := retuneQuietPeriod(ctx, tx, dialect, request); err != nil {
		return fmt.Errorf("retune repository pending CI quiet period: %w", err)
	}

	return nil
}

func inheritedPendingCIQuietPeriod(
	ctx context.Context,
	tx runner,
	deployment time.Duration,
) (time.Duration, error) {
	settings, err := getRuntimeSettings(ctx, tx)
	if errors.Is(err, sql.ErrNoRows) {
		return deployment, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read runtime pending CI quiet policy: %w", err)
	}
	if settings.PendingCIQuietPeriod != nil {
		return *settings.PendingCIQuietPeriod, nil
	}

	return deployment, nil
}
