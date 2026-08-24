package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

const queuePolicyColumns = `
    kind, target_id, enabled, cadence_seconds, profile_id, default_priority,
    retry_delay_seconds, retention_seconds, approval_ttl_seconds,
    configuration, revision, updated_by, updated_at`

func (s *Store) ListQueuePolicies(
	ctx context.Context,
	targetID *string,
) ([]workqueue.Policy, error) {
	query := "SELECT" + queuePolicyColumns + " FROM queue_policies WHERE target_id IS NULL"
	arguments := []any{}
	if targetID != nil {
		query = "SELECT" + queuePolicyColumns +
			" FROM queue_policies WHERE target_id IS NULL OR target_id = ?"
		arguments = append(arguments, *targetID)
	}
	query += " ORDER BY kind, target_id"
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, fmt.Errorf("list queue policies: %w", err)
	}
	policies, err := collectRows(rows, scanQueuePolicy)
	if err != nil {
		return nil, fmt.Errorf("read queue policies: %w", err)
	}

	return policies, nil
}

func (s *Store) ListAllQueuePolicies(ctx context.Context) ([]workqueue.Policy, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT"+queuePolicyColumns+" FROM queue_policies ORDER BY kind, target_id",
	)
	if err != nil {
		return nil, fmt.Errorf("list all queue policies: %w", err)
	}
	policies, err := collectRows(rows, scanQueuePolicy)
	if err != nil {
		return nil, fmt.Errorf("read all queue policies: %w", err)
	}

	return policies, nil
}

func (s *Store) GetEffectiveQueuePolicy(
	ctx context.Context,
	kind workqueue.Kind,
	targetID *string,
) (workqueue.Policy, error) {
	policy, err := getEffectiveQueuePolicy(ctx, s.db, kind, targetID)
	if errors.Is(err, sql.ErrNoRows) {
		return workqueue.Policy{}, storage.ErrNotFound
	}
	if err != nil {
		return workqueue.Policy{}, fmt.Errorf("get effective queue policy: %w", err)
	}

	return policy, nil
}

func getEffectiveQueuePolicy(
	ctx context.Context,
	runner runner,
	kind workqueue.Kind,
	targetID *string,
) (workqueue.Policy, error) {
	if targetID == nil {
		return scanQueuePolicy(runner.QueryRowContext(ctx, "SELECT"+queuePolicyColumns+`
FROM queue_policies WHERE kind = ? AND target_id IS NULL`, kind))
	}

	return scanQueuePolicy(runner.QueryRowContext(ctx, "SELECT"+queuePolicyColumns+`
FROM queue_policies WHERE kind = ? AND (target_id = ? OR target_id IS NULL)
ORDER BY CASE WHEN target_id = ? THEN 0 ELSE 1 END LIMIT 1`, kind, *targetID, *targetID))
}

func scanQueuePolicy(scanner rowScanner) (workqueue.Policy, error) {
	var policy workqueue.Policy
	var targetID, updatedBy sql.NullString
	var configuration string
	var cadence, retry int64
	var retention, approval sql.NullInt64
	var updated StoredTime
	if err := scanner.Scan(
		&policy.Kind, &targetID, &policy.Enabled, &cadence, &policy.ProfileID,
		&policy.DefaultPriority, &retry, &retention, &approval,
		&configuration, &policy.Revision, &updatedBy, &updated,
	); err != nil {
		return workqueue.Policy{}, err
	}
	policy.TargetID, policy.UpdatedBy = stringPointer(targetID), stringPointer(updatedBy)
	policy.Configuration = json.RawMessage(configuration)
	policy.Cadence, policy.RetryDelay = secondsDuration(cadence), secondsDuration(retry)
	policy.Retention, policy.ApprovalTTL = optionalDuration(retention), optionalDuration(approval)
	policy.UpdatedAt = updated.Time()

	return policy, nil
}

func secondsDuration(seconds int64) time.Duration { return time.Duration(seconds) * time.Second }

func optionalDuration(seconds sql.NullInt64) *time.Duration {
	return durationPointer(seconds)
}

func (s *Store) SaveQueuePolicy(
	ctx context.Context,
	change workqueue.PolicyChange,
) (workqueue.Policy, error) {
	if err := validateQueuePolicyChange(change); err != nil {
		return workqueue.Policy{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return workqueue.Policy{}, fmt.Errorf("begin queue policy save: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	profile, err := getScheduleProfile(ctx, tx, change.ProfileID)
	if err != nil {
		return workqueue.Policy{}, noRows(err)
	}
	if err := validateProfileScope(profile, change.TargetID); err != nil {
		return workqueue.Policy{}, err
	}
	if err := saveQueuePolicy(ctx, tx, change); err != nil {
		return workqueue.Policy{}, err
	}
	policy, err := getEffectiveQueuePolicy(ctx, tx, change.Kind, change.TargetID)
	if err != nil {
		return workqueue.Policy{}, err
	}
	if err := s.reschedulePolicyItems(
		ctx, tx, policy, profile, change.ChangedAt, change.ActorID, "Queue policy changed",
	); err != nil {
		return workqueue.Policy{}, err
	}
	if err := tx.Commit(); err != nil {
		return workqueue.Policy{}, fmt.Errorf("commit queue policy save: %w", err)
	}

	return policy, nil
}

func (s *Store) DeleteQueuePolicyOverride(
	ctx context.Context,
	kind workqueue.Kind,
	targetID string,
	expectedRevision int64,
	actorID string,
	at time.Time,
) (workqueue.Policy, error) {
	if !kind.InstallationConfigurable() || targetID == "" || expectedRevision < 1 {
		return workqueue.Policy{}, errors.New("queue policy override identity is invalid")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return workqueue.Policy{}, fmt.Errorf("begin queue policy override delete: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx,
		"DELETE FROM queue_policies WHERE kind = ? AND target_id = ? AND revision = ?",
		kind, targetID, expectedRevision,
	)
	if err != nil {
		return workqueue.Policy{}, fmt.Errorf("delete queue policy override: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return workqueue.Policy{}, storage.ErrConflict
	}
	effective, err := getEffectiveQueuePolicy(ctx, tx, kind, &targetID)
	if err != nil {
		return workqueue.Policy{}, err
	}
	profile, err := getScheduleProfile(ctx, tx, effective.ProfileID)
	if err != nil {
		return workqueue.Policy{}, err
	}
	effective.TargetID = &targetID
	if err := s.reschedulePolicyItems(
		ctx, tx, effective, profile, at, actorID, "Installation override removed",
	); err != nil {
		return workqueue.Policy{}, err
	}
	if err := tx.Commit(); err != nil {
		return workqueue.Policy{}, fmt.Errorf("commit queue policy override delete: %w", err)
	}

	return effective, nil
}

func validateQueuePolicyChange(change workqueue.PolicyChange) error {
	if !change.Kind.Valid() || change.Kind == workqueue.KindScheduleChange {
		return errors.New("queue policy kind is invalid")
	}
	if change.Cadence < 0 || change.RetryDelay < 0 || !change.DefaultPriority.Valid() {
		return errors.New("queue policy timing or priority is invalid")
	}
	if change.Retention != nil && *change.Retention < 0 {
		return errors.New("queue policy retention cannot be negative")
	}
	if change.ApprovalTTL != nil && *change.ApprovalTTL <= 0 {
		return errors.New("queue policy approval lifetime must be positive")
	}
	if change.Kind.Windowed() && change.ProfileID == "" {
		return errors.New("windowed queue policy needs a profile")
	}
	if err := workqueue.ValidatePolicyConfiguration(change.Kind, change.Configuration); err != nil {
		return err
	}

	return nil
}

func saveQueuePolicy(
	ctx context.Context,
	tx *transaction,
	change workqueue.PolicyChange,
) error {
	scopeID := "root"
	if change.TargetID != nil {
		scopeID = *change.TargetID
	}
	configuration := change.Configuration
	if len(configuration) == 0 {
		configuration = json.RawMessage(`{}`)
	}
	if change.ExpectedRevision == 0 {
		_, err := tx.ExecContext(ctx, `
INSERT INTO queue_policies (
    kind, scope_id, target_id, enabled, cadence_seconds, profile_id,
    default_priority, retry_delay_seconds, retention_seconds, approval_ttl_seconds,
    configuration, revision, updated_by, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
			change.Kind, scopeID, change.TargetID, change.Enabled,
			int64(change.Cadence/time.Second), change.ProfileID, change.DefaultPriority,
			int64(change.RetryDelay/time.Second), durationSeconds(change.Retention),
			durationSeconds(change.ApprovalTTL), string(configuration),
			change.ActorID, change.ChangedAt,
		)
		if err != nil {
			return fmt.Errorf("insert queue policy: %w", err)
		}

		return nil
	}
	result, err := tx.ExecContext(ctx, `
UPDATE queue_policies SET
    enabled = ?, cadence_seconds = ?, profile_id = ?, default_priority = ?,
    retry_delay_seconds = ?, retention_seconds = ?, approval_ttl_seconds = ?,
    configuration = ?, revision = revision + 1, updated_by = ?, updated_at = ?
WHERE kind = ? AND scope_id = ? AND revision = ?`,
		change.Enabled, int64(change.Cadence/time.Second), change.ProfileID,
		change.DefaultPriority, int64(change.RetryDelay/time.Second),
		durationSeconds(change.Retention), durationSeconds(change.ApprovalTTL),
		string(configuration), change.ActorID, change.ChangedAt,
		change.Kind, scopeID, change.ExpectedRevision,
	)
	if err != nil {
		return fmt.Errorf("update queue policy: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return storage.ErrConflict
	}

	return nil
}

func (s *Store) reschedulePolicyItems(
	ctx context.Context,
	tx *transaction,
	policy workqueue.Policy,
	profile workqueue.Profile,
	now time.Time,
	actorID string,
	reason string,
) error {
	clauses := "kind = ? AND immediate_dispatch = FALSE AND state IN ('scheduled', 'ready', 'retrying', 'blocked')"
	arguments := []any{policy.Kind}
	if policy.TargetID != nil {
		clauses += " AND target_id = ?"
		arguments = append(arguments, *policy.TargetID)
	}
	rows, err := tx.QueryContext(ctx, "SELECT"+queueItemColumns+" FROM queue_items WHERE "+clauses, arguments...)
	if err != nil {
		return fmt.Errorf("list queue items for policy: %w", err)
	}
	items, err := collectRows(rows, scanQueueItem)
	if err != nil {
		return fmt.Errorf("read queue items for policy: %w", err)
	}
	for _, item := range items {
		effective, effectiveProfile := policy, profile
		if policy.TargetID == nil && item.TargetID != nil {
			effective, err = getEffectiveQueuePolicy(ctx, tx, policy.Kind, item.TargetID)
			if err != nil {
				return err
			}
			effectiveProfile, err = getScheduleProfile(ctx, tx, effective.ProfileID)
			if err != nil {
				return err
			}
		}
		eligible := item.NotBefore
		if effective.Kind.Windowed() && item.WindowMode == workqueue.WindowRespect {
			eligible, err = workqueue.NextEligible(effectiveProfile, item.NotBefore)
			if err != nil {
				return err
			}
		}
		state, blockedReason := stateForEligibility(eligible, now), ""
		if !effective.Enabled {
			state, blockedReason = workqueue.StateBlocked, "Workload disabled by policy"
		}
		priority := effective.DefaultPriority
		if item.PriorityOverride {
			priority = item.Priority
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE queue_items SET profile_id = ?, priority = ?, eligible_at = ?,
	state = ?, blocked_reason = ?, revision = revision + 1, updated_at = ? WHERE id = ?`,
			effective.ProfileID, priority, eligible,
			state, blockedReason, now, item.ID,
		); err != nil {
			return fmt.Errorf("reschedule queue item for policy: %w", err)
		}
		if err := insertQueueEvent(ctx, tx, workqueue.Event{
			ItemID: item.ID, ActorID: queueEventActor(actorID), Kind: "schedule.recomputed",
			State: state, Summary: reason, CreatedAt: now,
		}); err != nil {
			return err
		}
	}

	return nil
}
