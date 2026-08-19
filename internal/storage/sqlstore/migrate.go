package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"testing/fstest"
)

// migrationDir is the directory an engine embeds its schema changes under.
const migrationDir = "migrations"

// errMigrationName reports a file the runner cannot order.
var errMigrationName = errors.New("migration name must start with a numeric version")

// errMigrationVersion reports two files claiming one version.
var errMigrationVersion = errors.New("two migrations share a version")

// Migrate applies every unapplied .sql file in fsys, in filename order, inside
// one transaction. A migration that fails leaves the schema untouched.
//
// Every statement runs on one reserved connection. A driver that speaks only
// the extended query protocol needs the dialect's own escape hatch to send a
// multi-statement file, and that escape hatch has to join the same transaction
// as the bookkeeping the runner writes around it.
func Migrate(ctx context.Context, pool *sql.DB, dialect Dialect, fsys fs.FS) (err error) {
	conn, err := pool.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve %s migration connection: %w", dialect.Name(), err)
	}

	defer func() {
		if closeErr := conn.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("release %s migration connection: %w", dialect.Name(), closeErr)
		}
	}()

	names, err := migrationNames(fsys)
	if err != nil {
		return err
	}

	// BEGIN is issued as a statement rather than through database/sql, because
	// the dialect's escape hatch reaches the driver connection directly and has
	// to land inside the same transaction as the bookkeeping around it.
	if _, err := conn.ExecContext(ctx, "BEGIN"); err != nil {
		return fmt.Errorf("begin %s migrations: %w", dialect.Name(), err)
	}

	defer func() {
		if err != nil {
			_, _ = conn.ExecContext(context.WithoutCancel(ctx), "ROLLBACK")
		}
	}()

	if _, err := conn.ExecContext(ctx, dialect.MigrationTableDDL()); err != nil {
		return fmt.Errorf("create %s migration table: %w", dialect.Name(), err)
	}

	for _, name := range names {
		if err := applyMigration(ctx, conn, dialect, fsys, name); err != nil {
			return err
		}
	}

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("commit %s migrations: %w", dialect.Name(), err)
	}

	return nil
}

// migrationNames returns the schema files in the order their versions apply.
//
// A version claimed twice is refused rather than run. The runner records a
// version once and skips anything that repeats it, so the second file would be
// dropped in silence - and which of the two is dropped depends on which one is
// already recorded, so two databases that ran the same code would end up with
// different schemas. Two branches open at once is how it happens, and neither
// author sees anything wrong.
func migrationNames(fsys fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(fsys, migrationDir)
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		names = append(names, entry.Name())
	}

	sort.Strings(names)

	claimed := make(map[int]string, len(names))
	for _, name := range names {
		version, err := migrationVersion(name)
		if err != nil {
			return nil, err
		}

		if first, taken := claimed[version]; taken {
			return nil, fmt.Errorf("%w: %q and %q", errMigrationVersion, first, name)
		}

		claimed[version] = name
	}

	return names, nil
}

// MigrationsBefore is the series as it stood before a given version.
//
// What a deployment holding real rows looks like when the next migration is
// about to run - which is the only state a destructive migration can be proved
// against, and the one the conformance suite never reaches because it builds
// every database from nothing.
//
// Here rather than in each engine's tests because the ordering rule is this
// file's: a helper that cut the series by string comparison would agree with
// `Migrate` until the day a version reached three digits.
func MigrationsBefore(fsys fs.FS, version int) (fs.FS, error) {
	names, err := migrationNames(fsys)
	if err != nil {
		return nil, err
	}

	earlier := fstest.MapFS{}
	for _, name := range names {
		at, err := migrationVersion(name)
		if err != nil {
			return nil, err
		}
		if at >= version {
			break
		}

		content, readErr := fs.ReadFile(fsys, migrationDir+"/"+name)
		if readErr != nil {
			return nil, fmt.Errorf("read migration %s: %w", name, readErr)
		}
		earlier[migrationDir+"/"+name] = &fstest.MapFile{Data: content}
	}

	return earlier, nil
}

func applyMigration(
	ctx context.Context,
	conn *sql.Conn,
	dialect Dialect,
	fsys fs.FS,
	name string,
) error {
	version, err := migrationVersion(name)
	if err != nil {
		return err
	}

	var applied int
	if err := conn.QueryRowContext(
		ctx,
		dialect.Rebind("SELECT COUNT(*) FROM schema_migrations WHERE version = ?"),
		version,
	).Scan(&applied); err != nil {
		return fmt.Errorf("read %s migration version: %w", dialect.Name(), err)
	}

	if applied != 0 {
		return nil
	}

	content, err := fs.ReadFile(fsys, migrationDir+"/"+name)
	if err != nil {
		return fmt.Errorf("read migration %q: %w", name, err)
	}

	if err := dialect.ExecScript(ctx, conn, string(content)); err != nil {
		return fmt.Errorf("apply %s migration %q: %w", dialect.Name(), name, err)
	}

	if _, err := conn.ExecContext(
		ctx,
		dialect.Rebind("INSERT INTO schema_migrations(version) VALUES (?)"),
		version,
	); err != nil {
		return fmt.Errorf("record %s migration %q: %w", dialect.Name(), name, err)
	}

	return nil
}

func migrationVersion(name string) (int, error) {
	versionText, _, ok := strings.Cut(name, "_")
	if !ok {
		return 0, fmt.Errorf("%w: %q", errMigrationName, name)
	}

	version, err := strconv.Atoi(versionText)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", errMigrationName, name)
	}

	return version, nil
}
