package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
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
    desired_mode = excluded.desired_mode,
    readiness = CASE
        WHEN pending_ci_repository_gates.desired_mode <> excluded.desired_mode
            THEN 'draining'
        ELSE pending_ci_repository_gates.readiness
    END,
    reason = CASE
        WHEN pending_ci_repository_gates.desired_mode <> excluded.desired_mode
            THEN 'Waiting for repository protection transition'
        ELSE pending_ci_repository_gates.reason
    END,
    generation = CASE
        WHEN pending_ci_repository_gates.desired_mode <> excluded.desired_mode
            THEN pending_ci_repository_gates.generation + 1
        ELSE pending_ci_repository_gates.generation
    END,
    updated_at = CASE
        WHEN pending_ci_repository_gates.desired_mode <> excluded.desired_mode
            THEN excluded.updated_at
        ELSE pending_ci_repository_gates.updated_at
    END,
    revision = CASE
        WHEN pending_ci_repository_gates.desired_mode <> excluded.desired_mode
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
	result, err := s.db.ExecContext(ctx, `
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

	return s.GetPendingCIRepositoryGate(ctx, change.RepositoryID)
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
