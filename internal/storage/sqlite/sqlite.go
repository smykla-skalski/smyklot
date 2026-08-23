// Package sqlite runs the shared SQL store on a CGo-free SQLite database.
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
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/storage/sqlstore"

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

// Store is the SQLite storage adapter. It inherits every query from the shared
// store and overrides nothing yet.
type Store struct {
	*sqlstore.Store
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

	pool, err := sql.Open("sqlite", dataSourceName(path))
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	// One connection matches the v1 single-replica deployment and makes every
	// connection-local PRAGMA deterministic. WAL still lets readers progress
	// while a writer commits.
	pool.SetMaxOpenConns(1)
	pool.SetMaxIdleConns(1)

	if err := sqlstore.Migrate(ctx, pool, Dialect{}, migrations); err != nil {
		_ = pool.Close()

		return nil, err
	}
	shared := sqlstore.New(pool, Dialect{})
	if err := shared.BackfillSettingsCheckpointBaselines(ctx, time.Now().UTC()); err != nil {
		_ = pool.Close()

		return nil, err
	}

	return &Store{Store: shared}, nil
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
