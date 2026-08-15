// Package postgres runs the shared SQL store on PostgreSQL.
package postgres

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/storage/sqlstore"

	// Register the pure-Go PostgreSQL database/sql driver used by this adapter.
	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	// driverName is what pgx registers itself as with database/sql.
	driverName = "pgx"

	// maxOpenConns bounds concurrent statements.
	//
	// The service runs one replica with a bounded worker queue, so this only
	// has to cover those workers plus the panel and the sweep. Leaving it
	// unbounded would let a stall open connections until the server refused
	// them, which is a worse failure than waiting for one.
	maxOpenConns = 16

	// maxIdleConns keeps the pool warm without holding server memory for
	// connections a quiet service is not using.
	maxIdleConns = 4

	// connMaxLifetime recycles connections so a restarted or failed-over
	// server does not leave the pool holding handles to something gone.
	connMaxLifetime = time.Hour
)

// migrations contains ordered, adapter-owned schema changes.
//
//go:embed migrations/*.sql
var migrations embed.FS

var errEmptyDSN = errors.New("postgres connection string must not be empty")

// Store is the PostgreSQL storage adapter. It inherits every query from the
// shared store and overrides nothing yet.
type Store struct {
	*sqlstore.Store
}

var _ storage.Store = (*Store)(nil)

// Open connects to PostgreSQL, configures the pool, and applies all embedded
// migrations before returning.
func Open(ctx context.Context, dsn string) (*Store, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errEmptyDSN
	}

	pool, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres database: %w", err)
	}

	pool.SetMaxOpenConns(maxOpenConns)
	pool.SetMaxIdleConns(maxIdleConns)
	pool.SetConnMaxLifetime(connMaxLifetime)

	if err := pool.PingContext(ctx); err != nil {
		_ = pool.Close()

		return nil, fmt.Errorf("reach postgres database: %w", err)
	}

	if err := sqlstore.Migrate(ctx, pool, Dialect{}, migrations); err != nil {
		_ = pool.Close()

		return nil, err
	}

	return &Store{Store: sqlstore.New(pool, Dialect{})}, nil
}
