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

// ColumnKinds reports nothing. SQLite stores a timestamp as text and a boolean
// as a number, and its driver accepts the time.Time and bool another engine
// hands back, so a copy into it converts nothing.
func (Dialect) ColumnKinds(
	_ context.Context,
	_ *sql.Conn,
	_ string,
) (map[string]sqlstore.ColumnKind, error) {
	return nil, nil
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
