// Package sqlstore implements storage.Store once, for every SQL engine.
//
// The queries live here in one portable form. What an engine spells
// differently it supplies through a Dialect, and what an engine can do better
// than the shared implementation it overrides by embedding this Store. An
// engine package therefore holds a driver, a DSN, a dialect and its own
// migrations, and nothing else.
package sqlstore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

// Store is the engine-neutral storage adapter.
type Store struct {
	db      handle
	dialect Dialect
}

var _ storage.Store = (*Store)(nil)

// New wraps an open pool. The caller owns opening the pool and applying
// migrations, because both are engine-specific.
func New(pool *sql.DB, dialect Dialect) *Store {
	return &Store{db: newHandle(pool, dialect, newQueryStats()), dialect: dialect}
}

// DB returns the underlying pool.
//
// This is for an engine adapter that needs to reach its own server directly,
// and for tests that inspect what a migration produced. The storage.Store port
// still exposes no handle, and depguard keeps database/sql inside this tree.
func (s *Store) DB() *sql.DB { return s.db.pool }

// Dialect returns how this store's engine spells its SQL. A copy between two
// engines needs the destination's, to know what it stores differently.
func (s *Store) Dialect() Dialect { return s.dialect }

// Ping verifies that the adapter can reach its database.
func (s *Store) Ping(ctx context.Context) error {
	if err := s.db.pool.PingContext(ctx); err != nil {
		return fmt.Errorf("ping %s database: %w", s.dialect.Name(), err)
	}

	return nil
}

// Close releases the connection pool.
func (s *Store) Close() error {
	return s.db.pool.Close()
}
