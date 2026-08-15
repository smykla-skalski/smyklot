package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/smykla-skalski/smyklot/internal/storage/sqlstore"
)

// uniqueViolation is the SQLSTATE code for a row that already exists.
const uniqueViolation = "23505"

// Dialect spells the shared store's SQL for PostgreSQL.
type Dialect struct{}

// Name identifies the engine in errors and logs.
func (Dialect) Name() string { return "postgres" }

// Rebind numbers the placeholders, because this engine addresses a parameter
// by position rather than by order of appearance.
//
// A ? inside a string literal or a quoted identifier is data, not a parameter,
// so the scan tracks whether it is inside one.
func (Dialect) Rebind(query string) string {
	var rebound strings.Builder
	rebound.Grow(len(query) + 8)

	parameter := 0
	quote := byte(0)
	for index := 0; index < len(query); index++ {
		character := query[index]

		if quote != 0 {
			rebound.WriteByte(character)
			// A doubled quote is an escaped one and does not close the literal.
			if character == quote {
				if index+1 < len(query) && query[index+1] == quote {
					rebound.WriteByte(quote)
					index++

					continue
				}
				quote = 0
			}

			continue
		}

		switch character {
		case '\'', '"':
			quote = character
			rebound.WriteByte(character)
		case '?':
			parameter++
			rebound.WriteByte('$')
			rebound.WriteString(strconv.Itoa(parameter))
		default:
			rebound.WriteByte(character)
		}
	}

	return rebound.String()
}

// MigrationTableDDL creates the bookkeeping table the migration runner owns.
func (Dialect) MigrationTableDDL() string {
	return `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`
}

// JSONKeyCount counts the top-level keys of a JSON document.
func (Dialect) JSONKeyCount(column string) string {
	return "(SELECT COUNT(*) FROM jsonb_object_keys(" + column + "))"
}

// JSONHasKey reports whether a JSON document has one top-level key.
//
// The function form is deliberate. The ? operator asks the same question, but
// a ? in a statement is a placeholder until Rebind has run, and an operator
// that looks like one would be renumbered into nonsense.
func (Dialect) JSONHasKey(column string) string {
	return "jsonb_exists(" + column + ", ?)"
}

// RowLock holds the selected row until the transaction ends.
//
// Read committed is the default here, so a transaction reads a snapshot taken
// when its statement began and cannot see rows another transaction has not
// committed yet. Two callers deciding what to keep would each decide from a
// view without the other, and both would keep everything.
func (Dialect) RowLock() string { return " FOR UPDATE" }

// TimeArg passes a time through, because this engine has a timestamp type.
func (Dialect) TimeArg(value time.Time) any { return value.UTC() }

// NullTimeArg passes an optional time through, and NULL for one that is absent.
func (Dialect) NullTimeArg(value *time.Time) any {
	if value == nil {
		return nil
	}

	return value.UTC()
}

// UniqueViolation reports a row that already exists, by SQLSTATE rather than
// by message, so it stays correct in any server locale.
func (Dialect) UniqueViolation(err error) bool {
	var pgErr *pgconn.PgError

	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}

// ColumnKinds reports the columns this engine stores as a real timestamp or a
// real boolean, which another engine may hand over as text or as 0 and 1.
func (Dialect) ColumnKinds(
	ctx context.Context,
	conn *sql.Conn,
	table string,
) (map[string]sqlstore.ColumnKind, error) {
	rows, err := conn.QueryContext(ctx, `
SELECT column_name, data_type
FROM information_schema.columns
WHERE table_schema = current_schema() AND table_name = $1`, table)
	if err != nil {
		return nil, fmt.Errorf("read postgres column types for %q: %w", table, err)
	}
	defer func() { _ = rows.Close() }()

	kinds := map[string]sqlstore.ColumnKind{}
	for rows.Next() {
		var column, dataType string
		if err := rows.Scan(&column, &dataType); err != nil {
			return nil, fmt.Errorf("scan postgres column type: %w", err)
		}
		switch {
		case strings.HasPrefix(dataType, "timestamp"):
			kinds[column] = sqlstore.ColumnTime
		case dataType == "boolean":
			kinds[column] = sqlstore.ColumnBool
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate postgres column types: %w", err)
	}

	return kinds, nil
}

// InsertOverride lets a copied row keep the key it already had. An identity
// column is declared ALWAYS so that ordinary code cannot assign one, and this
// is the one caller that must.
func (Dialect) InsertOverride() string { return " OVERRIDING SYSTEM VALUE" }

// AfterCopy advances each identity sequence past the keys just written.
//
// Writing a key directly leaves the sequence where it was, so without this the
// first insert after a copy would collide with a row the copy created.
func (Dialect) AfterCopy(ctx context.Context, conn *sql.Conn, tables []string) error {
	generated, err := identityTables(ctx, conn)
	if err != nil {
		return err
	}

	for _, table := range tables {
		// A table whose key the application supplies - an account id, a token
		// hash - has no sequence, and nothing to advance. Which tables those
		// are is asked rather than guarded with a WHERE, because a WHERE would
		// not help: PostgreSQL type-checks a statement whole before evaluating
		// any of it, and MAX(id) over a table keyed by text cannot be matched
		// with the bigint a sequence holds.
		if !generated[table] {
			continue
		}

		var sequence string
		// Deliberately unqualified, so the name resolves through search_path
		// exactly as the copy's own INSERT did.
		if err := conn.QueryRowContext(
			ctx, "SELECT pg_get_serial_sequence($1, 'id')", table,
		).Scan(&sequence); err != nil {
			return fmt.Errorf("look up %q identity sequence: %w", table, err)
		}

		// #nosec G202 -- the table name is quoted and comes from the caller's
		// own schema list; the sequence is bound as a parameter.
		if _, err := conn.ExecContext(ctx, `
SELECT setval(
    $1,
    COALESCE((SELECT MAX(id) FROM `+quoteIdentifier(table)+`), 1),
    (SELECT COUNT(*) FROM `+quoteIdentifier(table)+`) > 0
)`, sequence); err != nil {
			return fmt.Errorf("advance %q identity sequence: %w", table, err)
		}
	}

	return nil
}

// identityTables names every reachable table whose id the database generates.
func identityTables(ctx context.Context, conn *sql.Conn) (map[string]bool, error) {
	rows, err := conn.QueryContext(ctx, `
SELECT DISTINCT table_name
FROM information_schema.columns
WHERE column_name = 'id'
  AND is_identity = 'YES'
  AND table_schema = ANY (current_schemas(false))`)
	if err != nil {
		return nil, fmt.Errorf("read identity columns: %w", err)
	}
	defer func() { _ = rows.Close() }()

	generated := map[string]bool{}
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, fmt.Errorf("scan identity column: %w", err)
		}
		generated[table] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate identity columns: %w", err)
	}

	return generated, nil
}

// quoteIdentifier makes a table name safe to interpolate where a parameter is
// not allowed. Every caller passes a name from the adapter's own schema, and
// this keeps that true even if one day a caller does not.
func quoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// ExecScript runs a migration file.
//
// The database/sql driver speaks the extended query protocol, which carries
// one statement per message, so a file holding several is rejected. Reaching
// the pgx connection underneath allows the simple protocol, which accepts the
// whole file. The connection is the one the runner reserved, so the statements
// land in the transaction it opened.
func (Dialect) ExecScript(ctx context.Context, conn *sql.Conn, script string) error {
	if err := conn.Raw(func(driverConn any) error {
		connector, ok := driverConn.(*stdlib.Conn)
		if !ok {
			return fmt.Errorf("expected a pgx connection, got %T", driverConn)
		}
		_, err := connector.Conn().Exec(ctx, script)

		return err
	}); err != nil {
		return fmt.Errorf("exec postgres script: %w", err)
	}

	return nil
}
