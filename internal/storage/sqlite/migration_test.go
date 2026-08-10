package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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

	columns := tableColumns(t, ctx, store.db, "panel_users")
	if columns["root"] || columns["global_role"] {
		t.Fatalf("legacy panel user columns remain after migration: %#v", columns)
	}

	var revoked int
	err = store.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM app_audit_events
WHERE action = 'global_access.migration_revoked'
  AND subject_account_id = 'legacy-editor'`).Scan(&revoked)
	if err != nil || revoked != 1 {
		t.Fatalf("legacy grant audit count = %d, err = %v", revoked, err)
	}

	var role sql.NullString
	err = store.db.QueryRowContext(
		ctx, "SELECT role FROM user_invitations WHERE id = 'root-invitation'",
	).Scan(&role)
	if err != nil || role.Valid {
		t.Fatalf("migrated Root invitation role = %#v, err = %v", role, err)
	}

	var assignedRole string
	err = store.db.QueryRowContext(ctx, `
SELECT role FROM target_roles
WHERE account_id = 'legacy-editor' AND target_id = 'legacy-target'`).Scan(&assignedRole)
	if err != nil || assignedRole != "editor" {
		t.Fatalf("installation assignment = %q, err = %v", assignedRole, err)
	}
}

func seedLegacyGlobalAccess(t *testing.T, ctx context.Context, path string) {
	t.Helper()

	db, err := sql.Open("sqlite", dataSourceName(path))
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin legacy migrations: %v", err)
	}
	if _, err = tx.ExecContext(ctx, `
CREATE TABLE schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
)`); err != nil {
		t.Fatalf("create migration table: %v", err)
	}
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") ||
			strings.HasPrefix(entry.Name(), "011_") {
			continue
		}
		if err = applyMigration(ctx, tx, entry.Name()); err != nil {
			t.Fatalf("apply legacy migration %s: %v", entry.Name(), err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatalf("commit legacy migrations: %v", err)
	}

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
		if _, err = db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("seed legacy database: %v", err)
		}
	}
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
