package sqlstore

import (
	"context"
	"database/sql"
	"time"
)

// ColumnKind is a storage type that engines spell differently.
type ColumnKind int

const (
	// ColumnOther needs no conversion on the way in.
	ColumnOther ColumnKind = iota

	// ColumnTime is a real timestamp, where another engine may hand over text.
	ColumnTime

	// ColumnBool is a real boolean, where another engine may hand over 0 or 1.
	ColumnBool

	// ColumnText is text, where another engine may hand over bytes. A document
	// column is the case: PostgreSQL answers jsonb with bytes, and SQLite's
	// text affinity does not convert them, so the copy would hold a blob where
	// every native write holds text and equality against a string stops
	// matching.
	ColumnText
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

	// DisplayName names the engine for a person reading a panel, where Name
	// identifies it for a log line: "PostgreSQL" against "postgres". It is
	// spelled here so that nothing above the port has to map one to the other.
	DisplayName() string

	// VersionQuery selects the server's own release as a single string, and
	// only the release: an engine that reports its packaging alongside trims
	// that here, rather than leaving every caller to guess at the shape.
	VersionQuery() string

	// SizeQuery selects how many bytes the database occupies, as a single
	// integer. An engine with no answer returns an empty string, which is not
	// the same as returning zero.
	SizeQuery() string

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

	// ColumnKinds reports the columns of a table that the engine stores as
	// something other than text, so a copy from another engine can convert
	// what it reads. An engine that stores everything as text or a number
	// accepts the Go values another engine hands back and reports nothing.
	ColumnKinds(ctx context.Context, conn *sql.Conn, table string) (map[string]ColumnKind, error)

	// InsertOverride lets a bulk copy write a key the engine would otherwise
	// generate. Empty where the engine has no such objection.
	InsertOverride() string

	// AfterCopy repairs whatever a bulk copy left inconsistent, such as a
	// sequence that did not advance past the keys written around it.
	AfterCopy(ctx context.Context, conn *sql.Conn, tables []string) error

	// ExecScript runs a migration file, which holds more than one statement.
	// A driver that speaks only the extended query protocol cannot send those
	// in one round trip, so the engine decides how they reach the server.
	ExecScript(ctx context.Context, conn *sql.Conn, script string) error
}
