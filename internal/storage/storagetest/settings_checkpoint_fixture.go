package storagetest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/storage/sqlstore"
)

type settingsCheckpointFixtureStore interface {
	DB() *sql.DB
	Dialect() sqlstore.Dialect
}

// RewriteSettingsCheckpointDocumentVersion corrupts one immutable item only
// inside a SQL test store. Compatibility tests need a historical document
// written by another version, but production storage must expose no raw
// checkpoint writer capable of creating one.
func RewriteSettingsCheckpointDocumentVersion(
	ctx context.Context,
	store storage.Store,
	checkpointID int64,
	identity storage.SettingsCheckpointItemIdentity,
	documentVersion int,
) error {
	raw, ok := store.(settingsCheckpointFixtureStore)
	if !ok {
		return errors.New("settings checkpoint fixture rewrite requires a SQL test store")
	}
	result, err := raw.DB().ExecContext(ctx, raw.Dialect().Rebind(`
UPDATE settings_checkpoint_items
SET document_version = ?
WHERE checkpoint_id = ? AND item_kind = ? AND repository_id = ? AND sync_kind = ?`),
		documentVersion,
		checkpointID,
		identity.Kind,
		identity.RepositoryID,
		identity.SyncKind,
	)
	if err != nil {
		return fmt.Errorf("rewrite settings checkpoint item: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count rewritten settings checkpoint items: %w", err)
	}
	if updated != 1 {
		return fmt.Errorf("rewrote %d settings checkpoint items, want 1", updated)
	}

	return nil
}
