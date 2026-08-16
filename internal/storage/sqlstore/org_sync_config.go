package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

// syncConfigColumns is the select list, spelled once so a scanner and a query
// cannot drift apart.
const syncConfigColumns = `
    target_id, kind, enabled, document, digest, revision, updated_by, updated_at`

func scanSyncConfig(scanner rowScanner) (orgsync.Config, error) {
	var (
		config   orgsync.Config
		document []byte
		updated  StoredTime
	)

	if err := scanner.Scan(
		&config.TargetID, &config.Kind, &config.Enabled, &document,
		&config.Digest, &config.Revision, &config.UpdatedBy, &updated,
	); err != nil {
		return orgsync.Config{}, fmt.Errorf("scan sync config: %w", err)
	}

	config.Document = document
	config.UpdatedAt = updated.Time()

	return config, nil
}

// GetSyncConfig reads one kind's configuration.
func (s *Store) GetSyncConfig(
	ctx context.Context,
	targetID string,
	kind orgsync.Kind,
) (orgsync.Config, error) {
	config, err := scanSyncConfig(s.db.QueryRowContext(ctx, `
SELECT`+syncConfigColumns+`
FROM sync_configs
WHERE target_id = ? AND kind = ?`, targetID, kind))
	if errors.Is(err, sql.ErrNoRows) {
		return orgsync.Config{}, storage.ErrNotFound
	}

	return config, err
}

// ListSyncConfigs reads every kind an installation has configured.
//
// Only the kinds somebody has saved. A kind with no row has never been
// configured, which is not the same as a kind configured and switched off, and
// the fingerprint has to tell those apart.
func (s *Store) ListSyncConfigs(
	ctx context.Context,
	targetID string,
) ([]orgsync.Config, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT`+syncConfigColumns+`
FROM sync_configs
WHERE target_id = ?
ORDER BY kind`, targetID)
	if err != nil {
		return nil, fmt.Errorf("list sync configs: %w", err)
	}

	return collectRows(rows, scanSyncConfig)
}

// SetSyncConfig writes a configuration and invalidates every live plan computed
// from the old one, in the same transaction.
//
// Atomically, because the two must not be separable. A plan is approved against
// the fingerprint a browser rendered; if the write landed and the invalidation
// did not, that plan stays approvable for as long as the gap lasts and applies
// work nobody agreed to.
func (s *Store) SetSyncConfig(
	ctx context.Context,
	change orgsync.ConfigChange,
) (orgsync.Config, error) {
	if !change.Kind.Valid() {
		return orgsync.Config{}, fmt.Errorf("%w: unknown kind %q", storage.ErrNotFound, change.Kind)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return orgsync.Config{}, fmt.Errorf("begin sync config write: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	digest := orgsync.DigestConfig(change.Enabled, change.Document)

	revision, err := s.writeSyncConfig(ctx, tx, change, digest)
	if err != nil {
		return orgsync.Config{}, err
	}

	if err := invalidateLivePlans(ctx, tx, change.TargetID, change.Now); err != nil {
		return orgsync.Config{}, err
	}

	if err := tx.Commit(); err != nil {
		return orgsync.Config{}, fmt.Errorf("commit sync config write: %w", err)
	}

	return orgsync.Config{
		TargetID: change.TargetID,
		Kind:     change.Kind,
		Enabled:  change.Enabled,
		Document: change.Document,
		Digest:   digest,
		Revision: revision,

		UpdatedBy: change.ActorID,
		UpdatedAt: change.Now,
	}, nil
}

// writeSyncConfig inserts or updates the row, and reports the revision it left.
//
// The revision is checked rather than trusted. Two people editing the same
// label set from two tabs is the ordinary case, and the one who saved second
// should be told rather than silently winning.
func (s *Store) writeSyncConfig(
	ctx context.Context,
	tx *transaction,
	change orgsync.ConfigChange,
	digest string,
) (int64, error) {
	var current int64
	err := tx.QueryRowContext(ctx, `
SELECT revision FROM sync_configs WHERE target_id = ? AND kind = ?`+s.dialect.RowLock(),
		change.TargetID, change.Kind,
	).Scan(&current)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		// A first write. Zero says the writer knew there was nothing here; any
		// other number says it believed it was changing something that is gone.
		if change.Revision != 0 {
			return 0, storage.ErrConflict
		}

		if _, err := tx.ExecContext(ctx, `
INSERT INTO sync_configs (
    target_id, kind, enabled, document, digest, revision, updated_by, updated_at
) VALUES (?, ?, ?, ?, ?, 1, ?, ?)`,
			change.TargetID, change.Kind, change.Enabled, change.Document,
			digest, change.ActorID, change.Now,
		); err != nil {
			return 0, fmt.Errorf("insert sync config: %w", err)
		}

		return 1, nil

	case err != nil:
		return 0, fmt.Errorf("read sync config revision: %w", err)

	case change.Revision != current:
		return 0, storage.ErrConflict
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE sync_configs SET
    enabled = ?, document = ?, digest = ?, revision = revision + 1,
    updated_by = ?, updated_at = ?
WHERE target_id = ? AND kind = ?`,
		change.Enabled, change.Document, digest, change.ActorID,
		change.Now, change.TargetID, change.Kind,
	); err != nil {
		return 0, fmt.Errorf("update sync config: %w", err)
	}

	return current + 1, nil
}

func scanSyncOverride(scanner rowScanner) (orgsync.RepositoryOverride, error) {
	var (
		override orgsync.RepositoryOverride
		enabled  sql.NullBool
		updated  StoredTime
	)

	if err := scanner.Scan(
		&override.RepositoryID, &override.Kind, &enabled,
		&override.Revision, &override.UpdatedBy, &updated,
	); err != nil {
		return orgsync.RepositoryOverride{}, fmt.Errorf("scan sync override: %w", err)
	}

	if enabled.Valid {
		override.Enabled = &enabled.Bool
	}
	override.UpdatedAt = updated.Time()

	return override, nil
}

// ListSyncRepositoryOverrides reads every repository answer in an installation.
//
// Joined through repositories rather than filtered on a target column of its
// own, so the scope of an installation is stated once - in the catalog - and a
// repository that moves cannot leave an override behind describing it.
func (s *Store) ListSyncRepositoryOverrides(
	ctx context.Context,
	targetID string,
) ([]orgsync.RepositoryOverride, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT o.repository_id, o.kind, o.enabled_override, o.revision, o.updated_by, o.updated_at
FROM sync_repository_overrides o
JOIN repositories r ON r.id = o.repository_id
WHERE r.target_id = ?
ORDER BY o.repository_id, o.kind`, targetID)
	if err != nil {
		return nil, fmt.Errorf("list sync repository overrides: %w", err)
	}

	return collectRows(rows, scanSyncOverride)
}

// SetSyncRepositoryOverride writes one repository's answer, and invalidates the
// installation's live plans for the same reason SetSyncConfig does: turning a
// kind off for one repository removes its actions from a plan somebody may be
// reading.
func (s *Store) SetSyncRepositoryOverride(
	ctx context.Context,
	change orgsync.RepositoryOverrideChange,
) (orgsync.RepositoryOverride, error) {
	if !change.Kind.Valid() {
		return orgsync.RepositoryOverride{},
			fmt.Errorf("%w: unknown kind %q", storage.ErrNotFound, change.Kind)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return orgsync.RepositoryOverride{}, fmt.Errorf("begin sync override write: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var targetID string
	if err := tx.QueryRowContext(ctx,
		`SELECT target_id FROM repositories WHERE id = ?`, change.RepositoryID,
	).Scan(&targetID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return orgsync.RepositoryOverride{}, storage.ErrNotFound
		}

		return orgsync.RepositoryOverride{}, fmt.Errorf("read override repository: %w", err)
	}

	revision, err := s.writeSyncOverride(ctx, tx, change)
	if err != nil {
		return orgsync.RepositoryOverride{}, err
	}

	if err := invalidateLivePlans(ctx, tx, targetID, change.Now); err != nil {
		return orgsync.RepositoryOverride{}, err
	}

	if err := tx.Commit(); err != nil {
		return orgsync.RepositoryOverride{}, fmt.Errorf("commit sync override write: %w", err)
	}

	return orgsync.RepositoryOverride{
		RepositoryID: change.RepositoryID,
		Kind:         change.Kind,
		Enabled:      change.Enabled,
		Revision:     revision,
		UpdatedBy:    change.ActorID,
		UpdatedAt:    change.Now,
	}, nil
}

func (s *Store) writeSyncOverride(
	ctx context.Context,
	tx *transaction,
	change orgsync.RepositoryOverrideChange,
) (int64, error) {
	var current int64
	err := tx.QueryRowContext(ctx, `
SELECT revision FROM sync_repository_overrides
WHERE repository_id = ? AND kind = ?`+s.dialect.RowLock(),
		change.RepositoryID, change.Kind,
	).Scan(&current)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		if change.Revision != 0 {
			return 0, storage.ErrConflict
		}

		if _, err := tx.ExecContext(ctx, `
INSERT INTO sync_repository_overrides (
    repository_id, kind, enabled_override, revision, updated_by, updated_at
) VALUES (?, ?, ?, 1, ?, ?)`,
			change.RepositoryID, change.Kind, change.Enabled,
			change.ActorID, change.Now,
		); err != nil {
			return 0, fmt.Errorf("insert sync override: %w", err)
		}

		return 1, nil

	case err != nil:
		return 0, fmt.Errorf("read sync override revision: %w", err)

	case change.Revision != current:
		return 0, storage.ErrConflict
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE sync_repository_overrides SET
    enabled_override = ?, revision = revision + 1, updated_by = ?, updated_at = ?
WHERE repository_id = ? AND kind = ?`,
		change.Enabled, change.ActorID, change.Now,
		change.RepositoryID, change.Kind,
	); err != nil {
		return 0, fmt.Errorf("update sync override: %w", err)
	}

	return current + 1, nil
}

// ListSyncRepositoryState reads what each repository has already had applied.
func (s *Store) ListSyncRepositoryState(
	ctx context.Context,
	targetID string,
) ([]orgsync.RepositoryState, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT s.repository_id, s.kind, s.applied_digest, s.applied_at
FROM sync_repository_state s
JOIN repositories r ON r.id = s.repository_id
WHERE r.target_id = ?
ORDER BY s.repository_id, s.kind`, targetID)
	if err != nil {
		return nil, fmt.Errorf("list sync repository state: %w", err)
	}

	return collectRows(rows, func(scanner rowScanner) (orgsync.RepositoryState, error) {
		var (
			state   orgsync.RepositoryState
			applied StoredTime
		)
		if err := scanner.Scan(
			&state.RepositoryID, &state.Kind, &state.AppliedDigest, &applied,
		); err != nil {
			return orgsync.RepositoryState{}, fmt.Errorf("scan sync repository state: %w", err)
		}
		state.AppliedAt = applied.Time()

		return state, nil
	})
}
