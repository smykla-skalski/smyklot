package storagetest

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	// Register the PostgreSQL driver, so a suite that only asks for a
	// connection string does not also have to know which driver serves it.
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/smykla-skalski/smyklot/internal/storage/sqlite"
)

// TestingT is what Connection needs from a test.
//
// It is narrower than testing.TB on purpose: that interface has an unexported
// method and so cannot be implemented outside the testing package, which means
// GinkgoT() does not satisfy it. Naming the four methods actually used lets a
// plain go test and a Ginkgo spec share one helper.
type TestingT interface {
	Helper()
	Cleanup(func())
	Fatalf(format string, args ...any)
	Errorf(format string, args ...any)
}

// DSNVariable names the server the suites run against. There is no default: a
// test that invented a connection would either touch a database nobody meant
// it to, or pass by not running.
const DSNVariable = "SMYKLOT_TEST_POSTGRES_DSN"

// Connection returns a connection string for a suite above the storage port.
//
// It exists so that the panel and the service are exercised on whichever
// engine the environment provides, rather than on SQLite forever because that
// is what a hard-coded path selects. Those suites do not care which engine
// they got - that is the point of the port - so they should not have to say.
//
// With DSNVariable set it creates a schema of its own on that server and drops
// it afterwards. Without it, a SQLite file under dir.
func Connection(t TestingT, dir string) string {
	t.Helper()

	dsn := strings.TrimSpace(os.Getenv(DSNVariable))
	if dsn == "" {
		return sqliteFromTemplate(t, dir)
	}

	schema := uniqueSchema()

	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer func() { _ = admin.Close() }()

	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatalf("create schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		cleanup, err := sql.Open("pgx", dsn)
		if err != nil {
			t.Errorf("open postgres to drop %s: %v", schema, err)

			return
		}
		defer func() { _ = cleanup.Close() }()
		if _, err := cleanup.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
			t.Errorf("drop schema %s: %v", schema, err)
		}
	})

	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}

	return dsn + separator + "search_path=" + schema
}

// sqliteFromTemplate writes a database that has already been migrated.
//
// Opening a store replays the migration series, and on the pure-Go SQLite under
// the race detector that costs about a third of a second - which the suites
// above the port pay once per spec, and which was most of what they spent. The
// series runs once per process now and every caller after that gets a byte copy
// of what it produced.
//
// The store still migrates what it is handed. It reads the ledger, finds every
// version already applied and does nothing, which is the path a restarted
// service takes anyway. What migrating a *fresh* database does is proved where
// it belongs, in the adapter's own suite, which opens its own file rather than
// asking for one here.
func sqliteFromTemplate(t TestingT, dir string) string {
	t.Helper()

	migrated, err := sqliteTemplate()
	if err != nil {
		t.Fatalf("build the migrated SQLite template: %v", err)
	}

	path := filepath.Join(dir, "state.db")
	if err := os.WriteFile(path, migrated, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	return path
}

// sqliteTemplate migrates one database and returns its bytes.
var sqliteTemplate = sync.OnceValues(func() ([]byte, error) {
	dir, err := os.MkdirTemp("", "smyklot-storagetest")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	path := filepath.Join(dir, "template.db")
	store, err := sqlite.Open(context.Background(), path)
	if err != nil {
		return nil, err
	}
	// Closing the last connection checkpoints the write-ahead log into the file
	// and removes it, so the file alone is the whole database. Reading it while
	// a log was still beside it would hand every caller an empty one, and every
	// caller would silently migrate it again - back to being correct and slow,
	// with nothing to say so. Hence the check below.
	if err := store.Close(); err != nil {
		return nil, err
	}

	if err := assertMigrated(path); err != nil {
		return nil, err
	}

	return os.ReadFile(path)
})

// assertMigrated refuses a template that is not what it claims to be.
func assertMigrated(path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	for _, table := range SeededTables() {
		var name string
		err := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", table,
		).Scan(&name)
		if err != nil {
			return fmt.Errorf("template is missing %s: %w", table, err)
		}
	}

	return nil
}

// schemaCounter makes each schema name unique within this process. The pid
// makes it unique across the several processes `go test ./...` runs at once,
// and names the one to blame for a schema that outlived its suite.
var schemaCounter atomic.Uint64

func uniqueSchema() string {
	return fmt.Sprintf("smyklot_suite_%d_%d", os.Getpid(), schemaCounter.Add(1))
}
