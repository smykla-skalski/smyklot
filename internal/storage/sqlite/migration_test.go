package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/storage/sqlstore"
)

func TestPendingCICleanupMigration(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "pending-ci-cleanup.db")
	db := openLegacyDatabase(t, ctx, path, 18)
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	finishedAt := now.Add(time.Minute)
	insert := `INSERT INTO pending_ci_requests (
target_id, installation_id, repository_id, repository_full_name,
pull_request, head_sha, base_branch, merge_method, required_checks_only,
requester, source_comment_id, source_revision, label, lifecycle, schedule,
next_check_at, last_progress_at, reason, requested_at, updated_at, finished_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	values := []any{
		"installation:77", 77, "9001", "smykla-skalski/smyklot",
		198, "sha", "main", "squash", true,
		"bartsmykla", 101, now.Format(time.RFC3339Nano),
		"smyklot:pending:ci:squash:required", "cancelled", "active",
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), "cancelled",
		now.Format(time.RFC3339Nano), finishedAt.Format(time.RFC3339Nano),
		finishedAt.Format(time.RFC3339Nano),
	}
	if _, err := db.ExecContext(ctx, insert, values...); err != nil {
		t.Fatalf("seed terminal pending CI request: %v", err)
	}
	values[4] = 199
	values[13] = "armed"
	values[17] = ""
	values[20] = nil
	if _, err := db.ExecContext(ctx, insert, values...); err != nil {
		t.Fatalf("seed armed pending CI request: %v", err)
	}

	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open migrated store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	rows, err := store.DB().QueryContext(ctx, `
SELECT pull_request, cleanup_pending, cleanup_artifacts_done,
       cleanup_attempts, cleanup_error, next_check_at
FROM pending_ci_requests ORDER BY pull_request`)
	if err != nil {
		t.Fatalf("read migrated cleanup state: %v", err)
	}
	defer rows.Close()
	type cleanupState struct {
		pullRequest   int
		pending       bool
		artifactsDone bool
		attempts      int
		errorText     string
		nextCheck     string
	}
	var got []cleanupState
	for rows.Next() {
		var state cleanupState
		if err = rows.Scan(
			&state.pullRequest, &state.pending, &state.artifactsDone, &state.attempts,
			&state.errorText, &state.nextCheck,
		); err != nil {
			t.Fatalf("scan migrated cleanup state: %v", err)
		}
		got = append(got, state)
	}
	if err = rows.Err(); err != nil {
		t.Fatalf("iterate migrated cleanup state: %v", err)
	}
	if len(got) != 2 || !got[0].pending || got[0].artifactsDone ||
		got[0].attempts != 0 || got[0].errorText != "" ||
		got[0].nextCheck != finishedAt.Format(time.RFC3339Nano) || got[1].pending {
		t.Fatalf("migrated cleanup state = %#v", got)
	}
}

func TestRemoveGlobalAccessMigration(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy-panel.db")
	seedLegacyGlobalAccess(t, ctx, path)

	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open migrated store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	db := openDatabase(t, ctx, path)

	columns := tableColumns(t, ctx, db, "panel_users")
	if columns["root"] || columns["global_role"] {
		t.Fatalf("legacy panel user columns remain after migration: %#v", columns)
	}

	var revoked int
	err = db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM app_audit_events
WHERE action = 'global_access.migration_revoked'
  AND subject_account_id = 'legacy-editor'`).Scan(&revoked)
	if err != nil || revoked != 1 {
		t.Fatalf("legacy grant audit count = %d, err = %v", revoked, err)
	}

	var role sql.NullString
	err = db.QueryRowContext(
		ctx, "SELECT role FROM user_invitations WHERE id = 'root-invitation'",
	).Scan(&role)
	if err != nil || role.Valid {
		t.Fatalf("migrated Root invitation role = %#v, err = %v", role, err)
	}

	var assignedRole string
	err = db.QueryRowContext(ctx, `
SELECT role FROM target_roles
WHERE account_id = 'legacy-editor' AND target_id = 'legacy-target'`).Scan(&assignedRole)
	if err != nil || assignedRole != "editor" {
		t.Fatalf("installation assignment = %q, err = %v", assignedRole, err)
	}
}

func TestSystemRoleMigration(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy-system-roles.db")
	seedLegacySystemRoles(t, ctx, path)

	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open migrated store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	db := openDatabase(t, ctx, path)

	roles := map[string]string{}
	rows, err := db.QueryContext(ctx, `
SELECT account_id, system_role FROM panel_users ORDER BY account_id`)
	if err != nil {
		t.Fatalf("read migrated system roles: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var accountID, role string
		if err = rows.Scan(&accountID, &role); err != nil {
			t.Fatalf("scan migrated system role: %v", err)
		}
		roles[accountID] = role
	}
	if err = rows.Err(); err != nil {
		t.Fatalf("iterate migrated system roles: %v", err)
	}
	if roles["legacy-super"] != "super_root" || roles["legacy-owner"] != "root" ||
		roles["legacy-banned-owner"] != "none" || roles["legacy-editor"] != "none" {
		t.Fatalf("migrated system roles = %#v", roles)
	}
	restored, err := store.UpdatePanelUser(ctx, storage.PanelUserChange{
		AccountID: "legacy-banned-owner", ActorAccountID: "legacy-super",
		Status: storage.PanelUserActive, ExpectedRevision: 1,
		ChangedAt: time.Date(2026, time.August, 10, 12, 1, 0, 0, time.UTC),
	})
	if err != nil || restored.Status != storage.PanelUserActive {
		t.Fatalf("restore migrated banned Owner: status = %q, err = %v", restored.Status, err)
	}

	assertInvitationState(t, ctx, db, "global-invitation", "revoked")
	assertInvitationState(t, ctx, db, "target-invitation", "pending")
	assertAuditAction(t, ctx, db, "invitation.migration_revoked", "legacy-editor")
	assertAuditAction(t, ctx, db, "global_access.migration_revoked", "legacy-editor")

	var assignedRole string
	err = db.QueryRowContext(ctx, `
SELECT role FROM target_roles
WHERE account_id = 'legacy-editor' AND target_id = 'legacy-target'`).Scan(&assignedRole)
	if err != nil || assignedRole != "editor" {
		t.Fatalf("installation assignment = %q, err = %v", assignedRole, err)
	}
}

// TestFixedWidthTimestampMigration proves the ordering bug the migration
// exists to fix: values written with RFC3339Nano sort by string in an order
// that is not their order in time, and after the rewrite they agree.
func TestFixedWidthTimestampMigration(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy-timestamps.db")
	base := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	// Same second, different numbers of fractional digits - which is exactly
	// where the old layout puts string order and time order at odds.
	offsets := []time.Duration{
		0,
		100 * time.Millisecond,
		120 * time.Millisecond,
		500 * time.Millisecond,
		999999999 * time.Nanosecond,
	}

	db := openLegacyDatabase(t, ctx, path, 15)
	for index, offset := range offsets {
		written := base.Add(offset).Format(time.RFC3339Nano)
		if _, err := db.ExecContext(ctx, `
INSERT INTO accounts (id, provider, subject_id, login, display_name, updated_at)
VALUES (?, 'github', ?, ?, ?, ?)`,
			fmt.Sprintf("account-%d", index), fmt.Sprint(index),
			fmt.Sprintf("login-%d", index), fmt.Sprintf("Account %d", index), written,
		); err != nil {
			t.Fatalf("seed legacy timestamp: %v", err)
		}
	}

	if got := accountOrderByUpdatedAt(t, ctx, db); got == "01234" {
		t.Fatalf("legacy rows already sort correctly (%s), so this migration proves nothing", got)
	}

	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open migrated store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if got := accountOrderByUpdatedAt(t, ctx, db); got != "01234" {
		t.Fatalf("migrated order = %s, want 01234", got)
	}

	var updatedAt string
	if err := db.QueryRowContext(
		ctx, "SELECT updated_at FROM accounts WHERE id = 'account-0'",
	).Scan(&updatedAt); err != nil {
		t.Fatalf("read migrated timestamp: %v", err)
	}
	if want := "2026-08-10T12:00:00.000000000Z"; updatedAt != want {
		t.Fatalf("migrated timestamp = %q, want %q", updatedAt, want)
	}

	// Reading it back through the store must still yield the original instant.
	account, err := store.GetAccount(ctx, "account-2")
	if err != nil {
		t.Fatalf("read migrated account: %v", err)
	}
	if want := base.Add(120 * time.Millisecond); !account.UpdatedAt.Equal(want) {
		t.Fatalf("migrated account time = %s, want %s", account.UpdatedAt, want)
	}
}

// accountOrderByUpdatedAt returns the seeded account indexes in the order the
// database sorts them, as a compact string like "01234".
func accountOrderByUpdatedAt(t *testing.T, ctx context.Context, db *sql.DB) string {
	t.Helper()

	rows, err := db.QueryContext(ctx, "SELECT id FROM accounts ORDER BY updated_at ASC")
	if err != nil {
		t.Fatalf("order accounts: %v", err)
	}
	defer func() { _ = rows.Close() }()
	order := ""
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan ordered account: %v", err)
		}
		order += strings.TrimPrefix(id, "account-")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate ordered accounts: %v", err)
	}

	return order
}

func seedLegacySystemRoles(t *testing.T, ctx context.Context, path string) {
	t.Helper()

	db := openLegacyDatabase(t, ctx, path, 6)
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	statements := []string{
		`INSERT INTO accounts (id, provider, subject_id, login, display_name, updated_at) VALUES
('legacy-super', 'github', '1', 'super', 'Legacy Super', '` + now + `'),
('legacy-owner', 'github', '2', 'owner', 'Legacy Owner', '` + now + `'),
('legacy-editor', 'github', '3', 'editor', 'Legacy Editor', '` + now + `'),
('legacy-banned-owner', 'github', '4', 'banned-owner', 'Legacy Banned Owner', '` + now + `')`,
		`INSERT INTO panel_users (
account_id, root, status, global_role, revision, created_at, updated_at
) VALUES
('legacy-super', 1, 'active', 'owner', 1, '` + now + `', '` + now + `'),
('legacy-owner', 0, 'active', 'owner', 1, '` + now + `', '` + now + `'),
('legacy-editor', 0, 'active', 'editor', 1, '` + now + `', '` + now + `'),
('legacy-banned-owner', 0, 'banned', 'owner', 1, '` + now + `', '` + now + `')`,
		`INSERT INTO targets (
id, installation_id, kind, account_id, available, repository_default_enabled,
config_patch, revision, settings_updated_at, synced_at
) VALUES ('legacy-target', '10', 'Organization', 'legacy-owner', 1, 0, '{}', 1, '` + now + `', '` + now + `')`,
		`INSERT INTO target_roles (
account_id, target_id, role, suspended, revision, updated_by, updated_at
) VALUES ('legacy-editor', 'legacy-target', 'editor', 0, 1, 'legacy-super', '` + now + `')`,
		`INSERT INTO user_invitations (
id, token_hash, account_id, target_id, role, status, expires_at, created_by, created_at
) VALUES
('global-invitation', 'global-token', 'legacy-editor', NULL, 'owner', 'pending',
'2026-08-17T12:00:00Z', 'legacy-super', '` + now + `'),
('target-invitation', 'target-token', 'legacy-editor', 'legacy-target', 'viewer', 'pending',
'2026-08-17T12:00:00Z', 'legacy-super', '` + now + `')`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("seed legacy system roles: %v", err)
		}
	}
}

func assertInvitationState(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	id string,
	want string,
) {
	t.Helper()
	var got string
	if err := db.QueryRowContext(
		ctx, "SELECT status FROM user_invitations WHERE id = ?", id,
	).Scan(&got); err != nil || got != want {
		t.Fatalf("invitation %s status = %q, err = %v", id, got, err)
	}
}

func assertAuditAction(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	action string,
	subject string,
) {
	t.Helper()
	var count int
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM app_audit_events
WHERE action = ? AND subject_account_id = ?`, action, subject).Scan(&count); err != nil || count != 1 {
		t.Fatalf("audit action %s count = %d, err = %v", action, count, err)
	}
}

func seedLegacyGlobalAccess(t *testing.T, ctx context.Context, path string) {
	t.Helper()

	db := openLegacyDatabase(t, ctx, path, 11)
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	statements := []string{
		`INSERT INTO accounts (id, provider, subject_id, login, display_name, updated_at) VALUES
('legacy-editor', 'github', '1', 'editor', 'Legacy Editor', '` + now + `'),
('legacy-root', 'github', '2', 'root', 'Legacy Root', '` + now + `')`,
		`INSERT INTO panel_users (
account_id, root, status, global_role, revision, created_at, updated_at, system_role
) VALUES
('legacy-editor', 0, 'active', 'editor', 1, '` + now + `', '` + now + `', 'none'),
('legacy-root', 1, 'active', 'owner', 1, '` + now + `', '` + now + `', 'root')`,
		`INSERT INTO targets (
id, installation_id, kind, account_id, available, repository_default_enabled,
config_patch, revision, settings_updated_at, synced_at
) VALUES ('legacy-target', '10', 'Organization', 'legacy-root', 1, 0, '{}', 1, '` + now + `', '` + now + `')`,
		`INSERT INTO target_roles (
account_id, target_id, role, suspended, revision, updated_by, updated_at
) VALUES ('legacy-editor', 'legacy-target', 'editor', 0, 1, 'legacy-root', '` + now + `')`,
		`INSERT INTO user_invitations (
id, token_hash, account_id, target_id, role, status, expires_at,
created_by, created_at, system_role
) VALUES ('root-invitation', 'root-token', 'legacy-editor', NULL, 'owner', 'pending',
'2026-08-17T12:00:00Z', 'legacy-root', '` + now + `', 'root')`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("seed legacy database: %v", err)
		}
	}
}

// openDatabase returns a second handle on an already-migrated file, so a test
// can read raw rows without the store having to expose its own pool.
func openDatabase(t *testing.T, ctx context.Context, path string) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", dataSourceName(path))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err = db.PingContext(ctx); err != nil {
		t.Fatalf("reach database: %v", err)
	}

	return db
}

// openLegacyDatabase migrates a fresh file up to but not including the named
// migration, leaving the schema a released version once had. Reopening it with
// Open then exercises the upgrade under test.
func openLegacyDatabase(
	t *testing.T,
	ctx context.Context,
	path string,
	stopBefore int,
) *sql.DB {
	t.Helper()

	db := openDatabase(t, ctx, path)
	if err := sqlstore.Migrate(ctx, db, Dialect{}, migrationsBefore(t, stopBefore)); err != nil {
		t.Fatalf("apply legacy migrations: %v", err)
	}

	return db
}

// migrationsBefore is the series as it stood before a version, from `sqlstore`
// so both engines cut their history by the rule the runner orders it with.
func migrationsBefore(t *testing.T, version int) fs.FS {
	t.Helper()

	earlier, err := sqlstore.MigrationsBefore(migrations, version)
	if err != nil {
		t.Fatalf("read migrations before %d: %v", version, err)
	}

	return earlier
}

func tableColumns(t *testing.T, ctx context.Context, db *sql.DB, table string) map[string]bool {
	t.Helper()

	rows, err := db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		t.Fatalf("read %s columns: %v", table, err)
	}
	defer rows.Close()
	columns := make(map[string]bool)
	for rows.Next() {
		var sequence, notNull, primaryKey int
		var name, kind string
		var defaultValue any
		if err = rows.Scan(&sequence, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("scan %s column: %v", table, err)
		}
		columns[name] = true
	}
	if err = rows.Err(); err != nil {
		t.Fatalf("iterate %s columns: %v", table, err)
	}

	return columns
}

// TestSyncAuditCategoryMigration proves the rebuild of app_audit_events keeps
// what was in it.
//
// This is the one migration in this work with real production risk. SQLite
// cannot alter a CHECK, so widening the category rebuilds the largest table in
// the schema - and that table is the parent of security_notifications, whose
// rows point at its ids. A rebuild that renumbered them would leave every
// notification pointing at the wrong event, silently, with the audit trail as
// the thing that broke.
//
// So it is tested rather than reasoned about: rows on both sides of the
// relationship, applied across the migration, with the ids and the foreign keys
// checked afterwards.
func TestSyncAuditCategoryMigration(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sync-audit.db")
	db := openLegacyDatabase(t, ctx, path, 26)
	now := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)

	seedAuditRelationship(t, ctx, db, now)

	before := auditIDs(t, ctx, db)
	if len(before) != 2 {
		t.Fatalf("seeded %d audit events, wanted 2", len(before))
	}

	if err := sqlstore.Migrate(ctx, db, Dialect{}, migrations); err != nil {
		t.Fatalf("apply the sync audit migration: %v", err)
	}

	// The ids, because security_notifications points at them by value.
	if after := auditIDs(t, ctx, db); !equalInts(before, after) {
		t.Errorf("audit event ids changed across the rebuild: %v -> %v", before, after)
	}

	// The relationship itself. A dangling reference is exactly what a rebuild
	// that dropped and recreated the parent would leave, and nothing else in
	// the suite would notice.
	requireNoForeignKeyViolations(t, ctx, db)

	var notifications int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM security_notifications").Scan(&notifications); err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	if notifications != 1 {
		t.Errorf("security notifications after the rebuild = %d, wanted 1", notifications)
	}

	// The point of the exercise: the new category is accepted.
	if _, err := db.ExecContext(ctx, `
INSERT INTO app_audit_events (category, actor_account_id, action, summary, created_at)
VALUES ('sync', 'github:1', 'sync.plan.applied', 'applied', ?)`, now); err != nil {
		t.Errorf("the widened category refused a sync event: %v", err)
	}

	// And the CHECK still refuses everything else, rather than having been
	// dropped along the way.
	if _, err := db.ExecContext(ctx, `
INSERT INTO app_audit_events (category, actor_account_id, action, summary, created_at)
VALUES ('not-a-category', 'github:1', 'x', 'x', ?)`, now); err == nil {
		t.Error("the rebuilt table accepted a category nothing defines")
	}

	// AUTOINCREMENT keeps its high-water mark, so a new event cannot be handed
	// an id a notification already points at.
	var next int64
	if err := db.QueryRowContext(ctx,
		"SELECT MAX(id) FROM app_audit_events").Scan(&next); err != nil {
		t.Fatalf("read the new id: %v", err)
	}
	if next <= before[len(before)-1] {
		t.Errorf("a new event reused id %d, at or below the seeded %d",
			next, before[len(before)-1])
	}
}

// seedAuditRelationship writes an audit event with a notification pointing at
// it, which is the pair the rebuild has to keep together.
func seedAuditRelationship(t *testing.T, ctx context.Context, db *sql.DB, now string) {
	t.Helper()

	statements := []struct {
		query     string
		arguments []any
	}{
		{`INSERT INTO accounts (id, provider, subject_id, login, display_name, updated_at)
          VALUES ('github:1', 'github', '1', 'smykla-skalski', 'Smykla', ?)`, []any{now}},
		{
			`INSERT INTO targets (
              id, installation_id, kind, account_id, settings_updated_at, synced_at
          ) VALUES ('github:installation:1', '1', 'Organization', 'github:1', ?, ?)`,
			[]any{now, now},
		},
		{
			`INSERT INTO root_elevations (
              id, session_token_hash, root_account_id, target_id, reason,
              started_at, expires_at
          ) VALUES ('elev-1', 'hash', 'github:1', 'github:installation:1', 'why', ?, ?)`,
			[]any{now, now},
		},
		{`INSERT INTO app_audit_events (
              category, source_kind, source_id, target_id, actor_account_id,
              elevation_id, action, summary, created_at
          ) VALUES ('elevation', 'elevation', 1, 'github:installation:1', 'github:1',
                    'elev-1', 'elevation.begin', 'began', ?)`, []any{now}},
		{
			`INSERT INTO app_audit_events (
              category, actor_account_id, action, summary, created_at
          ) VALUES ('configuration', 'github:1', 'settings.update', 'changed', ?)`,
			[]any{now},
		},
		{`INSERT INTO security_notifications (
              recipient_account_id, target_id, actor_account_id, elevation_id,
              audit_event_id, action, created_at
          ) SELECT 'github:1', 'github:installation:1', 'github:1', 'elev-1',
                   MIN(id), 'elevation.begin', ? FROM app_audit_events`, []any{now}},
	}

	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement.query, statement.arguments...); err != nil {
			t.Fatalf("seed audit relationship: %v\n%s", err, statement.query)
		}
	}
}

func auditIDs(t *testing.T, ctx context.Context, db *sql.DB) []int64 {
	t.Helper()

	rows, err := db.QueryContext(ctx, "SELECT id FROM app_audit_events ORDER BY id")
	if err != nil {
		t.Fatalf("read audit ids: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan audit id: %v", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate audit ids: %v", err)
	}

	return ids
}

// requireNoForeignKeyViolations asks SQLite itself whether anything now points
// at a row that is not there.
func requireNoForeignKeyViolations(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()

	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		t.Fatalf("run foreign_key_check: %v", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var table, parent string
		var rowID, key any
		if err := rows.Scan(&table, &rowID, &parent, &key); err != nil {
			t.Fatalf("scan foreign_key_check: %v", err)
		}
		t.Errorf("%s has a row pointing at nothing in %s", table, parent)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate foreign_key_check: %v", err)
	}
}

func equalInts(left, right []int64) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}

	return true
}
