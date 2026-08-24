package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestWorkQueueMigrationBackfillsDurableSourcesOnce(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "work-queue.db")
	now := time.Date(2026, time.August, 24, 12, 0, 0, 0, time.UTC)
	legacy := openLegacyDatabase(t, ctx, path, 42)
	seedSQLiteQueueSources(t, ctx, legacy, now)
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy queue database: %v", err)
	}

	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open migrated queue store: %v", err)
	}
	assertQueueBackfill(t, ctx, store.DB())
	if err = store.Close(); err != nil {
		t.Fatalf("close migrated queue store: %v", err)
	}

	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen migrated queue store: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	assertQueueBackfill(t, ctx, reopened.DB())
}

func seedSQLiteQueueSources(t *testing.T, ctx context.Context, db *sql.DB, now time.Time) {
	t.Helper()

	stamp := now.Format(time.RFC3339Nano)
	next := now.Add(time.Minute).Format(time.RFC3339Nano)
	expires := now.Add(2 * time.Hour).Format(time.RFC3339Nano)
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO accounts (id, provider, subject_id, login, display_name, updated_at)
VALUES ('github:queue-owner', 'github', 'queue-owner', 'queue-owner', 'Queue Owner', ?)`, []any{stamp}},
		{`INSERT INTO targets (
id, installation_id, kind, account_id, settings_updated_at, synced_at
) VALUES ('installation:queue', '77', 'Organization', 'github:queue-owner', ?, ?)`, []any{stamp, stamp}},
		{`INSERT INTO repositories (
id, target_id, name, full_name, private, settings_updated_at, synced_at
) VALUES ('repository:queue', 'installation:queue', 'queue', 'owner/queue', 0, ?, ?)`, []any{stamp, stamp}},
		{`INSERT INTO deliveries (
claim_key, delivery_id, target_id, repository_id, repository_full_name,
event, status, claimed_at, payload, next_attempt_at, attempt_count
) VALUES ('claim:queue', 'delivery:queue', 'installation:queue', 'repository:queue',
          'owner/queue', 'push', 'running', ?, X'7B7D', ?, 2)`, []any{stamp, next}},
		{`INSERT INTO pending_ci_requests (
target_id, installation_id, repository_id, repository_full_name,
pull_request, head_sha, base_branch, merge_method, required_checks_only,
requester, source_comment_id, source_revision, artifact_kind, label,
authorized_by, authorized_at, lifecycle, schedule, next_check_at,
last_progress_at, requested_at, updated_at
) VALUES (
    'installation:queue', 77, 'repository:queue', 'owner/queue',
    317, 'head', 'main', 'squash', 1,
    'queue-owner', 9001, 'source:1', 'label', 'merge-when-ready',
    'queue-owner', ?, 'armed', 'active', ?, ?, ?, ?
)`, []any{stamp, next, stamp, stamp, stamp}},
		{`INSERT INTO sync_plans (
id, target_id, trigger_kind, actor_account_id, digest, state,
create_count, computed_at, expires_at
) VALUES ('plan:queue', 'installation:queue', 'manual', 'github:queue-owner',
          'digest', 'computed', 1, ?, ?)`, []any{stamp, expires}},
		{`INSERT INTO sync_plan_actions (
plan_id, repository_id, kind, operation, subject, state
) VALUES ('plan:queue', 'repository:queue', 'labels', 'create', 'queue', 'pending')`, nil},
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("seed queue migration source: %v\n%s", err, statement.query)
		}
	}
}

func assertQueueBackfill(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()

	type itemState struct {
		state         string
		attempt       int
		progressTotal int
	}
	want := map[string]itemState{
		"delivery:1":           {state: "retrying", attempt: 2},
		"pending-ci:1":         {state: "scheduled"},
		"sync-plan:plan:queue": {state: "awaiting_approval", progressTotal: 1},
	}
	rows, err := db.QueryContext(ctx, `
SELECT id, state, attempt, COALESCE(progress_total, -1)
FROM queue_items ORDER BY id`)
	if err != nil {
		t.Fatalf("read backfilled queue items: %v", err)
	}
	defer func() { _ = rows.Close() }()
	got := make(map[string]itemState)
	for rows.Next() {
		var id string
		var state itemState
		if err = rows.Scan(&id, &state.state, &state.attempt, &state.progressTotal); err != nil {
			t.Fatalf("scan backfilled queue item: %v", err)
		}
		got[id] = state
	}
	if err = rows.Err(); err != nil {
		t.Fatalf("iterate backfilled queue items: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("backfilled queue items = %#v, want %#v", got, want)
	}
	for id, expected := range want {
		if got[id] != expected {
			t.Fatalf("backfilled queue item %s = %#v, want %#v", id, got[id], expected)
		}
	}

	var events int
	if err = db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM queue_events WHERE kind = 'backfilled'`).Scan(&events); err != nil {
		t.Fatalf("count queue backfill events: %v", err)
	}
	if events != len(want) {
		t.Fatalf("queue backfill events = %d, want %d", events, len(want))
	}
}
