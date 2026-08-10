package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

type appAuditInsert struct {
	Category         string
	SourceKind       *string
	SourceID         *int64
	TargetID         *string
	ActorAccountID   string
	SubjectAccountID *string
	ElevationID      *string
	Action           string
	Summary          string
	CreatedAt        string
}

func insertAppAudit(
	ctx context.Context,
	executor accountExecutor,
	entry appAuditInsert,
) (int64, error) {
	result, err := executor.ExecContext(ctx, `
INSERT INTO app_audit_events (
    category, source_kind, source_id, target_id, actor_account_id,
    subject_account_id, elevation_id, action, summary, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.Category,
		entry.SourceKind,
		entry.SourceID,
		entry.TargetID,
		entry.ActorAccountID,
		entry.SubjectAccountID,
		entry.ElevationID,
		entry.Action,
		entry.Summary,
		entry.CreatedAt,
	)
	if err != nil {
		return 0, fmt.Errorf("insert app audit: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read app audit ID: %w", err)
	}

	return id, nil
}

func insertElevatedNotifications(
	ctx context.Context,
	tx *sql.Tx,
	elevation storage.Elevation,
	auditEventID int64,
	action string,
	createdAt string,
) error {
	result, err := tx.ExecContext(ctx, `
INSERT INTO security_notifications (
    recipient_account_id, target_id, actor_account_id, elevation_id,
    audit_event_id, action, reason, created_at
)
SELECT
    owner.account_id, ?, ?, ?, ?, ?, ?, ?
FROM target_owners owner
WHERE owner.target_id = ?`,
		elevation.TargetID,
		elevation.RootAccountID,
		elevation.ID,
		auditEventID,
		action,
		elevation.Reason,
		createdAt,
		elevation.TargetID,
	)
	if err != nil {
		return fmt.Errorf("insert Owner notifications: %w", err)
	}
	owners, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read Owner notification count: %w", err)
	}
	if owners == 0 {
		return fmt.Errorf("insert Owner notifications: %w", storage.ErrConflict)
	}

	return nil
}
