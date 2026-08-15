package sqlstore

import (
	"context"
	"database/sql"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

// schemaVersionQuery reads the highest applied migration. The bookkeeping
// table belongs to the migration runner, which writes it the same way on every
// engine, so this one needs nothing from the dialect.
const schemaVersionQuery = "SELECT MAX(version) FROM schema_migrations"

// Status describes the database this store is talking to.
//
// The reachability check runs first and is the only thing that can leave the
// rest unanswered. What follows it reads one detail at a time and stops at the
// first refusal: those queries run against a server that has just answered, so
// one failing means the database has stopped describing itself, and the
// remaining values would not explain that any better than the first cause.
func (s *Store) Status(ctx context.Context) storage.DatabaseStatus {
	status := storage.DatabaseStatus{Engine: s.dialect.DisplayName()}

	started := time.Now()
	err := s.db.pool.PingContext(ctx)
	status.Latency = time.Since(started)

	// Sampled after the round trip rather than before it, so the pool a reader
	// sees is the one that just served a statement, and so both the answered
	// and the unanswered case report it from the same place.
	status.Connections = connectionStats(s.db.pool.Stats())

	if err != nil {
		status.Error = err.Error()

		return status
	}
	status.Reachable = true

	// NULL rather than no row: MAX over an empty table is one NULL row, and a
	// database with no migrations at all is a database this service has never
	// started against.
	var schemaVersion sql.NullInt64
	if err := s.db.pool.QueryRowContext(ctx, schemaVersionQuery).Scan(&schemaVersion); err != nil {
		status.Error = err.Error()

		return status
	}
	status.SchemaVersion = int(schemaVersion.Int64)

	if err := s.db.pool.QueryRowContext(
		ctx, s.dialect.VersionQuery(),
	).Scan(&status.Version); err != nil {
		status.Error = err.Error()

		return status
	}

	sizeQuery := s.dialect.SizeQuery()
	if sizeQuery == "" {
		return status
	}
	if err := s.db.pool.QueryRowContext(ctx, sizeQuery).Scan(&status.SizeBytes); err != nil {
		status.Error = err.Error()
	}

	return status
}

func connectionStats(stats sql.DBStats) storage.ConnectionStats {
	return storage.ConnectionStats{
		Open:         stats.OpenConnections,
		InUse:        stats.InUse,
		Idle:         stats.Idle,
		Max:          stats.MaxOpenConnections,
		WaitCount:    stats.WaitCount,
		WaitDuration: stats.WaitDuration,
	}
}
