package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

const (
	pendingCIScopeRepository = "repository_id = ?"
	pendingCIScopeTarget     = "target_id = ?"
	pendingCIWakePrefix      = `
UPDATE pending_ci_requests SET
    schedule = ?, next_check_at = ?, lease_expires_at = NULL,
    next_check_trigger = ?, updated_at = ?, revision = revision + 1
WHERE `
	pendingCIWakeSuffix = " AND lifecycle = ? AND merge_phase = ?"
)

const pendingCIGateSelect = `
SELECT repository_id, target_id, desired_mode, effective_mode, readiness,
       reason, app_id, ruleset_id, ruleset_fingerprint, generation,
       observed_at, updated_at, revision
FROM pending_ci_repository_gates`

func ensurePendingCIGates(
	ctx context.Context,
	tx runner,
	targetID string,
	changedAt time.Time,
) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO pending_ci_repository_gates (
    repository_id, target_id, desired_mode, effective_mode,
    readiness, reason, updated_at
)
SELECT r.id, r.target_id,
       COALESCE(r.pending_ci_mode_override, t.pending_ci_mode_default),
       'none', 'provisioning', 'Waiting for repository protection reconciliation', ?
FROM repositories r
JOIN targets t ON t.id = r.target_id
WHERE r.target_id = ?
ON CONFLICT(repository_id) DO UPDATE SET
	target_id = excluded.target_id,
    desired_mode = excluded.desired_mode,
    readiness = CASE
        WHEN pending_ci_repository_gates.desired_mode <> excluded.desired_mode
            THEN 'draining'
		WHEN pending_ci_repository_gates.target_id <> excluded.target_id
			THEN 'provisioning'
        ELSE pending_ci_repository_gates.readiness
    END,
    reason = CASE
        WHEN pending_ci_repository_gates.desired_mode <> excluded.desired_mode
            THEN 'Waiting for repository protection transition'
		WHEN pending_ci_repository_gates.target_id <> excluded.target_id
			THEN 'Waiting for repository ownership reconciliation'
        ELSE pending_ci_repository_gates.reason
    END,
    generation = CASE
		WHEN pending_ci_repository_gates.desired_mode <> excluded.desired_mode
			OR pending_ci_repository_gates.target_id <> excluded.target_id
            THEN pending_ci_repository_gates.generation + 1
        ELSE pending_ci_repository_gates.generation
    END,
    updated_at = CASE
		WHEN pending_ci_repository_gates.desired_mode <> excluded.desired_mode
			OR pending_ci_repository_gates.target_id <> excluded.target_id
            THEN excluded.updated_at
        ELSE pending_ci_repository_gates.updated_at
    END,
    revision = CASE
		WHEN pending_ci_repository_gates.desired_mode <> excluded.desired_mode
			OR pending_ci_repository_gates.target_id <> excluded.target_id
            THEN pending_ci_repository_gates.revision + 1
        ELSE pending_ci_repository_gates.revision
    END`, changedAt, targetID)
	if err != nil {
		return fmt.Errorf("ensure pending CI repository gates: %w", err)
	}

	return nil
}

func (s *Store) GetPendingCIRepositoryGate(
	ctx context.Context,
	repositoryID string,
) (storage.PendingCIRepositoryGate, error) {
	gate, err := scanPendingCIGate(s.db.QueryRowContext(
		ctx,
		pendingCIGateSelect+" WHERE repository_id = ?",
		repositoryID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return storage.PendingCIRepositoryGate{}, storage.ErrNotFound
	}

	return gate, err
}

func (s *Store) ListPendingCIRepositoryGates(
	ctx context.Context,
	limit int,
) ([]storage.PendingCIRepositoryGate, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, pendingCIGateSelect+`
WHERE repository_id IN (
    SELECT r.id FROM repositories r
    JOIN targets t ON t.id = r.target_id
    WHERE r.available = TRUE AND t.available = TRUE
)
ORDER BY CASE readiness WHEN 'ready' THEN 1 ELSE 0 END, updated_at, repository_id
LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending CI repository gates: %w", err)
	}

	return collectRows(rows, scanPendingCIGate)
}

func (s *Store) UpdatePendingCIRepositoryGate(
	ctx context.Context,
	change storage.PendingCIGateChange,
) (storage.PendingCIRepositoryGate, error) {
	if change.RepositoryID == "" || change.ExpectedRevision <= 0 || change.ObservedAt.IsZero() {
		return storage.PendingCIRepositoryGate{}, fmt.Errorf("invalid pending CI gate change")
	}
	if change.EffectiveMode != storage.PendingCIEffectiveNone &&
		change.EffectiveMode != storage.PendingCIEffectiveLabels &&
		change.EffectiveMode != storage.PendingCIEffectiveChecks {
		return storage.PendingCIRepositoryGate{}, fmt.Errorf(
			"invalid pending CI effective mode %q", change.EffectiveMode,
		)
	}
	if change.Readiness != storage.PendingCIReady &&
		change.Readiness != storage.PendingCIProvisioning &&
		change.Readiness != storage.PendingCIDraining &&
		change.Readiness != storage.PendingCIBlocked {
		return storage.PendingCIRepositoryGate{}, fmt.Errorf(
			"invalid pending CI readiness %q", change.Readiness,
		)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.PendingCIRepositoryGate{}, fmt.Errorf("begin pending CI gate update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	current, err := scanPendingCIGate(tx.QueryRowContext(
		ctx,
		pendingCIGateSelect+" WHERE repository_id = ?"+s.dialect.RowLock(),
		change.RepositoryID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return storage.PendingCIRepositoryGate{}, storage.ErrNotFound
	}
	if err != nil {
		return storage.PendingCIRepositoryGate{}, fmt.Errorf("read pending CI gate for update: %w", err)
	}
	if current.Revision != change.ExpectedRevision {
		return storage.PendingCIRepositoryGate{}, storage.ErrConflict
	}
	result, err := tx.ExecContext(ctx, `
UPDATE pending_ci_repository_gates SET
    effective_mode = ?, readiness = ?, reason = ?, app_id = ?, ruleset_id = ?,
    ruleset_fingerprint = ?, observed_at = ?, updated_at = ?, revision = revision + 1
WHERE repository_id = ? AND revision = ?`,
		change.EffectiveMode, change.Readiness, change.Reason, change.AppID,
		change.RulesetID, change.RulesetFingerprint, change.ObservedAt,
		change.ObservedAt, change.RepositoryID, change.ExpectedRevision,
	)
	if err != nil {
		return storage.PendingCIRepositoryGate{}, fmt.Errorf("update pending CI gate: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return storage.PendingCIRepositoryGate{}, err
	}
	if current.Readiness != storage.PendingCIReady && change.Readiness == storage.PendingCIReady {
		if err := wakePendingCIChecksForRepository(
			ctx, tx, change.RepositoryID, change.ObservedAt,
		); err != nil {
			return storage.PendingCIRepositoryGate{}, err
		}
	}
	updated, err := scanPendingCIGate(tx.QueryRowContext(
		ctx,
		pendingCIGateSelect+" WHERE repository_id = ?",
		change.RepositoryID,
	))
	if err != nil {
		return storage.PendingCIRepositoryGate{}, fmt.Errorf("read updated pending CI gate: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return storage.PendingCIRepositoryGate{}, fmt.Errorf("commit pending CI gate update: %w", err)
	}

	return updated, nil
}

func wakePendingCIChecksForRepository(
	ctx context.Context,
	tx *transaction,
	repositoryID string,
	readyAt time.Time,
) error {
	result, err := tx.ExecContext(ctx, `
UPDATE pending_ci_requests SET
    schedule = ?, next_check_at = ?, lease_expires_at = NULL,
    next_check_trigger = ?, updated_at = ?, revision = revision + 1
WHERE repository_id = ? AND lifecycle = ? AND artifact_kind = ? AND merge_phase = ?`,
		pendingci.ScheduleActive,
		readyAt,
		pendingci.TriggerManual,
		readyAt,
		repositoryID,
		pendingci.LifecycleArmed,
		pendingci.ArtifactCheck,
		pendingci.MergeWaiting,
	)
	if err != nil {
		return fmt.Errorf("wake pending CI checks for ready gate: %w", err)
	}
	if _, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("read ready-gate pending CI wake result: %w", err)
	}
	return syncPendingCIQueueWhere(
		ctx, tx, "repository_id = ? AND lifecycle = ? AND updated_at = ?",
		repositoryID, pendingci.LifecycleArmed, readyAt,
	)
}

func wakePendingCIRequestsForRepository(
	ctx context.Context,
	tx *transaction,
	repositoryID string,
	wakeAt time.Time,
) error {
	return wakePendingCIRequests(
		ctx, tx, repositoryID, wakeAt,
		pendingCIWakePrefix+pendingCIScopeRepository+pendingCIWakeSuffix,
		pendingCIScopeRepository, "repository",
	)
}

func wakePendingCIRequestsForTarget(
	ctx context.Context,
	tx *transaction,
	targetID string,
	wakeAt time.Time,
) error {
	return wakePendingCIRequests(
		ctx, tx, targetID, wakeAt,
		pendingCIWakePrefix+pendingCIScopeTarget+pendingCIWakeSuffix,
		pendingCIScopeTarget, "target",
	)
}

func wakePendingCIRequests(
	ctx context.Context,
	tx *transaction,
	scopeID string,
	wakeAt time.Time,
	updateQuery string,
	scopeFilter string,
	scopeName string,
) error {
	// #nosec G202 -- both SQL fragments are package constants selected by the two wrappers above.
	result, err := tx.ExecContext(ctx, updateQuery,
		pendingci.ScheduleActive,
		wakeAt,
		pendingci.TriggerManual,
		wakeAt,
		scopeID,
		pendingci.LifecycleArmed,
		pendingci.MergeWaiting,
	)
	if err != nil {
		return fmt.Errorf("wake %s pending CI requests: %w", scopeName, err)
	}
	if _, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("read %s pending CI wake result: %w", scopeName, err)
	}

	return syncPendingCIQueueWhere(
		ctx, tx, scopeFilter+" AND lifecycle = ? AND updated_at = ?",
		scopeID, pendingci.LifecycleArmed, wakeAt,
	)
}

func scanPendingCIGate(scanner rowScanner) (storage.PendingCIRepositoryGate, error) {
	var gate storage.PendingCIRepositoryGate
	var appID, rulesetID sql.NullInt64
	var observedAt, updatedAt StoredTime
	err := scanner.Scan(
		&gate.RepositoryID, &gate.TargetID, &gate.DesiredMode,
		&gate.EffectiveMode, &gate.Readiness, &gate.Reason, &appID,
		&rulesetID, &gate.RulesetFingerprint, &gate.Generation,
		&observedAt, &updatedAt, &gate.Revision,
	)
	if err != nil {
		return storage.PendingCIRepositoryGate{}, err
	}
	if appID.Valid {
		gate.AppID = &appID.Int64
	}
	if rulesetID.Valid {
		gate.RulesetID = &rulesetID.Int64
	}
	gate.ObservedAt = observedAt.Pointer()
	gate.UpdatedAt = updatedAt.Time()

	return gate, nil
}
