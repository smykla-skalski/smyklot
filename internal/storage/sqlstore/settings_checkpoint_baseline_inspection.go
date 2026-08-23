package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

// InspectInstallationSettingsBaseline returns the stable initial snapshot for
// one installation without requiring callers to discover its numeric ID.
func (s *Store) InspectInstallationSettingsBaseline(
	ctx context.Context,
	targetID string,
) (storage.SettingsCheckpointInspection, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.SettingsCheckpointInspection{},
			fmt.Errorf("begin settings baseline inspection: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	id, err := getSettingsBaselineID(
		ctx, tx, storage.SettingsCheckpointScopeInstallation, targetID,
	)
	if err != nil {
		return storage.SettingsCheckpointInspection{}, err
	}
	inspection, err := inspectInstallationSettingsCheckpoint(
		ctx,
		tx,
		storage.SettingsCheckpointRef{
			ID: id, Scope: storage.SettingsCheckpointScopeInstallation, TargetID: targetID,
		},
	)
	if err != nil {
		return storage.SettingsCheckpointInspection{}, err
	}
	if err := tx.Commit(); err != nil {
		return storage.SettingsCheckpointInspection{},
			fmt.Errorf("commit settings baseline inspection: %w", err)
	}

	return inspection, nil
}

// InspectRootSettingsBaseline returns the stable initial Root runtime snapshot
// without requiring callers to discover its numeric ID.
func (s *Store) InspectRootSettingsBaseline(
	ctx context.Context,
) (storage.SettingsCheckpointInspection, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.SettingsCheckpointInspection{},
			fmt.Errorf("begin Root settings baseline inspection: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	id, err := getSettingsBaselineID(ctx, tx, storage.SettingsCheckpointScopeRoot, "")
	if err != nil {
		return storage.SettingsCheckpointInspection{}, err
	}
	inspection, err := inspectRootSettingsCheckpoint(
		ctx,
		tx,
		storage.SettingsCheckpointRef{ID: id, Scope: storage.SettingsCheckpointScopeRoot},
	)
	if err != nil {
		return storage.SettingsCheckpointInspection{}, err
	}
	if err := tx.Commit(); err != nil {
		return storage.SettingsCheckpointInspection{},
			fmt.Errorf("commit Root settings baseline inspection: %w", err)
	}

	return inspection, nil
}

func getSettingsBaselineID(
	ctx context.Context,
	queryer rowQuerier,
	scope storage.SettingsCheckpointScope,
	targetID string,
) (int64, error) {
	var id int64
	var err error
	if scope == storage.SettingsCheckpointScopeRoot {
		err = queryer.QueryRowContext(ctx, `
SELECT id FROM settings_checkpoints
WHERE scope = ? AND target_id IS NULL AND action = 'baseline'`, scope).Scan(&id)
	} else {
		err = queryer.QueryRowContext(ctx, `
SELECT id FROM settings_checkpoints
WHERE scope = ? AND target_id = ? AND action = 'baseline'`, scope, targetID).Scan(&id)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return 0, storage.ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("read settings baseline checkpoint: %w", err)
	}

	return id, nil
}
