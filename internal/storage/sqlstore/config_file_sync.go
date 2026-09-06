package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

func configFileScopeKey(repositoryID string) string {
	if repositoryID == "" {
		return "workspace"
	}
	return "repository:" + repositoryID
}

func (s *Store) GetConfigFileState(ctx context.Context, targetID, repositoryID string) (storage.ConfigFileState, error) {
	if _, _, err := configFileOwner(ctx, s.db, targetID, repositoryID); err != nil {
		return storage.ConfigFileState{}, err
	}
	return readConfigFileState(ctx, s.db, targetID, repositoryID)
}

func readConfigFileState(ctx context.Context, queryer rowQuerier, targetID, repositoryID string) (storage.ConfigFileState, error) {
	state := storage.ConfigFileState{TargetID: targetID, RepositoryID: repositoryID}
	var document string
	var updatedAt StoredTime
	err := queryer.QueryRowContext(ctx, `
SELECT revision, document, updated_at FROM config_file_connections
WHERE target_id = ? AND scope_key = ?`, targetID, configFileScopeKey(repositoryID)).Scan(
		&state.Revision, &document, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return state, nil
	}
	if err != nil {
		return storage.ConfigFileState{}, fmt.Errorf("read configuration file state: %w", err)
	}
	state.Document = []byte(document)
	state.UpdatedAt = updatedAt.Time()
	return state, nil
}

func (s *Store) SaveConfigFileState(ctx context.Context, change storage.ConfigFileStateChange) (storage.ConfigFileState, error) {
	if err := change.Validate(); err != nil {
		return storage.ConfigFileState{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.ConfigFileState{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.lockInstallationSettingsTarget(ctx, tx, change.TargetID); err != nil {
		return storage.ConfigFileState{}, err
	}
	if err := verifyConfigFileChange(ctx, tx, change); err != nil {
		return storage.ConfigFileState{}, err
	}
	if err := writeConfigFileState(ctx, tx, change); err != nil {
		return storage.ConfigFileState{}, err
	}
	if err := tx.Commit(); err != nil {
		return storage.ConfigFileState{}, err
	}
	return storage.ConfigFileState{
		TargetID: change.TargetID, RepositoryID: change.RepositoryID,
		Revision: change.ExpectedRevision + 1, Document: append([]byte(nil), change.Document...),
		UpdatedAt: change.ChangedAt,
	}, nil
}

func configFileOwner(ctx context.Context, queryer rowQuerier, targetID, repositoryID string) (bool, int64, error) {
	var enabled, available bool
	var revision int64
	var err error
	if repositoryID == "" {
		err = queryer.QueryRowContext(ctx, `
SELECT config_file_sync_enabled, revision, available FROM targets WHERE id = ?`, targetID).Scan(
			&enabled, &revision, &available,
		)
	} else {
		err = queryer.QueryRowContext(ctx, `
SELECT r.config_file_sync_enabled, r.revision, r.available AND t.available
FROM repositories r JOIN targets t ON t.id = r.target_id
WHERE r.target_id = ? AND r.id = ?`, targetID, repositoryID).Scan(&enabled, &revision, &available)
	}
	if err != nil {
		return false, 0, noRows(err)
	}
	return enabled && available, revision, nil
}

// The installation lock held by the caller serializes this check with panel
// saves and other reconciler writes, including first-row creation.
func verifyConfigFileChange(ctx context.Context, tx *transaction, change storage.ConfigFileStateChange) error {
	enabled, revision, err := configFileOwner(ctx, tx, change.TargetID, change.RepositoryID)
	if err != nil {
		return err
	}
	if !enabled || revision != change.OwnerRevision {
		return storage.ErrConflict
	}
	state, err := readConfigFileState(ctx, tx, change.TargetID, change.RepositoryID)
	if err != nil {
		return err
	}
	if state.Revision != change.ExpectedRevision {
		return storage.ErrConflict
	}
	for _, kind := range orgsync.Kinds() {
		var revision int64
		var err error
		if change.RepositoryID == "" {
			err = tx.QueryRowContext(ctx, `SELECT revision FROM sync_configs WHERE target_id = ? AND kind = ?`,
				change.TargetID, kind).Scan(&revision)
		} else {
			err = tx.QueryRowContext(ctx, `SELECT revision FROM sync_repository_overrides WHERE repository_id = ? AND kind = ?`,
				change.RepositoryID, kind).Scan(&revision)
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if revision != change.SyncRevisions[kind] {
			return storage.ErrConflict
		}
	}
	return nil
}

func writeConfigFileState(ctx context.Context, tx *transaction, change storage.ConfigFileStateChange) error {
	if change.ExpectedRevision == 0 {
		var repositoryID any
		if change.RepositoryID != "" {
			repositoryID = change.RepositoryID
		}
		_, err := tx.ExecContext(ctx, `
INSERT INTO config_file_connections (target_id, scope_key, repository_id, revision, document, updated_at)
VALUES (?, ?, ?, 1, ?, ?)`, change.TargetID, configFileScopeKey(change.RepositoryID), repositoryID,
			string(change.Document), change.ChangedAt,
		)
		return err
	}
	result, err := tx.ExecContext(ctx, `
UPDATE config_file_connections SET revision = revision + 1, document = ?, updated_at = ?
WHERE target_id = ? AND scope_key = ? AND revision = ?`, string(change.Document), change.ChangedAt,
		change.TargetID, configFileScopeKey(change.RepositoryID), change.ExpectedRevision,
	)
	if err != nil {
		return err
	}
	return checkInstallationSyncUpdate(result, "configuration file state")
}
