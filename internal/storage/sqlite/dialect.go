package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage/sqlstore"
)

// Dialect spells the shared store's SQL for SQLite.
type Dialect struct{}

// Name identifies the engine in errors and logs.
func (Dialect) Name() string { return "sqlite" }

// DisplayName names the engine for a person reading a panel.
func (Dialect) DisplayName() string { return "SQLite" }

// VersionQuery selects the library's release. SQLite is linked into the
// process rather than reached over a socket, so this is the version of the
// binary asking, which is also the version of the binary answering.
func (Dialect) VersionQuery() string { return "SELECT sqlite_version()" }

// SizeQuery selects the size of the database file, which SQLite reports as the
// pages it has allocated rather than as bytes. Space freed by a delete stays
// in the file until it is vacuumed, so this is what the file occupies and not
// what its rows need.
func (Dialect) SizeQuery() string {
	return "SELECT page_count * page_size FROM pragma_page_count(), pragma_page_size()"
}

// Rebind returns the statement unchanged, because ? is already SQLite's own
// placeholder.
func (Dialect) Rebind(query string) string { return query }

// MigrationTableDDL creates the bookkeeping table the migration runner owns.
func (Dialect) MigrationTableDDL() string {
	return `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
)`
}

// JSONKeyCount counts the top-level keys of a JSON document stored as text.
func (Dialect) JSONKeyCount(column string) string {
	return "(SELECT COUNT(*) FROM json_each(" + column + "))"
}

// JSONHasKey reports whether a JSON document stored as text has one top-level
// key. SQLite addresses a member by path, so the bound key becomes one.
func (Dialect) JSONHasKey(column string) string {
	return "json_type(" + column + ", '$.' || ?) IS NOT NULL"
}

// RowLock adds nothing. This adapter holds one connection and SQLite gives a
// write transaction exclusive access, so no two callers are ever inside the
// same read-modify-write at once.
func (Dialect) RowLock() string { return "" }

// timeLayout is how a timestamp is written to a TEXT column.
//
// SQLite has no timestamp type, so every comparison and every ORDER BY on a
// stored time is a string comparison. RFC3339Nano drops trailing zeros from
// the fractional part, which makes those two orders disagree: a whole second
// sorts after the same second plus a fraction, because Z outranks the dot.
// Padding the fraction to a fixed nine digits makes string order and time
// order the same thing, and the result still parses as RFC3339.
const timeLayout = "2006-01-02T15:04:05.000000000Z07:00"

// TimeArg writes a time as sortable UTC text.
func (Dialect) TimeArg(value time.Time) any {
	return value.UTC().Format(timeLayout)
}

// NullTimeArg writes an optional time, and NULL for one that is absent.
func (d Dialect) NullTimeArg(value *time.Time) any {
	if value == nil {
		return nil
	}

	return d.TimeArg(*value)
}

// UniqueViolation reports a row that already exists.
//
// SQLite carries the verdict in the message rather than in a code the driver
// exposes, and it names every broken constraint the same way, so a check or
// foreign key that fails reads as a conflict here too. That matches what the
// adapter did before the engines were split apart.
func (Dialect) UniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "constraint failed")
}

// ColumnKinds reports which columns were declared as text, and nothing else.
//
// SQLite cannot say which of them holds a time, because it stores one as text
// and it is indistinguishable from any other text; that is decided by the value
// the source handed back, in scanRow. What it can say is text from blob, and it
// has to: PostgreSQL answers a jsonb column with bytes, and text affinity does
// not convert a blob, so without this a copied document is stored as a blob
// where every native write stores text. Both are read back the same, and
// equality against a string stops matching.
func (Dialect) ColumnKinds(
	ctx context.Context,
	conn *sql.Conn,
	table string,
) (map[string]sqlstore.ColumnKind, error) {
	// #nosec G202 -- the table name comes from the copier's own list.
	rows, err := conn.QueryContext(ctx, `SELECT name, type FROM pragma_table_info(?)`, table)
	if err != nil {
		return nil, fmt.Errorf("read sqlite column types for %q: %w", table, err)
	}
	defer func() { _ = rows.Close() }()

	kinds := map[string]sqlstore.ColumnKind{}
	for rows.Next() {
		var column, declared string
		if err := rows.Scan(&column, &declared); err != nil {
			return nil, fmt.Errorf("scan sqlite column type: %w", err)
		}
		if strings.EqualFold(declared, "TEXT") {
			kinds[column] = sqlstore.ColumnText
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read sqlite column types for %q: %w", table, err)
	}

	return kinds, nil
}

// InsertOverride is empty. SQLite lets a row carry its own key.
func (Dialect) InsertOverride() string { return "" }

// AfterCopy has nothing to repair. A rowid key follows the largest value
// present, so copied keys already leave the next insert in the right place.
func (Dialect) AfterCopy(_ context.Context, _ *sql.Conn, _ []string) error { return nil }

// ExecScript runs a migration file. SQLite's driver accepts every statement in
// one call, so there is nothing to take apart.
func (Dialect) ExecScript(ctx context.Context, conn *sql.Conn, script string) error {
	if _, err := conn.ExecContext(ctx, script); err != nil {
		return fmt.Errorf("exec sqlite script: %w", err)
	}

	return nil
}
