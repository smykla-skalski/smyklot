package transfer

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/storage/sqlite"
)

// schemaManaged names tables a copy must not carry. The destination creates
// these singleton rows while migrating, so copying them would collide rather
// than preserve service state.
var schemaManaged = map[string]bool{
	"pending_ci_policy_lock": true,
	"schema_migrations":      true,
}

// TestTableListCoversSchema fails when the schema holds a table the copier does
// not know about.
//
// The list in transfer.go is written out rather than derived, so it can be read
// and its order reviewed. The cost of that choice is that adding a table to the
// migrations does not add it here, and the failure would be a silently
// incomplete copy - a migration that reports success having left one table's
// rows behind. This is the test that makes that impossible.
//
// SQLite is what it reads, because the schema-parity test already proves the
// two engines agree and this one then needs no server to run against.
func TestTableListCoversSchema(t *testing.T) {
	t.Parallel()

	inSchema := schemaTables(t)
	listed := listedTables(t)

	var missing []string
	for _, name := range inSchema {
		if schemaManaged[name] {
			if listed[name] {
				t.Errorf("%q must not be copied: the destination wrote its own", name)
			}

			continue
		}
		if !listed[name] {
			missing = append(missing, name)
		}
	}
	report(t, "tables in the schema but not in the copy, so their rows would be left behind", missing)

	known := map[string]bool{}
	for _, name := range inSchema {
		known[name] = true
	}
	var unknown []string
	for _, table := range tables {
		if !known[table] {
			unknown = append(unknown, table)
		}
	}
	report(t, "tables in the copy but not in the schema, so the copy would fail on them", unknown)
}

func TestCheckpointCopyOrdersParentsBeforeRestoreChildren(t *testing.T) {
	query := tableReadQuery("sync_config_checkpoints")
	if query != `SELECT * FROM "sync_config_checkpoints" ORDER BY id ASC` {
		t.Fatalf("checkpoint copy query = %q", query)
	}
	if query := tableReadQuery("accounts"); query != `SELECT * FROM "accounts"` {
		t.Fatalf("ordinary copy query = %q", query)
	}
}

// TestTableOrderSatisfiesReferences fails when a table is copied before one it
// references, which is the other way the list can be wrong.
//
// A row arriving before the row it points at is a foreign key violation, and
// the whole copy rolls back. Getting the order right by running a copy would
// need a populated database and both engines; reading the schema needs neither,
// so this check runs everywhere.
func TestTableOrderSatisfiesReferences(t *testing.T) {
	t.Parallel()

	position := map[string]int{}
	for index, table := range tables {
		position[table] = index
	}

	store := migratedSQLite(t)
	for index, table := range tables {
		for _, reference := range foreignKeys(t, store, table) {
			// A table may reference itself; that is satisfied by insert order
			// within the table, which the copy preserves.
			if reference.table == table {
				continue
			}
			at, listed := position[reference.table]
			if !listed {
				t.Errorf("%s.%s references %s, which the copy does not carry",
					table, reference.column, reference.table)

				continue
			}
			if at > index {
				t.Errorf(
					"%s (position %d) references %s (position %d), so its rows would arrive first",
					table, index, reference.table, at,
				)
			}
		}
	}
}

// listedTables is transfer.tables as a set, failing on a name written twice -
// which would copy those rows twice and collide on their keys.
func listedTables(t *testing.T) map[string]bool {
	t.Helper()

	listed := map[string]bool{}
	for _, table := range tables {
		if listed[table] {
			t.Errorf("table %q is listed twice, so its rows would be copied twice", table)
		}
		listed[table] = true
	}

	return listed
}

// schemaTables names every table the migrations produce.
func schemaTables(t *testing.T) []string {
	t.Helper()

	rows, err := migratedSQLite(t).DB().QueryContext(t.Context(), `
SELECT name FROM sqlite_master
WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
ORDER BY name`)
	if err != nil {
		t.Fatalf("read tables: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table: %v", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate tables: %v", err)
	}

	return names
}

// reference is one table pointing at another.
type reference struct {
	table  string
	column string
}

// foreignKeys reads what a table references.
func foreignKeys(t *testing.T, store *sqlite.Store, table string) []reference {
	t.Helper()

	// #nosec G202 -- the table name comes from this package's own list.
	rows, err := store.DB().QueryContext(t.Context(), `PRAGMA foreign_key_list("`+table+`")`)
	if err != nil {
		t.Fatalf("read foreign keys for %s: %v", table, err)
	}
	defer func() { _ = rows.Close() }()

	var references []reference
	for rows.Next() {
		var id, seq int
		var referenced, from, to, onUpdate, onDelete, match string
		if err := rows.Scan(
			&id, &seq, &referenced, &from, &to, &onUpdate, &onDelete, &match,
		); err != nil {
			t.Fatalf("scan foreign key: %v", err)
		}
		references = append(references, reference{table: referenced, column: from})
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate foreign keys for %s: %v", table, err)
	}

	return references
}

// migratedSQLite opens an empty database with the current schema.
func migratedSQLite(t *testing.T) *sqlite.Store {
	t.Helper()

	store, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "schema.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	return store
}

func report(t *testing.T, what string, names []string) {
	t.Helper()

	if len(names) == 0 {
		return
	}
	sort.Strings(names)
	t.Errorf("%s:\n  %s", what, strings.Join(names, "\n  "))
}
