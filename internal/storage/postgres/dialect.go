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
