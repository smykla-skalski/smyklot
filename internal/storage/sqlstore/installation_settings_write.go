package sqlstore

import (
	"context"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

func (s *Store) applyInstallationSettings(
	ctx context.Context,
	tx *transaction,
	request storage.SaveInstallationSettingsRequest,
	work installationSettingsWork,
) error {
	if work.target != nil && work.target.changed {
		if err := writeTargetSettings(ctx, tx, *work.target); err != nil {
			return err
		}
	}
	for _, repository := range work.repositories {
		if repository.changed {
			if err := writeRepositorySettings(ctx, tx, repository); err != nil {
				return err
			}
		}
	}

	if err := ensurePendingCIGates(ctx, tx, request.TargetID, request.ChangedAt); err != nil {
		return err
	}
	if err := s.retuneInstallationSettings(ctx, tx, work); err != nil {
		return err
	}
	if err := wakeInstallationSettings(ctx, tx, request, work); err != nil {
		return err
	}
	// These are the two inclusion selectors held by the target/repository
	// aggregates. Pending CI policy and command configuration do not decide a
	// Sync plan's repository set, so changing them leaves a live plan usable.
	if work.inclusionChanged {
		if err := invalidateLivePlans(ctx, tx, request.TargetID, request.ChangedAt); err != nil {
			return err
		}
	}

	return nil
}

func writeTargetSettings(
	ctx context.Context,
	tx *transaction,
	work targetSettingsWork,
) error {
	change := work.prepared.change
	result, err := tx.ExecContext(ctx, `
UPDATE targets SET
    repository_default_enabled = ?,
    pending_ci_mode_default = ?,
    pending_ci_branch_patterns_default = ?,
    pending_ci_quiet_period_seconds_override = ?,
    path_index_interval_seconds_override = ?,
    config_patch = ?,
    revision = revision + 1,
    settings_updated_at = ?
WHERE id = ? AND revision = ?`,
		change.RepositoryDefaultEnabled,
		change.PendingCIModeDefault,
		work.prepared.branchPatterns,
		durationSeconds(change.PendingCIQuietPeriodOverride),
		durationSeconds(change.PathIndexIntervalOverride),
		work.prepared.patch,
		change.ChangedAt,
		change.TargetID,
		change.ExpectedRevision,
	)
	if err != nil {
		return fmt.Errorf("update target settings: %w", err)
	}

	return checkTargetUpdate(ctx, tx, result, change.TargetID)
}

func writeRepositorySettings(
	ctx context.Context,
	tx *transaction,
	work repositorySettingsWork,
) error {
	change := work.prepared.change
	result, err := tx.ExecContext(ctx, `
UPDATE repositories SET
    enabled_override = ?,
    pending_ci_mode_override = ?,
    pending_ci_branch_patterns_override = ?,
    pending_ci_quiet_period_seconds_override = ?,
    path_index_interval_seconds_override = ?,
    config_patch = ?,
    ignore_repository_file = ?,
    revision = revision + 1,
    settings_updated_at = ?
WHERE target_id = ? AND id = ? AND revision = ?`,
		change.EnabledOverride,
		change.PendingCIModeOverride,
		work.prepared.branchPatterns,
		durationSeconds(change.PendingCIQuietPeriodOverride),
		durationSeconds(change.PathIndexIntervalOverride),
		work.prepared.patch,
		change.IgnoreRepositoryFile,
		change.ChangedAt,
		change.TargetID,
		change.RepositoryID,
		change.ExpectedRevision,
	)
	if err != nil {
		return fmt.Errorf("update repository settings: %w", err)
	}

	return checkRepositoryUpdate(ctx, tx, result, change.TargetID, change.RepositoryID)
}

func (s *Store) retuneInstallationSettings(
	ctx context.Context,
	tx *transaction,
	work installationSettingsWork,
) error {
	if work.target != nil && work.target.changed &&
		work.target.prepared.change.RetunePendingCIQuietPeriod {
		if err := retuneTargetPendingCIQuietPeriod(
			ctx, tx, s.dialect, work.target.prepared.change,
		); err != nil {
			return err
		}
	}
	for _, repository := range work.repositories {
		if !repository.changed || !repository.prepared.change.RetunePendingCIQuietPeriod {
			continue
		}
		if err := retuneRepositoryPendingCIQuietPeriod(
			ctx, tx, s.dialect, repository.prepared.change,
		); err != nil {
			return err
		}
	}

	return nil
}

func wakeInstallationSettings(
	ctx context.Context,
	tx *transaction,
	request storage.SaveInstallationSettingsRequest,
	work installationSettingsWork,
) error {
	if work.target != nil && work.target.changed {
		// A target wake covers every repository in this batch. Repeating a
		// repository wake would advance the same request twice for one save.
		return wakePendingCIRequestsForTarget(ctx, tx, request.TargetID, request.ChangedAt)
	}
	for _, repository := range work.repositories {
		if repository.changed {
			if err := wakePendingCIRequestsForRepository(
				ctx, tx, repository.current.ID, request.ChangedAt,
			); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *Store) recordInstallationSettings(
	ctx context.Context,
	tx *transaction,
	request storage.SaveInstallationSettingsRequest,
	work installationSettingsWork,
) (int64, int64, error) {
	checkpointID, err := s.createSettingsCheckpoint(ctx, tx, storage.SettingsCheckpointCreate{
		Scope: storage.SettingsCheckpointScopeInstallation, TargetID: request.TargetID,
		ActorAccountID: request.ActorAccountID, Action: storage.SettingsCheckpointActionSave,
		CreatedAt: request.ChangedAt, Items: work.items,
	})
	if err != nil {
		return 0, 0, err
	}
	action, summary, repositoryID, repositoryFullName := installationSettingsAudit(work)
	sourceKind := "settings_checkpoint"
	auditEventID, err := insertAudit(ctx, tx, auditInsert{
		TargetID: request.TargetID, RepositoryID: repositoryID,
		RepositoryFullName: repositoryFullName, SettingsCheckpointID: &checkpointID,
		ActorAccountID: request.ActorAccountID, ElevationID: request.ElevationID,
		SourceKind: &sourceKind, SourceID: &checkpointID,
		Action: action, Summary: summary, CreatedAt: request.ChangedAt,
	})
	if err != nil {
		return 0, 0, err
	}

	return checkpointID, auditEventID, nil
}

func installationSettingsAudit(
	work installationSettingsWork,
) (string, string, *string, *string) {
	if len(work.items) == 1 && work.target != nil && work.target.changed {
		return actionTargetSettings, "Updated account defaults", nil, nil
	}
	if len(work.items) == 1 {
		for _, repository := range work.repositories {
			if repository.changed {
				id := repository.current.ID
				fullName := repository.current.FullName

				return actionRepositorySettings, "Updated repository settings", &id, &fullName
			}
		}
	}

	return actionInstallationSettings,
		fmt.Sprintf("Updated %d installation settings", len(work.items)), nil, nil
}

func installationSettingsResult(
	work installationSettingsWork,
) storage.SaveInstallationSettingsResult {
	result := storage.SaveInstallationSettingsResult{
		Repositories: make([]storage.Repository, 0, len(work.repositories)),
	}
	if work.target != nil {
		target := work.target.current
		result.Target = &target
	}
	for _, repository := range work.repositories {
		result.Repositories = append(result.Repositories, repository.current)
	}

	return result
}

func readInstallationSettingsResult(
	ctx context.Context,
	tx *transaction,
	prepared preparedInstallationSettings,
) (storage.SaveInstallationSettingsResult, error) {
	result := storage.SaveInstallationSettingsResult{
		Repositories: make([]storage.Repository, 0, len(prepared.repositories)),
	}
	if prepared.target != nil {
		target, err := getTarget(ctx, tx, prepared.request.TargetID)
		if err != nil {
			return storage.SaveInstallationSettingsResult{}, fmt.Errorf(
				"read saved target settings: %w", err,
			)
		}
		result.Target = &target
	}
	for _, repository := range prepared.repositories {
		read, err := getRepository(
			ctx, tx, prepared.request.TargetID, repository.change.RepositoryID,
		)
		if err != nil {
			return storage.SaveInstallationSettingsResult{}, fmt.Errorf(
				"read saved repository settings: %w", err,
			)
		}
		result.Repositories = append(result.Repositories, read)
	}

	return result, nil
}
