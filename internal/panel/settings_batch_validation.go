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
	maxInstallationSettingsBatchResources = 4096
	installationSettingsBatchTooLargeCode = "request_too_large"
)

type installationSettingsBatchRequest struct {
	Target        *installationTargetBatchRequest        `json:"target"`
	Repositories  []installationRepositoryBatchRequest   `json:"repositories"`
	SyncConfigs   []installationSyncConfigBatchRequest   `json:"sync_configs"`
	SyncOverrides []installationSyncOverrideBatchRequest `json:"sync_overrides"`
}

type installationTargetBatchRequest struct {
	RepositoryDefaultEnabled       *bool                            `json:"repository_default_enabled"`
	PendingCIModeDefault           *storage.PendingCIMode           `json:"pending_ci_mode_default"`
	PendingCIBranchPatternsDefault *storage.PendingCIBranchPatterns `json:"pending_ci_branch_patterns_default"`
	PendingCIQuietPeriodSeconds    nullableValue[int64]             `json:"pending_ci_quiet_period_seconds_override"`
	PathIndexIntervalSeconds       nullableValue[int64]             `json:"path_index_interval_seconds_override"`
	ConfigPatch                    *config.Patch                    `json:"config_patch"`
	ExpectedRevision               *int64                           `json:"expected_revision"`
}

type installationRepositoryBatchRequest struct {
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

type installationSyncConfigBatchRequest struct {
	Kind             string                         `json:"kind"`
	Enabled          *bool                          `json:"enabled"`
	Labels           batchRequired[[]orgsync.Label] `json:"labels"`
	AllowRemoval     *bool                          `json:"allow_removal"`
	Excludes         batchRequired[[]string]        `json:"excludes"`
	Document         batchDocument                  `json:"document"`
	ExpectedRevision *int64                         `json:"expected_revision"`
}

type installationSyncOverrideBatchRequest struct {
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

type installationSettingsBatchInputError struct {
	status  int
	code    string
	message string
}

func (err *installationSettingsBatchInputError) Error() string { return err.message }

func invalidInstallationSettingsBatch(code, message string) error {
	return &installationSettingsBatchInputError{
		status: http.StatusBadRequest, code: code, message: message,
	}
}

func (s *Server) writeInstallationSettingsBatchPreparationError(
	w http.ResponseWriter,
	err error,
	writeStorageError func(http.ResponseWriter, error),
) {
	var inputError *installationSettingsBatchInputError
	if errors.As(err, &inputError) {
		s.writeError(w, inputError.status, inputError.code, inputError.message)
		return
	}
	writeStorageError(w, err)
}

func (s *Server) prepareInstallationSettingsBatch(
	r *http.Request,
	target storage.Target,
	actor installationSettingsBatchActor,
	input installationSettingsBatchRequest,
) (storage.SaveInstallationSettingsRequest, error) {
	if err := validateInstallationSettingsBatchShape(input); err != nil {
		return storage.SaveInstallationSettingsRequest{}, err
	}
	request := storage.SaveInstallationSettingsRequest{
		TargetID: target.ID, ActorAccountID: actor.accountID,
		ElevationID: actor.elevationID, SessionTokenHash: actor.sessionTokenHash,
		ChangedAt: s.now().UTC(),
	}
	if input.Target != nil {
		change, err := s.installationTargetBatchChange(target, *input.Target)
		if err != nil {
			return storage.SaveInstallationSettingsRequest{}, err
		}
		request.Target = &change
	}
	configs, proposedFiles, err := installationSyncConfigBatchChanges(input.SyncConfigs)
	if err != nil {
		return storage.SaveInstallationSettingsRequest{}, err
	}
	request.SyncConfigs = configs
	repositories, err := s.installationBatchRepositories(r, target.ID, input)
	if err != nil {
		return storage.SaveInstallationSettingsRequest{}, err
	}
	request.Repositories, err = s.installationRepositoryBatchChanges(input.Repositories, repositories)
	if err != nil {
		return storage.SaveInstallationSettingsRequest{}, err
	}
	request.SyncOverrides, err = s.installationSyncOverrideBatchChanges(
		r, target.ID, input.SyncOverrides, repositories, proposedFiles,
	)
	if err != nil {
		return storage.SaveInstallationSettingsRequest{}, err
	}

	return request, nil
}

func validateInstallationSettingsBatchShape(input installationSettingsBatchRequest) error {
	resources := len(input.Repositories) + len(input.SyncConfigs) + len(input.SyncOverrides)
	if input.Target != nil {
		resources++
	}
	if resources == 0 {
		return invalidInstallationSettingsBatch("invalid_request", "a settings save needs at least one resource")
	}
	if resources > maxInstallationSettingsBatchResources {
		return invalidInstallationSettingsBatch("invalid_request", "a settings save contains too many resources")
	}
	if input.Target != nil {
		if err := validateInstallationTargetBatchRequest(*input.Target); err != nil {
			return err
		}
	}
	seenRepositories := make(map[string]bool, len(input.Repositories))
	for _, repository := range input.Repositories {
		if err := validateInstallationRepositoryBatchRequest(repository); err != nil {
			return err
		}
		if seenRepositories[repository.RepositoryID] {
			return invalidInstallationSettingsBatch("invalid_request", "each repository settings resource must appear once")
		}
		seenRepositories[repository.RepositoryID] = true
	}
	if err := validateInstallationSyncConfigBatchRequests(input.SyncConfigs); err != nil {
		return err
	}

	return validateInstallationSyncOverrideBatchRequests(input.SyncOverrides)
}

func validateInstallationTargetBatchRequest(input installationTargetBatchRequest) error {
	if input.RepositoryDefaultEnabled == nil || input.PendingCIModeDefault == nil ||
		input.PendingCIBranchPatternsDefault == nil || !input.PendingCIQuietPeriodSeconds.Present ||
		!input.PathIndexIntervalSeconds.Present || input.ConfigPatch == nil ||
		input.ExpectedRevision == nil || *input.ExpectedRevision < 0 {
		return invalidInstallationSettingsBatch("invalid_request", "target settings are incomplete")
	}
	if err := validatePatch(*input.ConfigPatch); err != nil {
		return invalidInstallationSettingsBatch("invalid_config", err.Error())
	}

	return nil
}

func validateInstallationRepositoryBatchRequest(input installationRepositoryBatchRequest) error {
	if input.RepositoryID == "" || !input.EnabledOverride.Present ||
		!input.PendingCIModeOverride.Present || !input.PendingCIBranchPatternsOverride.Present ||
		!input.PendingCIQuietPeriodSeconds.Present || !input.PathIndexIntervalSeconds.Present ||
		input.ConfigPatch == nil || input.IgnoreRepositoryFile == nil ||
		input.ExpectedRevision == nil || *input.ExpectedRevision < 0 {
		return invalidInstallationSettingsBatch("invalid_request", "repository settings are incomplete")
	}
	if err := validatePatch(*input.ConfigPatch); err != nil {
		return invalidInstallationSettingsBatch("invalid_config", err.Error())
	}

	return nil
}

func validateInstallationSyncConfigBatchRequests(
	inputs []installationSyncConfigBatchRequest,
) error {
	seen := make(map[orgsync.Kind]bool, len(inputs))
	for _, input := range inputs {
		kind := orgsync.Kind(input.Kind)
		if !kind.Valid() || seen[kind] {
			return invalidInstallationSettingsBatch("invalid_request", "each sync config kind must be known and appear once")
		}
		seen[kind] = true
		if input.Enabled == nil || input.ExpectedRevision == nil || *input.ExpectedRevision < 0 {
			return invalidInstallationSettingsBatch("invalid_request", "sync config settings are incomplete")
		}
		if kind == orgsync.KindLabels {
			if !input.Labels.Present || input.AllowRemoval == nil || !input.Excludes.Present || input.Document.Present {
				return invalidInstallationSettingsBatch("invalid_request", "label sync settings need the complete typed document")
			}
		} else if !input.Document.Present || input.Document.Null || input.Labels.Present ||
			input.AllowRemoval != nil || input.Excludes.Present {
			return invalidInstallationSettingsBatch("invalid_request", "sync config settings need one complete document")
		}
	}

	return nil
}

func validateInstallationSyncOverrideBatchRequests(
	inputs []installationSyncOverrideBatchRequest,
) error {
	seen := make(map[string]bool, len(inputs))
	for _, input := range inputs {
		kind := orgsync.Kind(input.Kind)
		key := input.RepositoryID + "\x00" + input.Kind
		if input.RepositoryID == "" || !kind.Valid() || seen[key] {
			return invalidInstallationSettingsBatch("invalid_request", "each repository sync kind must be known and appear once")
		}
		seen[key] = true
		if !input.Enabled.Present || !input.Document.Present || input.Document.Null ||
			input.ExpectedRevision == nil || *input.ExpectedRevision < 0 {
			return invalidInstallationSettingsBatch("invalid_request", "repository sync settings are incomplete")
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

func sortedBatchRepositoryIDs(input installationSettingsBatchRequest) []string {
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

func (s *Server) installationBatchRepositories(
	r *http.Request,
	targetID string,
	input installationSettingsBatchRequest,
) (map[string]storage.Repository, error) {
	repositories := make(map[string]storage.Repository)
	for _, id := range sortedBatchRepositoryIDs(input) {
		repository, err := s.store.GetRepository(r.Context(), targetID, id)
		if errors.Is(err, storage.ErrNotFound) || (err == nil && !repository.Available) {
			return nil, &installationSettingsBatchInputError{
				status: http.StatusNotFound, code: "not_found",
				message: "a requested repository is unavailable for this installation",
			}
		}
		if err != nil {
			return nil, err
		}
		repositories[id] = repository
	}

	return repositories, nil
}

func (s *Server) installationTargetBatchChange(
	target storage.Target,
	input installationTargetBatchRequest,
) (storage.InstallationTargetSettingsChange, error) {
	quiet := pendingCIQuietDuration(input.PendingCIQuietPeriodSeconds.Value)
	if err := storage.ValidateTargetPendingCISettings(
		*input.PendingCIModeDefault, *input.PendingCIBranchPatternsDefault, quiet,
	); err != nil {
		return storage.InstallationTargetSettingsChange{}, invalidInstallationSettingsBatch(
			"invalid_pending_ci_settings", err.Error())
	}
	pathIndex, err := pathIndexOverride(target.PathIndexIntervalOverride, input.PathIndexIntervalSeconds)
	if err != nil {
		return storage.InstallationTargetSettingsChange{}, invalidInstallationSettingsBatch(
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

func (s *Server) installationRepositoryBatchChanges(
	inputs []installationRepositoryBatchRequest,
	repositories map[string]storage.Repository,
) ([]storage.InstallationRepositorySettingsChange, error) {
	changes := make([]storage.InstallationRepositorySettingsChange, 0, len(inputs))
	for _, input := range inputs {
		repository := repositories[input.RepositoryID]
		quiet := pendingCIQuietDuration(input.PendingCIQuietPeriodSeconds.Value)
		if err := storage.ValidateRepositoryPendingCISettings(
			input.PendingCIModeOverride.Value, input.PendingCIBranchPatternsOverride.Value, quiet,
		); err != nil {
			return nil, invalidInstallationSettingsBatch("invalid_pending_ci_settings", err.Error())
		}
		pathIndex, err := pathIndexOverride(repository.PathIndexIntervalOverride, input.PathIndexIntervalSeconds)
		if err != nil {
			return nil, invalidInstallationSettingsBatch("invalid_path_index_interval", err.Error())
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

func installationSyncConfigBatchChanges(
	inputs []installationSyncConfigBatchRequest,
) ([]storage.InstallationSyncConfigChange, *orgsync.FileConfig, error) {
	changes := make([]storage.InstallationSyncConfigChange, 0, len(inputs))
	var proposedFiles *orgsync.FileConfig
	for _, input := range inputs {
		kind := orgsync.Kind(input.Kind)
		if input.Document.Present && int64(len(input.Document.Value)) > bodyBoundFor(kind) {
			return nil, nil, &installationSettingsBatchInputError{
				status: http.StatusRequestEntityTooLarge, code: installationSettingsBatchTooLargeCode,
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
			return nil, nil, invalidInstallationSettingsBatch("invalid_sync_config", err.Error())
		}
		if int64(len(document)) > bodyBoundFor(kind) {
			return nil, nil, &installationSettingsBatchInputError{
				status: http.StatusRequestEntityTooLarge, code: installationSettingsBatchTooLargeCode,
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

func (s *Server) installationSyncOverrideBatchChanges(
	r *http.Request,
	targetID string,
	inputs []installationSyncOverrideBatchRequest,
	repositories map[string]storage.Repository,
	proposedFiles *orgsync.FileConfig,
) ([]storage.InstallationSyncOverrideChange, error) {
	changes := make([]storage.InstallationSyncOverrideChange, 0, len(inputs))
	for _, input := range inputs {
		kind := orgsync.Kind(input.Kind)
		if int64(len(input.Document.Value)) > maxRequestBody {
			return nil, &installationSettingsBatchInputError{
				status: http.StatusRequestEntityTooLarge, code: installationSettingsBatchTooLargeCode,
				message: fmt.Sprintf("the %s repository sync settings are too large", kind),
			}
		}
		document, err := s.syncOverrideBatchDocument(
			r, targetID, repositories[input.RepositoryID], kind, input.Document.Value, proposedFiles,
		)
		if err != nil {
			if errors.Is(err, orgsync.ErrInvalidConfig) {
				return nil, invalidInstallationSettingsBatch("invalid_sync_override", err.Error())
			}
			return nil, err
		}
		if int64(len(document)) > maxRequestBody {
			return nil, &installationSettingsBatchInputError{
				status: http.StatusRequestEntityTooLarge, code: installationSettingsBatchTooLargeCode,
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
