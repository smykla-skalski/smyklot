package postgres

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage/sqlstore"
)

// TestSyncConfigHistoryMigrationBaselinesEveryTarget is the production-engine
// counterpart to SQLite's upgrade proof. It includes an installation with no
// sync_configs rows because an empty pre-upgrade state must remain restorable.
func TestSyncConfigHistoryMigrationBaselinesEveryTarget(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("SMYKLOT_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("SMYKLOT_TEST_POSTGRES_DSN is not set, so there is no server to migrate")
	}

	ctx := context.Background()
	scoped := scopedSchema(t, ctx, dsn)
	legacy, err := sqlstore.MigrationsBefore(migrations, 23)
	if err != nil {
		t.Fatalf("cut the migration series: %v", err)
	}
	db := openPool(t, ctx, scoped)
	if err := sqlstore.Migrate(ctx, db, Dialect{}, legacy); err != nil {
		t.Fatalf("apply legacy migrations: %v", err)
	}

	targetAt := time.Date(2026, time.August, 22, 10, 0, 0, 0, time.UTC)
	configAt := targetAt.Add(time.Hour)
	statements := []struct {
		query     string
		arguments []any
	}{
		{`INSERT INTO accounts (id, provider, subject_id, login, display_name, updated_at)
VALUES ('github:owner', 'github', 'owner', 'owner', 'Owner', $1)`, []any{targetAt}},
		{`INSERT INTO accounts (id, provider, subject_id, login, display_name, updated_at)
VALUES ('github:editor', 'github', 'editor', 'editor', 'Editor', $1)`, []any{configAt}},
		{
			`INSERT INTO targets (
id, installation_id, kind, account_id, settings_updated_at, synced_at
) VALUES ('installation:configured', '1', 'Organization', 'github:owner', $1, $2)`,
			[]any{targetAt, targetAt},
		},
		{
			`INSERT INTO targets (
id, installation_id, kind, account_id, settings_updated_at, synced_at
) VALUES ('installation:empty', '2', 'Organization', 'github:owner', $1, $2)`,
			[]any{targetAt, targetAt},
		},
		{`INSERT INTO sync_configs (
target_id, kind, enabled, document, digest, revision, updated_by, updated_at
) VALUES ('installation:configured', 'labels', true, '{"labels":[]}', 'labels-digest', 4,
          'github:editor', $1)`, []any{configAt}},
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement.query, statement.arguments...); err != nil {
			t.Fatalf("seed pre-history state: %v\n%s", err, statement.query)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	store, err := Open(ctx, scoped)
	if err != nil {
		t.Fatalf("open migrated store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	type baseline struct {
		target, actor string
		created       time.Time
		items         int
	}
	rows, err := store.DB().QueryContext(ctx, `
SELECT checkpoint.target_id, checkpoint.actor_account_id, checkpoint.created_at,
       COUNT(item.kind)
FROM sync_config_checkpoints checkpoint
LEFT JOIN sync_config_checkpoint_items item ON item.checkpoint_id = checkpoint.id
WHERE checkpoint.action = 'baseline'
GROUP BY checkpoint.id
ORDER BY checkpoint.target_id`)
	if err != nil {
		t.Fatalf("read migrated baselines: %v", err)
	}
	defer rows.Close()
	var got []baseline
	for rows.Next() {
		var item baseline
		if err = rows.Scan(&item.target, &item.actor, &item.created, &item.items); err != nil {
			t.Fatalf("scan migrated baseline: %v", err)
		}
		got = append(got, item)
	}
	if err = rows.Err(); err != nil {
		t.Fatalf("iterate migrated baselines: %v", err)
	}
	if len(got) != 2 || got[0].target != "installation:configured" ||
		got[0].actor != "github:editor" || !got[0].created.Equal(configAt) || got[0].items != 1 ||
		got[1].target != "installation:empty" || got[1].actor != "github:owner" ||
		!got[1].created.Equal(targetAt) || got[1].items != 0 {
		t.Fatalf("migrated baselines = %#v", got)
	}

	var targetAudit, rootAudit int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_entries").
		Scan(&targetAudit); err != nil {
		t.Fatalf("count target audit: %v", err)
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM app_audit_events").
		Scan(&rootAudit); err != nil {
		t.Fatalf("count Root audit: %v", err)
	}
	if targetAudit != 0 || rootAudit != 0 {
		t.Fatalf("baseline migration wrote audit: target=%d root=%d", targetAudit, rootAudit)
	}
}
