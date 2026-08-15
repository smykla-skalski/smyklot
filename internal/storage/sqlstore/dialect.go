package sqlstore

import (
	"context"
	"database/sql"
	"time"
)

// Dialect describes the few places where SQL engines genuinely disagree.
//
// Every query in this package is written once, in one portable form, and asks
// the dialect only for the fragments an engine spells differently. An engine
// that can do better than the shared implementation overrides the method
// rather than growing this interface.
type Dialect interface {
	// Name identifies the engine in errors and logs.
	Name() string

	// Rebind rewrites a statement written with ? placeholders into the form
	// the engine expects. Every statement passes through it exactly once.
	Rebind(query string) string

	// MigrationTableDDL creates the migration bookkeeping table if it is
	// missing. It runs before any migration and must be idempotent.
	MigrationTableDDL() string

	// JSONKeyCount renders the number of top-level keys in a JSON document
	// column, as a scalar expression. It binds nothing.
	JSONKeyCount(column string) string

	// JSONHasKey renders a test for one top-level key in a JSON document
	// column. The key name is bound, so the fragment consumes one argument.
	JSONHasKey(column string) string

	// RowLock renders the clause that holds a selected row for the rest of the
	// transaction, so a read-modify-write on it cannot interleave with another
	// caller's. An engine whose transactions already exclude each other has
	// nothing to add and returns an empty string.
	RowLock() string

	// TimeArg renders a time for the engine's own timestamp column: a native
	// value where the engine has one, text where it does not.
	TimeArg(value time.Time) any

	// NullTimeArg renders an optional time, and nil for one that is absent.
	NullTimeArg(value *time.Time) any

	// UniqueViolation reports whether a driver error means a row already
	// exists. Engines carry that verdict differently, one in a message and
	// another in a SQLSTATE code.
	UniqueViolation(err error) bool

	// ExecScript runs a migration file, which holds more than one statement.
	// A driver that speaks only the extended query protocol cannot send those
	// in one round trip, so the engine decides how they reach the server.
	ExecScript(ctx context.Context, conn *sql.Conn, script string) error
}
