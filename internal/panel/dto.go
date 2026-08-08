package panel

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

type accountResponse struct {
	ID          string  `json:"id"`
	Provider    string  `json:"provider"`
	SubjectID   string  `json:"subject_id"`
	Login       string  `json:"login"`
	DisplayName string  `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
}

type viewerResponse struct {
	Account     accountResponse `json:"account"`
	TargetCount int             `json:"target_count"`
}

type targetResponse struct {
	ID                       string                   `json:"id"`
	InstallationID           string                   `json:"installation_id"`
	Type                     storage.TargetKind       `json:"type"`
	Account                  accountResponse          `json:"account"`
	RepositoryDefaultEnabled bool                     `json:"repository_default_enabled"`
	ConfigPatch              config.Patch             `json:"config_patch"`
	InheritedConfig          config.Config            `json:"inherited_config"`
	EffectiveConfig          config.Config            `json:"effective_config"`
	ConfigSources            map[string]config.Source `json:"config_sources"`
	Revision                 int64                    `json:"revision"`
	RepositoryCounts         storage.RepositoryCounts `json:"repository_counts"`
}

type repositorySummaryResponse struct {
	ID                  string                       `json:"id"`
	Name                string                       `json:"name"`
	FullName            string                       `json:"full_name"`
	Private             bool                         `json:"private"`
	Available           bool                         `json:"available"`
	EnabledOverride     *bool                        `json:"enabled_override"`
	EffectiveEnabled    bool                         `json:"effective_enabled"`
	EnabledSource       string                       `json:"enabled_source"`
	ConfigOverrideCount int                          `json:"config_override_count"`
	ConfigFileStatus    storage.RepositoryFileStatus `json:"config_file_status"`
	UpdatedAt           time.Time                    `json:"updated_at"`
}

type repositoryDetailResponse struct {
	Repository           repositorySummaryResponse `json:"repository"`
	ConfigPatch          config.Patch              `json:"config_patch"`
	InheritedConfig      config.Config             `json:"inherited_config"`
	EffectiveConfig      config.Config             `json:"effective_config"`
	ConfigSources        map[string]config.Source  `json:"config_sources"`
	ConfigFilePatch      config.Patch              `json:"config_file_patch"`
	ConfigFileError      *string                   `json:"config_file_error,omitempty"`
	IgnoreRepositoryFile bool                      `json:"ignore_repository_file"`
	Revision             int64                     `json:"revision"`
}

type auditResponse struct {
	ID                 string          `json:"id"`
	Actor              accountResponse `json:"actor"`
	Action             string          `json:"action"`
	Summary            string          `json:"summary"`
	RepositoryFullName *string         `json:"repository_full_name,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
}

type failureResponse struct {
	ID                 string    `json:"id"`
	DeliveryID         string    `json:"delivery_id"`
	RepositoryFullName string    `json:"repository_full_name"`
	Event              string    `json:"event"`
	Stage              string    `json:"stage"`
	Reason             string    `json:"reason"`
	Retryable          bool      `json:"retryable"`
	OccurredAt         time.Time `json:"occurred_at"`
}

type pageResponse[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"next_cursor"`
	Total      int     `json:"total"`
}

func accountDTO(account storage.Account) accountResponse {
	return accountResponse{
		ID:          account.ID,
		Provider:    account.Provider,
		SubjectID:   account.SubjectID,
		Login:       account.Login,
		DisplayName: account.DisplayName,
		AvatarURL:   account.AvatarURL,
	}
}

func targetDTO(process *config.Config, target storage.Target) targetResponse {
	inherited := config.Resolve(process)
	resolved := config.Resolve(process, config.Layer{
		Source: config.SourceTarget,
		Patch:  target.ConfigPatch,
	})

	return targetResponse{
		ID:                       target.ID,
		InstallationID:           target.InstallationID,
		Type:                     target.Kind,
		Account:                  accountDTO(target.Account),
		RepositoryDefaultEnabled: target.RepositoryDefaultEnabled,
		ConfigPatch:              target.ConfigPatch,
		InheritedConfig:          inherited.Values,
		EffectiveConfig:          resolved.Values,
		ConfigSources:            resolved.Sources,
		Revision:                 target.Revision,
		RepositoryCounts:         target.RepositoryCounts,
	}
}

func repositorySummaryDTO(
	target storage.Target,
	repository storage.Repository,
) repositorySummaryResponse {
	enabled := target.RepositoryDefaultEnabled
	source := "target"
	if repository.EnabledOverride != nil {
		enabled = *repository.EnabledOverride
		source = "repository"
	}

	return repositorySummaryResponse{
		ID:                  repository.ID,
		Name:                repository.Name,
		FullName:            repository.FullName,
		Private:             repository.Private,
		Available:           repository.Available,
		EnabledOverride:     repository.EnabledOverride,
		EffectiveEnabled:    enabled,
		EnabledSource:       source,
		ConfigOverrideCount: patchSize(repository.ConfigPatch),
		ConfigFileStatus:    repository.ConfigFileStatus,
		UpdatedAt:           repository.UpdatedAt,
	}
}

func repositoryDetailDTO(
	process *config.Config,
	target storage.Target,
	repository storage.Repository,
) repositoryDetailResponse {
	layers := []config.Layer{{Source: config.SourceTarget, Patch: target.ConfigPatch}}
	if !repository.IgnoreRepositoryFile {
		layers = append(layers, config.Layer{
			Source: config.SourceRepositoryFile,
			Patch:  repository.ConfigFilePatch,
		})
	}
	inherited := config.Resolve(process, layers...)
	layers = append(layers, config.Layer{
		Source: config.SourceRepositoryPanel,
		Patch:  repository.ConfigPatch,
	})
	resolved := config.Resolve(process, layers...)

	return repositoryDetailResponse{
		Repository:           repositorySummaryDTO(target, repository),
		ConfigPatch:          repository.ConfigPatch,
		InheritedConfig:      inherited.Values,
		EffectiveConfig:      resolved.Values,
		ConfigSources:        resolved.Sources,
		ConfigFilePatch:      repository.ConfigFilePatch,
		ConfigFileError:      repository.ConfigFileError,
		IgnoreRepositoryFile: repository.IgnoreRepositoryFile,
		Revision:             repository.Revision,
	}
}

func auditPageDTO(page storage.AuditPage) pageResponse[auditResponse] {
	items := make([]auditResponse, 0, len(page.Items))
	for _, entry := range page.Items {
		items = append(items, auditResponse{
			ID:                 strconv.FormatInt(entry.ID, 10),
			Actor:              accountDTO(entry.Actor),
			Action:             entry.Action,
			Summary:            entry.Summary,
			RepositoryFullName: entry.RepositoryFullName,
			CreatedAt:          entry.CreatedAt,
		})
	}

	return pageResponse[auditResponse]{
		Items: items, NextCursor: cursor(page.NextCursor), Total: page.Total,
	}
}

func failurePageDTO(page storage.FailurePage) pageResponse[failureResponse] {
	items := make([]failureResponse, 0, len(page.Items))
	for _, failure := range page.Items {
		items = append(items, failureResponse{
			ID:                 strconv.FormatInt(failure.ID, 10),
			DeliveryID:         failure.DeliveryID,
			RepositoryFullName: failure.RepositoryFullName,
			Event:              failure.Event,
			Stage:              failure.Stage,
			Reason:             failure.Reason,
			Retryable:          failure.Retryable,
			OccurredAt:         failure.OccurredAt,
		})
	}

	return pageResponse[failureResponse]{
		Items: items, NextCursor: cursor(page.NextCursor), Total: page.Total,
	}
}

func cursor(value int64) *string {
	if value == 0 {
		return nil
	}
	formatted := strconv.FormatInt(value, 10)

	return &formatted
}

func patchSize(patch config.Patch) int {
	count := 0
	for _, present := range []bool{
		patch.QuietSuccess != nil,
		patch.QuietReactions != nil,
		patch.QuietPending != nil,
		patch.AllowedCommands != nil,
		patch.CommandAliases != nil,
		patch.CommandPrefix != nil,
		patch.DisableMentions != nil,
		patch.DisableBareCommands != nil,
		patch.DisableUnapprove != nil,
		patch.DisableReactions != nil,
		patch.DisableDeletedComments != nil,
		patch.AllowSelfApproval != nil,
	} {
		if present {
			count++
		}
	}

	return count
}

func parseHistoryPage(values url.Values) (storage.HistoryPageRequest, error) {
	page := storage.HistoryPageRequest{
		Limit: DefaultPageSize,
		Order: storage.HistoryNewest,
		Query: strings.TrimSpace(values.Get("q")),
	}
	if len(page.Query) > 200 || strings.ContainsFunc(page.Query, unicode.IsControl) {
		return storage.HistoryPageRequest{}, fmt.Errorf("invalid history query")
	}
	if raw := values.Get("cursor"); raw != "" {
		cursorID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || cursorID <= 0 {
			return storage.HistoryPageRequest{}, fmt.Errorf("invalid history cursor")
		}
		page.CursorID = cursorID
	}
	if raw := values.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit <= 0 || limit > MaxPageSize {
			return storage.HistoryPageRequest{}, fmt.Errorf("invalid history page size")
		}
		page.Limit = limit
	}
	switch raw := values.Get("sort"); raw {
	case "", string(storage.HistoryNewest):
	case string(storage.HistoryOldest):
		page.Order = storage.HistoryOldest
	default:
		return storage.HistoryPageRequest{}, fmt.Errorf("invalid history sort order")
	}

	return page, nil
}
