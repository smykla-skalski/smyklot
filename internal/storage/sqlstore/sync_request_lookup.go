package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

// GetSyncRequest reads the indexed receipt identity, never its worker snapshot.
// Like history, it permits viewers and checks session access before and after IO.
func (s *Store) GetSyncRequest(ctx context.Context, query orgsync.RequestLookup, now func() time.Time) (orgsync.RequestAcceptance, error) {
	empty := orgsync.RequestAcceptance{}
	if now == nil || query.ActorID == "" || query.TargetID == "" || query.RequestKey == "" || len(query.RequestKey) > 200 || strings.TrimSpace(query.RequestKey) != query.RequestKey {
		return empty, storage.ErrConflict
	}
	var statement string
	switch query.Action {
	case orgsync.RequestActionCheck:
		statement = `SELECT queue_id, '' AS plan_id, 0 AS expected_revision, reason, requested_at FROM recurring_request_receipts WHERE actor_account_id = ? AND target_id = ? AND request_key = ? AND kind = 'sync_scan'`
	case orgsync.RequestActionDispatch:
		statement = `SELECT queue_id, plan_id, expected_revision, reason, accepted_at FROM sync_dispatch_receipts WHERE actor_account_id = ? AND target_id = ? AND request_key = ?`
	default:
		return empty, storage.ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return empty, err
	}
	defer func() { _ = tx.Rollback() }()
	authority := workspaceCommandAuthority{ActorAccountID: query.ActorID, SessionTokenHash: query.SessionTokenHash, TargetID: query.TargetID, Clock: now}
	if err := s.lockWorkspaceAuthority(ctx, tx, authority); err != nil {
		return empty, err
	}
	if err := s.validateSyncHistoryAccess(ctx, tx, authority); err != nil {
		return empty, err
	}
	item := orgsync.RequestAcceptance{Action: query.Action, RequestKey: query.RequestKey}
	var accepted StoredTime
	readErr := tx.QueryRowContext(ctx, statement, query.ActorID, query.TargetID, query.RequestKey).Scan(&item.QueueID, &item.PlanID, &item.ExpectedRevision, &item.Reason, &accepted)
	if readErr != nil && !errors.Is(readErr, sql.ErrNoRows) {
		return empty, readErr
	}
	if err := s.validateSyncHistoryAccess(ctx, tx, authority); err != nil {
		return empty, err
	}
	if errors.Is(readErr, sql.ErrNoRows) {
		return empty, storage.ErrNotFound
	}
	item.AcceptedAt = accepted.Time()
	return item, nil
}
