package panel

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

const (
	maxWorkspaceSettingsBatchResources = 4096
	workspaceSettingsBatchTooLargeCode = "request_too_large"
)

type workspaceSettingsBatchRequest struct {
	Target        *workspaceTargetBatchRequest        `json:"target"`
	Repositories  []workspaceRepositoryBatchRequest   `json:"repositories"`
	SyncConfigs   []workspaceSyncConfigBatchRequest   `json:"sync_configs"`
	SyncOverrides []workspaceSyncOverrideBatchRequest `json:"sync_overrides"`
}

type workspaceTargetBatchRequest struct {
	RepositoryDefaultEnabled       *bool                            `json:"repository_default_enabled"`
	PendingCIModeDefault           *storage.PendingCIMode           `json:"pending_ci_mode_default"`
	PendingCIBranchPatternsDefault *storage.PendingCIBranchPatterns `json:"pending_ci_branch_patterns_default"`
	PendingCIQuietPeriodSeconds    nullableValue[int64]             `json:"pending_ci_quiet_period_seconds_override"`
	PathIndexIntervalSeconds       nullableValue[int64]             `json:"path_index_interval_seconds_override"`
	ConfigPatch                    *config.Patch                    `json:"config_patch"`
	ExpectedRevision               *int64                           `json:"expected_revision"`
}

type workspaceRepositoryBatchRequest struct {
	RepositoryID                    string                                         `json:"repository_id"`
	EnabledOverride                 nullableBool                                   `json:"enabled_override"`
	PendingCIModeOverride           batchNullable[storage.PendingCIMode]           `json:"pending_ci_mode_override"`
	PendingCIBranchPatternsOverride batchNullable[storage.PendingCIBranchPatterns] `json:"pending_ci_branch_patterns_override"`
	PendingCIQuietPeriodSeconds     nullableValue[int64]                           `json:"pending_ci_quiet_period_seconds_override"`
	PathIndexIntervalSeconds        nullableValue[int64]                           `json:"path_index_interval_seconds_override"`
	ConfigPatch                     *config.Patch                                  `json:"config_patch"`
	IgnoreRepositoryFile            *bool                                          `json:"ignore_repository_file"`
	ExpectedRevision                *int64                                         `json:"expected_revision"`
}

type workspaceSyncConfigBatchRequest struct {
	Kind             string                         `json:"kind"`
	Enabled          *bool                          `json:"enabled"`
	Labels           batchRequired[[]orgsync.Label] `json:"labels"`
	AllowRemoval     *bool                          `json:"allow_removal"`
	Excludes         batchRequired[[]string]        `json:"excludes"`
	Document         batchDocument                  `json:"document"`
	ExpectedRevision *int64                         `json:"expected_revision"`
}

type workspaceSyncOverrideBatchRequest struct {
	RepositoryID     string        `json:"repository_id"`
	Kind             string        `json:"kind"`
	Enabled          nullableBool  `json:"enabled"`
	Document         batchDocument `json:"document"`
	ExpectedRevision *int64        `json:"expected_revision"`
}

type batchRequired[T any] struct {
	Value   T
	Present bool
}

func (value *batchRequired[T]) UnmarshalJSON(data []byte) error {
	value.Present = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return errors.New("value cannot be null")
	}

	return decodeStrictJSONValue(data, &value.Value)
}

type batchNullable[T any] struct {
	Value   *T
	Present bool
}

func (value *batchNullable[T]) UnmarshalJSON(data []byte) error {
	value.Present = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		value.Value = nil
		return nil
	}
	var decoded T
	if err := decodeStrictJSONValue(data, &decoded); err != nil {
		return err
	}
	value.Value = &decoded

	return nil
}

type batchDocument struct {
	Value   json.RawMessage
	Present bool
	Null    bool
}

func (document *batchDocument) UnmarshalJSON(data []byte) error {
	document.Present = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		document.Null = true
		document.Value = nil
		return nil
	}
	document.Value = append(document.Value[:0], data...)

	return nil
}

func decodeStrictJSONValue(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("value must contain one JSON document")
	}

	return nil
}

type workspaceSettingsBatchInputError struct {
	status  int
	code    string
	message string
}

func (err *workspaceSettingsBatchInputError) Error() string { return err.message }

func invalidWorkspaceSettingsBatch(code, message string) error {
	return &workspaceSettingsBatchInputError{
		status: http.StatusBadRequest, code: code, message: message,
	}
}

func (s *Server) writeWorkspaceSettingsBatchPreparationError(
	w http.ResponseWriter,
	err error,
	writeStorageError func(http.ResponseWriter, error),
) {
	var inputError *workspaceSettingsBatchInputError
	if errors.As(err, &inputError) {
		s.writeError(w, inputError.status, inputError.code, inputError.message)
		return
	}
	writeStorageError(w, err)
}

func (s *Server) prepareWorkspaceSettingsBatch(
	r *http.Request,
	target storage.Target,
	actor workspaceSettingsBatchActor,
	input workspaceSettingsBatchRequest,
) (storage.SaveInstallationSettingsRequest, error) {
	if err := validateWorkspaceSettingsBatchShape(input); err != nil {
		return storage.SaveInstallationSettingsRequest{}, err
	}
	request := storage.SaveInstallationSettingsRequest{
		TargetID: target.ID, ActorAccountID: actor.accountID,
		ElevationID: actor.elevationID, SessionTokenHash: actor.sessionTokenHash,
		ChangedAt: s.now().UTC(),
	}
	if input.Target != nil {
		change, err := s.workspaceTargetBatchChange(target, *input.Target)
		if err != nil {
			return storage.SaveInstallationSettingsRequest{}, err
		}
		request.Target = &change
	}
	configs, proposedFiles, err := workspaceSyncConfigBatchChanges(input.SyncConfigs)
	if err != nil {
		return storage.SaveInstallationSettingsRequest{}, err
	}
	request.SyncConfigs = configs
	repositories, err := s.workspaceBatchRepositories(r, target.ID, input)
	if err != nil {
		return storage.SaveInstallationSettingsRequest{}, err
	}
	request.Repositories, err = s.workspaceRepositoryBatchChanges(input.Repositories, repositories)
	if err != nil {
		return storage.SaveInstallationSettingsRequest{}, err
	}
	request.SyncOverrides, err = s.workspaceSyncOverrideBatchChanges(
		r, target.ID, input.SyncOverrides, repositories, proposedFiles,
	)
	if err != nil {
		return storage.SaveInstallationSettingsRequest{}, err
	}

	return request, nil
}

func validateWorkspaceSettingsBatchShape(input workspaceSettingsBatchRequest) error {
	resources := len(input.Repositories) + len(input.SyncConfigs) + len(input.SyncOverrides)
	if input.Target != nil {
		resources++
	}
	if resources == 0 {
		return invalidWorkspaceSettingsBatch("invalid_request", "a settings save needs at least one resource")
	}
	if resources > maxWorkspaceSettingsBatchResources {
		return invalidWorkspaceSettingsBatch("invalid_request", "a settings save contains too many resources")
	}
	if input.Target != nil {
		if err := validateWorkspaceTargetBatchRequest(*input.Target); err != nil {
			return err
		}
	}
	seenRepositories := make(map[string]bool, len(input.Repositories))
	for _, repository := range input.Repositories {
		if err := validateWorkspaceRepositoryBatchRequest(repository); err != nil {
			return err
		}
		if seenRepositories[repository.RepositoryID] {
			return invalidWorkspaceSettingsBatch("invalid_request", "each repository settings resource must appear once")
		}
		seenRepositories[repository.RepositoryID] = true
	}
	if err := validateWorkspaceSyncConfigBatchRequests(input.SyncConfigs); err != nil {
		return err
	}

	return validateWorkspaceSyncOverrideBatchRequests(input.SyncOverrides)
}

func validateWorkspaceTargetBatchRequest(input workspaceTargetBatchRequest) error {
	if input.RepositoryDefaultEnabled == nil || input.PendingCIModeDefault == nil ||
		input.PendingCIBranchPatternsDefault == nil || !input.PendingCIQuietPeriodSeconds.Present ||
		!input.PathIndexIntervalSeconds.Present || input.ConfigPatch == nil ||
		input.ExpectedRevision == nil || *input.ExpectedRevision < 0 {
		return invalidWorkspaceSettingsBatch("invalid_request", "target settings are incomplete")
	}
	if err := validatePatch(*input.ConfigPatch); err != nil {
		return invalidWorkspaceSettingsBatch("invalid_config", err.Error())
	}

	return nil
}

func validateWorkspaceRepositoryBatchRequest(input workspaceRepositoryBatchRequest) error {
	if input.RepositoryID == "" || !input.EnabledOverride.Present ||
		!input.PendingCIModeOverride.Present || !input.PendingCIBranchPatternsOverride.Present ||
		!input.PendingCIQuietPeriodSeconds.Present || !input.PathIndexIntervalSeconds.Present ||
		input.ConfigPatch == nil || input.IgnoreRepositoryFile == nil ||
		input.ExpectedRevision == nil || *input.ExpectedRevision < 0 {
		return invalidWorkspaceSettingsBatch("invalid_request", "repository settings are incomplete")
	}
	if err := validatePatch(*input.ConfigPatch); err != nil {
		return invalidWorkspaceSettingsBatch("invalid_config", err.Error())
	}

	return nil
}

func validateWorkspaceSyncConfigBatchRequests(
	inputs []workspaceSyncConfigBatchRequest,
) error {
	seen := make(map[orgsync.Kind]bool, len(inputs))
	for _, input := range inputs {
		kind := orgsync.Kind(input.Kind)
		if !kind.Valid() || seen[kind] {
			return invalidWorkspaceSettingsBatch("invalid_request", "each sync config kind must be known and appear once")
		}
		seen[kind] = true
		if input.Enabled == nil || input.ExpectedRevision == nil || *input.ExpectedRevision < 0 {
			return invalidWorkspaceSettingsBatch("invalid_request", "sync config settings are incomplete")
		}
		if kind == orgsync.KindLabels {
			if !input.Labels.Present || input.AllowRemoval == nil || !input.Excludes.Present || input.Document.Present {
				return invalidWorkspaceSettingsBatch("invalid_request", "label sync settings need the complete typed document")
			}
		} else if !input.Document.Present || input.Document.Null || input.Labels.Present ||
			input.AllowRemoval != nil || input.Excludes.Present {
			return invalidWorkspaceSettingsBatch("invalid_request", "sync config settings need one complete document")
		}
	}

	return nil
}

func validateWorkspaceSyncOverrideBatchRequests(
	inputs []workspaceSyncOverrideBatchRequest,
) error {
	seen := make(map[string]bool, len(inputs))
	for _, input := range inputs {
		kind := orgsync.Kind(input.Kind)
		key := input.RepositoryID + "\x00" + input.Kind
		if input.RepositoryID == "" || !kind.Valid() || seen[key] {
			return invalidWorkspaceSettingsBatch("invalid_request", "each repository sync kind must be known and appear once")
		}
		seen[key] = true
		if !input.Enabled.Present || !input.Document.Present || input.Document.Null ||
			input.ExpectedRevision == nil || *input.ExpectedRevision < 0 {
			return invalidWorkspaceSettingsBatch("invalid_request", "repository sync settings are incomplete")
		}
	}

	return nil
}

func sameDuration(left, right *time.Duration) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}

	return *left == *right
}

func sortedBatchRepositoryIDs(input workspaceSettingsBatchRequest) []string {
	seen := make(map[string]bool)
	for _, repository := range input.Repositories {
		seen[repository.RepositoryID] = true
	}
	for _, override := range input.SyncOverrides {
		seen[override.RepositoryID] = true
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	return ids
}

func (s *Server) workspaceBatchRepositories(
	r *http.Request,
	targetID string,
	input workspaceSettingsBatchRequest,
) (map[string]storage.Repository, error) {
	repositories := make(map[string]storage.Repository)
	for _, id := range sortedBatchRepositoryIDs(input) {
		repository, err := s.store.GetRepository(r.Context(), targetID, id)
		if errors.Is(err, storage.ErrNotFound) || (err == nil && !repository.Available) {
			return nil, &workspaceSettingsBatchInputError{
				status: http.StatusNotFound, code: "not_found",
				message: "a requested repository is unavailable in this workspace",
			}
		}
		if err != nil {
			return nil, err
		}
		repositories[id] = repository
	}

	return repositories, nil
}

func (s *Server) workspaceTargetBatchChange(
	target storage.Target,
	input workspaceTargetBatchRequest,
) (storage.InstallationTargetSettingsChange, error) {
	quiet := pendingCIQuietDuration(input.PendingCIQuietPeriodSeconds.Value)
	if err := storage.ValidateTargetPendingCISettings(
		*input.PendingCIModeDefault, *input.PendingCIBranchPatternsDefault, quiet,
	); err != nil {
		return storage.InstallationTargetSettingsChange{}, invalidWorkspaceSettingsBatch(
			"invalid_pending_ci_settings", err.Error())
	}
	pathIndex, err := pathIndexOverride(target.PathIndexIntervalOverride, input.PathIndexIntervalSeconds)
	if err != nil {
		return storage.InstallationTargetSettingsChange{}, invalidWorkspaceSettingsBatch(
			"invalid_path_index_interval", err.Error())
	}

	return storage.InstallationTargetSettingsChange{
		RepositoryDefaultEnabled:       *input.RepositoryDefaultEnabled,
		PendingCIModeDefault:           *input.PendingCIModeDefault,
		PendingCIBranchPatternsDefault: *input.PendingCIBranchPatternsDefault,
		PendingCIQuietPeriodOverride:   quiet, PathIndexIntervalOverride: pathIndex,
		ConfigPatch: *input.ConfigPatch, ExpectedRevision: *input.ExpectedRevision,
		RetunePendingCIQuietPeriod:     !sameDuration(target.PendingCIQuietPeriodOverride, quiet),
		DeploymentPendingCIQuietPeriod: s.cfg.PendingCIQuietPeriod,
	}, nil
}

func (s *Server) workspaceRepositoryBatchChanges(
	inputs []workspaceRepositoryBatchRequest,
	repositories map[string]storage.Repository,
) ([]storage.InstallationRepositorySettingsChange, error) {
	changes := make([]storage.InstallationRepositorySettingsChange, 0, len(inputs))
	for _, input := range inputs {
		repository := repositories[input.RepositoryID]
		quiet := pendingCIQuietDuration(input.PendingCIQuietPeriodSeconds.Value)
		if err := storage.ValidateRepositoryPendingCISettings(
			input.PendingCIModeOverride.Value, input.PendingCIBranchPatternsOverride.Value, quiet,
		); err != nil {
			return nil, invalidWorkspaceSettingsBatch("invalid_pending_ci_settings", err.Error())
		}
		pathIndex, err := pathIndexOverride(repository.PathIndexIntervalOverride, input.PathIndexIntervalSeconds)
		if err != nil {
			return nil, invalidWorkspaceSettingsBatch("invalid_path_index_interval", err.Error())
		}
		changes = append(changes, storage.InstallationRepositorySettingsChange{
			RepositoryID: input.RepositoryID, EnabledOverride: input.EnabledOverride.Value,
			PendingCIModeOverride:           input.PendingCIModeOverride.Value,
			PendingCIBranchPatternsOverride: input.PendingCIBranchPatternsOverride.Value,
			PendingCIQuietPeriodOverride:    quiet, PathIndexIntervalOverride: pathIndex,
			ConfigPatch: *input.ConfigPatch, IgnoreRepositoryFile: *input.IgnoreRepositoryFile,
			ExpectedRevision:               *input.ExpectedRevision,
			RetunePendingCIQuietPeriod:     !sameDuration(repository.PendingCIQuietPeriodOverride, quiet),
			DeploymentPendingCIQuietPeriod: s.cfg.PendingCIQuietPeriod,
		})
	}

	return changes, nil
}

func workspaceSyncConfigBatchChanges(
	inputs []workspaceSyncConfigBatchRequest,
) ([]storage.InstallationSyncConfigChange, *orgsync.FileConfig, error) {
	changes := make([]storage.InstallationSyncConfigChange, 0, len(inputs))
	var proposedFiles *orgsync.FileConfig
	for _, input := range inputs {
		kind := orgsync.Kind(input.Kind)
		if input.Document.Present && int64(len(input.Document.Value)) > bodyBoundFor(kind) {
			return nil, nil, &workspaceSettingsBatchInputError{
				status: http.StatusRequestEntityTooLarge, code: workspaceSettingsBatchTooLargeCode,
				message: fmt.Sprintf("the %s sync configuration is too large", kind),
			}
		}
		request := syncConfigRequest{Document: input.Document.Value}
		if kind == orgsync.KindLabels {
			request.Labels = input.Labels.Value
			request.AllowRemoval = *input.AllowRemoval
			request.Excludes = input.Excludes.Value
		}
		document, err := syncDocumentFor(kind, request)
		if err != nil {
			return nil, nil, invalidWorkspaceSettingsBatch("invalid_sync_config", err.Error())
		}
		if int64(len(document)) > bodyBoundFor(kind) {
			return nil, nil, &workspaceSettingsBatchInputError{
				status: http.StatusRequestEntityTooLarge, code: workspaceSettingsBatchTooLargeCode,
				message: fmt.Sprintf("the %s sync configuration is too large", kind),
			}
		}
		changes = append(changes, storage.InstallationSyncConfigChange{
			Kind: kind, Enabled: *input.Enabled, Document: document,
			ExpectedRevision: *input.ExpectedRevision,
		})
		if kind == orgsync.KindFiles {
			var files orgsync.FileConfig
			if err := json.Unmarshal(document, &files); err != nil {
				return nil, nil, err
			}
			proposedFiles = &files
		}
	}

	return changes, proposedFiles, nil
}

func (s *Server) workspaceSyncOverrideBatchChanges(
	r *http.Request,
	targetID string,
	inputs []workspaceSyncOverrideBatchRequest,
	repositories map[string]storage.Repository,
	proposedFiles *orgsync.FileConfig,
) ([]storage.InstallationSyncOverrideChange, error) {
	changes := make([]storage.InstallationSyncOverrideChange, 0, len(inputs))
	for _, input := range inputs {
		kind := orgsync.Kind(input.Kind)
		if int64(len(input.Document.Value)) > maxRequestBody {
			return nil, &workspaceSettingsBatchInputError{
				status: http.StatusRequestEntityTooLarge, code: workspaceSettingsBatchTooLargeCode,
				message: fmt.Sprintf("the %s repository sync settings are too large", kind),
			}
		}
		document, err := s.syncOverrideBatchDocument(
			r, targetID, repositories[input.RepositoryID], kind, input.Document.Value, proposedFiles,
		)
		if err != nil {
			if errors.Is(err, orgsync.ErrInvalidConfig) {
				return nil, invalidWorkspaceSettingsBatch("invalid_sync_override", err.Error())
			}
			return nil, err
		}
		if int64(len(document)) > maxRequestBody {
			return nil, &workspaceSettingsBatchInputError{
				status: http.StatusRequestEntityTooLarge, code: workspaceSettingsBatchTooLargeCode,
				message: fmt.Sprintf("the %s repository sync settings are too large", kind),
			}
		}
		changes = append(changes, storage.InstallationSyncOverrideChange{
			RepositoryID: input.RepositoryID, Kind: kind, Enabled: input.Enabled.Value,
			Document: document, ExpectedRevision: *input.ExpectedRevision,
		})
	}

	return changes, nil
}
