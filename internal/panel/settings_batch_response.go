package panel

import (
	"encoding/json"
	"sort"
	"strconv"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

type workspaceSettingsBatchResponse struct {
	CheckpointID  *string                              `json:"checkpoint_id,omitempty"`
	Target        *workspaceTargetSettingsState        `json:"target,omitempty"`
	Repositories  []workspaceRepositorySettingsState   `json:"repositories,omitempty"`
	SyncConfigs   []workspaceSyncConfigSettingsState   `json:"sync_configs,omitempty"`
	SyncOverrides []workspaceSyncOverrideSettingsState `json:"sync_overrides,omitempty"`
}

type workspaceTargetSettingsState struct {
	TargetID                            string                          `json:"target_id"`
	RepositoryDefaultEnabled            bool                            `json:"repository_default_enabled"`
	PendingCIModeDefault                storage.PendingCIMode           `json:"pending_ci_mode_default"`
	PendingCIBranchPatternsDefault      storage.PendingCIBranchPatterns `json:"pending_ci_branch_patterns_default"`
	PendingCIQuietPeriodSecondsOverride *int64                          `json:"pending_ci_quiet_period_seconds_override"`
	PathIndexIntervalSecondsOverride    *int64                          `json:"path_index_interval_seconds_override"`
	ConfigPatch                         config.Patch                    `json:"config_patch"`
	Revision                            int64                           `json:"revision"`
}

type workspaceRepositorySettingsState struct {
	RepositoryID                        string                           `json:"repository_id"`
	EnabledOverride                     *bool                            `json:"enabled_override"`
	PendingCIModeOverride               *storage.PendingCIMode           `json:"pending_ci_mode_override"`
	PendingCIBranchPatternsOverride     *storage.PendingCIBranchPatterns `json:"pending_ci_branch_patterns_override"`
	PendingCIQuietPeriodSecondsOverride *int64                           `json:"pending_ci_quiet_period_seconds_override"`
	PathIndexIntervalSecondsOverride    *int64                           `json:"path_index_interval_seconds_override"`
	ConfigPatch                         config.Patch                     `json:"config_patch"`
	IgnoreRepositoryFile                bool                             `json:"ignore_repository_file"`
	Revision                            int64                            `json:"revision"`
}

type workspaceSyncConfigSettingsState struct {
	TargetID string          `json:"target_id"`
	Kind     orgsync.Kind    `json:"kind"`
	Enabled  bool            `json:"enabled"`
	Document json.RawMessage `json:"document"`
	Revision int64           `json:"revision"`
}

type workspaceSyncOverrideSettingsState struct {
	TargetID     string          `json:"target_id"`
	RepositoryID string          `json:"repository_id"`
	Kind         orgsync.Kind    `json:"kind"`
	Enabled      *bool           `json:"enabled"`
	Document     json.RawMessage `json:"document"`
	Revision     int64           `json:"revision"`
}

func workspaceSettingsBatchAnswer(
	request storage.SaveInstallationSettingsRequest,
	result storage.SaveInstallationSettingsResult,
) workspaceSettingsBatchResponse {
	answer := workspaceSettingsBatchResponse{}
	if result.CheckpointID != nil {
		checkpointID := strconv.FormatInt(*result.CheckpointID, 10)
		answer.CheckpointID = &checkpointID
	}
	if request.Target != nil && result.Target != nil {
		state := workspaceTargetSettingsStateFrom(*result.Target)
		answer.Target = &state
	}
	if len(request.Repositories) > 0 {
		answer.Repositories = make([]workspaceRepositorySettingsState, 0, len(result.Repositories))
		for _, repository := range result.Repositories {
			answer.Repositories = append(
				answer.Repositories, workspaceRepositorySettingsStateFrom(repository),
			)
		}
		sort.Slice(answer.Repositories, func(left, right int) bool {
			return answer.Repositories[left].RepositoryID < answer.Repositories[right].RepositoryID
		})
	}
	answer.SyncConfigs = workspaceSyncConfigSettingsStates(request.TargetID, result.SyncConfigs)
	answer.SyncOverrides = workspaceSyncOverrideSettingsStates(request.TargetID, result.SyncOverrides)

	return answer
}

func workspaceTargetSettingsStateFrom(
	target storage.Target,
) workspaceTargetSettingsState {
	return workspaceTargetSettingsState{
		TargetID: target.ID, RepositoryDefaultEnabled: target.RepositoryDefaultEnabled,
		PendingCIModeDefault:                target.PendingCIModeDefault,
		PendingCIBranchPatternsDefault:      target.PendingCIBranchPatternsDefault,
		PendingCIQuietPeriodSecondsOverride: durationSecondsDTO(target.PendingCIQuietPeriodOverride),
		PathIndexIntervalSecondsOverride:    durationSecondsDTO(target.PathIndexIntervalOverride),
		ConfigPatch:                         target.ConfigPatch, Revision: target.Revision,
	}
}

func workspaceRepositorySettingsStateFrom(
	repository storage.Repository,
) workspaceRepositorySettingsState {
	return workspaceRepositorySettingsState{
		RepositoryID: repository.ID, EnabledOverride: repository.EnabledOverride,
		PendingCIModeOverride:               repository.PendingCIModeOverride,
		PendingCIBranchPatternsOverride:     repository.PendingCIBranchPatternsOverride,
		PendingCIQuietPeriodSecondsOverride: durationSecondsDTO(repository.PendingCIQuietPeriodOverride),
		PathIndexIntervalSecondsOverride:    durationSecondsDTO(repository.PathIndexIntervalOverride),
		ConfigPatch:                         repository.ConfigPatch,
		IgnoreRepositoryFile:                repository.IgnoreRepositoryFile, Revision: repository.Revision,
	}
}

func workspaceSyncConfigSettingsStates(
	targetID string,
	configs []orgsync.Config,
) []workspaceSyncConfigSettingsState {
	if configs == nil {
		return nil
	}
	states := make([]workspaceSyncConfigSettingsState, 0, len(configs))
	for _, config := range configs {
		states = append(states, workspaceSyncConfigSettingsState{
			TargetID: targetID, Kind: config.Kind, Enabled: config.Enabled,
			Document: documentOrEmpty(config.Document), Revision: config.Revision,
		})
	}
	sort.Slice(states, func(left, right int) bool { return states[left].Kind < states[right].Kind })

	return states
}

func workspaceSyncOverrideSettingsStates(
	targetID string,
	overrides []orgsync.RepositoryOverride,
) []workspaceSyncOverrideSettingsState {
	if overrides == nil {
		return nil
	}
	states := make([]workspaceSyncOverrideSettingsState, 0, len(overrides))
	for _, override := range overrides {
		states = append(states, workspaceSyncOverrideSettingsState{
			TargetID: targetID, RepositoryID: override.RepositoryID, Kind: override.Kind,
			Enabled: override.Enabled, Document: documentOrEmpty(override.Document),
			Revision: override.Revision,
		})
	}
	sort.Slice(states, func(left, right int) bool {
		if states[left].RepositoryID != states[right].RepositoryID {
			return states[left].RepositoryID < states[right].RepositoryID
		}
		return states[left].Kind < states[right].Kind
	})

	return states
}
