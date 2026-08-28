package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

const actionInstallationSettingsSaved = "installation.settings.saved"

type preparedInstallationSettings struct {
	request       storage.SaveInstallationSettingsRequest
	target        *preparedTargetSettings
	repositories  []preparedRepositorySettings
	syncConfigs   []preparedSyncConfigSettings
	syncOverrides []preparedSyncOverrideSettings
	locksPolicy   bool
}

type preparedTargetSettings struct {
	change         storage.TargetSettingsChange
	patch          string
	branchPatterns string
}

type preparedRepositorySettings struct {
	change         storage.RepositorySettingsChange
	patch          string
	branchPatterns any
}

type installationSettingsWork struct {
	target            *targetSettingsWork
	repositories      []repositorySettingsWork
	syncConfigs       []syncConfigSettingsWork
	syncOverrides     []syncOverrideSettingsWork
	items             []storage.SettingsCheckpointItem
	snapshotBefore    []storage.SettingsCheckpointItem
	inclusionChanged  bool
	syncChanged       bool
	formattingChanged bool
}

type targetSettingsWork struct {
	prepared preparedTargetSettings
	current  storage.Target
	changed  bool
}

type repositorySettingsWork struct {
	prepared preparedRepositorySettings
	current  storage.Repository
	changed  bool
}

// SaveInstallationSettings writes all changed installation settings, one
// checkpoint, and one audit event in the same transaction.
func (s *Store) SaveInstallationSettings(
	ctx context.Context,
	request storage.SaveInstallationSettingsRequest,
) (storage.SaveInstallationSettingsResult, error) {
	prepared, err := prepareInstallationSettings(request)
	if err != nil {
		return storage.SaveInstallationSettingsResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.SaveInstallationSettingsResult{}, fmt.Errorf(
			"begin installation settings save: %w", err,
		)
	}
	defer func() { _ = tx.Rollback() }()

	if prepared.locksPolicy {
		if err := lockPendingCIPolicy(ctx, tx, s.dialect); err != nil {
			return storage.SaveInstallationSettingsResult{}, err
		}
	}
	if err := s.lockInstallationSettingsTarget(ctx, tx, request.TargetID); err != nil {
		return storage.SaveInstallationSettingsResult{}, err
	}
	work, err := loadInstallationSettingsWork(ctx, tx, prepared)
	if err != nil {
		return storage.SaveInstallationSettingsResult{}, err
	}
	work, err = s.loadInstallationSyncSettingsWork(ctx, tx, prepared, work)
	if err != nil {
		return storage.SaveInstallationSettingsResult{}, err
	}
	if len(work.items) == 0 {
		return installationSettingsResult(work), nil
	}
	work.snapshotBefore, err = captureInstallationSettingsSnapshot(
		ctx, tx, request.TargetID,
	)
	if err != nil {
		return storage.SaveInstallationSettingsResult{}, err
	}

	elevation, err := s.elevatedWrite(
		ctx, tx, request.ElevationID, request.SessionTokenHash,
		request.ActorAccountID, request.TargetID, request.ChangedAt,
	)
	if err != nil {
		return storage.SaveInstallationSettingsResult{}, err
	}
	if err := s.applyInstallationSettings(ctx, tx, request, work); err != nil {
		return storage.SaveInstallationSettingsResult{}, err
	}
	checkpointID, auditEventID, err := s.recordInstallationSettings(
		ctx, tx, request, work,
	)
	if err != nil {
		return storage.SaveInstallationSettingsResult{}, err
	}
	if elevation != nil {
		if err := insertElevatedNotifications(
			ctx, tx, *elevation, auditEventID, actionInstallationSettingsSaved,
			request.ChangedAt,
		); err != nil {
			return storage.SaveInstallationSettingsResult{}, err
		}
	}

	result, err := s.readInstallationSettingsResult(ctx, tx, prepared)
	if err != nil {
		return storage.SaveInstallationSettingsResult{}, err
	}
	appendInstallationSettingsChanges(&result, work)
	result.CheckpointID = &checkpointID
	if err := tx.Commit(); err != nil {
		return storage.SaveInstallationSettingsResult{}, fmt.Errorf(
			"commit installation settings save: %w", err,
		)
	}

	return result, nil
}

func prepareInstallationSettings(
	request storage.SaveInstallationSettingsRequest,
) (preparedInstallationSettings, error) {
	if strings.TrimSpace(request.TargetID) == "" ||
		strings.TrimSpace(request.ActorAccountID) == "" || request.ChangedAt.IsZero() {
		return preparedInstallationSettings{}, errors.New(
			"installation settings target, actor, and change time are required",
		)
	}
	if request.Target == nil && len(request.Repositories) == 0 &&
		len(request.SyncConfigs) == 0 && len(request.SyncOverrides) == 0 {
		return preparedInstallationSettings{}, errors.New(
			"installation settings save needs at least one resource",
		)
	}

	prepared := preparedInstallationSettings{request: request}
	if request.Target != nil {
		target, err := prepareInstallationTargetSettings(request, *request.Target)
		if err != nil {
			return preparedInstallationSettings{}, err
		}
		prepared.target = &target
		prepared.locksPolicy = request.Target.RetunePendingCIQuietPeriod
	}
	for _, repository := range request.Repositories {
		item, err := prepareInstallationRepositorySettings(request, repository)
		if err != nil {
			return preparedInstallationSettings{}, err
		}
		prepared.repositories = append(prepared.repositories, item)
		prepared.locksPolicy = prepared.locksPolicy || repository.RetunePendingCIQuietPeriod
	}
	sort.Slice(prepared.repositories, func(left, right int) bool {
		return prepared.repositories[left].change.RepositoryID <
			prepared.repositories[right].change.RepositoryID
	})
	for index := 1; index < len(prepared.repositories); index++ {
		if prepared.repositories[index-1].change.RepositoryID ==
			prepared.repositories[index].change.RepositoryID {
			return preparedInstallationSettings{}, fmt.Errorf(
				"duplicate repository settings change %q",
				prepared.repositories[index].change.RepositoryID,
			)
		}
	}
	if err := prepareInstallationSyncSettings(&prepared); err != nil {
		return preparedInstallationSettings{}, err
	}

	return prepared, nil
}

func prepareInstallationTargetSettings(
	request storage.SaveInstallationSettingsRequest,
	settings storage.InstallationTargetSettingsChange,
) (preparedTargetSettings, error) {
	if settings.ExpectedRevision < 0 {
		return preparedTargetSettings{}, errors.New("target settings revision cannot be negative")
	}
	change, patch, patterns, err := prepareTargetSettings(storage.TargetSettingsChange{
		TargetID: request.TargetID, ActorAccountID: request.ActorAccountID,
		ElevationID: request.ElevationID, SessionTokenHash: request.SessionTokenHash,
		RepositoryDefaultEnabled:       settings.RepositoryDefaultEnabled,
		PendingCIModeDefault:           settings.PendingCIModeDefault,
		PendingCIBranchPatternsDefault: settings.PendingCIBranchPatternsDefault,
		PendingCIQuietPeriodOverride:   settings.PendingCIQuietPeriodOverride,
		PathIndexIntervalOverride:      settings.PathIndexIntervalOverride,
		ConfigPatch:                    settings.ConfigPatch, ExpectedRevision: settings.ExpectedRevision,
		RetunePendingCIQuietPeriod:     settings.RetunePendingCIQuietPeriod,
		DeploymentPendingCIQuietPeriod: settings.DeploymentPendingCIQuietPeriod,
		ChangedAt:                      request.ChangedAt,
	})
	if err != nil {
		return preparedTargetSettings{}, err
	}

	return preparedTargetSettings{change: change, patch: patch, branchPatterns: patterns}, nil
}

func prepareInstallationRepositorySettings(
	request storage.SaveInstallationSettingsRequest,
	settings storage.InstallationRepositorySettingsChange,
) (preparedRepositorySettings, error) {
	if strings.TrimSpace(settings.RepositoryID) == "" || settings.ExpectedRevision < 0 {
		return preparedRepositorySettings{}, errors.New(
			"repository settings identity and revision are required",
		)
	}
	change := storage.RepositorySettingsChange{
		TargetID: request.TargetID, RepositoryID: settings.RepositoryID,
		ActorAccountID: request.ActorAccountID, ElevationID: request.ElevationID,
		SessionTokenHash: request.SessionTokenHash, EnabledOverride: settings.EnabledOverride,
		PendingCIModeOverride:           settings.PendingCIModeOverride,
		PendingCIBranchPatternsOverride: settings.PendingCIBranchPatternsOverride,
		PendingCIQuietPeriodOverride:    settings.PendingCIQuietPeriodOverride,
		PathIndexIntervalOverride:       settings.PathIndexIntervalOverride,
		ConfigPatch:                     settings.ConfigPatch, IgnoreRepositoryFile: settings.IgnoreRepositoryFile,
		ExpectedRevision:               settings.ExpectedRevision,
		RetunePendingCIQuietPeriod:     settings.RetunePendingCIQuietPeriod,
		DeploymentPendingCIQuietPeriod: settings.DeploymentPendingCIQuietPeriod,
		ChangedAt:                      request.ChangedAt,
	}
	if err := storage.ValidateRepositoryPendingCISettings(
		change.PendingCIModeOverride, change.PendingCIBranchPatternsOverride,
		change.PendingCIQuietPeriodOverride,
	); err != nil {
		return preparedRepositorySettings{}, err
	}
	patch, err := marshalPatch(change.ConfigPatch)
	if err != nil {
		return preparedRepositorySettings{}, err
	}
	var patterns any
	if change.PendingCIBranchPatternsOverride != nil {
		patterns, err = marshalPendingCIBranchPatterns(*change.PendingCIBranchPatternsOverride)
		if err != nil {
			return preparedRepositorySettings{}, err
		}
	}

	return preparedRepositorySettings{change: change, patch: patch, branchPatterns: patterns}, nil
}

func (s *Store) lockInstallationSettingsTarget(
	ctx context.Context,
	tx *transaction,
	targetID string,
) error {
	var held string
	err := tx.QueryRowContext(ctx,
		"SELECT id FROM targets WHERE id = ?"+s.dialect.RowLock(), targetID,
	).Scan(&held)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock installation settings target: %w", err)
	}

	return nil
}

func loadInstallationSettingsWork(
	ctx context.Context,
	tx *transaction,
	prepared preparedInstallationSettings,
) (installationSettingsWork, error) {
	work := installationSettingsWork{}
	if prepared.target != nil {
		current, err := getTarget(ctx, tx, prepared.request.TargetID)
		if err != nil {
			return work, fmt.Errorf("read target settings: %w", noRows(err))
		}
		work.target = &targetSettingsWork{prepared: *prepared.target, current: current}
	}
	for _, repository := range prepared.repositories {
		current, err := getRepository(
			ctx, tx, prepared.request.TargetID, repository.change.RepositoryID,
		)
		if err != nil {
			return work, fmt.Errorf("read repository settings: %w", noRows(err))
		}
		if current.ID != repository.change.RepositoryID {
			return work, storage.ErrNotFound
		}
		work.repositories = append(work.repositories, repositorySettingsWork{
			prepared: repository, current: current,
		})
	}
	if err := validateInstallationSettingsRevisions(work); err != nil {
		return installationSettingsWork{}, err
	}

	return buildInstallationSettingsItems(work)
}

func validateInstallationSettingsRevisions(work installationSettingsWork) error {
	if work.target != nil &&
		work.target.current.Revision != work.target.prepared.change.ExpectedRevision {
		return storage.ErrConflict
	}
	for _, repository := range work.repositories {
		if repository.current.Revision != repository.prepared.change.ExpectedRevision {
			return storage.ErrConflict
		}
	}

	return nil
}

func buildInstallationSettingsItems(
	work installationSettingsWork,
) (installationSettingsWork, error) {
	if work.target != nil {
		item, changed, err := targetSettingsCheckpointItem(*work.target)
		if err != nil {
			return installationSettingsWork{}, err
		}
		work.target.changed = changed
		if changed {
			work.items = append(work.items, item)
			work.formattingChanged = !reflect.DeepEqual(
				work.target.current.ConfigPatch.Formatting,
				work.target.prepared.change.ConfigPatch.Formatting,
			)
			work.inclusionChanged = work.target.current.RepositoryDefaultEnabled !=
				work.target.prepared.change.RepositoryDefaultEnabled
		}
	}
	for index := range work.repositories {
		item, changed, err := repositorySettingsCheckpointItem(work.repositories[index])
		if err != nil {
			return installationSettingsWork{}, err
		}
		work.repositories[index].changed = changed
		if changed {
			work.items = append(work.items, item)
			work.formattingChanged = work.formattingChanged || !reflect.DeepEqual(
				work.repositories[index].current.ConfigPatch.Formatting,
				work.repositories[index].prepared.change.ConfigPatch.Formatting,
			)
			work.inclusionChanged = work.inclusionChanged || !sameOptionalBool(
				work.repositories[index].current.EnabledOverride,
				work.repositories[index].prepared.change.EnabledOverride,
			)
		}
	}

	return work, nil
}

func targetSettingsCheckpointItem(
	work targetSettingsWork,
) (storage.SettingsCheckpointItem, bool, error) {
	before, err := targetSettingsState(targetSettingsDocument(work.current), work.current.Revision)
	if err != nil {
		return storage.SettingsCheckpointItem{}, false, err
	}
	afterDocument := storage.TargetSettingsDocument{
		RepositoryDefaultEnabled:       work.prepared.change.RepositoryDefaultEnabled,
		PendingCIModeDefault:           work.prepared.change.PendingCIModeDefault,
		PendingCIBranchPatternsDefault: work.prepared.change.PendingCIBranchPatternsDefault,
		PendingCIQuietPeriodOverride:   work.prepared.change.PendingCIQuietPeriodOverride,
		PathIndexIntervalOverride:      work.prepared.change.PathIndexIntervalOverride,
		ConfigPatch:                    work.prepared.change.ConfigPatch,
	}
	after, err := targetSettingsState(afterDocument, work.current.Revision+1)
	if err != nil {
		return storage.SettingsCheckpointItem{}, false, err
	}
	item := storage.SettingsCheckpointItem{
		Kind:            storage.SettingsCheckpointItemTarget,
		DocumentVersion: storage.SettingsCheckpointDocumentVersion,
		Before:          before, After: after,
	}

	return item, before.Digest != after.Digest, nil
}

func repositorySettingsCheckpointItem(
	work repositorySettingsWork,
) (storage.SettingsCheckpointItem, bool, error) {
	before, err := repositorySettingsState(
		repositorySettingsDocument(work.current), work.current.Revision,
	)
	if err != nil {
		return storage.SettingsCheckpointItem{}, false, err
	}
	afterDocument := storage.RepositorySettingsDocument{
		EnabledOverride:                 work.prepared.change.EnabledOverride,
		PendingCIModeOverride:           work.prepared.change.PendingCIModeOverride,
		PendingCIBranchPatternsOverride: work.prepared.change.PendingCIBranchPatternsOverride,
		PendingCIQuietPeriodOverride:    work.prepared.change.PendingCIQuietPeriodOverride,
		PathIndexIntervalOverride:       work.prepared.change.PathIndexIntervalOverride,
		ConfigPatch:                     work.prepared.change.ConfigPatch,
		IgnoreRepositoryFile:            work.prepared.change.IgnoreRepositoryFile,
	}
	after, err := repositorySettingsState(afterDocument, work.current.Revision+1)
	if err != nil {
		return storage.SettingsCheckpointItem{}, false, err
	}
	item := storage.SettingsCheckpointItem{
		Kind:         storage.SettingsCheckpointItemRepository,
		RepositoryID: work.current.ID, RepositoryFullName: work.current.FullName,
		DocumentVersion: storage.SettingsCheckpointDocumentVersion,
		Before:          before, After: after,
	}

	return item, before.Digest != after.Digest, nil
}

func targetSettingsState(
	document storage.TargetSettingsDocument,
	revision int64,
) (*storage.SettingsCheckpointState, error) {
	return installationSettingsState(document, revision)
}

func repositorySettingsState(
	document storage.RepositorySettingsDocument,
	revision int64,
) (*storage.SettingsCheckpointState, error) {
	return installationSettingsState(document, revision)
}

func installationSettingsState(document any, revision int64) (*storage.SettingsCheckpointState, error) {
	content, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("encode settings checkpoint document: %w", err)
	}
	state := storage.NewSettingsCheckpointState(content, revision)

	return &state, nil
}

func targetSettingsDocument(target storage.Target) storage.TargetSettingsDocument {
	return storage.TargetSettingsDocument{
		RepositoryDefaultEnabled:       target.RepositoryDefaultEnabled,
		PendingCIModeDefault:           target.PendingCIModeDefault,
		PendingCIBranchPatternsDefault: target.PendingCIBranchPatternsDefault,
		PendingCIQuietPeriodOverride:   target.PendingCIQuietPeriodOverride,
		PathIndexIntervalOverride:      target.PathIndexIntervalOverride,
		ConfigPatch:                    target.ConfigPatch,
	}
}

func repositorySettingsDocument(repository storage.Repository) storage.RepositorySettingsDocument {
	return storage.RepositorySettingsDocument{
		EnabledOverride:                 repository.EnabledOverride,
		PendingCIModeOverride:           repository.PendingCIModeOverride,
		PendingCIBranchPatternsOverride: repository.PendingCIBranchPatternsOverride,
		PendingCIQuietPeriodOverride:    repository.PendingCIQuietPeriodOverride,
		PathIndexIntervalOverride:       repository.PathIndexIntervalOverride,
		ConfigPatch:                     repository.ConfigPatch,
		IgnoreRepositoryFile:            repository.IgnoreRepositoryFile,
	}
}

func sameOptionalBool(left, right *bool) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}

	return *left == *right
}
