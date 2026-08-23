package sqlstore

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

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
	TargetID             string
	RepositoryID         *string
	RepositoryFullName   *string
	SettingsCheckpointID *int64
	ActorAccountID       string
	ElevationID          *string
	SourceKind           *string
	SourceID             *int64
	Action               string
	Summary              string
	CreatedAt            time.Time
}

func insertAudit(ctx context.Context, tx runner, entry auditInsert) (int64, error) {
	var auditEntryID int64
	err := tx.QueryRowContext(ctx, `
INSERT INTO audit_entries (
    target_id, repository_id, repository_full_name,
    settings_checkpoint_id,
    actor_account_id, action, summary, created_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id`,
		entry.TargetID,
		entry.RepositoryID,
		entry.RepositoryFullName,
		entry.SettingsCheckpointID,
		entry.ActorAccountID,
		entry.Action,
		entry.Summary,
		entry.CreatedAt,
	).Scan(&auditEntryID)
	if err != nil {
		return 0, fmt.Errorf("insert settings audit: %w", err)
	}
	sourceKind := "settings"
	sourceID := auditEntryID
	if entry.SourceKind != nil {
		sourceKind = *entry.SourceKind
	}
	if entry.SourceID != nil {
		sourceID = *entry.SourceID
	}
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
