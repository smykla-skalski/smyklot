package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

// linkSyncCheckResult is part of plan creation, so a failed insert rolls the link
// back too. The occurrence lock fences a stale worker and prevents overwriting a
// result after that plan has completed and released the target's live slot.
func (s *Store) linkSyncCheckResult(ctx context.Context, tx *transaction, create orgsync.PlanCreate) error {
	check := create.OriginCheck
	if check == nil {
		return nil
	}
	item, err := getQueueItem(ctx, tx, check.QueueID, s.dialect.RowLock())
	if errors.Is(err, sql.ErrNoRows) {
		return orgsync.ErrStaleCheck
	}
	if err != nil {
		return fmt.Errorf("read originating sync check: %w", err)
	}
	if item.Kind != workqueue.KindSyncScan || item.TargetID == nil || *item.TargetID != create.TargetID ||
		item.State != workqueue.StateRunning || check.Attempt < 1 || item.Attempt != check.Attempt ||
		item.LeaseExpiresAt == nil || !item.LeaseExpiresAt.After(create.Now) {
		return orgsync.ErrStaleCheck
	}
	if len(item.Details) == 0 {
		item.Details = json.RawMessage(`{}`)
	}
	var held workqueue.SyncScanDetails
	if err := json.Unmarshal(item.Details, &held); err != nil {
		return fmt.Errorf("read sync check result: %w", err)
	}
	if held.ResultPlanID != "" {
		return orgsync.ErrStaleCheck
	}
	var details map[string]json.RawMessage
	if err := json.Unmarshal(item.Details, &details); err != nil {
		return fmt.Errorf("read sync check details: %w", err)
	}
	if details == nil {
		details = make(map[string]json.RawMessage)
	}
	encodedID, err := json.Marshal(create.ID)
	if err != nil {
		return fmt.Errorf("encode sync result identity: %w", err)
	}
	details["result_plan_id"] = encodedID
	encoded, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("encode sync check result: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
UPDATE queue_items SET details = ?, updated_at = ?, revision = revision + 1 WHERE id = ?`,
		string(encoded), create.Now, item.ID)
	if err != nil {
		return fmt.Errorf("link originating sync check: %w", err)
	}
	return nil
}
