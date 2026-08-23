// Package transfer copies a service's state from one storage engine to
// another, so changing engines does not mean starting from an empty database.
package transfer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage/open"
	"github.com/smykla-skalski/smyklot/internal/storage/sqlstore"
)

// tables lists every table a copy carries, ordered so that a row's references
// are already present when it arrives.
//
// schema_migrations and pending_ci_policy_lock are deliberately absent: both
// are schema-owned singleton state the destination creates while migrating.
// Copying either would collide with the destination rather than carry user or
// service data.
//
// The order is the schema's own dependency order and is stated rather than
// derived, so it can be read and checked. TestTableListCoversSchema fails if a
// table is added to the schema and not to this list, which is the only way it
// could go stale.
var tables = []string{
	"accounts",
	// Sessions are copied so a cutover does not sign everyone out, and because
	// root_elevations records a session_token_hash. Dropping the sessions while
	// keeping the elevations would leave a live elevation whose session the
	// panel cannot find.
	"sessions",
	"panel_users",
	"panel_owner",
	"targets",
	"target_ownership",
	"target_owners",
	"target_roles",
	"repositories",
	"pending_ci_repository_gates",
	"pending_ci_check_slots",
	"root_elevations",
	"sync_config_checkpoints",
	"sync_config_checkpoint_items",
	"audit_entries",
	"access_audit_entries",
	"app_audit_events",
	"security_notifications",
	"deliveries",
	"pending_ci_requests",
	"pending_ci_events",
	"pending_ci_intents",
	"pending_ci_source_revisions",
	"user_invitations",
	"runtime_settings",
	"user_preferences",
	// Active Sync state, then the plans computed from it and their actions.
	"sync_configs",
	"sync_repository_overrides",
	"sync_repository_paths",
	"sync_repository_state",
	"sync_plans",
	"sync_plan_actions",
	"sync_audit_entries",
}

// Engine is what a copy needs from a store: a connection to read or write on,
// and the dialect that says how this engine spells things.
//
// It is stated as an interface rather than a concrete type so that an engine
// which one day overrides half the shared store still satisfies it, and so
// that the copier depends on the two things it uses instead of on an adapter.
type Engine interface {
	DB() *sql.DB
	Dialect() sqlstore.Dialect
}

// Options are the choices an operator makes about a copy.
type Options struct {
	// Force empties the destination before copying into it.
	//
	// Without it a destination holding rows is refused. There is no merge
	// option, because there is no sound way to merge: every row carries the key
	// it had at the source, so a merge either collides on a key or silently
	// keeps one side of a row that exists in both. Emptying first is the only
	// reading of "do it anyway" that produces a database matching the source.
	Force bool
}

// ErrDestinationNotEmpty is returned when the destination already holds rows
// and Force was not given.
var ErrDestinationNotEmpty = errors.New("destination database is not empty")

// Report says how many rows each table received, newest engine first.
type Report struct {
	// Rows maps a table to the number of rows copied into it.
	Rows map[string]int

	// Duration is how long the copy took.
	Duration time.Duration
}

// Total returns the number of rows copied across every table.
func (r Report) Total() int {
	total := 0
	for _, count := range r.Rows {
		total += count
	}

	return total
}

// Between copies one database into another, naming each by connection string.
//
// This is the whole operation: it chooses both engines, brings both to the
// current schema by opening them, copies, and closes both. Nothing above the
// storage tree has to know which engines are involved, or that a copy reaches
// below the port to do its work.
//
// Both sides are migrated, the source included - it has to be, because the
// copy reads its columns and a source two releases behind would be missing
// some. That means the source file is written to. Copy it before pointing this
// at anything you cannot replace.
func Between(ctx context.Context, from, to string, options Options) (Report, error) {
	if from == to {
		return Report{}, fmt.Errorf("source and destination name the same database")
	}

	// Opened first so that a typo in the source is reported before the
	// destination has been created and migrated for a copy that cannot happen.
	source, err := openEngine(ctx, from)
	if err != nil {
		return Report{}, fmt.Errorf("open source: %w", err)
	}
	defer func() { _ = source.close() }()

	destination, err := openEngine(ctx, to)
	if err != nil {
		return Report{}, fmt.Errorf("open destination: %w", err)
	}
	defer func() { _ = destination.close() }()

	return Copy(ctx, source.engine, destination.engine, options)
}

// opened is a store held as both the interface a copy uses and the thing that
// has to be closed afterwards.
type opened struct {
	engine Engine
	close  func() error
}

// openEngine opens a connection string and produces what a copy needs from it.
func openEngine(ctx context.Context, connection string) (opened, error) {
	store, err := open.Store(ctx, connection)
	if err != nil {
		return opened{}, err
	}

	// Every adapter embeds the shared store, which is where DB and Dialect
	// live, so this holds for any engine that exists or is added. It is checked
	// rather than assumed because the port itself promises neither.
	engine, ok := store.(Engine)
	if !ok {
		_ = store.Close()

		return opened{}, fmt.Errorf("%T cannot be copied: it exposes no connection", store)
	}

	return opened{engine: engine, close: store.Close}, nil
}

// Copy moves every row from one store to another.
//
// The destination must be freshly migrated and empty; a copy into a database
// that already holds rows would merge two histories and is refused unless
// Force says to empty it first. Everything runs in one transaction on the
// destination, so a failure leaves it as it was rather than half-populated.
func Copy(ctx context.Context, from, to Engine, options Options) (Report, error) {
	started := time.Now()

	source, err := from.DB().Conn(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("reserve source connection: %w", err)
	}
	defer func() { _ = source.Close() }()

	destination, err := to.DB().Conn(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("reserve destination connection: %w", err)
	}
	defer func() { _ = destination.Close() }()

	if !options.Force {
		if err := requireEmpty(ctx, destination); err != nil {
			return Report{}, err
		}
	}

	report := Report{Rows: map[string]int{}}

	if _, err := destination.ExecContext(ctx, "BEGIN"); err != nil {
		return Report{}, fmt.Errorf("begin copy: %w", err)
	}
	if options.Force {
		if err := empty(ctx, destination); err != nil {
			_, _ = destination.ExecContext(context.WithoutCancel(ctx), "ROLLBACK")

			return Report{}, err
		}
	}
	for _, table := range tables {
		copied, copyErr := copyTable(ctx, source, destination, to.Dialect(), table)
		if copyErr != nil {
			_, _ = destination.ExecContext(context.WithoutCancel(ctx), "ROLLBACK")

			return Report{}, copyErr
		}
		report.Rows[table] = copied
	}
	if _, err := destination.ExecContext(ctx, "COMMIT"); err != nil {
		return Report{}, fmt.Errorf("commit copy: %w", err)
	}

	if err := to.Dialect().AfterCopy(ctx, destination, tables); err != nil {
		return Report{}, err
	}

	if err := verify(ctx, source, destination, report); err != nil {
		return Report{}, err
	}

	report.Duration = time.Since(started)

	return report, nil
}

// requireEmpty refuses a destination that already holds service state.
func requireEmpty(ctx context.Context, destination *sql.Conn) error {
	for _, table := range tables {
		count, err := countRows(ctx, destination, table)
		if err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("%w: table %q already holds %d rows", ErrDestinationNotEmpty, table, count)
		}
	}

	return nil
}

// empty clears the destination so a copy can replace what is there.
//
// Deletes run in reverse dependency order, so a row is gone before the row it
// references. This runs inside the copy's transaction: a failure anywhere
// afterwards leaves the destination holding what it held before.
func empty(ctx context.Context, destination *sql.Conn) error {
	for index := len(tables) - 1; index >= 0; index-- {
		table := tables[index]
		// #nosec G202 -- the table name comes from this package's own list.
		if _, err := destination.ExecContext(ctx, "DELETE FROM "+quote(table)); err != nil {
			return fmt.Errorf("empty %q: %w", table, err)
		}
	}

	return nil
}

// copyTable moves one table's rows and returns how many it moved.
func copyTable(
	ctx context.Context,
	source, destination *sql.Conn,
	dialect sqlstore.Dialect,
	table string,
) (int, error) {
	kinds, err := dialect.ColumnKinds(ctx, destination, table)
	if err != nil {
		return 0, err
	}

	// #nosec G202 -- the table name comes from this package's own list.
	rows, err := source.QueryContext(ctx, tableReadQuery(table))
	if err != nil {
		return 0, fmt.Errorf("read %q: %w", table, err)
	}
	defer func() { _ = rows.Close() }()

	columns, err := rows.Columns()
	if err != nil {
		return 0, fmt.Errorf("read %q columns: %w", table, err)
	}

	insert := insertStatement(dialect, table, columns)
	copied := 0
	for rows.Next() {
		values, scanErr := scanRow(rows, columns, kinds)
		if scanErr != nil {
			return 0, fmt.Errorf("read %q row: %w", table, scanErr)
		}
		if _, err := destination.ExecContext(ctx, insert, values...); err != nil {
			return 0, fmt.Errorf("write %q row: %w", table, err)
		}
		copied++
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate %q: %w", table, err)
	}

	return copied, nil
}

func tableReadQuery(table string) string {
	query := "SELECT * FROM " + quote(table)
	if table == "sync_config_checkpoints" {
		// A restore points at an older checkpoint in this same table. Immediate
		// foreign keys require the parent to arrive before the child regardless
		// of the source engine's physical row order.
		query += " ORDER BY id ASC"
	}

	return query
}

// scanRow reads one row, converting the values the destination stores as
// something other than what the source handed back.
func scanRow(rows *sql.Rows, columns []string, kinds map[string]sqlstore.ColumnKind) ([]any, error) {
	targets := make([]any, len(columns))
	times := make([]*sqlstore.StoredTime, len(columns))
	booleans := make([]*sql.NullBool, len(columns))
	raw := make([]any, len(columns))

	for index, column := range columns {
		switch kinds[column] {
		case sqlstore.ColumnTime:
			times[index] = &sqlstore.StoredTime{}
			targets[index] = times[index]
		case sqlstore.ColumnBool:
			booleans[index] = &sql.NullBool{}
			targets[index] = booleans[index]
		case sqlstore.ColumnOther:
			targets[index] = &raw[index]
		}
	}

	if err := rows.Scan(targets...); err != nil {
		return nil, err
	}

	values := make([]any, len(columns))
	for index := range columns {
		switch {
		case times[index] != nil:
			values[index] = times[index].Pointer()
		case booleans[index] != nil:
			values[index] = nullBool(*booleans[index])
		default:
			values[index] = raw[index]
		}
	}

	return values, nil
}

func nullBool(value sql.NullBool) any {
	if !value.Valid {
		return nil
	}

	return value.Bool
}

// insertStatement writes one row of a table, keeping the key it arrived with.
func insertStatement(dialect sqlstore.Dialect, table string, columns []string) string {
	quoted := make([]string, len(columns))
	placeholders := make([]string, len(columns))
	for index, column := range columns {
		quoted[index] = quote(column)
		placeholders[index] = "?"
	}

	// #nosec G201 -- table and column names come from the destination's own
	// schema, and every value remains a bound parameter.
	return dialect.Rebind(fmt.Sprintf(
		"INSERT INTO %s (%s)%s VALUES (%s)",
		quote(table),
		strings.Join(quoted, ", "),
		dialect.InsertOverride(),
		strings.Join(placeholders, ", "),
	))
}

// verify re-counts both sides, so a copy that silently dropped rows fails
// rather than reporting success.
func verify(ctx context.Context, source, destination *sql.Conn, report Report) error {
	for _, table := range tables {
		want, err := countRows(ctx, source, table)
		if err != nil {
			return err
		}
		got, err := countRows(ctx, destination, table)
		if err != nil {
			return err
		}
		if want != got || got != report.Rows[table] {
			return fmt.Errorf(
				"table %q: source has %d rows, destination has %d, copy reported %d",
				table, want, got, report.Rows[table],
			)
		}
	}

	return nil
}

func countRows(ctx context.Context, conn *sql.Conn, table string) (int, error) {
	var count int
	// #nosec G201 -- the table name comes from this package's own list.
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+quote(table)).Scan(&count); err != nil {
		return 0, fmt.Errorf("count %q: %w", table, err)
	}

	return count, nil
}

// quote makes an identifier safe to interpolate where a parameter cannot go.
func quote(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
