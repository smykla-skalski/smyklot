package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

const pendingCICheckSlotSelect = `
SELECT id, target_id, installation_id, repository_id, repository_full_name,
       pull_request, head_sha, app_id, name, external_id, generation,
       check_run_id, check_url, state, desired_status, desired_conclusion,
       desired_title, desired_summary, desired_actions, desired_digest,
       applied_digest, retry_at, last_error, updated_at, revision
FROM pending_ci_check_slots`

func (s *Store) EnsureCheckSlot(
	ctx context.Context,
	request pendingci.EnsureCheckSlotRequest,
) (pendingci.CheckSlot, error) {
	if err := request.Validate(); err != nil {
		return pendingci.CheckSlot{}, err
	}
	actions, err := json.Marshal(request.DesiredActions)
	if err != nil {
		return pendingci.CheckSlot{}, fmt.Errorf("encode desired check actions: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return pendingci.CheckSlot{}, fmt.Errorf("begin check slot ensure: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	current, err := scanPendingCICheckSlot(tx.QueryRowContext(
		ctx,
		pendingCICheckSlotSelect+" WHERE repository_id = ? AND head_sha = ?",
		request.RepositoryID,
		request.HeadSHA,
	))
	if err == nil {
		return ensureExistingCheckSlot(ctx, tx, current, request, string(actions))
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return pendingci.CheckSlot{}, fmt.Errorf("read check slot: %w", err)
	}

	var id int64
	err = tx.QueryRowContext(ctx, `
INSERT INTO pending_ci_check_slots (
    target_id, installation_id, repository_id, repository_full_name,
    pull_request, head_sha, app_id, name, external_id,
    desired_status, desired_conclusion, desired_title, desired_summary,
    desired_actions, desired_digest, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id`,
		request.TargetID, request.InstallationID, request.RepositoryID,
		request.RepositoryFullName, request.PullRequest, request.HeadSHA,
		request.AppID, request.Name, request.ExternalID, request.DesiredStatus,
		nullableString(request.DesiredConclusion), request.DesiredTitle,
		request.DesiredSummary, string(actions), request.DesiredDigest, request.ChangedAt,
	).Scan(&id)
	if err != nil {
		return pendingci.CheckSlot{}, fmt.Errorf("insert check slot: %w", err)
	}
	created, err := getPendingCICheckSlot(ctx, tx, id)
	if err != nil {
		return pendingci.CheckSlot{}, err
	}
	if err := tx.Commit(); err != nil {
		return pendingci.CheckSlot{}, fmt.Errorf("commit check slot ensure: %w", err)
	}

	return created, nil
}

func ensureExistingCheckSlot(
	ctx context.Context,
	tx *transaction,
	current pendingci.CheckSlot,
	request pendingci.EnsureCheckSlotRequest,
	actions string,
) (pendingci.CheckSlot, error) {
	if current.PullRequest != request.PullRequest {
		return pendingci.CheckSlot{}, pendingci.ErrSharedHead
	}
	if current.AppID != request.AppID || current.Name != request.Name ||
		(current.ExternalID != request.ExternalID &&
			!strings.HasPrefix(current.ExternalID, request.ExternalID+":g")) {
		return pendingci.CheckSlot{}, storage.ErrConflict
	}
	metadataChanged := current.TargetID != request.TargetID ||
		current.InstallationID != request.InstallationID ||
		current.RepositoryFullName != request.RepositoryFullName
	if current.DesiredDigest != request.DesiredDigest || metadataChanged {
		updated, err := updateCheckSlotDesired(ctx, tx, current, request, actions)
		if err != nil {
			return pendingci.CheckSlot{}, err
		}
		current = updated
	}
	if err := tx.Commit(); err != nil {
		return pendingci.CheckSlot{}, fmt.Errorf("commit check slot ensure: %w", err)
	}

	return current, nil
}

func (s *Store) RenewCheckSlot(
	ctx context.Context,
	request pendingci.RenewCheckSlotRequest,
) (pendingci.CheckSlot, error) {
	if err := request.Validate(); err != nil {
		return pendingci.CheckSlot{}, err
	}
	result, err := s.db.ExecContext(ctx, `
UPDATE pending_ci_check_slots SET
    generation = generation + 1, external_id = ?, check_run_id = NULL,
    check_url = '', state = 'provisioning', applied_digest = '',
    retry_at = NULL, last_error = '', updated_at = ?, revision = revision + 1
WHERE id = ? AND revision = ?`,
		request.ExternalID,
		request.RenewedAt,
		request.ID,
		request.ExpectedRevision,
	)
	if err != nil {
		return pendingci.CheckSlot{}, fmt.Errorf("renew check slot: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return pendingci.CheckSlot{}, err
	}

	return s.GetCheckSlot(ctx, request.ID)
}

func (s *Store) ReassignCheckSlot(
	ctx context.Context,
	request pendingci.ReassignCheckSlotRequest,
) (pendingci.CheckSlot, error) {
	if err := request.Validate(); err != nil {
		return pendingci.CheckSlot{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return pendingci.CheckSlot{}, fmt.Errorf("begin check slot reassignment: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	current, err := scanPendingCICheckSlot(tx.QueryRowContext(
		ctx,
		pendingCICheckSlotSelect+" WHERE id = ?"+s.dialect.RowLock(),
		request.ID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return pendingci.CheckSlot{}, storage.ErrNotFound
	}
	if err != nil {
		return pendingci.CheckSlot{}, fmt.Errorf("read check slot for reassignment: %w", err)
	}
	if current.Revision != request.ExpectedRevision {
		return pendingci.CheckSlot{}, storage.ErrConflict
	}
	var referenced bool
	err = tx.QueryRowContext(ctx, `
SELECT EXISTS (
    SELECT 1 FROM pending_ci_requests
    WHERE (check_slot_id = ? OR retired_check_slot_id = ?)
      AND (lifecycle = ? OR cleanup_pending = TRUE)
)`, current.ID, current.ID, pendingci.LifecycleArmed).Scan(&referenced)
	if err != nil {
		return pendingci.CheckSlot{}, fmt.Errorf("read check slot request ownership: %w", err)
	}
	if referenced {
		return pendingci.CheckSlot{}, storage.ErrConflict
	}
	result, err := tx.ExecContext(ctx, `
UPDATE pending_ci_check_slots SET
    pull_request = ?, updated_at = ?, revision = revision + 1
WHERE id = ? AND revision = ?`,
		request.PullRequest, request.ReassignedAt, request.ID, request.ExpectedRevision,
	)
	if err != nil {
		return pendingci.CheckSlot{}, fmt.Errorf("reassign check slot: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return pendingci.CheckSlot{}, err
	}
	updated, err := getPendingCICheckSlot(ctx, tx, request.ID)
	if err != nil {
		return pendingci.CheckSlot{}, err
	}
	if err := tx.Commit(); err != nil {
		return pendingci.CheckSlot{}, fmt.Errorf("commit check slot reassignment: %w", err)
	}

	return updated, nil
}

func (s *Store) RefreshCheckSlot(
	ctx context.Context,
	request pendingci.RefreshCheckSlotRequest,
) (pendingci.CheckSlot, error) {
	if err := request.Validate(); err != nil {
		return pendingci.CheckSlot{}, err
	}
	result, err := s.db.ExecContext(ctx, `
UPDATE pending_ci_check_slots SET
    state = 'provisioning', applied_digest = '', retry_at = NULL,
    last_error = '', updated_at = ?, revision = revision + 1
WHERE id = ? AND revision = ?`,
		request.RefreshedAt, request.ID, request.ExpectedRevision,
	)
	if err != nil {
		return pendingci.CheckSlot{}, fmt.Errorf("refresh check slot: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return pendingci.CheckSlot{}, err
	}

	return s.GetCheckSlot(ctx, request.ID)
}

func updateCheckSlotDesired(
	ctx context.Context,
	tx *transaction,
	current pendingci.CheckSlot,
	request pendingci.EnsureCheckSlotRequest,
	actions string,
) (pendingci.CheckSlot, error) {
	result, err := tx.ExecContext(ctx, `
UPDATE pending_ci_check_slots SET
	target_id = ?, installation_id = ?, repository_full_name = ?,
	desired_status = ?, desired_conclusion = ?, desired_title = ?,
    desired_summary = ?, desired_actions = ?, desired_digest = ?,
    state = 'provisioning', retry_at = NULL, last_error = '',
    updated_at = ?, revision = revision + 1
WHERE id = ? AND revision = ?`,
		request.TargetID, request.InstallationID, request.RepositoryFullName,
		request.DesiredStatus, nullableString(request.DesiredConclusion),
		request.DesiredTitle, request.DesiredSummary, actions, request.DesiredDigest,
		request.ChangedAt, current.ID, current.Revision,
	)
	if err != nil {
		return pendingci.CheckSlot{}, fmt.Errorf("update desired check slot: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return pendingci.CheckSlot{}, err
	}

	return getPendingCICheckSlot(ctx, tx, current.ID)
}

func (s *Store) GetCheckSlot(ctx context.Context, id int64) (pendingci.CheckSlot, error) {
	slot, err := getPendingCICheckSlot(ctx, s.db, id)
	if errors.Is(err, sql.ErrNoRows) {
		return pendingci.CheckSlot{}, storage.ErrNotFound
	}

	return slot, err
}

func (s *Store) GetCheckSlotByHead(
	ctx context.Context,
	repositoryID, headSHA string,
) (pendingci.CheckSlot, error) {
	slot, err := scanPendingCICheckSlot(s.db.QueryRowContext(
		ctx,
		pendingCICheckSlotSelect+" WHERE repository_id = ? AND head_sha = ?",
		repositoryID,
		headSHA,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return pendingci.CheckSlot{}, storage.ErrNotFound
	}

	return slot, err
}

func (s *Store) ListPendingCheckSlots(
	ctx context.Context,
	now time.Time,
	limit int,
) ([]pendingci.CheckSlot, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, pendingCICheckSlotSelect+`
WHERE desired_digest <> applied_digest AND (retry_at IS NULL OR retry_at <= ?)
ORDER BY updated_at, id
LIMIT ?`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending check slots: %w", err)
	}

	return collectRows(rows, scanPendingCICheckSlot)
}

func (s *Store) BindCheckRun(
	ctx context.Context,
	request pendingci.BindCheckRunRequest,
) (pendingci.CheckSlot, error) {
	if err := request.Validate(); err != nil {
		return pendingci.CheckSlot{}, err
	}
	result, err := s.db.ExecContext(ctx, `
UPDATE pending_ci_check_slots SET
    check_run_id = ?, check_url = ?, state = 'provisioning',
    retry_at = NULL, last_error = '', updated_at = ?, revision = revision + 1
WHERE id = ? AND revision = ? AND (check_run_id IS NULL OR check_run_id = ?)`,
		request.CheckRunID, request.CheckURL, request.BoundAt,
		request.ID, request.ExpectedRevision, request.CheckRunID,
	)
	if err != nil {
		return pendingci.CheckSlot{}, fmt.Errorf("bind check run: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return pendingci.CheckSlot{}, err
	}

	return s.GetCheckSlot(ctx, request.ID)
}

func (s *Store) ApplyCheckSlot(
	ctx context.Context,
	request pendingci.ApplyCheckSlotRequest,
) (pendingci.CheckSlot, error) {
	if err := request.Validate(); err != nil {
		return pendingci.CheckSlot{}, err
	}
	result, err := s.db.ExecContext(ctx, `
UPDATE pending_ci_check_slots SET
    check_run_id = ?, check_url = ?, applied_digest = ?, state = 'ready',
    retry_at = NULL, last_error = '', updated_at = ?, revision = revision + 1
WHERE id = ? AND revision = ? AND (check_run_id IS NULL OR check_run_id = ?)`,
		request.CheckRunID, request.CheckURL, request.AppliedDigest,
		request.AppliedAt, request.ID, request.ExpectedRevision, request.CheckRunID,
	)
	if err != nil {
		return pendingci.CheckSlot{}, fmt.Errorf("apply check slot: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return pendingci.CheckSlot{}, err
	}

	return s.GetCheckSlot(ctx, request.ID)
}

func (s *Store) RetryCheckSlot(
	ctx context.Context,
	request pendingci.RetryCheckSlotRequest,
) (pendingci.CheckSlot, error) {
	if err := request.Validate(); err != nil {
		return pendingci.CheckSlot{}, err
	}
	result, err := s.db.ExecContext(ctx, `
UPDATE pending_ci_check_slots SET
    state = 'blocked', retry_at = ?, last_error = ?,
    updated_at = ?, revision = revision + 1
WHERE id = ? AND revision = ?`,
		request.RetryAt, request.Error, request.FailedAt, request.ID, request.ExpectedRevision,
	)
	if err != nil {
		return pendingci.CheckSlot{}, fmt.Errorf("schedule check slot retry: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return pendingci.CheckSlot{}, err
	}

	return s.GetCheckSlot(ctx, request.ID)
}

func getPendingCICheckSlot(
	ctx context.Context,
	queryer rowQuerier,
	id int64,
) (pendingci.CheckSlot, error) {
	return scanPendingCICheckSlot(queryer.QueryRowContext(
		ctx,
		pendingCICheckSlotSelect+" WHERE id = ?",
		id,
	))
}

func scanPendingCICheckSlot(scanner rowScanner) (pendingci.CheckSlot, error) {
	var slot pendingci.CheckSlot
	var checkRunID sql.NullInt64
	var conclusion sql.NullString
	var actions string
	var retryAt, updatedAt StoredTime
	err := scanner.Scan(
		&slot.ID, &slot.TargetID, &slot.InstallationID, &slot.RepositoryID,
		&slot.RepositoryFullName, &slot.PullRequest, &slot.HeadSHA, &slot.AppID,
		&slot.Name, &slot.ExternalID, &slot.Generation, &checkRunID, &slot.CheckURL,
		&slot.State, &slot.DesiredStatus, &conclusion, &slot.DesiredTitle,
		&slot.DesiredSummary, &actions, &slot.DesiredDigest, &slot.AppliedDigest,
		&retryAt, &slot.LastError, &updatedAt, &slot.Revision,
	)
	if err != nil {
		return pendingci.CheckSlot{}, err
	}
	if checkRunID.Valid {
		slot.CheckRunID = &checkRunID.Int64
	}
	slot.DesiredConclusion = conclusion.String
	if err := json.Unmarshal([]byte(actions), &slot.DesiredActions); err != nil {
		return pendingci.CheckSlot{}, fmt.Errorf("decode desired check actions: %w", err)
	}
	slot.RetryAt = retryAt.Pointer()
	slot.UpdatedAt = updatedAt.Time()

	return slot, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}

	return value
}

func requireOneRow(result sql.Result) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read check slot transition: %w", err)
	}
	if rows != 1 {
		return storage.ErrConflict
	}

	return nil
}
