// Package open chooses a storage engine from a connection string.
//
// It exists so that nothing above the storage port names an engine. A caller
// passes what the operator configured and receives a storage.Store; which
// package served it is not something the caller can observe or depend on.
package open

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/storage/postgres"
	"github.com/smykla-skalski/smyklot/internal/storage/sqlite"
)

// Engine names a storage backend in logs and errors.
type Engine string

const (
	// EngineSQLite is a file on local disk.
	EngineSQLite Engine = "sqlite"

	// EnginePostgres is a PostgreSQL server.
	EnginePostgres Engine = "postgres"
)

// Store opens the engine the connection string names.
//
// A postgres:// or postgresql:// URL selects PostgreSQL. A sqlite:// URL or a
// bare filesystem path selects SQLite, which keeps every existing deployment
// working without its operator learning a new spelling.
func Store(ctx context.Context, connection string) (storage.Store, error) {
	engine, target, err := Resolve(connection)
	if err != nil {
		return nil, err
	}

	switch engine {
	case EnginePostgres:
		store, openErr := postgres.Open(ctx, target)
		if openErr != nil {
			return nil, openErr
		}

		return store, nil
	case EngineSQLite:
		store, openErr := sqlite.Open(ctx, target)
		if openErr != nil {
			return nil, openErr
		}

		return store, nil
	default:
		return nil, fmt.Errorf("unsupported storage engine %q", engine)
	}
}

// Resolve reports which engine a connection string names and what to hand it.
//
// It is separate from Store so that configuration can be validated, and the
// choice reported, before anything tries to connect.
func Resolve(connection string) (Engine, string, error) {
	trimmed := strings.TrimSpace(connection)
	if trimmed == "" {
		return "", "", fmt.Errorf("storage connection must not be empty")
	}

	scheme, rest, found := strings.Cut(trimmed, "://")
	if !found {
		// A bare path is a SQLite file. Every deployment before a second
		// engine existed configured one, and they keep working unchanged.
		return EngineSQLite, trimmed, nil
	}

	switch strings.ToLower(scheme) {
	case "postgres", "postgresql":
		return EnginePostgres, trimmed, nil
	case "sqlite", "sqlite3", "file":
		path, err := sqlitePath(trimmed, rest)
		if err != nil {
			return "", "", err
		}

		return EngineSQLite, path, nil
	default:
		return "", "", fmt.Errorf("unsupported storage scheme %q", scheme)
	}
}

// sqlitePath extracts the file a sqlite:// URL points at. Both sqlite:/path
// and sqlite:///path name an absolute path, and the adapter wants the path
// rather than the URL.
func sqlitePath(connection, rest string) (string, error) {
	parsed, err := url.Parse(connection)
	if err != nil {
		return "", fmt.Errorf("parse sqlite connection: %w", err)
	}
	if parsed.Path != "" {
		return parsed.Path, nil
	}
	if parsed.Opaque != "" {
		return parsed.Opaque, nil
	}

	return rest, nil
}
