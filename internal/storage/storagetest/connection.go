package storagetest

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	// Register the PostgreSQL driver, so a suite that only asks for a
	// connection string does not also have to know which driver serves it.
	_ "github.com/jackc/pgx/v5/stdlib"
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
		return filepath.Join(dir, "state.db")
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

// schemaCounter makes each schema name unique within this process. The pid
// makes it unique across the several processes `go test ./...` runs at once,
// and names the one to blame for a schema that outlived its suite.
var schemaCounter atomic.Uint64

func uniqueSchema() string {
	return fmt.Sprintf("smyklot_suite_%d_%d", os.Getpid(), schemaCounter.Add(1))
}
