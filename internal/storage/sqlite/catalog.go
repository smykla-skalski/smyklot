package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

const targetSelect = `
SELECT
    t.id,
    t.installation_id,
    t.kind,
    t.available,
    t.repository_default_enabled,
    t.config_patch,
    t.revision,
    t.settings_updated_at,
    a.id,
    a.provider,
    a.subject_id,
    a.login,
    a.display_name,
    a.avatar_url,
    a.updated_at,
    COALESCE(SUM(CASE WHEN r.available = 1 THEN 1 ELSE 0 END), 0),
    COALESCE(SUM(CASE
        WHEN r.available = 1
         AND COALESCE(r.enabled_override, t.repository_default_enabled) = 1
        THEN 1 ELSE 0 END), 0)
FROM targets t
JOIN accounts a ON a.id = t.account_id
LEFT JOIN repositories r ON r.target_id = t.id`

// ReconcileInstallation replaces GitHub-owned catalog state while preserving
// every panel-owned control and revision.
func (s *Store) ReconcileInstallation(
	ctx context.Context,
	snapshot storage.InstallationSnapshot,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin installation reconcile: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	if err := reconcileInstallation(ctx, tx, snapshot); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit installation reconcile: %w", err)
	}

	return nil
}

// ReconcileCatalog replaces the complete available installation catalog in a
// single transaction. A target omitted by GitHub becomes unavailable without
// losing panel-owned settings if the App is installed again later.
func (s *Store) ReconcileCatalog(
	ctx context.Context,
	snapshots []storage.InstallationSnapshot,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin catalog reconcile: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, "UPDATE targets SET available = 0"); err != nil {
		return fmt.Errorf("mark installation targets unavailable: %w", err)
	}
	for _, snapshot := range snapshots {
		if err := reconcileInstallation(ctx, tx, snapshot); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit catalog reconcile: %w", err)
	}

	return nil
}

func reconcileInstallation(
	ctx context.Context,
	tx *sql.Tx,
	snapshot storage.InstallationSnapshot,
) error {
	if err := upsertCatalogAccount(ctx, tx, snapshot.Account); err != nil {
		return fmt.Errorf("reconcile installation account: %w", err)
	}
	if err := upsertTarget(ctx, tx, snapshot); err != nil {
		return err
	}
	if _, err := tx.ExecContext(
		ctx,
		"UPDATE repositories SET available = 0 WHERE target_id = ?",
		snapshot.TargetID,
	); err != nil {
		return fmt.Errorf("mark installation repositories unavailable: %w", err)
	}
	for _, repository := range snapshot.Repositories {
		if err := upsertRepository(ctx, tx, snapshot.TargetID, repository, snapshot.SyncedAt); err != nil {
			return err
		}
	}

	return nil
}

func upsertCatalogAccount(
	ctx context.Context,
	tx *sql.Tx,
	account storage.Account,
) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO accounts (id, provider, subject_id, login, display_name, avatar_url, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    provider = excluded.provider,
    subject_id = excluded.subject_id,
    login = excluded.login,
    display_name = CASE
        WHEN excluded.display_name = excluded.login THEN accounts.display_name
        ELSE excluded.display_name
    END,
    avatar_url = COALESCE(excluded.avatar_url, accounts.avatar_url),
    updated_at = excluded.updated_at`,
		account.ID,
		account.Provider,
		account.SubjectID,
		account.Login,
		account.DisplayName,
		account.AvatarURL,
		formatTime(account.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("upsert installation account: %w", err)
	}

	return nil
}

// GetTarget returns one installation regardless of viewer access.
func (s *Store) GetTarget(ctx context.Context, targetID string) (storage.Target, error) {
	target, err := getTarget(ctx, s.db, targetID)
	if err != nil {
		return storage.Target{}, fmt.Errorf("get target: %w", noRows(err))
	}

	return target, nil
}

// ListRepositories returns currently available repositories for a target.
func (s *Store) ListRepositories(
	ctx context.Context,
	targetID string,
) ([]storage.Repository, error) {
	rows, err := s.db.QueryContext(ctx, repositorySelect+`
WHERE target_id = ? AND available = 1
ORDER BY full_name`, targetID)
	if err != nil {
		return nil, fmt.Errorf("list repositories: %w", err)
	}

	repositories, err := collectRows(rows, scanRepository)
	if err != nil {
		return nil, fmt.Errorf("read repositories: %w", err)
	}

	return repositories, nil
}

// ListRepositoryPage returns one filtered page of currently available repositories.
func (s *Store) ListRepositoryPage(
	ctx context.Context,
	targetID string,
	page storage.RepositoryPageRequest,
) (storage.RepositoryPage, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.RepositoryPage{}, fmt.Errorf("begin repository page: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	limit := pageLimit(page.Limit)
	clauses, arguments, err := repositoryPageFilters(targetID, page)
	if err != nil {
		return storage.RepositoryPage{}, err
	}

	var total int
	countQuery := `
SELECT COUNT(*)
FROM repositories r
JOIN targets t ON t.id = r.target_id
WHERE ` + strings.Join(clauses, " AND ")
	if err := tx.QueryRowContext(ctx, countQuery, arguments...).Scan(&total); err != nil {
		return storage.RepositoryPage{}, fmt.Errorf("count repositories: %w", err)
	}
	var repositoryDefaultEnabled bool
	if err := tx.QueryRowContext(
		ctx,
		"SELECT repository_default_enabled FROM targets WHERE id = ?",
		targetID,
	).Scan(&repositoryDefaultEnabled); err != nil {
		return storage.RepositoryPage{}, fmt.Errorf("read repository default: %w", noRows(err))
	}

	order, err := repositoryPageOrder(page.Order)
	if err != nil {
		return storage.RepositoryPage{}, err
	}
	queryArguments := append(append([]any{}, arguments...), limit+1, max(page.Offset, 0))
	// #nosec G202 -- clauses and order come only from fixed internal constants;
	// every request value remains a bound parameter.
	query := repositoryPageSelect + " WHERE " + strings.Join(clauses, " AND ") +
		" ORDER BY " + order + " LIMIT ? OFFSET ?"
	rows, err := tx.QueryContext(ctx, query, queryArguments...)
	if err != nil {
		return storage.RepositoryPage{}, fmt.Errorf("list repository page: %w", err)
	}
	repositories, err := collectRows(rows, scanRepository)
	if err != nil {
		return storage.RepositoryPage{}, fmt.Errorf("read repository page: %w", err)
	}

	result := storage.RepositoryPage{
		Items:                    repositories,
		Total:                    total,
		RepositoryDefaultEnabled: repositoryDefaultEnabled,
	}
	if len(result.Items) > limit {
		result.Items = result.Items[:limit]
		result.NextOffset = max(page.Offset, 0) + limit
	}

	if err := tx.Commit(); err != nil {
		return storage.RepositoryPage{}, fmt.Errorf("commit repository page: %w", err)
	}

	return result, nil
}

func repositoryPageFilters(
	targetID string,
	page storage.RepositoryPageRequest,
) ([]string, []any, error) {
	clauses := []string{"r.target_id = ?", "r.available = 1"}
	arguments := []any{targetID}
	if page.Query != "" {
		clauses = append(clauses, "instr(lower(r.full_name), lower(?)) > 0")
		arguments = append(arguments, page.Query)
	}
	if page.EffectiveEnabled != nil {
		clauses = append(clauses, "COALESCE(r.enabled_override, t.repository_default_enabled) = ?")
		arguments = append(arguments, *page.EffectiveEnabled)
	}
	if len(page.FileStatuses) > 0 {
		fileClauses := make([]string, 0, len(page.FileStatuses))
		for _, status := range page.FileStatuses {
			switch status {
			case storage.RepositoryFileBypassed:
				fileClauses = append(fileClauses, "r.ignore_repository_file = 1")
			case storage.RepositoryFileMissing,
				storage.RepositoryFileValid,
				storage.RepositoryFileInvalid:
				fileClauses = append(
					fileClauses,
					"(r.ignore_repository_file = 0 AND r.config_file_status = ?)",
				)
				arguments = append(arguments, status)
			default:
				return nil, nil, fmt.Errorf("unsupported repository file status %q", status)
			}
		}
		clauses = append(clauses, "("+strings.Join(fileClauses, " OR ")+")")
	}
	if page.HasConfigOverrides != nil {
		expression := "EXISTS (SELECT 1 FROM json_each(r.config_patch))"
		if !*page.HasConfigOverrides {
			expression = "NOT " + expression
		}
		clauses = append(clauses, expression)
	}
	if len(page.ConfigOverrideKeys) > 0 {
		keyClauses := make([]string, 0, len(page.ConfigOverrideKeys))
		for _, key := range page.ConfigOverrideKeys {
			if !supportedConfigOverride(key) {
				return nil, nil, fmt.Errorf("unsupported repository config override %q", key)
			}
			keyClauses = append(keyClauses, "json_type(r.config_patch, ?) IS NOT NULL")
			arguments = append(arguments, "$."+key)
		}
		clauses = append(clauses, "("+strings.Join(keyClauses, " OR ")+")")
	}

	return clauses, arguments, nil
}

func supportedConfigOverride(key string) bool {
	switch key {
	case config.KeyQuietSuccess,
		config.KeyQuietReactions,
		config.KeyQuietPending,
		config.KeyAllowedCommands,
		config.KeyCommandAliases,
		config.KeyCommandPrefix,
		config.KeyDisableMentions,
		config.KeyDisableBareCommands,
		config.KeyDisableUnapprove,
		config.KeyDisableReactions,
		config.KeyDisableDeletedComments,
		config.KeyAllowSelfApproval:
		return true
	default:
		return false
	}
}

func repositoryPageOrder(order storage.RepositoryOrder) (string, error) {
	switch order {
	case "", storage.RepositoryNameAscending:
		return "r.full_name COLLATE NOCASE ASC, r.id ASC", nil
	case storage.RepositoryNameDescending:
		return "r.full_name COLLATE NOCASE DESC, r.id DESC", nil
	case storage.RepositoryFileAscending:
		return `(CASE WHEN r.ignore_repository_file = 1
            THEN 'bypassed' ELSE r.config_file_status END) COLLATE NOCASE ASC,
            r.full_name COLLATE NOCASE ASC, r.id ASC`, nil
	case storage.RepositoryFileDescending:
		return `(CASE WHEN r.ignore_repository_file = 1
            THEN 'bypassed' ELSE r.config_file_status END) COLLATE NOCASE DESC,
            r.full_name COLLATE NOCASE ASC, r.id ASC`, nil
	case storage.RepositoryOverridesAscending:
		return `(SELECT COUNT(*) FROM json_each(r.config_patch)) ASC,
            r.full_name COLLATE NOCASE ASC, r.id ASC`, nil
	case storage.RepositoryOverridesDescending:
		return `(SELECT COUNT(*) FROM json_each(r.config_patch)) DESC,
            r.full_name COLLATE NOCASE ASC, r.id ASC`, nil
	case storage.RepositoryNewest:
		return "r.settings_updated_at DESC, r.id DESC", nil
	case storage.RepositoryOldest:
		return "r.settings_updated_at ASC, r.id ASC", nil
	default:
		return "", fmt.Errorf("unsupported repository order %q", order)
	}
}

// GetRepository returns one repository belonging to the given target.
func (s *Store) GetRepository(
	ctx context.Context,
	targetID, repositoryID string,
) (storage.Repository, error) {
	repository, err := getRepository(ctx, s.db, targetID, repositoryID)
	if err != nil {
		return storage.Repository{}, fmt.Errorf("get repository: %w", noRows(err))
	}

	return repository, nil
}

func upsertTarget(
	ctx context.Context,
	tx *sql.Tx,
	snapshot storage.InstallationSnapshot,
) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO targets (
    id, installation_id, kind, account_id, available,
    repository_default_enabled, config_patch, revision,
    settings_updated_at, synced_at
)
VALUES (?, ?, ?, ?, 1, 0, '{}', 1, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    installation_id = excluded.installation_id,
    kind = excluded.kind,
    account_id = excluded.account_id,
    available = 1,
    synced_at = excluded.synced_at`,
		snapshot.TargetID,
		snapshot.InstallationID,
		snapshot.Kind,
		snapshot.Account.ID,
		formatTime(snapshot.SyncedAt),
		formatTime(snapshot.SyncedAt),
	)
	if err != nil {
		return fmt.Errorf("upsert installation target: %w", err)
	}

	return nil
}

func upsertRepository(
	ctx context.Context,
	tx *sql.Tx,
	targetID string,
	repository storage.RepositorySnapshot,
	syncedAt time.Time,
) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO repositories (
    id, target_id, name, full_name, private, default_branch, available,
    enabled_override, config_patch, ignore_repository_file,
    config_file_status, config_file_patch, revision,
    settings_updated_at, synced_at
)
VALUES (?, ?, ?, ?, ?, ?, 1, NULL, '{}', 0, 'missing', '{}', 1, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    target_id = excluded.target_id,
    name = excluded.name,
    full_name = excluded.full_name,
    private = excluded.private,
    default_branch = excluded.default_branch,
    available = 1,
    synced_at = excluded.synced_at`,
		repository.ID,
		targetID,
		repository.Name,
		repository.FullName,
		repository.Private,
		repository.DefaultBranch,
		formatTime(syncedAt),
		formatTime(syncedAt),
	)
	if err != nil {
		return fmt.Errorf("upsert installation repository: %w", err)
	}

	return nil
}

func getTarget(
	ctx context.Context,
	queryer rowQuerier,
	targetID string,
) (storage.Target, error) {
	return scanTarget(queryer.QueryRowContext(ctx, targetSelect+`
WHERE t.id = ?
GROUP BY t.id, a.id`, targetID))
}

func scanTarget(scanner rowScanner) (storage.Target, error) {
	var target storage.Target
	var avatarURL sql.NullString
	var targetPatch, targetUpdatedAt, accountUpdatedAt string
	var enabled int

	err := scanner.Scan(
		&target.ID,
		&target.InstallationID,
		&target.Kind,
		&target.Available,
		&target.RepositoryDefaultEnabled,
		&targetPatch,
		&target.Revision,
		&targetUpdatedAt,
		&target.Account.ID,
		&target.Account.Provider,
		&target.Account.SubjectID,
		&target.Account.Login,
		&target.Account.DisplayName,
		&avatarURL,
		&accountUpdatedAt,
		&target.RepositoryCounts.Total,
		&enabled,
	)
	if err != nil {
		return storage.Target{}, err
	}

	target.Account.AvatarURL = stringPointer(avatarURL)
	target.RepositoryCounts.Enabled = enabled
	target.RepositoryCounts.Disabled = target.RepositoryCounts.Total - enabled

	return finishTarget(target, targetPatch, targetUpdatedAt, accountUpdatedAt)
}

func finishTarget(
	target storage.Target,
	patch, targetUpdatedAt, accountUpdatedAt string,
) (storage.Target, error) {
	var err error
	target.ConfigPatch, err = unmarshalPatch(patch)
	if err != nil {
		return storage.Target{}, err
	}

	target.UpdatedAt, err = parseTime(targetUpdatedAt)
	if err != nil {
		return storage.Target{}, err
	}

	target.Account.UpdatedAt, err = parseTime(accountUpdatedAt)

	return target, err
}

const repositoryColumns = `
SELECT
    r.id,
    r.target_id,
    r.name,
    r.full_name,
    r.private,
    r.default_branch,
    r.available,
    r.enabled_override,
    r.config_patch,
    r.ignore_repository_file,
    r.config_file_status,
    r.config_file_patch,
    r.config_file_error,
    r.revision,
    r.settings_updated_at
`

const repositorySelect = repositoryColumns + "FROM repositories r\n"

const repositoryPageSelect = repositoryColumns + `
FROM repositories r
JOIN targets t ON t.id = r.target_id
`

func getRepository(
	ctx context.Context,
	queryer rowQuerier,
	targetID, repositoryID string,
) (storage.Repository, error) {
	return scanRepository(queryer.QueryRowContext(ctx, repositorySelect+`
WHERE r.target_id = ? AND r.id = ?`, targetID, repositoryID))
}

func scanRepository(scanner rowScanner) (storage.Repository, error) {
	var repository storage.Repository
	var enabledOverride sql.NullBool
	var fileError sql.NullString
	var panelPatch, filePatch, updatedAt string

	err := scanner.Scan(
		&repository.ID,
		&repository.TargetID,
		&repository.Name,
		&repository.FullName,
		&repository.Private,
		&repository.DefaultBranch,
		&repository.Available,
		&enabledOverride,
		&panelPatch,
		&repository.IgnoreRepositoryFile,
		&repository.ConfigFileStatus,
		&filePatch,
		&fileError,
		&repository.Revision,
		&updatedAt,
	)
	if err != nil {
		return storage.Repository{}, err
	}

	repository.EnabledOverride = boolPointer(enabledOverride)
	repository.ConfigFileError = stringPointer(fileError)
	if repository.IgnoreRepositoryFile {
		repository.ConfigFileStatus = storage.RepositoryFileBypassed
	}

	return finishRepository(repository, panelPatch, filePatch, updatedAt)
}

func finishRepository(
	repository storage.Repository,
	panelPatch, filePatch, updatedAt string,
) (storage.Repository, error) {
	var err error
	repository.ConfigPatch, err = unmarshalPatch(panelPatch)
	if err != nil {
		return storage.Repository{}, err
	}

	repository.ConfigFilePatch, err = unmarshalPatch(filePatch)
	if err != nil {
		return storage.Repository{}, err
	}

	repository.UpdatedAt, err = parseTime(updatedAt)

	return repository, err
}
