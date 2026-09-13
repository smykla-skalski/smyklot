package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage/sqlstore"
)

func TestSyncObservationsMigrationKeepsLegacyEvidenceUnknown(t *testing.T) {
	ctx := context.Background()
	db := openLegacyDatabase(t, ctx, filepath.Join(t.TempDir(), "observations.db"), 56)

	dialect := Dialect{}
	statements := []string{
		`INSERT INTO accounts (id, provider, subject_id, login, display_name, updated_at)
VALUES ('owner', 'github', '1', 'owner', 'Owner', '2026-09-13T12:00:00.000000000Z')`,
		`INSERT INTO targets (id, installation_id, kind, account_id, settings_updated_at, synced_at)
VALUES ('target', '1', 'Organization', 'owner', '2026-09-13T12:00:00.000000000Z', '2026-09-13T12:00:00.000000000Z')`,
		`INSERT INTO repositories (id, target_id, name, full_name, private, settings_updated_at, synced_at)
VALUES ('repo', 'target', 'repo', 'owner/repo', FALSE, '2026-09-13T12:00:00.000000000Z', '2026-09-13T12:00:00.000000000Z')`,
		`INSERT INTO sync_repository_state (repository_id, kind, applied_digest, applied_at, problem)
VALUES ('repo', 'files', 'legacy-digest', '2026-09-13T12:00:00.000000000Z', '')`,
		`INSERT INTO sync_repository_state (repository_id, kind, applied_digest, applied_at, problem)
VALUES ('repo', 'labels', '', '2026-09-13T12:00:00.000000000Z', 'retained problem')`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	classified, err := sqlstore.MigrationsBefore(migrations, 57)
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlstore.Migrate(ctx, db, dialect, classified); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO sync_repository_state
 (repository_id, kind, applied_digest, applied_at, problem, observation)
 VALUES ('repo', 'rulesets', 'classified-input', '2026-09-13T12:00:00.000000000Z', '', 'matched')`); err != nil {
		t.Fatal(err)
	}
	seedLegacySyncAction(t, db)
	for range 2 {
		if err := sqlstore.Migrate(ctx, db, dialect, migrations); err != nil {
			t.Fatal(err)
		}
		assertLegacySyncAction(t, db)
		assertLegacySyncObservation(t, db, "files", "legacy-digest", "", "")
		assertLegacySyncObservation(t, db, "labels", "", "retained problem", "")
		assertLegacySyncObservation(t, db, "rulesets", "classified-input", "", "matched")
	}
}

func assertLegacySyncObservation(t *testing.T, db *sql.DB, kind, wantDigest, wantProblem, wantObservation string) {
	t.Helper()
	var digest, problem, observation, inputDigest string
	var observed sqlstore.StoredTime
	err := db.QueryRowContext(t.Context(), (Dialect{}).Rebind(`SELECT applied_digest, applied_at, problem, observation, observed_digest
FROM sync_repository_state WHERE repository_id = 'repo' AND kind = ?`), kind).
		Scan(&digest, &observed, &problem, &observation, &inputDigest)
	if err != nil {
		t.Fatal(err)
	}
	if observation != wantObservation || !observed.Time().Equal(time.Date(2026, time.September, 13, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("invented evidence: %q at %v", observation, observed.Time())
	}
	wantInput := ""
	if wantObservation != "" {
		wantInput = wantDigest
	}
	if inputDigest != wantInput {
		t.Fatalf("input digest = %q, want %q", inputDigest, wantInput)
	}
	if digest != wantDigest || problem != wantProblem {
		t.Fatalf("changed legacy evidence: %s %q %q", kind, digest, problem)
	}
}

func seedLegacySyncAction(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, statement := range []string{
		`INSERT INTO sync_plans (id, target_id, trigger_kind, actor_account_id, digest, state, computed_at, expires_at)
VALUES ('legacy-plan', 'target', 'manual', 'owner', 'old-scope', 'computed',
 '2026-09-13T12:00:00.000000000Z', '2026-09-13T13:00:00.000000000Z')`,
		`INSERT INTO sync_plan_actions (plan_id, repository_id, kind, operation, subject, payload, state)
VALUES ('legacy-plan', 'repo', 'labels', 'create', 'bug', '{"name":"bug"}', 'pending')`,
	} {
		if _, err := db.ExecContext(t.Context(), statement); err != nil {
			t.Fatal(err)
		}
	}
}

func assertLegacySyncAction(t *testing.T, db *sql.DB) {
	t.Helper()
	var input, payload string
	if err := db.QueryRowContext(t.Context(), `SELECT input_digest, payload FROM sync_plan_actions WHERE plan_id = 'legacy-plan'`).Scan(&input, &payload); err != nil {
		t.Fatal(err)
	}
	if input != "" || payload != `{"name":"bug"}` {
		t.Fatalf("changed legacy action: input=%q payload=%q", input, payload)
	}
}
