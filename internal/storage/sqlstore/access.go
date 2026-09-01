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

const panelUserSelect = `
SELECT
    a.id,
    a.provider,
    a.subject_id,
    a.login,
    a.display_name,
    a.avatar_url,
    a.updated_at,
    pu.system_role,
    pu.status,
    pu.ban_reason,
    pu.banned_at,
    pu.removed_at,
    pu.last_login_at,
    pu.revision,
    pu.created_at,
    pu.updated_at
FROM panel_users pu
JOIN accounts a ON a.id = pu.account_id`

// GetPanelUser returns one persisted panel identity and lifecycle.
func (s *Store) GetPanelUser(ctx context.Context, accountID string) (storage.PanelUser, error) {
	user, err := getPanelUser(ctx, s.db, accountID)
	if err != nil {
		return storage.PanelUser{}, fmt.Errorf("get panel user: %w", noRows(err))
	}

	return user, nil
}

// CreatePanelUser activates a known account and records the access mutation.
func (s *Store) CreatePanelUser(
	ctx context.Context,
	change storage.PanelUserCreate,
) (storage.PanelUser, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.PanelUser{}, fmt.Errorf("begin panel user create: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var previousStatus storage.PanelUserStatus
	previousErr := tx.QueryRowContext(
		ctx,
		"SELECT status FROM panel_users WHERE account_id = ?",
		change.AccountID,
	).Scan(&previousStatus)
	if previousErr != nil && !errors.Is(previousErr, sql.ErrNoRows) {
		return storage.PanelUser{}, fmt.Errorf("read existing panel user: %w", previousErr)
	}
	if previousErr == nil && previousStatus != storage.PanelUserRemoved {
		return storage.PanelUser{}, storage.ErrConflict
	}
	result, err := tx.ExecContext(ctx, `
INSERT INTO panel_users (
    account_id, status, system_role, revision, created_at, updated_at
) VALUES (?, 'active', 'none', 1, ?, ?)
ON CONFLICT(account_id) DO UPDATE SET
    status = 'active',
    ban_reason = NULL,
    banned_at = NULL,
    removed_at = NULL,
    revision = panel_users.revision + 1,
    updated_at = excluded.updated_at
WHERE panel_users.system_role = 'none' AND panel_users.status = 'removed'`,
		change.AccountID,
		change.ChangedAt,
		change.ChangedAt,
	)
	if err != nil {
		return storage.PanelUser{}, fmt.Errorf("insert panel user: %w", s.conflictConstraint(err))
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return storage.PanelUser{}, fmt.Errorf("read panel user create result: %w", err)
	}
	if changed != 1 {
		return storage.PanelUser{}, storage.ErrConflict
	}
	action := "user.created"
	summary := "added user"
	if previousErr == nil {
		action = "user.readded"
		summary = "re-added user"
	}
	if err := insertAccessAudit(
		ctx,
		tx,
		nil,
		change.ActorAccountID,
		change.AccountID,
		action,
		summary,
		change.ChangedAt,
	); err != nil {
		return storage.PanelUser{}, err
	}
	user, err := getPanelUser(ctx, tx, change.AccountID)
	if err != nil {
		return storage.PanelUser{}, fmt.Errorf("read created panel user: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return storage.PanelUser{}, fmt.Errorf("commit panel user create: %w", err)
	}

	return user, nil
}

// SetTargetAccess replaces one local role and suspension overlay.
func (s *Store) SetTargetAccess(
	ctx context.Context,
	change storage.TargetAccessChange,
) (storage.TargetAccessOverride, error) {
	if change.Role != nil && !validTargetRole(*change.Role) {
		return storage.TargetAccessOverride{}, fmt.Errorf("unsupported target role %q", *change.Role)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.TargetAccessOverride{}, fmt.Errorf("begin target access change: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	elevation, err := s.elevatedWrite(
		ctx,
		tx,
		change.ElevationID,
		change.SessionTokenHash,
		change.ActorAccountID,
		change.TargetID,
		change.ChangedAt,
	)
	if err != nil {
		return storage.TargetAccessOverride{}, err
	}

	currentRevision, found, err := targetAccessRevision(ctx, tx, change)
	if err != nil {
		return storage.TargetAccessOverride{}, err
	}
	if currentRevision != change.ExpectedRevision {
		return storage.TargetAccessOverride{}, storage.ErrConflict
	}
	previouslySuspended := false
	if found {
		current, readErr := getTargetAccessOverride(
			ctx, tx, change.SubjectAccountID, change.TargetID,
		)
		if readErr != nil {
			return storage.TargetAccessOverride{}, fmt.Errorf("read current target access: %w", readErr)
		}
		previouslySuspended = current.Suspended
	}
	nextRevision := currentRevision + 1
	if !found {
		err = insertTargetAccess(ctx, tx, change, nextRevision)
	} else {
		err = updateTargetAccess(ctx, tx, change, nextRevision)
	}
	if err != nil {
		return storage.TargetAccessOverride{}, err
	}
	action := "target.access.updated"
	summary := "updated workspace access"
	if change.Suspended {
		action = "target.access.suspended"
		summary = "suspended workspace access" + accessReasonSuffix(change.SuspensionReason)
	} else if previouslySuspended {
		action = "target.access.restored"
		summary = "restored workspace access"
	}
	auditEventID, err := insertAccessAuditEvent(
		ctx,
		tx,
		&change.TargetID,
		change.ActorAccountID,
		change.SubjectAccountID,
		action,
		summary,
		change.ChangedAt,
		change.ElevationID,
	)
	if err != nil {
		return storage.TargetAccessOverride{}, err
	}
	if elevation != nil {
		if err := insertElevatedNotifications(
			ctx, tx, *elevation, auditEventID, action, change.ChangedAt,
		); err != nil {
			return storage.TargetAccessOverride{}, err
		}
	}
	override, err := getTargetAccessOverride(ctx, tx, change.SubjectAccountID, change.TargetID)
	if err != nil {
		return storage.TargetAccessOverride{}, fmt.Errorf("read changed target access: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return storage.TargetAccessOverride{}, fmt.Errorf("commit target access change: %w", err)
	}

	return override, nil
}

func accessReasonSuffix(value *string) string {
	if value == nil {
		return ""
	}
	reason := strings.TrimSpace(*value)
	if reason == "" {
		return ""
	}

	return ": " + reason
}

// GetTargetAccessOverride returns one persisted installation-specific policy.
func (s *Store) GetTargetAccessOverride(
	ctx context.Context,
	accountID, targetID string,
) (storage.TargetAccessOverride, error) {
	override, err := getTargetAccessOverride(ctx, s.db, accountID, targetID)
	if err != nil {
		return storage.TargetAccessOverride{}, fmt.Errorf(
			"get target access override: %w",
			noRows(err),
		)
	}

	return override, nil
}

// ResolveTargetAccess applies account status, fresh ownership, system role,
// suspension, and explicit installation role precedence.
func (s *Store) ResolveTargetAccess(
	ctx context.Context,
	accountID, targetID string,
	now time.Time,
) (storage.TargetAccess, error) {
	var systemRole storage.SystemRole
	var suspended bool
	var status storage.PanelUserStatus
	var targetRole, suspensionReason, ownershipStatus sql.NullString
	var ownershipSyncedAt StoredTime
	var ownerCount int
	var owned bool
	err := s.db.QueryRowContext(ctx, `
SELECT
    pu.system_role,
    pu.status,
    tr.role,
    COALESCE(tr.suspended, FALSE),
    tr.suspension_reason,
    ownership.status,
    ownership.synced_at,
    (SELECT COUNT(*) FROM target_owners owners WHERE owners.target_id = t.id),
    EXISTS(SELECT 1 FROM target_owners owners WHERE owners.target_id = t.id AND owners.account_id = pu.account_id)
FROM panel_users pu
JOIN targets t ON t.id = ? AND t.available = TRUE
LEFT JOIN target_roles tr ON tr.account_id = pu.account_id AND tr.target_id = t.id
LEFT JOIN target_ownership ownership ON ownership.target_id = t.id
WHERE pu.account_id = ?`, targetID, accountID).Scan(
		&systemRole,
		&status,
		&targetRole,
		&suspended,
		&suspensionReason,
		&ownershipStatus,
		&ownershipSyncedAt,
		&ownerCount,
		&owned,
	)
	if err != nil {
		return storage.TargetAccess{}, fmt.Errorf("resolve target access: %w", noRows(err))
	}

	fresh := freshOwnership(ownershipStatus, ownershipSyncedAt, ownerCount, now)
	access := resolvedTargetAccess(systemRole, status, targetRole, suspended, owned, fresh)
	access.SuspensionReason = stringPointer(suspensionReason)
	access.Capabilities = storage.EffectiveCapabilities(access.Role)

	return access, nil
}

// ListTargets returns installations permitted by the current access policy.
func (s *Store) ListTargets(
	ctx context.Context,
	accountID string,
	now time.Time,
) ([]storage.Target, error) {
	rows, err := s.db.QueryContext(ctx, targetSelect+`
JOIN panel_users pu ON pu.account_id = ?
LEFT JOIN target_owners current_owner
  ON current_owner.account_id = pu.account_id AND current_owner.target_id = t.id
LEFT JOIN target_roles tr ON tr.account_id = pu.account_id AND tr.target_id = t.id
WHERE t.available = TRUE
  AND pu.status = 'active'
  AND o.status = 'fresh'
  AND o.synced_at >= ?
  AND EXISTS(SELECT 1 FROM target_owners owners WHERE owners.target_id = t.id)
  AND (
      current_owner.account_id IS NOT NULL
      OR (
          pu.system_role = 'none'
          AND COALESCE(tr.suspended, FALSE) = FALSE
          AND tr.role IN ('viewer', 'editor', 'admin')
      )
  )
`+targetGroup+`
ORDER BY a.login`, accountID, now.Add(-storage.OwnershipFreshFor))
	if err != nil {
		return nil, fmt.Errorf("list targets: %w", err)
	}

	targets, err := collectRows(rows, scanTarget)
	if err != nil {
		return nil, fmt.Errorf("read targets: %w", err)
	}

	return targets, nil
}

func resolvedTargetAccess(
	systemRole storage.SystemRole,
	status storage.PanelUserStatus,
	targetRole sql.NullString,
	suspended bool,
	owned bool,
	ownershipFresh bool,
) storage.TargetAccess {
	root := systemRole.IsRoot()
	if status != storage.PanelUserActive || !ownershipFresh {
		return storage.TargetAccess{Role: storage.InstallationRoleNone, Source: storage.AccessSourceDenied}
	}
	if owned {
		return storage.TargetAccess{
			Role: storage.InstallationRoleOwner, Source: storage.AccessSourceOwner, Root: root,
		}
	}
	if root {
		return storage.TargetAccess{
			Role: storage.InstallationRoleNone, Source: storage.AccessSourceDenied, Root: true,
		}
	}
	if suspended {
		return storage.TargetAccess{
			Role: storage.InstallationRoleNone, Source: storage.AccessSourceSuspended, Root: root,
		}
	}
	if targetRole.Valid && storage.InstallationRole(targetRole.String) != storage.InstallationRoleNone {
		return storage.TargetAccess{
			Role: storage.InstallationRole(targetRole.String), Source: storage.AccessSourceTarget, Root: root,
		}
	}

	return storage.TargetAccess{Role: storage.InstallationRoleNone, Source: storage.AccessSourceDenied}
}

func freshOwnership(
	status sql.NullString,
	syncedAt StoredTime,
	ownerCount int,
	now time.Time,
) bool {
	if !status.Valid || storage.OwnershipStatus(status.String) != storage.OwnershipStatusFresh ||
		!syncedAt.Valid() || ownerCount == 0 {
		return false
	}

	return !syncedAt.Time().Before(now.Add(-storage.OwnershipFreshFor))
}

func targetAccessRevision(
	ctx context.Context,
	tx runner,
	change storage.TargetAccessChange,
) (int64, bool, error) {
	var revision int64
	err := tx.QueryRowContext(ctx, `
SELECT revision FROM target_roles WHERE account_id = ? AND target_id = ?`,
		change.SubjectAccountID,
		change.TargetID,
	).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("read target access revision: %w", err)
	}

	return revision, true, nil
}

func insertTargetAccess(
	ctx context.Context,
	tx runner,
	change storage.TargetAccessChange,
	revision int64,
) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO target_roles (
    account_id, target_id, role, suspended, suspension_reason,
    revision, updated_by, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		change.SubjectAccountID,
		change.TargetID,
		change.Role,
		change.Suspended,
		change.SuspensionReason,
		revision,
		change.ActorAccountID,
		change.ChangedAt,
	)
	if err != nil {
		return fmt.Errorf("insert target access: %w", err)
	}

	return nil
}

func updateTargetAccess(
	ctx context.Context,
	tx runner,
	change storage.TargetAccessChange,
	revision int64,
) error {
	_, err := tx.ExecContext(ctx, `
UPDATE target_roles
SET role = ?, suspended = ?, suspension_reason = ?, revision = ?, updated_by = ?, updated_at = ?
WHERE account_id = ? AND target_id = ?`,
		change.Role,
		change.Suspended,
		change.SuspensionReason,
		revision,
		change.ActorAccountID,
		change.ChangedAt,
		change.SubjectAccountID,
		change.TargetID,
	)
	if err != nil {
		return fmt.Errorf("update target access: %w", err)
	}

	return nil
}

func getTargetAccessOverride(
	ctx context.Context,
	queryer rowQuerier,
	accountID, targetID string,
) (storage.TargetAccessOverride, error) {
	var override storage.TargetAccessOverride
	var role, reason sql.NullString
	var updatedAt StoredTime
	err := queryer.QueryRowContext(ctx, `
SELECT target_id, account_id, role, suspended, suspension_reason, revision, updated_at
FROM target_roles WHERE account_id = ? AND target_id = ?`, accountID, targetID).Scan(
		&override.TargetID,
		&override.AccountID,
		&role,
		&override.Suspended,
		&reason,
		&override.Revision,
		&updatedAt,
	)
	if err != nil {
		return storage.TargetAccessOverride{}, err
	}
	if role.Valid {
		parsed := storage.InstallationRole(role.String)
		override.Role = &parsed
	}
	override.SuspensionReason = stringPointer(reason)
	override.UpdatedAt = updatedAt.Time()

	return override, nil
}

func getPanelUser(
	ctx context.Context,
	queryer rowQuerier,
	accountID string,
) (storage.PanelUser, error) {
	return scanPanelUser(queryer.QueryRowContext(
		ctx,
		panelUserSelect+" WHERE pu.account_id = ?",
		accountID,
	))
}

func scanPanelUser(scanner rowScanner) (storage.PanelUser, error) {
	var user storage.PanelUser
	var avatar, banReason sql.NullString
	var bannedAt, removedAt, lastLoginAt StoredTime
	var accountUpdatedAt, createdAt, updatedAt StoredTime
	err := scanner.Scan(
		&user.Account.ID,
		&user.Account.Provider,
		&user.Account.SubjectID,
		&user.Account.Login,
		&user.Account.DisplayName,
		&avatar,
		&accountUpdatedAt,
		&user.SystemRole,
		&user.Status,
		&banReason,
		&bannedAt,
		&removedAt,
		&lastLoginAt,
		&user.Revision,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return storage.PanelUser{}, err
	}
	user.Account.AvatarURL = stringPointer(avatar)
	user.BanReason = stringPointer(banReason)
	user.Account.UpdatedAt = accountUpdatedAt.Time()
	user.CreatedAt = createdAt.Time()
	user.UpdatedAt = updatedAt.Time()
	user.BannedAt = bannedAt.Pointer()
	user.RemovedAt = removedAt.Pointer()
	user.LastLoginAt = lastLoginAt.Pointer()

	return user, nil
}

func insertAccessAudit(
	ctx context.Context,
	executor runner,
	targetID *string,
	actorAccountID, subjectAccountID, action, summary string,
	changedAt time.Time,
) error {
	_, err := insertAccessAuditEvent(
		ctx, executor, targetID, actorAccountID, subjectAccountID,
		action, summary, changedAt, nil,
	)

	return err
}

func insertAccessAuditEvent(
	ctx context.Context,
	executor runner,
	targetID *string,
	actorAccountID, subjectAccountID, action, summary string,
	changedAt time.Time,
	elevationID *string,
) (int64, error) {
	var sourceID int64
	err := executor.QueryRowContext(ctx, `
INSERT INTO access_audit_entries (
    target_id, actor_account_id, subject_account_id, action, summary, created_at
) VALUES (?, ?, ?, ?, ?, ?)
RETURNING id`,
		targetID,
		actorAccountID,
		subjectAccountID,
		action,
		summary,
		changedAt,
	).Scan(&sourceID)
	if err != nil {
		return 0, fmt.Errorf("insert access audit: %w", err)
	}
	sourceKind := "access"
	auditEventID, err := insertAppAudit(ctx, executor, appAuditInsert{
		Category:         "access",
		SourceKind:       &sourceKind,
		SourceID:         &sourceID,
		TargetID:         targetID,
		ActorAccountID:   actorAccountID,
		SubjectAccountID: &subjectAccountID,
		ElevationID:      elevationID,
		Action:           action,
		Summary:          summary,
		CreatedAt:        changedAt,
	})
	if err != nil {
		return 0, err
	}

	return auditEventID, nil
}

func validInstallationRole(role storage.InstallationRole) bool {
	switch role {
	case storage.InstallationRoleNone,
		storage.InstallationRoleViewer,
		storage.InstallationRoleEditor,
		storage.InstallationRoleAdmin,
		storage.InstallationRoleOwner:
		return true
	default:
		return false
	}
}

func validTargetRole(role storage.InstallationRole) bool {
	return validInstallationRole(role) && role != storage.InstallationRoleOwner
}

// conflictConstraint reports a row that already exists as a conflict the
// caller can act on, and leaves every other failure alone.
func (s *Store) conflictConstraint(err error) error {
	if err == nil {
		return nil
	}
	if s.dialect.UniqueViolation(err) {
		return storage.ErrConflict
	}

	return err
}
