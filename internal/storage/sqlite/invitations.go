package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

const invitationSelect = `
SELECT
    ui.id,
    invited.id,
    invited.provider,
    invited.subject_id,
    invited.login,
    invited.display_name,
    invited.avatar_url,
    invited.updated_at,
    ui.target_id,
    target_account.display_name,
    target_account.login,
    target.kind,
    ui.role,
    ui.system_role,
    ui.status,
    ui.expires_at,
    creator.id,
    creator.provider,
    creator.subject_id,
    creator.login,
    creator.display_name,
    creator.avatar_url,
    creator.updated_at,
    ui.created_at,
    ui.responded_at
FROM user_invitations ui
JOIN accounts invited ON invited.id = ui.account_id
JOIN accounts creator ON creator.id = ui.created_by
LEFT JOIN targets target ON target.id = ui.target_id
LEFT JOIN accounts target_account ON target_account.id = target.account_id`

// ListInvitations returns invitations for exactly one global or installation scope.
func (s *Store) ListInvitations(
	ctx context.Context,
	targetID *string,
	now time.Time,
) ([]storage.Invitation, error) {
	rows, err := s.db.QueryContext(ctx, invitationSelect+`
WHERE (ui.target_id IS NULL AND ? IS NULL) OR ui.target_id = ?
ORDER BY ui.created_at DESC, ui.id DESC`, targetID, targetID)
	if err != nil {
		return nil, fmt.Errorf("list invitations: %w", err)
	}
	invitations, err := collectRows(rows, func(scanner rowScanner) (storage.Invitation, error) {
		return scanInvitation(scanner, now)
	})
	if err != nil {
		return nil, fmt.Errorf("read invitations: %w", err)
	}

	return invitations, nil
}

// GetInvitation returns one invitation by its stable management identifier.
func (s *Store) GetInvitation(
	ctx context.Context,
	id string,
	now time.Time,
) (storage.Invitation, error) {
	invitation, err := getInvitation(ctx, s.db, "ui.id = ?", id, now)
	if err != nil {
		return storage.Invitation{}, fmt.Errorf("get invitation: %w", noRows(err))
	}

	return invitation, nil
}

// GetInvitationByToken returns a pending invitation addressed by its token digest.
func (s *Store) GetInvitationByToken(
	ctx context.Context,
	tokenHash string,
	now time.Time,
) (storage.Invitation, error) {
	invitation, err := getInvitation(ctx, s.db, "ui.token_hash = ?", tokenHash, now)
	if err != nil {
		return storage.Invitation{}, fmt.Errorf("get invitation by token: %w", noRows(err))
	}
	if invitation.Status == storage.InvitationExpired {
		return invitation, storage.ErrExpired
	}

	return invitation, nil
}

// CreateInvitation persists a named offer and revokes older pending offers in
// the same scope atomically.
func (s *Store) CreateInvitation(
	ctx context.Context,
	change storage.InvitationCreate,
) (storage.Invitation, error) {
	if !validInvitationPolicy(change.Role, change.SystemRole, change.TargetID) ||
		!change.CreatedAt.Before(change.ExpiresAt) {
		return storage.Invitation{}, errors.New("unsupported invitation policy")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.Invitation{}, fmt.Errorf("begin invitation create: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	elevation, err := elevatedInvitationWrite(
		ctx,
		tx,
		change.TargetID,
		change.ElevationID,
		change.SessionTokenHash,
		change.CreatedByAccount,
		change.CreatedAt,
	)
	if err != nil {
		return storage.Invitation{}, err
	}

	if err = invitationOfferable(ctx, tx, change); err != nil {
		return storage.Invitation{}, err
	}
	if _, err = tx.ExecContext(ctx, `
UPDATE user_invitations
SET status = 'revoked', responded_at = ?
WHERE account_id = ? AND status = 'pending'
  AND ((target_id IS NULL AND ? IS NULL) OR target_id = ?)`,
		formatTime(change.CreatedAt), change.AccountID, change.TargetID, change.TargetID,
	); err != nil {
		return storage.Invitation{}, fmt.Errorf("revoke earlier invitations: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
INSERT INTO user_invitations (
    id, token_hash, account_id, target_id, role, system_role, status,
    expires_at, created_by, created_at
) VALUES (?, ?, ?, ?, ?, ?, 'pending', ?, ?, ?)`,
		change.ID,
		change.TokenHash,
		change.AccountID,
		change.TargetID,
		change.Role,
		change.SystemRole,
		formatTime(change.ExpiresAt),
		change.CreatedByAccount,
		formatTime(change.CreatedAt),
	); err != nil {
		return storage.Invitation{}, fmt.Errorf("insert invitation: %w", conflictConstraint(err))
	}
	auditEventID, err := insertAccessAuditEvent(
		ctx,
		tx,
		change.TargetID,
		change.CreatedByAccount,
		change.AccountID,
		"invitation.created",
		invitationRoleSummary(change.Role, change.SystemRole),
		change.CreatedAt,
		change.ElevationID,
	)
	if err != nil {
		return storage.Invitation{}, err
	}
	if elevation != nil {
		if err := insertElevatedNotifications(
			ctx, tx, *elevation, auditEventID, "invitation.created", formatTime(change.CreatedAt),
		); err != nil {
			return storage.Invitation{}, err
		}
	}
	invitation, err := getInvitation(ctx, tx, "ui.id = ?", change.ID, change.CreatedAt)
	if err != nil {
		return storage.Invitation{}, fmt.Errorf("read created invitation: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return storage.Invitation{}, fmt.Errorf("commit invitation create: %w", err)
	}

	return invitation, nil
}

// invitationOfferable rejects an offer that cannot mean anything to the identity it names.
//
// It runs inside the create transaction rather than beside it: the handler reads access and
// invitation history before it writes, so two managers pressing at once would both pass a check
// made outside. Here the same transaction that inserts the row is the one that read the state.
//
// Standing that still allows an offer is left alone on purpose. A pending offer is replaced -
// somebody has not answered and the manager wants a fresh link - and an expired or revoked one
// says nothing about whether the identity wants in.
func invitationOfferable(
	ctx context.Context,
	tx *sql.Tx,
	change storage.InvitationCreate,
) error {
	held, err := invitedIdentityHoldsAccess(ctx, tx, change.AccountID, change.TargetID)
	if err != nil {
		return err
	}
	if held {
		return storage.ErrAlreadyMember
	}
	if change.AcknowledgeDeclined {
		return nil
	}
	declined, err := invitedIdentityDeclinedLast(ctx, tx, change.AccountID, change.TargetID)
	if err != nil {
		return err
	}
	if declined {
		return storage.ErrDeclinedEarlier
	}

	return nil
}

// invitedIdentityHoldsAccess reports whether the offer would grant what the identity already has.
func invitedIdentityHoldsAccess(
	ctx context.Context,
	tx *sql.Tx,
	accountID string,
	targetID *string,
) (bool, error) {
	var status storage.PanelUserStatus
	err := tx.QueryRowContext(
		ctx,
		"SELECT status FROM panel_users WHERE account_id = ?",
		accountID,
	).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read invited panel user: %w", err)
	}
	if targetID == nil {
		// A Root offer only reaches an identity the app does not already hold: accepting one
		// refuses on any live panel user, so the link would be one nobody could take. An existing
		// user is promoted through Root user management instead.
		return status != storage.PanelUserRemoved, nil
	}
	if status != storage.PanelUserActive {
		return false, nil
	}
	var role storage.InstallationRole
	var owner bool
	// Suspension is deliberately not consulted. A suspended user holds a role here and is
	// restored rather than invited, so the offer is as empty for them as for an active one.
	err = tx.QueryRowContext(ctx, `
SELECT
    COALESCE((SELECT role FROM target_roles WHERE account_id = ? AND target_id = ?), 'none'),
    EXISTS(SELECT 1 FROM target_owners WHERE account_id = ? AND target_id = ?)`,
		accountID, *targetID, accountID, *targetID,
	).Scan(&role, &owner)
	if err != nil {
		return false, fmt.Errorf("read invited target access: %w", err)
	}

	return owner || role != storage.InstallationRoleNone, nil
}

// invitedIdentityDeclinedLast reports whether the identity's last word in this scope was no.
//
// Only the last one counts. Declining and later accepting, or declining an installation and being
// invited to another, are not standing refusals, and gating on any decline ever recorded would
// make the confirmation permanent noise.
func invitedIdentityDeclinedLast(
	ctx context.Context,
	tx *sql.Tx,
	accountID string,
	targetID *string,
) (bool, error) {
	var status storage.InvitationStatus
	err := tx.QueryRowContext(ctx, `
SELECT status FROM user_invitations
WHERE account_id = ? AND ((target_id IS NULL AND ? IS NULL) OR target_id = ?)
ORDER BY created_at DESC, id DESC
LIMIT 1`, accountID, targetID, targetID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read last invitation: %w", err)
	}

	return status == storage.InvitationDeclined, nil
}

// ReissueInvitation replaces the secret for a pending or expired offer.
func (s *Store) ReissueInvitation(
	ctx context.Context,
	change storage.InvitationReissue,
) (storage.Invitation, error) {
	if !change.CreatedAt.Before(change.ExpiresAt) {
		return storage.Invitation{}, errors.New("invitation expiry must be in the future")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.Invitation{}, fmt.Errorf("begin invitation reissue: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	current, err := getInvitation(ctx, tx, "ui.id = ?", change.ID, change.CreatedAt)
	if err != nil {
		return storage.Invitation{}, fmt.Errorf("read invitation for reissue: %w", noRows(err))
	}
	if current.Status != storage.InvitationPending && current.Status != storage.InvitationExpired {
		return storage.Invitation{}, storage.ErrConflict
	}
	elevation, err := elevatedInvitationWrite(
		ctx,
		tx,
		current.TargetID,
		change.ElevationID,
		change.SessionTokenHash,
		change.CreatedByAccount,
		change.CreatedAt,
	)
	if err != nil {
		return storage.Invitation{}, err
	}
	// The same standing that stops an offer being made stops one being renewed. An outstanding
	// offer outlives the reason for it: granted access directly while it sat unanswered, and
	// reissuing would mint a live link for somebody who already holds a role - one that overwrites
	// that role on acceptance, so a stale viewer offer can quietly demote an editor.
	//
	// Checked after the elevation, as in CreateInvitation. A caller whose elevation has lapsed is
	// told that, rather than being told what standing the invited identity has.
	//
	// A decline is not consulted here because a declined offer is not reissuable at all: the status
	// check above already refused it.
	held, err := invitedIdentityHoldsAccess(ctx, tx, current.Account.ID, current.TargetID)
	if err != nil {
		return storage.Invitation{}, err
	}
	if held {
		return storage.Invitation{}, storage.ErrAlreadyMember
	}
	result, err := tx.ExecContext(ctx, `
UPDATE user_invitations
SET token_hash = ?, status = 'pending', expires_at = ?, created_by = ?, created_at = ?,
    responded_at = NULL
WHERE id = ? AND status = 'pending'`,
		change.TokenHash,
		formatTime(change.ExpiresAt),
		change.CreatedByAccount,
		formatTime(change.CreatedAt),
		change.ID,
	)
	if err != nil {
		return storage.Invitation{}, fmt.Errorf("reissue invitation: %w", conflictConstraint(err))
	}
	if !oneRowChanged(result) {
		return storage.Invitation{}, storage.ErrConflict
	}
	auditEventID, err := insertAccessAuditEvent(
		ctx,
		tx,
		current.TargetID,
		change.CreatedByAccount,
		current.Account.ID,
		"invitation.reissued",
		"reissued invitation",
		change.CreatedAt,
		change.ElevationID,
	)
	if err != nil {
		return storage.Invitation{}, err
	}
	if elevation != nil {
		if err := insertElevatedNotifications(
			ctx, tx, *elevation, auditEventID, "invitation.reissued", formatTime(change.CreatedAt),
		); err != nil {
			return storage.Invitation{}, err
		}
	}
	updated, err := getInvitation(ctx, tx, "ui.id = ?", change.ID, change.CreatedAt)
	if err != nil {
		return storage.Invitation{}, fmt.Errorf("read reissued invitation: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return storage.Invitation{}, fmt.Errorf("commit invitation reissue: %w", err)
	}

	return updated, nil
}

// RevokeInvitation invalidates one pending offer while retaining its history.
func (s *Store) RevokeInvitation(
	ctx context.Context,
	change storage.InvitationRevoke,
) (storage.Invitation, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.Invitation{}, fmt.Errorf("begin invitation revoke: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	current, err := getInvitation(ctx, tx, "ui.id = ?", change.ID, change.RevokedAt)
	if err != nil {
		return storage.Invitation{}, fmt.Errorf("read invitation for revoke: %w", noRows(err))
	}
	if current.Status != storage.InvitationPending && current.Status != storage.InvitationExpired {
		return storage.Invitation{}, storage.ErrConflict
	}
	elevation, err := elevatedInvitationWrite(
		ctx,
		tx,
		current.TargetID,
		change.ElevationID,
		change.SessionTokenHash,
		change.ActorAccountID,
		change.RevokedAt,
	)
	if err != nil {
		return storage.Invitation{}, err
	}
	result, err := tx.ExecContext(ctx, `
UPDATE user_invitations SET status = 'revoked', responded_at = ?
WHERE id = ? AND status = 'pending'`, formatTime(change.RevokedAt), change.ID)
	if err != nil {
		return storage.Invitation{}, fmt.Errorf("revoke invitation: %w", err)
	}
	if !oneRowChanged(result) {
		return storage.Invitation{}, storage.ErrConflict
	}
	auditEventID, err := insertAccessAuditEvent(
		ctx,
		tx,
		current.TargetID,
		change.ActorAccountID,
		current.Account.ID,
		"invitation.revoked",
		"revoked invitation",
		change.RevokedAt,
		change.ElevationID,
	)
	if err != nil {
		return storage.Invitation{}, err
	}
	if elevation != nil {
		if err := insertElevatedNotifications(
			ctx, tx, *elevation, auditEventID, "invitation.revoked", formatTime(change.RevokedAt),
		); err != nil {
			return storage.Invitation{}, err
		}
	}
	updated, err := getInvitation(ctx, tx, "ui.id = ?", change.ID, change.RevokedAt)
	if err != nil {
		return storage.Invitation{}, fmt.Errorf("read revoked invitation: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return storage.Invitation{}, fmt.Errorf("commit invitation revoke: %w", err)
	}

	return updated, nil
}

func elevatedInvitationWrite(
	ctx context.Context,
	tx *sql.Tx,
	targetID, elevationID *string,
	sessionTokenHash, actorAccountID string,
	changedAt time.Time,
) (*storage.Elevation, error) {
	if targetID == nil {
		if elevationID != nil {
			return nil, storage.ErrConflict
		}
		return nil, nil
	}

	return elevatedWrite(
		ctx, tx, elevationID, sessionTokenHash, actorAccountID, *targetID, changedAt,
	)
}

// RespondToInvitation verifies the named identity and grants or declines the
// offer in the same transaction as its audit record.
func (s *Store) RespondToInvitation(
	ctx context.Context,
	change storage.InvitationResponse,
) (storage.Invitation, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.Invitation{}, fmt.Errorf("begin invitation response: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	invitation, err := getInvitation(ctx, tx, "ui.token_hash = ?", change.TokenHash, change.At)
	if err != nil {
		return storage.Invitation{}, fmt.Errorf("read invitation response: %w", noRows(err))
	}
	if invitation.Account.ID != change.AccountID {
		return storage.Invitation{}, storage.ErrIdentityMismatch
	}
	if invitation.Status == storage.InvitationExpired {
		return storage.Invitation{}, storage.ErrExpired
	}
	if invitation.Status != storage.InvitationPending {
		return storage.Invitation{}, storage.ErrConflict
	}
	status := storage.InvitationDeclined
	action := "invitation.declined"
	summary := "declined invitation"
	if change.Accept {
		if err = acceptInvitation(ctx, tx, invitation, change.At); err != nil {
			return storage.Invitation{}, err
		}
		status = storage.InvitationAccepted
		action = "invitation.accepted"
		summary = invitationRoleSummary(invitation.Role, invitation.SystemRole)
	}
	result, err := tx.ExecContext(ctx, `
UPDATE user_invitations SET status = ?, responded_at = ?
WHERE id = ? AND status = 'pending'`, status, formatTime(change.At), invitation.ID)
	if err != nil {
		return storage.Invitation{}, fmt.Errorf("record invitation response: %w", err)
	}
	if !oneRowChanged(result) {
		return storage.Invitation{}, storage.ErrConflict
	}
	if err = insertAccessAudit(
		ctx,
		tx,
		invitation.TargetID,
		change.AccountID,
		change.AccountID,
		action,
		summary,
		change.At,
	); err != nil {
		return storage.Invitation{}, err
	}
	updated, err := getInvitation(ctx, tx, "ui.id = ?", invitation.ID, change.At)
	if err != nil {
		return storage.Invitation{}, fmt.Errorf("read invitation response result: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return storage.Invitation{}, fmt.Errorf("commit invitation response: %w", err)
	}

	return updated, nil
}

func acceptInvitation(
	ctx context.Context,
	tx *sql.Tx,
	invitation storage.Invitation,
	at time.Time,
) error {
	if invitation.TargetID == nil {
		if invitation.SystemRole != nil {
			return activateInvitedRoot(ctx, tx, invitation.Account.ID, *invitation.SystemRole, at)
		}
		return storage.ErrConflict
	}
	if invitation.Role == nil {
		return storage.ErrConflict
	}
	if err := activateInvitedPanelUser(ctx, tx, invitation.Account.ID, at, true); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO target_roles (
    account_id, target_id, role, suspended, revision, updated_by, updated_at
) VALUES (?, ?, ?, 0, 1, ?, ?)
ON CONFLICT(account_id, target_id) DO UPDATE SET
    role = excluded.role,
    suspended = 0,
    suspension_reason = NULL,
    revision = target_roles.revision + 1,
    updated_by = excluded.updated_by,
    updated_at = excluded.updated_at`,
		invitation.Account.ID,
		*invitation.TargetID,
		*invitation.Role,
		invitation.Account.ID,
		formatTime(at),
	)
	if err != nil {
		return fmt.Errorf("grant invited target access: %w", err)
	}

	return nil
}

func activateInvitedPanelUser(
	ctx context.Context,
	tx *sql.Tx,
	accountID string,
	at time.Time,
	allowActive bool,
) error {
	var status storage.PanelUserStatus
	err := tx.QueryRowContext(
		ctx,
		"SELECT status FROM panel_users WHERE account_id = ?",
		accountID,
	).Scan(&status)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		_, err = tx.ExecContext(ctx, `
INSERT INTO panel_users (
    account_id, status, system_role, revision, created_at, updated_at
) VALUES (?, 'active', 'none', 1, ?, ?)`, accountID, formatTime(at), formatTime(at))
	case err != nil:
		return fmt.Errorf("read invited user state: %w", err)
	case status == storage.PanelUserRemoved:
		_, err = tx.ExecContext(ctx, `
UPDATE panel_users
SET status = 'active', ban_reason = NULL, banned_at = NULL,
    removed_at = NULL, revision = revision + 1, updated_at = ?
WHERE account_id = ? AND system_role = 'none'`, formatTime(at), accountID)
	case status == storage.PanelUserActive && allowActive:
		return nil
	default:
		return storage.ErrConflict
	}
	if err != nil {
		return fmt.Errorf("activate invited panel user: %w", err)
	}

	return nil
}

func activateInvitedRoot(
	ctx context.Context,
	tx *sql.Tx,
	accountID string,
	role storage.SystemRole,
	at time.Time,
) error {
	if role != storage.SystemRoleRoot {
		return errors.New("unsupported invited system role")
	}
	var status storage.PanelUserStatus
	err := tx.QueryRowContext(
		ctx,
		"SELECT status FROM panel_users WHERE account_id = ?",
		accountID,
	).Scan(&status)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		_, err = tx.ExecContext(ctx, `
INSERT INTO panel_users (
    account_id, status, system_role, revision, created_at, updated_at
) VALUES (?, 'active', 'root', 1, ?, ?)`, accountID, formatTime(at), formatTime(at))
	case err != nil:
		return fmt.Errorf("read invited Root state: %w", err)
	case status == storage.PanelUserRemoved:
		_, err = tx.ExecContext(ctx, `
UPDATE panel_users
SET status = 'active', system_role = 'root',
    ban_reason = NULL, banned_at = NULL, removed_at = NULL,
    revision = revision + 1, updated_at = ?
WHERE account_id = ? AND system_role <> 'super_root'`, formatTime(at), accountID)
	default:
		return storage.ErrConflict
	}
	if err != nil {
		return fmt.Errorf("activate invited Root: %w", err)
	}

	return nil
}

func getInvitation(
	ctx context.Context,
	queryer rowQuerier,
	where string,
	argument string,
	now time.Time,
) (storage.Invitation, error) {
	return scanInvitation(queryer.QueryRowContext(ctx, invitationSelect+" WHERE "+where, argument), now)
}

func scanInvitation(scanner rowScanner, now time.Time) (storage.Invitation, error) {
	var invitation storage.Invitation
	var invitedAvatar, targetID, targetName, installationRole, systemRole sql.NullString
	var targetLogin, targetKind sql.NullString
	var creatorAvatar, respondedAt sql.NullString
	var invitedUpdatedAt, creatorUpdatedAt, expiresAt, createdAt string
	err := scanner.Scan(
		&invitation.ID,
		&invitation.Account.ID,
		&invitation.Account.Provider,
		&invitation.Account.SubjectID,
		&invitation.Account.Login,
		&invitation.Account.DisplayName,
		&invitedAvatar,
		&invitedUpdatedAt,
		&targetID,
		&targetName,
		&targetLogin,
		&targetKind,
		&installationRole,
		&systemRole,
		&invitation.Status,
		&expiresAt,
		&invitation.CreatedBy.ID,
		&invitation.CreatedBy.Provider,
		&invitation.CreatedBy.SubjectID,
		&invitation.CreatedBy.Login,
		&invitation.CreatedBy.DisplayName,
		&creatorAvatar,
		&creatorUpdatedAt,
		&createdAt,
		&respondedAt,
	)
	if err != nil {
		return storage.Invitation{}, err
	}
	invitation.Account.AvatarURL = stringPointer(invitedAvatar)
	invitation.CreatedBy.AvatarURL = stringPointer(creatorAvatar)
	invitation.TargetID = stringPointer(targetID)
	invitation.TargetName = stringPointer(targetName)
	invitation.TargetLogin = stringPointer(targetLogin)
	invitation.TargetKind = stringPointer(targetKind)
	if installationRole.Valid {
		role := storage.InstallationRole(installationRole.String)
		invitation.Role = &role
	}
	if systemRole.Valid {
		role := storage.SystemRole(systemRole.String)
		invitation.SystemRole = &role
	}
	if invitation.Account.UpdatedAt, err = parseTime(invitedUpdatedAt); err != nil {
		return storage.Invitation{}, err
	}
	if invitation.CreatedBy.UpdatedAt, err = parseTime(creatorUpdatedAt); err != nil {
		return storage.Invitation{}, err
	}
	if invitation.ExpiresAt, err = parseTime(expiresAt); err != nil {
		return storage.Invitation{}, err
	}
	if invitation.CreatedAt, err = parseTime(createdAt); err != nil {
		return storage.Invitation{}, err
	}
	if invitation.RespondedAt, err = nullableTime(respondedAt); err != nil {
		return storage.Invitation{}, err
	}
	if invitation.Status == storage.InvitationPending && !now.Before(invitation.ExpiresAt) {
		invitation.Status = storage.InvitationExpired
	}

	return invitation, nil
}

func validInvitationPolicy(
	role *storage.InstallationRole,
	systemRole *storage.SystemRole,
	targetID *string,
) bool {
	if systemRole != nil {
		return targetID == nil && role == nil && *systemRole == storage.SystemRoleRoot
	}

	return targetID != nil && role != nil && validTargetRole(*role) &&
		*role != storage.InstallationRoleNone
}

func invitationRoleSummary(role *storage.InstallationRole, systemRole *storage.SystemRole) string {
	if systemRole != nil {
		return fmt.Sprintf("invited user as %s", *systemRole)
	}
	if role == nil {
		return "invited user"
	}

	return fmt.Sprintf("invited user as %s", *role)
}

func oneRowChanged(result sql.Result) bool {
	changed, err := result.RowsAffected()

	return err == nil && changed == 1
}
