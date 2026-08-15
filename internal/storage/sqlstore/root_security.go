package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

// BeginElevation creates one absolute, session-bound Root installation grant.
func (s *Store) BeginElevation(
	ctx context.Context,
	grant storage.ElevationGrant,
) (storage.Elevation, error) {
	if strings.TrimSpace(grant.ID) == "" || strings.TrimSpace(grant.SessionTokenHash) == "" ||
		strings.TrimSpace(grant.RootAccountID) == "" || strings.TrimSpace(grant.TargetID) == "" {
		return storage.Elevation{}, errors.New("elevation identity must not be blank")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.Elevation{}, fmt.Errorf("begin Root elevation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := validateRootSession(ctx, tx, grant.SessionTokenHash, grant.RootAccountID, grant.StartedAt); err != nil {
		return storage.Elevation{}, err
	}
	if err := expireSessionElevations(ctx, tx, grant.SessionTokenHash, grant.StartedAt); err != nil {
		return storage.Elevation{}, err
	}
	var active int
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM root_elevations
WHERE session_token_hash = ? AND ended_at IS NULL AND expires_at > ?`,
		grant.SessionTokenHash,
		grant.StartedAt,
	).Scan(&active); err != nil {
		return storage.Elevation{}, fmt.Errorf("check active Root elevation: %w", err)
	}
	if active != 0 {
		return storage.Elevation{}, storage.ErrConflict
	}
	var available bool
	if err := tx.QueryRowContext(ctx, "SELECT available FROM targets WHERE id = ?", grant.TargetID).
		Scan(&available); err != nil {
		return storage.Elevation{}, fmt.Errorf("read elevation installation: %w", noRows(err))
	}
	if !available {
		return storage.Elevation{}, storage.ErrNotFound
	}
	owned, err := targetOwnedBy(ctx, tx, grant.TargetID, grant.RootAccountID)
	if err != nil {
		return storage.Elevation{}, err
	}
	if owned {
		return storage.Elevation{}, storage.ErrConflict
	}
	ownership, err := readTargetOwnership(ctx, tx, grant.TargetID)
	if err != nil {
		return storage.Elevation{}, err
	}
	if !ownership.FreshAt(grant.StartedAt) {
		return storage.Elevation{}, storage.ErrConflict
	}

	elevation := storage.Elevation{
		ID: grant.ID, SessionTokenHash: grant.SessionTokenHash,
		RootAccountID: grant.RootAccountID, TargetID: grant.TargetID,
		Reason: grant.Reason, StartedAt: grant.StartedAt,
		ExpiresAt: grant.StartedAt.Add(storage.ElevationLifetime),
	}
	if err := s.insertElevation(ctx, tx, elevation); err != nil {
		return storage.Elevation{}, err
	}
	if err := insertElevationAudit(ctx, tx, elevation, "elevation.started", "started elevated installation access", grant.StartedAt); err != nil {
		return storage.Elevation{}, err
	}
	if err := tx.Commit(); err != nil {
		return storage.Elevation{}, fmt.Errorf("commit Root elevation: %w", err)
	}

	return elevation, nil
}

// GetElevation returns the active grant for one session and installation.
func (s *Store) GetElevation(
	ctx context.Context,
	sessionTokenHash, targetID string,
	now time.Time,
) (storage.Elevation, error) {
	elevation, err := getElevationBySessionTarget(ctx, s.db, sessionTokenHash, targetID)
	if err != nil {
		return storage.Elevation{}, fmt.Errorf("get Root elevation: %w", noRows(err))
	}
	if !now.Before(elevation.ExpiresAt) {
		_, endErr := s.EndElevation(ctx, elevation.ID, sessionTokenHash, storage.ElevationExpired, now)
		if endErr != nil {
			return storage.Elevation{}, endErr
		}
		return storage.Elevation{}, storage.ErrExpired
	}
	if elevation.EndedAt != nil {
		return storage.Elevation{}, storage.ErrRevoked
	}
	if err := validateRootSession(ctx, s.db, sessionTokenHash, elevation.RootAccountID, now); err != nil {
		return storage.Elevation{}, err
	}

	return elevation, nil
}

// EndElevation ends one session-owned grant without extending its lifetime.
func (s *Store) EndElevation(
	ctx context.Context,
	id, sessionTokenHash string,
	reason storage.ElevationEndReason,
	endedAt time.Time,
) (storage.Elevation, error) {
	if !validElevationEndReason(reason) {
		return storage.Elevation{}, fmt.Errorf("unsupported elevation end reason %q", reason)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.Elevation{}, fmt.Errorf("begin Root elevation end: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	elevation, err := getElevationByID(ctx, tx, id, sessionTokenHash)
	if err != nil {
		return storage.Elevation{}, fmt.Errorf("read Root elevation for end: %w", noRows(err))
	}
	if elevation.EndedAt == nil {
		if err := endElevation(ctx, tx, &elevation, reason, endedAt); err != nil {
			return storage.Elevation{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return storage.Elevation{}, fmt.Errorf("commit Root elevation end: %w", err)
	}

	return elevation, nil
}

// EndSessionElevations terminates every live grant bound to one session.
func (s *Store) EndSessionElevations(
	ctx context.Context,
	sessionTokenHash string,
	reason storage.ElevationEndReason,
	endedAt time.Time,
) error {
	if !validElevationEndReason(reason) {
		return fmt.Errorf("unsupported elevation end reason %q", reason)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session elevation end: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := endSessionElevations(ctx, tx, sessionTokenHash, reason, endedAt); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit session elevation end: %w", err)
	}

	return nil
}

func endSessionElevations(
	ctx context.Context,
	tx runner,
	sessionTokenHash string,
	reason storage.ElevationEndReason,
	endedAt time.Time,
) error {
	elevations, err := listOpenSessionElevations(ctx, tx, sessionTokenHash)
	if err != nil {
		return err
	}
	for index := range elevations {
		if err := endElevation(ctx, tx, &elevations[index], reason, endedAt); err != nil {
			return err
		}
	}

	return nil
}

func elevatedWrite(
	ctx context.Context,
	tx runner,
	elevationID *string,
	sessionTokenHash, actorAccountID, targetID string,
	changedAt time.Time,
) (*storage.Elevation, error) {
	if elevationID == nil {
		return nil, nil
	}
	elevation, err := getElevationByID(ctx, tx, *elevationID, sessionTokenHash)
	if err != nil {
		return nil, fmt.Errorf("validate elevated write: %w", noRows(err))
	}
	if elevation.RootAccountID != actorAccountID || elevation.TargetID != targetID {
		return nil, storage.ErrNotFound
	}
	owned, err := targetOwnedBy(ctx, tx, targetID, actorAccountID)
	if err != nil {
		return nil, err
	}
	if owned {
		return nil, storage.ErrConflict
	}
	if !elevation.ActiveAt(changedAt) {
		return nil, storage.ErrExpired
	}
	if err := validateRootSession(ctx, tx, sessionTokenHash, actorAccountID, changedAt); err != nil {
		return nil, err
	}
	ownership, err := readTargetOwnership(ctx, tx, targetID)
	if err != nil {
		return nil, err
	}
	if !ownership.FreshAt(changedAt) {
		return nil, storage.ErrConflict
	}

	return &elevation, nil
}

func targetOwnedBy(
	ctx context.Context,
	queryer rowQuerier,
	targetID, accountID string,
) (bool, error) {
	var owned bool
	if err := queryer.QueryRowContext(ctx, `
SELECT EXISTS(
    SELECT 1 FROM target_owners WHERE target_id = ? AND account_id = ?
)`, targetID, accountID).Scan(&owned); err != nil {
		return false, fmt.Errorf("resolve Root installation ownership: %w", err)
	}

	return owned, nil
}

func validateRootSession(
	ctx context.Context,
	queryer rowQuerier,
	sessionTokenHash, rootAccountID string,
	now time.Time,
) error {
	var accountID string
	var expiresAt, revokedAt StoredTime
	var role storage.SystemRole
	var status storage.PanelUserStatus
	err := queryer.QueryRowContext(ctx, `
SELECT s.account_id, s.expires_at, s.revoked_at, pu.system_role, pu.status
FROM sessions s
JOIN panel_users pu ON pu.account_id = s.account_id
WHERE s.token_hash = ?`, sessionTokenHash).Scan(
		&accountID, &expiresAt, &revokedAt, &role, &status,
	)
	if err != nil {
		return fmt.Errorf("validate Root session: %w", noRows(err))
	}
	if accountID != rootAccountID || revokedAt.Valid() || !now.Before(expiresAt.Time()) ||
		status != storage.PanelUserActive || !role.IsRoot() {
		return storage.ErrRevoked
	}

	return nil
}

func readTargetOwnership(
	ctx context.Context,
	queryer rowQuerier,
	targetID string,
) (storage.TargetOwnership, error) {
	var ownership storage.TargetOwnership
	var detail sql.NullString
	var syncedAt StoredTime
	err := queryer.QueryRowContext(ctx, `
SELECT source, status, detail, synced_at,
       (SELECT COUNT(*) FROM target_owners WHERE target_id = ?)
FROM target_ownership WHERE target_id = ?`, targetID, targetID).Scan(
		&ownership.Source, &ownership.Status, &detail, &syncedAt, &ownership.OwnerCount,
	)
	if err != nil {
		return storage.TargetOwnership{}, fmt.Errorf("read elevated ownership: %w", noRows(err))
	}
	ownership.Detail = stringPointer(detail)
	ownership.SyncedAt = syncedAt.Time()

	return ownership, nil
}

func (s *Store) insertElevation(
	ctx context.Context,
	tx runner,
	elevation storage.Elevation,
) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO root_elevations (
    id, session_token_hash, root_account_id, target_id, reason, started_at, expires_at
) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		elevation.ID,
		elevation.SessionTokenHash,
		elevation.RootAccountID,
		elevation.TargetID,
		elevation.Reason,
		elevation.StartedAt,
		elevation.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("insert Root elevation: %w", s.conflictConstraint(err))
	}

	return nil
}

func insertElevationAudit(
	ctx context.Context,
	tx runner,
	elevation storage.Elevation,
	action, summary string,
	createdAt time.Time,
) error {
	targetID := elevation.TargetID
	_, err := insertAppAudit(ctx, tx, appAuditInsert{
		Category: "elevation", TargetID: &targetID,
		ActorAccountID: elevation.RootAccountID, ElevationID: &elevation.ID,
		Action: action, Summary: summary, CreatedAt: createdAt,
	})

	return err
}

func endElevation(
	ctx context.Context,
	tx runner,
	elevation *storage.Elevation,
	reason storage.ElevationEndReason,
	endedAt time.Time,
) error {
	result, err := tx.ExecContext(ctx, `
UPDATE root_elevations SET ended_at = ?, end_reason = ?
WHERE id = ? AND ended_at IS NULL`,
		endedAt, reason, elevation.ID,
	)
	if err != nil {
		return fmt.Errorf("end Root elevation: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read Root elevation end result: %w", err)
	}
	if changed != 1 {
		return storage.ErrConflict
	}
	elevation.EndedAt = &endedAt
	elevation.EndReason = &reason
	action := "elevation.ended"
	summary := "ended elevated installation access"
	switch reason {
	case storage.ElevationExpired:
		action = "elevation.expired"
		summary = "elevated installation access expired"
	case storage.ElevationRevoked:
		action = "elevation.revoked"
		summary = "revoked elevated installation access"
	}

	return insertElevationAudit(ctx, tx, *elevation, action, summary, endedAt)
}

func expireSessionElevations(
	ctx context.Context,
	tx runner,
	sessionTokenHash string,
	now time.Time,
) error {
	elevations, err := listOpenSessionElevations(ctx, tx, sessionTokenHash)
	if err != nil {
		return err
	}
	for index := range elevations {
		if !now.Before(elevations[index].ExpiresAt) {
			if err := endElevation(ctx, tx, &elevations[index], storage.ElevationExpired, now); err != nil {
				return err
			}
		}
	}

	return nil
}

func validElevationEndReason(reason storage.ElevationEndReason) bool {
	return reason == storage.ElevationEnded || reason == storage.ElevationExpired ||
		reason == storage.ElevationRevoked
}
