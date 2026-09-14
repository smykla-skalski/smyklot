package sqlstore

import (
	"context"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

// ListSyncRequests discovers only the caller's accepted commands. Read access
// survives loss of command authority, but not loss of workspace or session access.
func (s *Store) ListSyncRequests(ctx context.Context, query orgsync.RequestHistoryQuery, now func() time.Time) (orgsync.RequestHistoryPage, error) {
	empty := orgsync.RequestHistoryPage{Items: []orgsync.RequestAcceptance{}}
	if err := validSyncHistoryQuery(query, now); err != nil {
		return empty, err
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
	statement, args := syncHistorySQL(query)
	rows, err := tx.QueryContext(ctx, statement, args...)
	if err != nil {
		return empty, fmt.Errorf("list sync acceptance: %w", err)
	}
	page := empty
	limit := pageLimit(query.Limit)
	for rows.Next() {
		var item orgsync.RequestAcceptance
		var accepted StoredTime
		var action int
		if err := rows.Scan(&action, &item.RequestKey, &item.QueueID, &item.PlanID, &item.ExpectedRevision, &item.Reason, &accepted); err != nil {
			_ = rows.Close()
			return empty, err
		}
		if len(page.Items) == limit {
			last := page.Items[len(page.Items)-1]
			page.Next = &orgsync.RequestHistoryPosition{ActorID: query.ActorID, TargetID: query.TargetID, AcceptedAt: last.AcceptedAt, Action: last.Action, RequestKey: last.RequestKey}
			break
		}
		item.Action = orgsync.RequestActionCheck
		if action == 1 {
			item.Action = orgsync.RequestActionDispatch
		}
		item.AcceptedAt = accepted.Time()
		page.Items = append(page.Items, item)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return empty, err
	}
	// A slow read cannot expose history after the session expires.
	if err := s.validateSyncHistoryAccess(ctx, tx, authority); err != nil {
		return empty, err
	}
	return page, nil
}

func validSyncHistoryQuery(query orgsync.RequestHistoryQuery, now func() time.Time) error {
	if now == nil || query.ActorID == "" || query.TargetID == "" {
		return storage.ErrConflict
	}
	if after := query.After; after != nil {
		if after.ActorID != query.ActorID || after.TargetID != query.TargetID || after.AcceptedAt.IsZero() || after.RequestKey == "" || (after.Action != orgsync.RequestActionCheck && after.Action != orgsync.RequestActionDispatch) {
			return storage.ErrConflict
		}
	}
	return nil
}

func (s *Store) validateSyncHistoryAccess(ctx context.Context, tx *transaction, authority workspaceCommandAuthority) error {
	access, err := s.workspaceSessionAccess(ctx, tx, authority, authority.Clock().UTC())
	if err != nil {
		return err
	}
	switch access.Role {
	case storage.InstallationRoleViewer, storage.InstallationRoleEditor, storage.InstallationRoleAdmin, storage.InstallationRoleOwner:
		return nil
	default:
		return storage.ErrRevoked
	}
}

func syncHistorySQL(query orgsync.RequestHistoryQuery) (string, []any) {
	// Each source contributes at most one page plus a sentinel. The final merge
	// sorts at most twice that bound and never reads full acceptance snapshots.
	branches := make([]string, 0, 2)
	args := make([]any, 0, 16)
	for index, source := range []struct{ table, at, fields, scope string }{
		{"recurring_request_receipts", "requested_at", "queue_id, '' AS plan_id, 0 AS expected_revision, reason", " AND kind = 'sync_scan'"},
		{"sync_dispatch_receipts", "accepted_at", "queue_id, plan_id, expected_revision, reason", ""},
	} {
		branch := fmt.Sprintf("SELECT %d AS action, request_key, %s, %s AS accepted_at FROM %s WHERE actor_account_id = ? AND target_id = ?%s", index, source.fields, source.at, source.table, source.scope)
		args = append(args, query.ActorID, query.TargetID)
		if after := query.After; after != nil {
			action := 0
			if after.Action == orgsync.RequestActionDispatch {
				action = 1
			}
			branch += fmt.Sprintf(" AND (%s, %d, request_key) < (?, ?, ?)", source.at, index)
			args = append(args, after.AcceptedAt, action, after.RequestKey)
		}
		branch += " ORDER BY accepted_at DESC, request_key DESC LIMIT ?"
		args = append(args, pageLimit(query.Limit)+1)
		branches = append(branches, "SELECT * FROM ("+branch+") AS source_page")
	}
	statement := "SELECT * FROM (" + branches[0] + " UNION ALL " + branches[1] + ") AS acceptance ORDER BY accepted_at DESC, action DESC, request_key DESC LIMIT ?"
	args = append(args, pageLimit(query.Limit)+1)
	return statement, args
}
