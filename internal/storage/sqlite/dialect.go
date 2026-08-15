package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
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

// UniqueViolation reports a row that already exists.
//
// SQLite carries the verdict in the message rather than in a code the driver
// exposes, and it names every broken constraint the same way, so a check or
// foreign key that fails reads as a conflict here too. That matches what the
// adapter did before the engines were split apart.
func (Dialect) UniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "constraint failed")
}

// ExecScript runs a migration file. SQLite's driver accepts every statement in
// one call, so there is nothing to take apart.
func (Dialect) ExecScript(ctx context.Context, conn *sql.Conn, script string) error {
	if _, err := conn.ExecContext(ctx, script); err != nil {
		return fmt.Errorf("exec sqlite script: %w", err)
	}

	return nil
}
