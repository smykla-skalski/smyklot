package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

// linkSyncCheckResult is part of plan creation, so a failed insert rolls the link
// back too. The occurrence lock fences a stale worker and prevents overwriting a
// result after that plan has completed and released the target's live slot.
func (s *Store) linkSyncCheckResult(ctx context.Context, tx *transaction, create orgsync.PlanCreate) error {
	if create.OriginCheck == nil {
		return nil
	}
	return s.writeSyncCheckResult(ctx, tx, *create.OriginCheck, create.TargetID, create.ID, create.CheckResult, create.Now)
}

// RecordSyncCheckResult retains a completed comparison even when no plan exists.
func (s *Store) RecordSyncCheckResult(ctx context.Context, create orgsync.CheckResultCreate) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sync check result: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.writeSyncCheckResult(ctx, tx, create.Check, create.TargetID, "", &create.Result, create.Now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) writeSyncCheckResult(ctx context.Context, tx *transaction, check orgsync.CheckReference, targetID, planID string, result *orgsync.CheckResult, now time.Time) error {
	if result != nil {
		if err := result.Validate(); err != nil {
			return err
		}
	}
	item, err := getQueueItem(ctx, tx, check.QueueID, s.dialect.RowLock())
	if errors.Is(err, sql.ErrNoRows) {
		return orgsync.ErrStaleCheck
	}
	if err != nil {
		return fmt.Errorf("read originating sync check: %w", err)
	}
	if item.Kind != workqueue.KindSyncScan || item.TargetID == nil || *item.TargetID != targetID ||
		item.State != workqueue.StateRunning || check.Attempt < 1 || item.Attempt != check.Attempt ||
		item.LeaseExpiresAt == nil || !item.LeaseExpiresAt.After(now) {
		return orgsync.ErrStaleCheck
	}
	if len(item.Details) == 0 {
		item.Details = json.RawMessage(`{}`)
	}
	var held orgsync.CheckDetails
	if err := json.Unmarshal(item.Details, &held); err != nil {
		return fmt.Errorf("read sync check result: %w", err)
	}
	if held.ResultPlanID != "" || held.Outcome != nil {
		return orgsync.ErrStaleCheck
	}
	encoded, err := encodeCheckDetails(item.Details, planID, result)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO sync_check_results (check_id, target_id, details) VALUES (?, ?, ?)", item.ID, targetID, string(encoded)); err != nil {
		return fmt.Errorf("retain sync check result: %w", err)
	}
	if result != nil {
		if err := insertCheckEvidence(ctx, tx, item.ID, result.Observations); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `
UPDATE queue_items SET details = ?, updated_at = ?, revision = revision + 1 WHERE id = ?`,
		string(encoded), now, item.ID)
	if err != nil {
		return fmt.Errorf("link originating sync check: %w", err)
	}
	return nil
}

func encodeCheckDetails(raw json.RawMessage, planID string, result *orgsync.CheckResult) ([]byte, error) {
	var details map[string]json.RawMessage
	if err := json.Unmarshal(raw, &details); err != nil {
		return nil, fmt.Errorf("read sync check details: %w", err)
	}
	if details == nil {
		details = make(map[string]json.RawMessage)
	}
	if planID != "" {
		encoded, err := json.Marshal(planID)
		if err != nil {
			return nil, err
		}
		details["result_plan_id"] = encoded
	}
	if result != nil {
		encoded, err := json.Marshal(result.Outcome)
		if err != nil {
			return nil, fmt.Errorf("encode check outcome: %w", err)
		}
		details["outcome"] = encoded
	}
	return json.Marshal(details)
}

func insertCheckEvidence(ctx context.Context, tx *transaction, checkID string, observations []orgsync.CheckObservation) error {
	for index, observation := range observations {
		encoded, err := json.Marshal(observation)
		if err != nil {
			return fmt.Errorf("encode check observation: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO sync_check_observations (queue_id, ordinal, evidence) VALUES (?, ?, ?)", checkID, index+1, string(encoded)); err != nil {
			return fmt.Errorf("record check observation: %w", err)
		}
	}
	return nil
}
