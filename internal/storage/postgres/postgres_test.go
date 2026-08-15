package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/storage"
	storagepostgres "github.com/smykla-skalski/smyklot/internal/storage/postgres"
	"github.com/smykla-skalski/smyklot/internal/storage/storagetest"
)

// dsnVariable names the server the suite runs against. There is no default:
// a suite that quietly invents a connection would either touch a database
// nobody meant it to, or pass by not running.
const dsnVariable = "SMYKLOT_TEST_POSTGRES_DSN"

func TestPostgres(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "PostgreSQL Storage Suite")
}

var _ = Describe("PostgreSQL store [Unit]", func() {
	// Each spec gets its own schema on the shared server, so specs cannot see
	// one another's rows and the suite needs no database of its own per spec.
	var schema string

	BeforeEach(func() {
		if strings.TrimSpace(os.Getenv(dsnVariable)) == "" {
			Skip(dsnVariable + " is not set, so there is no server to run against")
		}
	})

	storagetest.DeclareSpecs(storagetest.Harness{
		Open: func(ctx context.Context) storage.Store {
			schema = uniqueSchema()
			admin := connect(ctx)
			defer func() { Expect(admin.Close()).To(Succeed()) }()

			_, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func(ctx context.Context) {
				cleanup := connect(ctx)
				defer func() { Expect(cleanup.Close()).To(Succeed()) }()
				_, dropErr := cleanup.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE")
				Expect(dropErr).NotTo(HaveOccurred())
			})

			store, err := storagepostgres.Open(ctx, dsnForSchema(schema))
			Expect(err).NotTo(HaveOccurred())

			return store
		},
		RejectSecurityNotifications: func(ctx context.Context) {
			raw := connect(ctx)
			defer func() { Expect(raw.Close()).To(Succeed()) }()

			_, err := raw.ExecContext(ctx, fmt.Sprintf(`
CREATE FUNCTION %[1]s.reject_security_notification() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'notification write rejected';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER reject_security_notification
BEFORE INSERT ON %[1]s.security_notifications
FOR EACH ROW EXECUTE FUNCTION %[1]s.reject_security_notification();`, schema))
			Expect(err).NotTo(HaveOccurred())
		},
	})
})

// uniqueSchema names a schema no other spec is using.
func uniqueSchema() string {
	return fmt.Sprintf("smyklot_test_%d", GinkgoRandomSeed()+int64(GinkgoParallelProcess())+nextSchema())
}

var schemaCounter int64

func nextSchema() int64 {
	schemaCounter++

	return schemaCounter
}

// connect opens a handle on the configured server with no schema of its own.
func connect(ctx context.Context) *sql.DB {
	db, err := sql.Open("pgx", os.Getenv(dsnVariable))
	Expect(err).NotTo(HaveOccurred())
	Expect(db.PingContext(ctx)).To(Succeed())

	return db
}

// dsnForSchema points a connection at one schema, so the adapter creates its
// tables there and finds only its own.
func dsnForSchema(schema string) string {
	dsn := os.Getenv(dsnVariable)
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}

	return dsn + separator + "search_path=" + schema
}
