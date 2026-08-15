package sqlite

import (
	"context"
	"database/sql"
	"fmt"
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

// ExecScript runs a migration file. SQLite's driver accepts every statement in
// one call, so there is nothing to take apart.
func (Dialect) ExecScript(ctx context.Context, conn *sql.Conn, script string) error {
	if _, err := conn.ExecContext(ctx, script); err != nil {
		return fmt.Errorf("exec sqlite script: %w", err)
	}

	return nil
}
