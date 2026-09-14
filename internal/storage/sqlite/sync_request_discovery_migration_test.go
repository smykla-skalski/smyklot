package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage/sqlstore"
)

func TestSyncRequestDiscoveryPreservesLegacyReceipts(t *testing.T) {
	ctx := context.Background()
	db := openLegacyDatabase(t, ctx, filepath.Join(t.TempDir(), "request-history.db"), 64)

	dialect := Dialect{}
	at := "2026-09-13T12:00:00.123456000Z"
	if _, err := db.ExecContext(ctx, dialect.Rebind(`INSERT INTO accounts (id, provider, subject_id, login, display_name, updated_at) VALUES ('owner', 'github', '1', 'owner', 'Owner', ?)`), at); err != nil {
		t.Fatal(err)
	}
	inputs := []string{
		`{"Kind":"sync_scan","TargetID":"removed-workspace","RepositoryID":null,"Title":"Check","Reason":"Verify original settings"}`,
		`{"Kind":"catalog_refresh","TargetID":null,"RepositoryID":null,"Title":"Refresh","Reason":"Refresh catalog"}`,
	}
	snapshot := `{"id":"pruned-occurrence","state":"scheduled","summary":"Original acceptance"}`
	for index, input := range inputs {
		_, err := db.ExecContext(ctx, dialect.Rebind(`INSERT INTO recurring_request_receipts (actor_account_id, request_key, input_json, accepted_item, requested_at) VALUES ('owner', ?, ?, ?, ?)`), fmt.Sprint(index), input, snapshot, at)
		if err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := sqlstore.Migrate(ctx, db, dialect, migrations); err != nil {
			t.Fatal(err)
		}
		for index, input := range inputs {
			assertLegacyRequestDiscovery(t, db, index, input, snapshot, at)
		}
	}
}

func assertLegacyRequestDiscovery(t *testing.T, db *sql.DB, index int, input, snapshot, at string) {
	t.Helper()
	ctx, dialect := t.Context(), Dialect{}

	var kind, queue, reason, originalInput, originalSnapshot string
	var target sql.NullString
	var accepted sqlstore.StoredTime
	err := db.QueryRowContext(ctx, dialect.Rebind(`SELECT kind, target_id, queue_id, reason, input_json, accepted_item, requested_at FROM recurring_request_receipts WHERE actor_account_id = 'owner' AND request_key = ?`), fmt.Sprint(index)).Scan(&kind, &target, &queue, &reason, &originalInput, &originalSnapshot, &accepted)
	if err != nil {
		t.Fatal(err)
	}
	wantKind, wantReason := "sync_scan", "Verify original settings"
	if index == 1 {
		wantKind, wantReason = "catalog_refresh", "Refresh catalog"
	}
	wantTime, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		t.Fatal(err)
	}
	if kind != wantKind || reason != wantReason || queue != "pruned-occurrence" || originalInput != input || originalSnapshot != snapshot || !accepted.Time().Equal(wantTime) {
		t.Fatalf("receipt was not preserved: %q %q %q %q %q %v", kind, reason, queue, originalInput, originalSnapshot, accepted.Time())
	}
	if target.Valid != (index == 0) || (index == 0 && target.String != "removed-workspace") {
		t.Fatalf("changed scope: %#v", target)
	}
}
