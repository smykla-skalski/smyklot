// Package sqlite implements storage.Store with a CGo-free SQLite database.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/smykla-skalski/smyklot/internal/storage"

	// Register the CGo-free SQLite database/sql driver used by this adapter.
	_ "modernc.org/sqlite"
)

const inMemoryPath = ":memory:"

var (
	// migrations contains ordered, adapter-owned schema changes.
	//
	//go:embed migrations/*.sql
	migrations embed.FS

	errEmptyPath = errors.New("sqlite path must not be empty")
)

// Store is the SQLite storage adapter.
type Store struct {
	db *sql.DB
}

var _ storage.Store = (*Store)(nil)

// Open opens a SQLite database, configures safe service defaults, and applies
// all embedded migrations before returning.
func Open(ctx context.Context, path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errEmptyPath
	}

	if path != inMemoryPath {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return nil, fmt.Errorf("create sqlite directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite", dataSourceName(path))
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	// One connection matches the v1 single-replica deployment and makes every
	// connection-local PRAGMA deterministic. WAL still lets readers progress
	// while a writer commits.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	store := &Store{db: db}
	if err := store.migrate(ctx); err != nil {
		_ = db.Close()

		return nil, err
	}

	return store, nil
}

// Ping verifies that the adapter can reach its database.
func (s *Store) Ping(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping sqlite database: %w", err)
	}

	return nil
}

// Close releases the SQLite connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func dataSourceName(path string) string {
	values := url.Values{}
	values.Add("_pragma", "busy_timeout(5000)")
	values.Add("_pragma", "foreign_keys(1)")
	values.Add("_pragma", "journal_mode(WAL)")
	// A webhook is acknowledged before its command runs. FULL ensures its WAL
	// commit survives an OS or Fly-host crash, so a redelivery cannot repeat
	// work whose durable claim vanished with the machine.
	values.Add("_pragma", "synchronous(FULL)")

	if path == inMemoryPath {
		values.Set("mode", "memory")
		values.Set("cache", "shared")

		return "file:smyklot-panel?" + values.Encode()
	}

	return (&url.URL{Scheme: "file", Path: path, RawQuery: values.Encode()}).String()
}

func (s *Store) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sqlite migrations: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
)`); err != nil {
		return fmt.Errorf("create sqlite migration table: %w", err)
	}

	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read sqlite migrations: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		if err := applyMigration(ctx, tx, entry.Name()); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sqlite migrations: %w", err)
	}

	return nil
}

func applyMigration(ctx context.Context, tx *sql.Tx, name string) error {
	versionText, _, ok := strings.Cut(name, "_")
	if !ok {
		return fmt.Errorf("invalid sqlite migration name %q", name)
	}

	version, err := strconv.Atoi(versionText)
	if err != nil {
		return fmt.Errorf("parse sqlite migration version %q: %w", name, err)
	}

	var applied int
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM schema_migrations WHERE version = ?",
		version,
	).Scan(&applied); err != nil {
		return fmt.Errorf("read sqlite migration version: %w", err)
	}

	if applied != 0 {
		return nil
	}

	content, err := migrations.ReadFile("migrations/" + name)
	if err != nil {
		return fmt.Errorf("read sqlite migration %q: %w", name, err)
	}

	if _, err := tx.ExecContext(ctx, string(content)); err != nil {
		return fmt.Errorf("apply sqlite migration %q: %w", name, err)
	}

	if _, err := tx.ExecContext(
		ctx,
		"INSERT INTO schema_migrations(version) VALUES (?)",
		version,
	); err != nil {
		return fmt.Errorf("record sqlite migration %q: %w", name, err)
	}

	return nil
}
