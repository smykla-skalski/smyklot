package sqlstore

import (
	"context"
	"database/sql"
	"time"
)

// runner is the statement surface every query in this package needs. The
// store's pooled handle and an open transaction both satisfy it, so a helper
// takes one parameter whether or not its caller opened a transaction.
type runner interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// binder rewrites every statement into the engine's placeholder form before it
// reaches database/sql. Query code holds a binder, never a raw handle, so a
// statement written with ? runs unchanged on an engine that numbers its
// parameters.
type binder struct {
	raw     runner
	dialect Dialect
	stats   *queryStats
}

func (b binder) ExecContext(
	ctx context.Context,
	query string,
	arguments ...any,
) (sql.Result, error) {
	started := time.Now()
	result, err := b.raw.ExecContext(ctx, b.dialect.Rebind(query), b.bind(arguments)...)
	b.stats.observe(queryCaller(), time.Since(started), err != nil)

	return result, err
}

func (b binder) QueryContext(
	ctx context.Context,
	query string,
	arguments ...any,
) (*sql.Rows, error) {
	started := time.Now()
	rows, err := b.raw.QueryContext(ctx, b.dialect.Rebind(query), b.bind(arguments)...)
	b.stats.observe(queryCaller(), time.Since(started), err != nil)

	return rows, err
}

func (b binder) QueryRowContext(
	ctx context.Context,
	query string,
	arguments ...any,
) *sql.Row {
	started := time.Now()
	row := b.raw.QueryRowContext(ctx, b.dialect.Rebind(query), b.bind(arguments)...)
	b.stats.observe(queryCaller(), time.Since(started), row.Err() != nil)

	return row
}

// bind hands each argument to the engine in the shape that engine stores.
//
// Only time needs this. One engine has a real timestamp type; another keeps
// timestamps as text and has to be told which text. Converting here means a
// query passes the time.Time it already holds and no caller formats anything.
func (b binder) bind(arguments []any) []any {
	bound := make([]any, len(arguments))
	for index, argument := range arguments {
		switch value := argument.(type) {
		case time.Time:
			bound[index] = b.dialect.TimeArg(value)
		case *time.Time:
			bound[index] = b.dialect.NullTimeArg(value)
		default:
			bound[index] = argument
		}
	}

	return bound
}

// handle is the store's pooled connection. It binds statements like any other
// runner and additionally opens transactions.
type handle struct {
	binder

	pool *sql.DB
}

func newHandle(pool *sql.DB, dialect Dialect, stats *queryStats) handle {
	return handle{binder: binder{raw: pool, dialect: dialect, stats: stats}, pool: pool}
}

// BeginTx opens a transaction whose statements are bound the same way. The
// error is returned unwrapped, because every caller already says which piece
// of work it was beginning.
func (h handle) BeginTx(ctx context.Context, options *sql.TxOptions) (*transaction, error) {
	tx, err := h.pool.BeginTx(ctx, options)
	if err != nil {
		return nil, err
	}

	return &transaction{
		binder: binder{raw: tx, dialect: h.dialect, stats: h.stats}, tx: tx,
	}, nil
}

// transaction is one open transaction. Callers commit it or let the deferred
// rollback discard it, exactly as they would a database/sql transaction.
type transaction struct {
	binder

	tx *sql.Tx
}

func (t *transaction) Commit() error   { return t.tx.Commit() }
func (t *transaction) Rollback() error { return t.tx.Rollback() }
