package sqlstore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

func (s *Store) writeInstallationSyncSettings(
	ctx context.Context,
	tx *transaction,
	request storage.SaveInstallationSettingsRequest,
	work installationSettingsWork,
) error {
	for _, config := range work.syncConfigs {
		if config.changed {
			if err := writeInstallationSyncConfig(ctx, tx, request, config); err != nil {
				return err
			}
		}
	}
	for _, override := range work.syncOverrides {
		if override.changed {
			if err := writeInstallationSyncOverride(ctx, tx, request, override); err != nil {
				return err
			}
		}
	}

	return nil
}

func writeInstallationSyncConfig(
	ctx context.Context,
	tx *transaction,
	request storage.SaveInstallationSettingsRequest,
	work syncConfigSettingsWork,
) error {
	change := work.prepared.change
	if work.prepared.remove {
		result, err := tx.ExecContext(ctx, `
DELETE FROM sync_configs
WHERE target_id = ? AND kind = ? AND revision = ?`,
			request.TargetID, change.Kind, change.ExpectedRevision,
		)
		if err != nil {
			return fmt.Errorf("remove restored installation sync config: %w", err)
		}

		return checkInstallationSyncUpdate(result, "sync config")
	}
	if work.current == nil {
		_, err := tx.ExecContext(ctx, `
INSERT INTO sync_configs (
    target_id, kind, enabled, document, digest, revision, updated_by, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			request.TargetID, change.Kind, change.Enabled, work.prepared.document,
			work.prepared.digest, work.afterRevision, request.ActorAccountID, request.ChangedAt,
		)
		if err != nil {
			return fmt.Errorf("insert installation sync config: %w", err)
		}

		return nil
	}
	result, err := tx.ExecContext(ctx, `
UPDATE sync_configs SET
    enabled = ?, document = ?, digest = ?, revision = ?, updated_by = ?, updated_at = ?
WHERE target_id = ? AND kind = ? AND revision = ?`,
		change.Enabled, work.prepared.document, work.prepared.digest, work.afterRevision,
		request.ActorAccountID, request.ChangedAt, request.TargetID, change.Kind,
		change.ExpectedRevision,
	)
	if err != nil {
		return fmt.Errorf("update installation sync config: %w", err)
	}

	return checkInstallationSyncUpdate(result, "sync config")
}

func writeInstallationSyncOverride(
	ctx context.Context,
	tx *transaction,
	request storage.SaveInstallationSettingsRequest,
	work syncOverrideSettingsWork,
) error {
	change := work.prepared.change
	if work.prepared.remove {
		result, err := tx.ExecContext(ctx, `
DELETE FROM sync_repository_overrides
WHERE repository_id = ? AND kind = ? AND revision = ?`,
			work.repository.ID, change.Kind, change.ExpectedRevision,
		)
		if err != nil {
			return fmt.Errorf("remove restored installation sync override: %w", err)
		}

		return checkInstallationSyncUpdate(result, "sync override")
	}
	if work.current == nil {
		_, err := tx.ExecContext(ctx, `
INSERT INTO sync_repository_overrides (
    repository_id, kind, enabled_override, document, revision, updated_by, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			work.repository.ID, change.Kind, change.Enabled, string(work.prepared.document),
			work.afterRevision, request.ActorAccountID, request.ChangedAt,
		)
		if err != nil {
			return fmt.Errorf("insert installation sync override: %w", err)
		}

		return nil
	}
	result, err := tx.ExecContext(ctx, `
UPDATE sync_repository_overrides SET
    enabled_override = ?, document = ?, revision = ?, updated_by = ?, updated_at = ?
WHERE repository_id = ? AND kind = ? AND revision = ?`,
		change.Enabled, string(work.prepared.document), work.afterRevision,
		request.ActorAccountID, request.ChangedAt, work.repository.ID, change.Kind,
		change.ExpectedRevision,
	)
	if err != nil {
		return fmt.Errorf("update installation sync override: %w", err)
	}

	return checkInstallationSyncUpdate(result, "sync override")
}

func checkInstallationSyncUpdate(result sql.Result, resource string) error {
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read installation %s update result: %w", resource, err)
	}
	if changed != 1 {
		return storage.ErrConflict
	}

	return nil
}

func appendInstallationSyncSettingsResult(
	result *storage.SaveInstallationSettingsResult,
	work installationSettingsWork,
) {
	result.SyncConfigs = make([]orgsync.Config, 0, len(work.syncConfigs))
	for _, config := range work.syncConfigs {
		if config.current != nil {
			result.SyncConfigs = append(result.SyncConfigs, *config.current)
		}
	}
	result.SyncOverrides = make([]orgsync.RepositoryOverride, 0, len(work.syncOverrides))
	for _, override := range work.syncOverrides {
		if override.current != nil {
			result.SyncOverrides = append(result.SyncOverrides, *override.current)
		}
	}
}

func (s *Store) readInstallationSyncSettingsResult(
	ctx context.Context,
	tx *transaction,
	prepared preparedInstallationSettings,
	result *storage.SaveInstallationSettingsResult,
) error {
	result.SyncConfigs = make([]orgsync.Config, 0, len(prepared.syncConfigs))
	for _, config := range prepared.syncConfigs {
		if config.remove {
			continue
		}
		read, err := syncConfigForUpdate(
			ctx, tx, s.dialect, prepared.request.TargetID, config.change.Kind,
		)
		if err != nil {
			return fmt.Errorf("read saved sync config: %w", err)
		}
		result.SyncConfigs = append(result.SyncConfigs, read)
	}
	result.SyncOverrides = make([]orgsync.RepositoryOverride, 0, len(prepared.syncOverrides))
	for _, override := range prepared.syncOverrides {
		if override.remove {
			continue
		}
		read, err := syncOverrideForUpdate(
			ctx, tx, s.dialect, override.change.RepositoryID, override.change.Kind,
		)
		if err != nil {
			return fmt.Errorf("read saved sync override: %w", err)
		}
		result.SyncOverrides = append(result.SyncOverrides, read)
	}

	return nil
}
