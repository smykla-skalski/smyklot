package sqlite

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
    pu.root,
    pu.status,
    pu.global_role,
    pu.ban_reason,
    pu.banned_at,
    pu.removed_at,
    pu.last_login_at,
    pu.revision,
    pu.created_at,
    pu.updated_at
FROM panel_users pu
JOIN accounts a ON a.id = pu.account_id`

// GetPanelUser returns one persisted panel identity and global policy.
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
	if !validGlobalRole(change.GlobalRole) {
		return storage.PanelUser{}, fmt.Errorf("unsupported global role %q", change.GlobalRole)
	}
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
    account_id, root, status, global_role, revision, created_at, updated_at
) VALUES (?, 0, 'active', ?, 1, ?, ?)
ON CONFLICT(account_id) DO UPDATE SET
    status = 'active',
    global_role = excluded.global_role,
    ban_reason = NULL,
    banned_at = NULL,
    removed_at = NULL,
    revision = panel_users.revision + 1,
    updated_at = excluded.updated_at
WHERE panel_users.root = 0 AND panel_users.status = 'removed'`,
		change.AccountID,
		change.GlobalRole,
		formatTime(change.ChangedAt),
		formatTime(change.ChangedAt),
	)
	if err != nil {
		return storage.PanelUser{}, fmt.Errorf("insert panel user: %w", conflictConstraint(err))
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return storage.PanelUser{}, fmt.Errorf("read panel user create result: %w", err)
	}
	if changed != 1 {
		return storage.PanelUser{}, storage.ErrConflict
	}
	action := "user.created"
	summary := fmt.Sprintf("added user as %s", change.GlobalRole)
	if previousErr == nil {
		action = "user.readded"
		summary = fmt.Sprintf("re-added user as %s", change.GlobalRole)
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
	summary := "updated installation access"
	if change.Suspended {
		action = "target.access.suspended"
		summary = "suspended installation access" + accessReasonSuffix(change.SuspensionReason)
	} else if previouslySuspended {
		action = "target.access.restored"
		summary = "restored installation access"
	}
	if err := insertAccessAudit(
		ctx,
		tx,
		&change.TargetID,
		change.ActorAccountID,
		change.SubjectAccountID,
		action,
		summary,
		change.ChangedAt,
	); err != nil {
		return storage.TargetAccessOverride{}, err
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

// ResolveTargetAccess applies account status, root, suspension, override, and
// global role precedence to one available installation.
func (s *Store) ResolveTargetAccess(
	ctx context.Context,
	accountID, targetID string,
) (storage.TargetAccess, error) {
	var root, suspended bool
	var status storage.PanelUserStatus
	var globalRole storage.PanelRole
	var targetRole, suspensionReason sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT pu.root, pu.status, pu.global_role, tr.role, COALESCE(tr.suspended, 0), tr.suspension_reason
FROM panel_users pu
JOIN targets t ON t.id = ? AND t.available = 1
LEFT JOIN target_roles tr ON tr.account_id = pu.account_id AND tr.target_id = t.id
WHERE pu.account_id = ?`, targetID, accountID).Scan(
		&root,
		&status,
		&globalRole,
		&targetRole,
		&suspended,
		&suspensionReason,
	)
	if err != nil {
		return storage.TargetAccess{}, fmt.Errorf("resolve target access: %w", noRows(err))
	}

	access := resolvedTargetAccess(root, status, globalRole, targetRole, suspended)
	access.SuspensionReason = stringPointer(suspensionReason)
	access.Capabilities = storage.EffectiveCapabilities(access.Role, root)

	return access, nil
}

// ListTargets returns installations permitted by the current access policy.
func (s *Store) ListTargets(ctx context.Context, accountID string) ([]storage.Target, error) {
	rows, err := s.db.QueryContext(ctx, targetSelect+`
JOIN panel_users pu ON pu.account_id = ?
LEFT JOIN target_roles tr ON tr.account_id = pu.account_id AND tr.target_id = t.id
WHERE t.available = 1
  AND pu.status = 'active'
  AND (
      pu.root = 1
      OR pu.global_role = 'owner'
      OR (
          COALESCE(tr.suspended, 0) = 0
          AND COALESCE(tr.role, pu.global_role) IN ('viewer', 'editor', 'admin')
      )
  )
GROUP BY t.id, a.id
ORDER BY a.login`, accountID)
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
	root bool,
	status storage.PanelUserStatus,
	globalRole storage.PanelRole,
	targetRole sql.NullString,
	suspended bool,
) storage.TargetAccess {
	if status != storage.PanelUserActive {
		return storage.TargetAccess{Role: storage.PanelRoleNone, Source: storage.AccessSourceDenied}
	}
	if root || globalRole == storage.PanelRoleOwner {
		return storage.TargetAccess{Role: storage.PanelRoleOwner, Source: rootSource(root), Root: root}
	}
	if suspended {
		return storage.TargetAccess{
			Role: storage.PanelRoleNone, Source: storage.AccessSourceSuspended, Root: root,
		}
	}
	if targetRole.Valid {
		return storage.TargetAccess{
			Role: storage.PanelRole(targetRole.String), Source: storage.AccessSourceTarget, Root: root,
		}
	}

	return storage.TargetAccess{Role: globalRole, Source: storage.AccessSourceGlobal, Root: root}
}

func rootSource(root bool) storage.AccessSource {
	if root {
		return storage.AccessSourceRoot
	}

	return storage.AccessSourceGlobal
}

func targetAccessRevision(
	ctx context.Context,
	tx *sql.Tx,
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
	tx *sql.Tx,
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
		formatTime(change.ChangedAt),
	)
	if err != nil {
		return fmt.Errorf("insert target access: %w", err)
	}

	return nil
}

func updateTargetAccess(
	ctx context.Context,
	tx *sql.Tx,
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
		formatTime(change.ChangedAt),
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
	var updatedAt string
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
		parsed := storage.PanelRole(role.String)
		override.Role = &parsed
	}
	override.SuspensionReason = stringPointer(reason)
	override.UpdatedAt, err = parseTime(updatedAt)

	return override, err
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
	var avatar, banReason, bannedAt, removedAt, lastLoginAt sql.NullString
	var accountUpdatedAt, createdAt, updatedAt string
	err := scanner.Scan(
		&user.Account.ID,
		&user.Account.Provider,
		&user.Account.SubjectID,
		&user.Account.Login,
		&user.Account.DisplayName,
		&avatar,
		&accountUpdatedAt,
		&user.Root,
		&user.Status,
		&user.GlobalRole,
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
	user.Account.UpdatedAt, err = parseTime(accountUpdatedAt)
	if err != nil {
		return storage.PanelUser{}, err
	}
	user.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return storage.PanelUser{}, err
	}
	user.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return storage.PanelUser{}, err
	}
	if user.BannedAt, err = nullableTime(bannedAt); err != nil {
		return storage.PanelUser{}, err
	}
	if user.RemovedAt, err = nullableTime(removedAt); err != nil {
		return storage.PanelUser{}, err
	}
	user.LastLoginAt, err = nullableTime(lastLoginAt)

	return user, err
}

func nullableTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	parsed, err := parseTime(value.String)
	if err != nil {
		return nil, err
	}

	return &parsed, nil
}

func insertAccessAudit(
	ctx context.Context,
	executor accountExecutor,
	targetID *string,
	actorAccountID, subjectAccountID, action, summary string,
	changedAt time.Time,
) error {
	_, err := executor.ExecContext(ctx, `
INSERT INTO access_audit_entries (
    target_id, actor_account_id, subject_account_id, action, summary, created_at
) VALUES (?, ?, ?, ?, ?, ?)`,
		targetID,
		actorAccountID,
		subjectAccountID,
		action,
		summary,
		formatTime(changedAt),
	)
	if err != nil {
		return fmt.Errorf("insert access audit: %w", err)
	}

	return nil
}

func validGlobalRole(role storage.PanelRole) bool {
	switch role {
	case storage.PanelRoleNone,
		storage.PanelRoleViewer,
		storage.PanelRoleEditor,
		storage.PanelRoleAdmin,
		storage.PanelRoleOwner:
		return true
	default:
		return false
	}
}

func validTargetRole(role storage.PanelRole) bool {
	return validGlobalRole(role) && role != storage.PanelRoleOwner
}

func conflictConstraint(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "constraint failed") {
		return storage.ErrConflict
	}

	return err
}
