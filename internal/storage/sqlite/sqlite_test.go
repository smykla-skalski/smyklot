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
		RejectSettingsCheckpoints: func(ctx context.Context) {
			raw, err := sql.Open("sqlite", path)
			Expect(err).NotTo(HaveOccurred())
			_, err = raw.ExecContext(ctx, `
CREATE TRIGGER reject_settings_checkpoint
BEFORE INSERT ON settings_checkpoints
BEGIN
    SELECT RAISE(ABORT, 'settings checkpoint write rejected');
END`)
			Expect(err).NotTo(HaveOccurred())
			Expect(raw.Close()).To(Succeed())
		},
		CountSettingsCheckpoints: func(ctx context.Context) int64 {
			raw, err := sql.Open("sqlite", path)
			Expect(err).NotTo(HaveOccurred())
			defer func() { Expect(raw.Close()).To(Succeed()) }()
			var count int64
			Expect(raw.QueryRowContext(
				ctx,
				"SELECT COUNT(*) FROM settings_checkpoints",
			).Scan(&count)).To(Succeed())

			return count
		},
		RewriteSettingsCheckpointItem: func(
			ctx context.Context,
			rewrite storagetest.SettingsCheckpointItemRewrite,
		) {
			raw, err := sql.Open("sqlite", path)
			Expect(err).NotTo(HaveOccurred())
			defer func() { Expect(raw.Close()).To(Succeed()) }()
			result, err := raw.ExecContext(ctx, `
UPDATE settings_checkpoint_items
SET document_version = ?, after_document = ?, after_digest = ?
WHERE checkpoint_id = ? AND item_kind = ? AND repository_id = ? AND sync_kind = ?`,
				rewrite.DocumentVersion,
				string(rewrite.AfterDocument),
				storage.DigestSettingsCheckpointDocument(rewrite.AfterDocument),
				rewrite.CheckpointID,
				rewrite.Identity.Kind,
				rewrite.Identity.RepositoryID,
				rewrite.Identity.SyncKind,
			)
			Expect(err).NotTo(HaveOccurred())
			updated, err := result.RowsAffected()
			Expect(err).NotTo(HaveOccurred())
			Expect(updated).To(Equal(int64(1)))
		},
	})
})
