package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

const systemAuditAccountID = "smyklot:system"

type ownershipState struct {
	exists   bool
	source   storage.OwnershipSource
	status   storage.OwnershipStatus
	detail   string
	ownerIDs []string
}

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
    t.permissions,
    a.id,
    a.provider,
    a.subject_id,
    a.login,
    a.display_name,
    a.avatar_url,
    a.updated_at,
    COALESCE(SUM(CASE WHEN r.available = TRUE THEN 1 ELSE 0 END), 0),
    COALESCE(SUM(CASE
        WHEN r.available = TRUE
         AND COALESCE(r.enabled_override, t.repository_default_enabled) = TRUE
        THEN 1 ELSE 0 END), 0),
    COALESCE(o.source, CASE WHEN t.kind = 'User' THEN 'personal' ELSE 'organization_admin' END),
    COALESCE(o.status, 'error'),
    CASE WHEN o.target_id IS NULL THEN 'ownership has not been synchronized' ELSE o.detail END,
    COALESCE(o.synced_at, t.synced_at),
    (SELECT COUNT(*) FROM target_owners owners WHERE owners.target_id = t.id),
    (SELECT COUNT(*) FROM deliveries delivery
        WHERE delivery.target_id = t.id AND delivery.status = 'failed'),
    (SELECT MAX(delivery.finished_at) FROM deliveries delivery
        WHERE delivery.target_id = t.id AND delivery.status = 'failed')
FROM targets t
JOIN accounts a ON a.id = t.account_id
LEFT JOIN target_ownership o ON o.target_id = t.id
LEFT JOIN repositories r ON r.target_id = t.id`

// targetGroup closes targetSelect's per-installation aggregate.
//
// Every joined table is grouped by its own primary key, which is what lets a
// strict engine accept the plain columns the select reads from targets,
// accounts and target_ownership alongside the repository counts.
const targetGroup = `
GROUP BY t.id, a.id, o.target_id`

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

	if _, err := tx.ExecContext(ctx, "UPDATE targets SET available = FALSE"); err != nil {
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

// ListRootTargets returns the complete retained installation catalog. Root
// diagnostics include unavailable installations and unhealthy ownership.
func (s *Store) ListRootTargets(ctx context.Context) ([]storage.Target, error) {
	rows, err := s.db.QueryContext(ctx, targetSelect+targetGroup+`
ORDER BY lower(a.login), t.id`)
	if err != nil {
		return nil, fmt.Errorf("list Root installation targets: %w", err)
	}
	targets, err := collectRows(rows, scanTarget)
	if err != nil {
		return nil, fmt.Errorf("read Root installation targets: %w", err)
	}

	return targets, nil
}

func reconcileInstallation(
	ctx context.Context,
	tx runner,
	snapshot storage.InstallationSnapshot,
) error {
	if err := upsertCatalogAccount(ctx, tx, snapshot.Account); err != nil {
		return fmt.Errorf("reconcile installation account: %w", err)
	}
	if err := upsertTarget(ctx, tx, snapshot); err != nil {
		return err
	}
	if err := reconcileOwnership(ctx, tx, snapshot); err != nil {
		return err
	}
	if _, err := tx.ExecContext(
		ctx,
		"UPDATE repositories SET available = FALSE WHERE target_id = ?",
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

func reconcileOwnership(
	ctx context.Context,
	tx runner,
	snapshot storage.InstallationSnapshot,
) error {
	ownership := normalizedOwnership(snapshot)
	previous, err := readOwnershipState(ctx, tx, snapshot.TargetID)
	if err != nil {
		return err
	}
	for _, owner := range ownership.Owners {
		if err := upsertCatalogAccount(ctx, tx, owner); err != nil {
			return fmt.Errorf("reconcile installation owner: %w", err)
		}
	}
	if err := replaceOwnership(ctx, tx, snapshot.TargetID, ownership); err != nil {
		return err
	}
	if ownershipChanged(previous, ownership) {
		if err := recordOwnershipAudit(ctx, tx, snapshot.TargetID, ownership); err != nil {
			return err
		}
	}

	return nil
}

func normalizedOwnership(snapshot storage.InstallationSnapshot) storage.OwnershipSnapshot {
	if snapshot.Ownership.Source != "" {
		return snapshot.Ownership
	}
	if snapshot.Kind == storage.TargetUser {
		return storage.OwnershipSnapshot{
			Source: storage.OwnershipSourcePersonal, Status: storage.OwnershipStatusFresh,
			Owners: []storage.Account{snapshot.Account}, SyncedAt: snapshot.SyncedAt,
		}
	}
	detail := "ownership has not been synchronized"

	return storage.OwnershipSnapshot{
		Source: storage.OwnershipSourceOrganizationAdmin,
		Status: storage.OwnershipStatusError, Detail: &detail, SyncedAt: snapshot.SyncedAt,
	}
}

func readOwnershipState(
	ctx context.Context,
	tx runner,
	targetID string,
) (ownershipState, error) {
	var state ownershipState
	var detail sql.NullString
	err := tx.QueryRowContext(ctx, `
SELECT source, status, detail FROM target_ownership WHERE target_id = ?`, targetID).
		Scan(&state.source, &state.status, &detail)
	if errors.Is(err, sql.ErrNoRows) {
		return state, nil
	}
	if err != nil {
		return ownershipState{}, fmt.Errorf("read previous installation ownership: %w", err)
	}
	state.exists = true
	state.detail = detail.String
	rows, err := tx.QueryContext(ctx, `
SELECT account_id FROM target_owners WHERE target_id = ? ORDER BY account_id`, targetID)
	if err != nil {
		return ownershipState{}, fmt.Errorf("read previous installation Owners: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var ownerID string
		if err := rows.Scan(&ownerID); err != nil {
			return ownershipState{}, fmt.Errorf("scan previous installation Owner: %w", err)
		}
		state.ownerIDs = append(state.ownerIDs, ownerID)
	}
	if err := rows.Err(); err != nil {
		return ownershipState{}, fmt.Errorf("iterate previous installation Owners: %w", err)
	}

	return state, nil
}

func replaceOwnership(
	ctx context.Context,
	tx runner,
	targetID string,
	ownership storage.OwnershipSnapshot,
) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO target_ownership (target_id, source, status, detail, synced_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(target_id) DO UPDATE SET
    source = excluded.source,
    status = excluded.status,
		detail = excluded.detail,
		synced_at = excluded.synced_at`,
		targetID,
		ownership.Source,
		ownership.Status,
		ownership.Detail,
		ownership.SyncedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert installation ownership: %w", err)
	}
	if _, err := tx.ExecContext(
		ctx, "DELETE FROM target_owners WHERE target_id = ?", targetID,
	); err != nil {
		return fmt.Errorf("replace installation owners: %w", err)
	}
	for _, owner := range ownership.Owners {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO target_owners (target_id, account_id, synced_at) VALUES (?, ?, ?)`,
			targetID, owner.ID, ownership.SyncedAt,
		); err != nil {
			return fmt.Errorf("insert installation owner: %w", err)
		}
	}

	return nil
}

func ownershipChanged(previous ownershipState, current storage.OwnershipSnapshot) bool {
	if !previous.exists || previous.source != current.Source || previous.status != current.Status {
		return true
	}
	currentDetail := ""
	if current.Detail != nil {
		currentDetail = *current.Detail
	}
	ownerIDs := make([]string, 0, len(current.Owners))
	for _, owner := range current.Owners {
		ownerIDs = append(ownerIDs, owner.ID)
	}
	slices.Sort(ownerIDs)

	return previous.detail != currentDetail || !slices.Equal(previous.ownerIDs, ownerIDs)
}

func recordOwnershipAudit(
	ctx context.Context,
	tx runner,
	targetID string,
	ownership storage.OwnershipSnapshot,
) error {
	system := storage.Account{
		ID: systemAuditAccountID, Provider: "smyklot", SubjectID: "system",
		Login: "smyklot", DisplayName: "Smyklot", UpdatedAt: ownership.SyncedAt,
	}
	if err := upsertCatalogAccount(ctx, tx, system); err != nil {
		return fmt.Errorf("reconcile ownership audit identity: %w", err)
	}
	action := "ownership.synced"
	summary := fmt.Sprintf("Synchronized %d installation Owners", len(ownership.Owners))
	switch ownership.Status {
	case storage.OwnershipStatusPermissionPending:
		action = "ownership.permission_pending"
		summary = "Owner synchronization awaits GitHub permission approval"
	case storage.OwnershipStatusError:
		action = "ownership.failed"
		summary = "Owner synchronization failed"
	}
	if _, err := insertAppAudit(ctx, tx, appAuditInsert{
		Category: string(storage.AuditCategoryOwnership), TargetID: &targetID,
		ActorAccountID: system.ID, Action: action, Summary: summary,
		CreatedAt: ownership.SyncedAt,
	}); err != nil {
		return fmt.Errorf("record ownership synchronization: %w", err)
	}

	return nil
}

func upsertCatalogAccount(
	ctx context.Context,
	tx runner,
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
		account.UpdatedAt,
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
WHERE target_id = ? AND available = TRUE
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
	clauses, arguments, err := s.repositoryPageFilters(targetID, page)
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

	order, err := s.repositoryPageOrder(page.Order)
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

func (s *Store) repositoryPageFilters(
	targetID string,
	page storage.RepositoryPageRequest,
) ([]string, []any, error) {
	clauses := []string{"r.target_id = ?", "r.available = TRUE"}
	arguments := []any{targetID}
	if page.Query != "" {
		clauses = append(clauses, containsClause("r.full_name"))
		arguments = append(arguments, containsArgument(page.Query))
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
				fileClauses = append(fileClauses, "r.ignore_repository_file = TRUE")
			case storage.RepositoryFileMissing,
				storage.RepositoryFileValid,
				storage.RepositoryFileInvalid:
				fileClauses = append(
					fileClauses,
					"(r.ignore_repository_file = FALSE AND r.config_file_status = ?)",
				)
				arguments = append(arguments, status)
			default:
				return nil, nil, fmt.Errorf("unsupported repository file status %q", status)
			}
		}
		clauses = append(clauses, "("+strings.Join(fileClauses, " OR ")+")")
	}
	if page.HasConfigOverrides != nil {
		comparison := " > 0"
		if !*page.HasConfigOverrides {
			comparison = " = 0"
		}
		clauses = append(clauses, s.dialect.JSONKeyCount("r.config_patch")+comparison)
	}
	if len(page.ConfigOverrideKeys) > 0 {
		keyClauses := make([]string, 0, len(page.ConfigOverrideKeys))
		for _, key := range page.ConfigOverrideKeys {
			if !supportedConfigOverride(key) {
				return nil, nil, fmt.Errorf("unsupported repository config override %q", key)
			}
			keyClauses = append(keyClauses, s.dialect.JSONHasKey("r.config_patch"))
			arguments = append(arguments, key)
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

func (s *Store) repositoryPageOrder(order storage.RepositoryOrder) (string, error) {
	byName := caseFold("r.full_name")
	nameAscending := byName + " ASC, r.id ASC"
	byFileStatus := caseFold(`(CASE WHEN r.ignore_repository_file = TRUE
            THEN 'bypassed' ELSE r.config_file_status END)`)
	byOverrides := s.dialect.JSONKeyCount("r.config_patch")

	switch order {
	case "", storage.RepositoryNameAscending:
		return nameAscending, nil
	case storage.RepositoryNameDescending:
		return byName + " DESC, r.id DESC", nil
	case storage.RepositoryFileAscending:
		return byFileStatus + " ASC, " + nameAscending, nil
	case storage.RepositoryFileDescending:
		return byFileStatus + " DESC, " + nameAscending, nil
	case storage.RepositoryOverridesAscending:
		return byOverrides + " ASC, " + nameAscending, nil
	case storage.RepositoryOverridesDescending:
		return byOverrides + " DESC, " + nameAscending, nil
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
	tx runner,
	snapshot storage.InstallationSnapshot,
) error {
	permissions, err := marshalPermissions(snapshot.Permissions)
	if err != nil {
		return err
	}

	// permissions is refreshed on every reconcile, because it is GitHub's
	// answer rather than anything Smyklot decides. An operator who grants a
	// permission expects the next sweep to notice, and one who revokes it
	// expects the same.
	_, err = tx.ExecContext(ctx, `
INSERT INTO targets (
    id, installation_id, kind, account_id, available,
    repository_default_enabled, config_patch, revision,
    settings_updated_at, synced_at, permissions
)
VALUES (?, ?, ?, ?, TRUE, FALSE, '{}', 1, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    installation_id = excluded.installation_id,
    kind = excluded.kind,
    account_id = excluded.account_id,
    available = TRUE,
    synced_at = excluded.synced_at,
    permissions = excluded.permissions`,
		snapshot.TargetID,
		snapshot.InstallationID,
		snapshot.Kind,
		snapshot.Account.ID,
		snapshot.SyncedAt,
		snapshot.SyncedAt,
		permissions,
	)
	if err != nil {
		return fmt.Errorf("upsert installation target: %w", err)
	}

	return nil
}

func upsertRepository(
	ctx context.Context,
	tx runner,
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
VALUES (?, ?, ?, ?, ?, ?, TRUE, NULL, '{}', FALSE, 'missing', '{}', 1, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    target_id = excluded.target_id,
    name = excluded.name,
    full_name = excluded.full_name,
    private = excluded.private,
    default_branch = excluded.default_branch,
    available = TRUE,
    synced_at = excluded.synced_at`,
		repository.ID,
		targetID,
		repository.Name,
		repository.FullName,
		repository.Private,
		repository.DefaultBranch,
		syncedAt,
		syncedAt,
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
WHERE t.id = ?`+targetGroup, targetID))
}

func scanTarget(scanner rowScanner) (storage.Target, error) {
	var target storage.Target
	var avatarURL, ownershipDetail sql.NullString
	var lastFailureAt, targetUpdatedAt, accountUpdatedAt, ownershipSyncedAt StoredTime
	var targetPatch, targetPermissions string
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
		&targetPermissions,
		&target.Account.ID,
		&target.Account.Provider,
		&target.Account.SubjectID,
		&target.Account.Login,
		&target.Account.DisplayName,
		&avatarURL,
		&accountUpdatedAt,
		&target.RepositoryCounts.Total,
		&enabled,
		&target.Ownership.Source,
		&target.Ownership.Status,
		&ownershipDetail,
		&ownershipSyncedAt,
		&target.Ownership.OwnerCount,
		&target.DeliveryHealth.Failed,
		&lastFailureAt,
	)
	if err != nil {
		return storage.Target{}, err
	}

	target.Account.AvatarURL = stringPointer(avatarURL)
	target.RepositoryCounts.Enabled = enabled
	target.RepositoryCounts.Disabled = target.RepositoryCounts.Total - enabled
	target.Ownership.Detail = stringPointer(ownershipDetail)
	target.DeliveryHealth.LastFailureAt = lastFailureAt.Pointer()
	target.Ownership.SyncedAt = ownershipSyncedAt.Time()
	target.Permissions = unmarshalPermissions(targetPermissions)

	return finishTarget(target, targetPatch, targetUpdatedAt, accountUpdatedAt)
}

func finishTarget(
	target storage.Target,
	patch string,
	targetUpdatedAt, accountUpdatedAt StoredTime,
) (storage.Target, error) {
	var err error
	target.ConfigPatch, err = unmarshalPatch(patch)
	if err != nil {
		return storage.Target{}, err
	}

	target.UpdatedAt = targetUpdatedAt.Time()
	target.Account.UpdatedAt = accountUpdatedAt.Time()

	return target, nil
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
    r.config_file_path,
    r.config_file_superseded,
    r.config_migration,
    r.config_migration_pr,
    r.revision,
    r.settings_updated_at
`

const repositorySelect = repositoryColumns + "FROM repositories r\n"

const repositoryPageSelect = repositoryColumns + `
FROM repositories r
JOIN targets t ON t.id = r.target_id
`

/*
getRepository finds one repository of an installation by id or by name.

Both, because the panel's addresses name repositories the way people do. The id
is what the rest of the panel carries, but a link somebody sends a colleague says
`/repositories/api-gateway`, and a name is unique within an installation, so it
identifies exactly as well.

The id is tried first. A repository may legitimately be named "123", and if some
other repository carries 123 as its id, the id is the reading that was meant.
*/
func getRepository(
	ctx context.Context,
	queryer rowQuerier,
	targetID, repository string,
) (storage.Repository, error) {
	return scanRepository(queryer.QueryRowContext(ctx, repositorySelect+`
WHERE r.target_id = ? AND (r.id = ? OR r.name = ?)
ORDER BY CASE WHEN r.id = ? THEN 0 ELSE 1 END
LIMIT 1`, targetID, repository, repository, repository))
}

func scanRepository(scanner rowScanner) (storage.Repository, error) {
	var repository storage.Repository
	var enabledOverride sql.NullBool
	var fileError sql.NullString
	var panelPatch, filePatch, superseded string
	var migrationPR sql.NullInt64
	var updatedAt StoredTime

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
		&repository.ConfigFilePath,
		&superseded,
		&repository.ConfigMigration,
		&migrationPR,
		&repository.Revision,
		&updatedAt,
	)
	if err != nil {
		return storage.Repository{}, err
	}

	repository.EnabledOverride = boolPointer(enabledOverride)
	repository.ConfigFileError = stringPointer(fileError)
	repository.ConfigMigrationPR = intPointer(migrationPR)
	if repository.IgnoreRepositoryFile {
		repository.ConfigFileStatus = storage.RepositoryFileBypassed
	}

	return finishRepository(repository, panelPatch, filePatch, superseded, updatedAt)
}

func finishRepository(
	repository storage.Repository,
	panelPatch, filePatch, superseded string,
	updatedAt StoredTime,
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

	repository.ConfigFileSuperseded, err = unmarshalPaths(superseded)
	if err != nil {
		return storage.Repository{}, err
	}

	repository.UpdatedAt = updatedAt.Time()

	return repository, nil
}
