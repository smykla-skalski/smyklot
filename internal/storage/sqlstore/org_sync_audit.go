package sqlstore

import (
	"context"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

// syncAuditSourceKind names the detail table the trunk points at for a sync
// entry, and is what the panel filters on.
const syncAuditSourceKind = "sync"

// RecordSyncAudit writes a sync audit entry and mirrors it into the trunk.
//
// One transaction, like every other detail insert. A detail row without its
// mirror is invisible to the history page, and a mirror without its detail row
// points at nothing - either half alone is worse than neither.
func (s *Store) RecordSyncAudit(ctx context.Context, entry orgsync.AuditEntry) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sync audit: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var sourceID int64
	if err := tx.QueryRowContext(ctx, `
INSERT INTO sync_audit_entries (
    target_id, plan_id, actor_account_id, action, summary,
    create_count, update_count, delete_count, failed_count, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id`,
		entry.TargetID, entry.PlanID, entry.ActorID, string(entry.Action), entry.Summary,
		entry.Counts.Create, entry.Counts.Update, entry.Counts.Delete, entry.Failed,
		entry.Now,
	).Scan(&sourceID); err != nil {
		return fmt.Errorf("insert sync audit: %w", err)
	}

	sourceKind := syncAuditSourceKind
	targetID := entry.TargetID

	if _, err := insertAppAudit(ctx, tx, appAuditInsert{
		Category:       string(storage.AuditCategorySync),
		SourceKind:     &sourceKind,
		SourceID:       &sourceID,
		TargetID:       &targetID,
		ActorAccountID: entry.ActorID,
		Action:         string(entry.Action),
		Summary:        entry.Summary,
		CreatedAt:      entry.Now,
	}); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sync audit: %w", err)
	}

	return nil
}
