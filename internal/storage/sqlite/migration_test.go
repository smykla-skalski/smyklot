package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/storage/sqlstore"
)

func TestPendingCICleanupMigration(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "pending-ci-cleanup.db")
	db := openLegacyDatabase(t, ctx, path, "018_")
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

	db := openLegacyDatabase(t, ctx, path, "015_")
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

	db := openLegacyDatabase(t, ctx, path, "006_")
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

	db := openLegacyDatabase(t, ctx, path, "011_")
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
	stopBeforePrefix string,
) *sql.DB {
	t.Helper()

	db := openDatabase(t, ctx, path)
	if err := sqlstore.Migrate(ctx, db, Dialect{}, migrationsBefore(t, stopBeforePrefix)); err != nil {
		t.Fatalf("apply legacy migrations: %v", err)
	}

	return db
}

// migrationsBefore copies the embedded schema files that sort before prefix.
func migrationsBefore(t *testing.T, prefix string) fs.FS {
	t.Helper()

	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	earlier := fstest.MapFS{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		if strings.HasPrefix(entry.Name(), prefix) {
			break
		}
		content, readErr := migrations.ReadFile("migrations/" + entry.Name())
		if readErr != nil {
			t.Fatalf("read migration %s: %v", entry.Name(), readErr)
		}
		earlier["migrations/"+entry.Name()] = &fstest.MapFile{Data: content}
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
