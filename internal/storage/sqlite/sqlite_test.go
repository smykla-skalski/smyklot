package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/storage"
	storagesqlite "github.com/smykla-skalski/smyklot/internal/storage/sqlite"
	"github.com/smykla-skalski/smyklot/internal/storage/storagetest"
)

func TestSQLite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SQLite Storage Suite")
}

var _ = Describe("SQLite store [Unit]", func() {
	// path is the database the current spec opened. The notification hook
	// reaches the same file through a second connection.
	var path string

	storagetest.DeclareSpecs(storagetest.Harness{
		Open: func(ctx context.Context) storage.Store {
			path = filepath.Join(GinkgoT().TempDir(), "panel.db")
			store, err := storagesqlite.Open(ctx, path)
			Expect(err).NotTo(HaveOccurred())

			return store
		},
		RejectSecurityNotifications: func(_ context.Context) {
			raw, err := sql.Open("sqlite", path)
			Expect(err).NotTo(HaveOccurred())
			_, err = raw.Exec(`
CREATE TRIGGER reject_security_notification
BEFORE INSERT ON security_notifications
BEGIN
    SELECT RAISE(ABORT, 'notification write rejected');
END`)
			Expect(err).NotTo(HaveOccurred())
			Expect(raw.Close()).To(Succeed())
		},
	})
})
