package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

// syncConfigColumns is the select list, spelled once so a scanner and a query
// cannot drift apart.
const syncConfigColumns = `
    target_id, kind, enabled, document, digest, revision, updated_by, updated_at`

// syncOverrideFrom is the same for a repository's answer, and it carries its
// join: the scope of an installation is the catalog's, so a query that reads
// one of these without going through repositories is one that could read
// another installation's row.
const syncOverrideFrom = `
SELECT o.repository_id, o.kind, o.enabled_override, o.document,
       o.revision, o.updated_by, o.updated_at
FROM sync_repository_overrides o
JOIN repositories r ON r.id = o.repository_id`

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
	return listSyncConfigs(ctx, s.db, targetID)
}

func scanSyncOverride(scanner rowScanner) (orgsync.RepositoryOverride, error) {
	var (
		override orgsync.RepositoryOverride
		enabled  sql.NullBool
		document string
		updated  StoredTime
	)

	if err := scanner.Scan(
		&override.RepositoryID, &override.Kind, &enabled, &document,
		&override.Revision, &override.UpdatedBy, &updated,
	); err != nil {
		return orgsync.RepositoryOverride{}, fmt.Errorf("scan sync override: %w", err)
	}

	if enabled.Valid {
		override.Enabled = &enabled.Bool
	}

	override.Document = []byte(document)
	override.UpdatedAt = updated.Time()

	return override, nil
}

// syncDocumentColumn is what a document column holds where a caller passed
// nothing, so the value stored and the column's own default agree.
func syncDocumentColumn(document []byte) string {
	if len(document) == 0 {
		return emptyDocument
	}

	return string(document)
}

// GetSyncRepositoryOverride reads one repository's answer about one kind.
//
// Scoped through the installation as well as the repository, so a caller
// holding an identifier from one installation cannot read a row belonging to
// another - the same join the listing goes through, for the same reason.
func (s *Store) GetSyncRepositoryOverride(
	ctx context.Context,
	targetID, repositoryID string,
	kind orgsync.Kind,
) (orgsync.RepositoryOverride, error) {
	override, err := scanSyncOverride(s.db.QueryRowContext(ctx, syncOverrideFrom+`
WHERE r.target_id = ? AND o.repository_id = ? AND o.kind = ?`,
		targetID, repositoryID, kind))
	if errors.Is(err, sql.ErrNoRows) {
		return orgsync.RepositoryOverride{}, storage.ErrNotFound
	}

	return override, err
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
	rows, err := s.db.QueryContext(ctx, syncOverrideFrom+`
WHERE r.target_id = ?
ORDER BY o.repository_id, o.kind`, targetID)
	if err != nil {
		return nil, fmt.Errorf("list sync repository overrides: %w", err)
	}

	return collectRows(rows, scanSyncOverride)
}

// syncStateColumns is what both state reads select, spelled once so neither can
// drift from scanSyncRepositoryState. Aliased, because both go through the
// repositories join that scopes them to an installation.
const syncStateColumns = `
    s.repository_id, s.kind, s.applied_digest, s.applied_at, s.problem`

// ListSyncRepositoryState reads what is known about each repository: what it
// has already had applied, or why nothing could be.
func (s *Store) ListSyncRepositoryState(
	ctx context.Context,
	targetID string,
) ([]orgsync.RepositoryState, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT`+syncStateColumns+`
FROM sync_repository_state s
JOIN repositories r ON r.id = s.repository_id
WHERE r.target_id = ?
ORDER BY s.repository_id, s.kind`, targetID)
	if err != nil {
		return nil, fmt.Errorf("list sync repository state: %w", err)
	}

	return collectRows(rows, scanSyncRepositoryState)
}

// GetSyncRepositoryState reads one repository's row for one kind.
//
// Carrying its installation, and joining on it, like every other read of these
// tables: the scope of an installation is the catalog's, so a query that reads
// one of these rows without going through repositories is one that could read
// another installation's. The caller here happens to resolve the repository
// against the target first - and a caller holding a repository id off a plan
// action, a stream event or the Root console would not.
func (s *Store) GetSyncRepositoryState(
	ctx context.Context,
	targetID, repositoryID string,
	kind orgsync.Kind,
) (orgsync.RepositoryState, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT`+syncStateColumns+`
FROM sync_repository_state s
JOIN repositories r ON r.id = s.repository_id
WHERE r.target_id = ? AND s.repository_id = ? AND s.kind = ?`,
		targetID, repositoryID, kind)

	state, err := scanSyncRepositoryState(row)
	if errors.Is(err, sql.ErrNoRows) {
		return orgsync.RepositoryState{}, storage.ErrNotFound
	}

	return state, err
}

func scanSyncRepositoryState(scanner rowScanner) (orgsync.RepositoryState, error) {
	var (
		state   orgsync.RepositoryState
		applied StoredTime
	)
	if err := scanner.Scan(
		&state.RepositoryID, &state.Kind, &state.AppliedDigest, &applied, &state.Problem,
	); err != nil {
		return orgsync.RepositoryState{}, fmt.Errorf("scan sync repository state: %w", err)
	}
	state.AppliedAt = applied.Time()

	return state, nil
}

// ListSyncRepositoryPaths reads every path an installation's repositories are
// known to hold, one row per repository.
//
// Through the repositories join like every other read of these tables: the
// scope of an installation is the catalog's, and a repository that moves cannot
// leave a path list behind describing it.
func (s *Store) ListSyncRepositoryPaths(
	ctx context.Context,
	targetID string,
) ([]orgsync.RepositoryPaths, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT p.repository_id, p.target_id, p.paths, p.observed_at, p.head_sha, p.partial
FROM sync_repository_paths p
JOIN repositories r ON r.id = p.repository_id
WHERE r.target_id = ?
ORDER BY p.repository_id`, targetID)
	if err != nil {
		return nil, fmt.Errorf("list sync repository paths: %w", err)
	}

	return collectRows(rows, scanSyncRepositoryPaths)
}

// ListSyncRepositoryPathScans reads when each list was taken and at which
// commit, without reading the lists.
//
// The same rows as above with the one large column left out. A refresh decides
// what to do with a row from these four fields alone, so selecting `paths` to
// answer them read - and decoded - every path in the installation on every
// tick, then discarded all of it.
func (s *Store) ListSyncRepositoryPathScans(
	ctx context.Context,
	targetID string,
) ([]orgsync.RepositoryPathScan, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT p.repository_id, p.observed_at, p.head_sha, p.partial
FROM sync_repository_paths p
JOIN repositories r ON r.id = p.repository_id
WHERE r.target_id = ?
ORDER BY p.repository_id`, targetID)
	if err != nil {
		return nil, fmt.Errorf("list sync repository path scans: %w", err)
	}

	return collectRows(rows, scanSyncRepositoryPathScan)
}

// TouchSyncRepositoryPaths records that a list was checked and had not changed.
//
// An UPDATE rather than the insert-or-replace above, because the list is not
// what is being written: a branch that has not moved still holds the paths that
// were read from it, and re-encoding fifty thousand of them to move one
// timestamp is the cost this avoids. A repository with no row yet matches
// nothing, which is right - there is no list to still be current.
func (s *Store) TouchSyncRepositoryPaths(
	ctx context.Context,
	repositoryID string,
	observedAt time.Time,
) error {
	if _, err := s.db.ExecContext(ctx, `
UPDATE sync_repository_paths SET observed_at = ? WHERE repository_id = ?`,
		observedAt, repositoryID,
	); err != nil {
		return fmt.Errorf("touch sync repository paths: %w", err)
	}

	return nil
}

// PruneSyncRepositoryPaths drops the lists of repositories an installation no
// longer synchronizes.
//
// The catalog decides. A repository that left the installation has no row in
// it at all, and one that is archived or whose access was withdrawn is there
// with `available` clear - the sweep skips both, so nothing was ever going to
// replace their lists, and the finder went on offering paths from repositories
// nobody could configure a file at.
//
// Scoped to the installation and not to a moment: a row for a repository under
// some other target is that target's business, and one written a second ago by
// a sweep still running is kept because its repository is in the catalog.
func (s *Store) PruneSyncRepositoryPaths(ctx context.Context, targetID string) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
DELETE FROM sync_repository_paths
WHERE target_id = ?
  AND repository_id NOT IN (
      SELECT id FROM repositories WHERE target_id = ? AND available = ?
  )`, targetID, targetID, true)
	if err != nil {
		return 0, fmt.Errorf("prune sync repository paths: %w", err)
	}

	dropped, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("prune sync repository paths: %w", err)
	}

	return dropped, nil
}

// SetSyncRepositoryPaths replaces one repository's list.
//
// Replaced whole rather than merged: this is a picture of what a repository
// held when it was last looked at, and a merge would remember paths that have
// since been deleted for ever.
func (s *Store) SetSyncRepositoryPaths(
	ctx context.Context,
	paths orgsync.RepositoryPaths,
) error {
	// JSON rather than one string with a newline between the paths, which is a
	// separator that can appear in the data: git permits a newline in a
	// filename, and one such file came back as two paths that do not exist.
	encoded, err := marshalPaths(paths.Paths)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
INSERT INTO sync_repository_paths (
    repository_id, target_id, paths, observed_at, head_sha, partial
)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (repository_id) DO UPDATE SET
    target_id = excluded.target_id,
    paths = excluded.paths,
    observed_at = excluded.observed_at,
    head_sha = excluded.head_sha,
    partial = excluded.partial`,
		paths.RepositoryID,
		paths.TargetID,
		encoded,
		paths.ObservedAt,
		paths.HeadSHA,
		paths.Partial,
	)
	if err != nil {
		return fmt.Errorf("set sync repository paths: %w", err)
	}

	return nil
}

func scanSyncRepositoryPathScan(scanner rowScanner) (orgsync.RepositoryPathScan, error) {
	var (
		scan     orgsync.RepositoryPathScan
		observed StoredTime
	)
	if err := scanner.Scan(
		&scan.RepositoryID, &observed, &scan.HeadSHA, &scan.Partial,
	); err != nil {
		return orgsync.RepositoryPathScan{}, fmt.Errorf("scan sync repository path scan: %w", err)
	}

	scan.ObservedAt = observed.Time()

	return scan, nil
}

func scanSyncRepositoryPaths(scanner rowScanner) (orgsync.RepositoryPaths, error) {
	var (
		paths    orgsync.RepositoryPaths
		encoded  string
		observed StoredTime
	)
	if err := scanner.Scan(
		&paths.RepositoryID, &paths.TargetID, &encoded, &observed,
		&paths.HeadSHA, &paths.Partial,
	); err != nil {
		return orgsync.RepositoryPaths{}, fmt.Errorf("scan sync repository paths: %w", err)
	}

	list, err := unmarshalPaths(encoded)
	if err != nil {
		return orgsync.RepositoryPaths{}, err
	}

	paths.Paths = list
	paths.ObservedAt = observed.Time()

	return paths, nil
}
