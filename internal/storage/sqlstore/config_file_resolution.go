package sqlstore

import (
	"context"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

const actionConfigFileResolved = "configuration_file.resolved"

func (s *Store) SaveConfigFileResolution(ctx context.Context, request storage.ConfigFileResolution) (storage.ConfigFileState, error) {
	if err := request.Validate(); err != nil {
		return storage.ConfigFileState{}, err
	}
	change := request.State
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
	elevation, err := s.elevatedWrite(ctx, tx, request.ElevationID, request.SessionTokenHash,
		request.ActorAccountID, change.TargetID, change.ChangedAt)
	if err != nil {
		return storage.ConfigFileState{}, err
	}
	if err := writeConfigFileState(ctx, tx, change); err != nil {
		return storage.ConfigFileState{}, err
	}
	auditEventID, err := recordConfigFileResolution(ctx, tx, request)
	if err != nil {
		return storage.ConfigFileState{}, err
	}
	if elevation != nil {
		if err := insertElevatedNotifications(ctx, tx, *elevation, auditEventID, actionConfigFileResolved, change.ChangedAt); err != nil {
			return storage.ConfigFileState{}, err
		}
	}
	if err := notifyConfigFileChange(ctx, tx, change.TargetID, change.RepositoryID, change.ChangedAt); err != nil {
		return storage.ConfigFileState{}, err
	}
	state, err := readConfigFileState(ctx, tx, change.TargetID, change.RepositoryID)
	if err != nil {
		return storage.ConfigFileState{}, err
	}
	if err := tx.Commit(); err != nil {
		return storage.ConfigFileState{}, err
	}
	return state, nil
}

func recordConfigFileResolution(ctx context.Context, tx *transaction, request storage.ConfigFileResolution) (int64, error) {
	change := request.State
	entry := auditInsert{
		TargetID: change.TargetID, ActorAccountID: request.ActorAccountID, ElevationID: request.ElevationID,
		Action: actionConfigFileResolved, CreatedAt: change.ChangedAt,
		Summary: "Chose " + request.Side + " values for conflicting configuration settings",
	}
	if change.RepositoryID != "" {
		var fullName string
		if err := tx.QueryRowContext(ctx, `SELECT full_name FROM repositories WHERE target_id = ? AND id = ?`,
			change.TargetID, change.RepositoryID).Scan(&fullName); err != nil {
			return 0, noRows(err)
		}
		entry.RepositoryID, entry.RepositoryFullName = &change.RepositoryID, &fullName
	}
	return insertAudit(ctx, tx, entry)
}
