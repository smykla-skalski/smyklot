package panel

import (
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

const allFilter = "all"

type accountResponse struct {
	ID          string  `json:"id"`
	Provider    string  `json:"provider"`
	SubjectID   string  `json:"subject_id"`
	Login       string  `json:"login"`
	DisplayName string  `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
}

type viewerResponse struct {
	Account      accountResponse         `json:"account"`
	Root         bool                    `json:"root"`
	Status       storage.PanelUserStatus `json:"status"`
	GlobalRole   storage.PanelRole       `json:"global_role"`
	Capabilities capabilityResponse      `json:"capabilities"`
	TargetCount  int                     `json:"target_count"`
}

type capabilityResponse struct {
	Read              bool `json:"read"`
	Write             bool `json:"write"`
	ManageTargetUsers bool `json:"manage_target_users"`
	ManageGlobalUsers bool `json:"manage_global_users"`
	ManageOwners      bool `json:"manage_owners"`
}

type panelUserResponse struct {
	Account      accountResponse         `json:"account"`
	Root         bool                    `json:"root"`
	Status       storage.PanelUserStatus `json:"status"`
	GlobalRole   storage.PanelRole       `json:"global_role"`
	BanReason    *string                 `json:"ban_reason,omitempty"`
	BannedAt     *time.Time              `json:"banned_at,omitempty"`
	LastLoginAt  *time.Time              `json:"last_login_at,omitempty"`
	Revision     int64                   `json:"revision"`
	CreatedAt    time.Time               `json:"created_at"`
	UpdatedAt    time.Time               `json:"updated_at"`
	Manageable   bool                    `json:"manageable"`
	TargetAccess *targetUserAccess       `json:"target_access,omitempty"`
}

type targetUserAccess struct {
	Role             *storage.PanelRole   `json:"role"`
	Suspended        bool                 `json:"suspended"`
	SuspensionReason *string              `json:"suspension_reason,omitempty"`
	Revision         int64                `json:"revision"`
	UpdatedAt        *time.Time           `json:"updated_at,omitempty"`
	EffectiveRole    storage.PanelRole    `json:"effective_role"`
	Source           storage.AccessSource `json:"source"`
	Capabilities     capabilityResponse   `json:"capabilities"`
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
	EffectiveRole            storage.PanelRole        `json:"effective_role"`
	AccessSource             storage.AccessSource     `json:"access_source"`
	Capabilities             capabilityResponse       `json:"capabilities"`
	SuspensionReason         *string                  `json:"suspension_reason,omitempty"`
}

type repositorySummaryResponse struct {
	ID                  string                       `json:"id"`
	Name                string                       `json:"name"`
	FullName            string                       `json:"full_name"`
	Private             bool                         `json:"private"`
	DefaultBranch       string                       `json:"default_branch"`
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

func viewerDTO(user storage.PanelUser, targetCount int) viewerResponse {
	return viewerResponse{
		Account:      accountDTO(user.Account),
		Root:         user.Root,
		Status:       user.Status,
		GlobalRole:   user.GlobalRole,
		Capabilities: capabilitiesDTO(storage.EffectiveCapabilities(user.GlobalRole, user.Root)),
		TargetCount:  targetCount,
	}
}

func panelUserDTO(user storage.PanelUser, manageable bool) panelUserResponse {
	return panelUserResponse{
		Account: accountDTO(user.Account), Root: user.Root, Status: user.Status,
		GlobalRole: user.GlobalRole, BanReason: user.BanReason, BannedAt: user.BannedAt,
		LastLoginAt: user.LastLoginAt, Revision: user.Revision, CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt, Manageable: manageable,
	}
}

func targetPanelUserDTO(user storage.TargetPanelUser, manageable bool) panelUserResponse {
	response := panelUserDTO(user.User, manageable)
	access := targetUserAccess{
		EffectiveRole: user.Access.Role,
		Source:        user.Access.Source,
		Capabilities:  capabilitiesDTO(user.Access.Capabilities),
	}
	if user.Override != nil {
		access.Role = user.Override.Role
		access.Suspended = user.Override.Suspended
		access.SuspensionReason = user.Override.SuspensionReason
		access.Revision = user.Override.Revision
		access.UpdatedAt = &user.Override.UpdatedAt
	}
	response.TargetAccess = &access

	return response
}

func capabilitiesDTO(capabilities storage.AccessCapabilities) capabilityResponse {
	return capabilityResponse{
		Read:              capabilities.Read,
		Write:             capabilities.Write,
		ManageTargetUsers: capabilities.ManageTargetUsers,
		ManageGlobalUsers: capabilities.ManageGlobalUsers,
		ManageOwners:      capabilities.ManageOwners,
	}
}

func targetDTO(
	process *config.Config,
	target storage.Target,
	access storage.TargetAccess,
) targetResponse {
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
		EffectiveRole:            access.Role,
		AccessSource:             access.Source,
		Capabilities:             capabilitiesDTO(access.Capabilities),
		SuspensionReason:         access.SuspensionReason,
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
		DefaultBranch:       repository.DefaultBranch,
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

func repositoryPageDTO(
	target storage.Target,
	page storage.RepositoryPage,
) pageResponse[repositorySummaryResponse] {
	items := make([]repositorySummaryResponse, 0, len(page.Items))
	for _, repository := range page.Items {
		items = append(items, repositorySummaryDTO(target, repository))
	}

	return pageResponse[repositorySummaryResponse]{
		Items: items, NextCursor: offsetCursor(page.NextOffset), Total: page.Total,
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
		Items: items, NextCursor: offsetCursor(page.NextOffset), Total: page.Total,
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
		Items: items, NextCursor: offsetCursor(page.NextOffset), Total: page.Total,
	}
}

func offsetCursor(value int) *string {
	if value == 0 {
		return nil
	}
	formatted := strconv.Itoa(value)

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
		offset, err := strconv.Atoi(raw)
		if err != nil || offset <= 0 {
			return storage.HistoryPageRequest{}, fmt.Errorf("invalid history cursor")
		}
		page.Offset = offset
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

func parseRepositoryPage(values url.Values) (storage.RepositoryPageRequest, error) {
	page := storage.RepositoryPageRequest{
		Limit: DefaultPageSize,
		Order: storage.RepositoryNameAscending,
		Query: strings.TrimSpace(values.Get("q")),
	}
	if len(page.Query) > 200 || strings.ContainsFunc(page.Query, unicode.IsControl) {
		return storage.RepositoryPageRequest{}, fmt.Errorf("invalid repository search")
	}
	if raw := values.Get("cursor"); raw != "" {
		offset, err := strconv.Atoi(raw)
		if err != nil || offset <= 0 {
			return storage.RepositoryPageRequest{}, fmt.Errorf("invalid repository cursor")
		}
		page.Offset = offset
	}
	if raw := values.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit <= 0 || limit > MaxPageSize {
			return storage.RepositoryPageRequest{}, fmt.Errorf("invalid repository page size")
		}
		page.Limit = limit
	}
	switch order := storage.RepositoryOrder(values.Get("sort")); order {
	case "", storage.RepositoryNameAscending:
	case storage.RepositoryNameDescending, storage.RepositoryNewest, storage.RepositoryOldest:
		page.Order = order
	default:
		return storage.RepositoryPageRequest{}, fmt.Errorf("invalid repository sort order")
	}
	switch values.Get("state") {
	case "", allFilter:
	case "enabled":
		value := true
		page.EffectiveEnabled = &value
	case "disabled":
		value := false
		page.EffectiveEnabled = &value
	default:
		return storage.RepositoryPageRequest{}, fmt.Errorf("invalid repository state")
	}
	fileStatuses, err := parseRepositoryFileStatuses(values["file"])
	if err != nil {
		return storage.RepositoryPageRequest{}, err
	}
	page.FileStatuses = fileStatuses

	hasOverrides, overrideKeys, err := parseRepositorySettings(values["setting"])
	if err != nil {
		return storage.RepositoryPageRequest{}, err
	}
	page.HasConfigOverrides = hasOverrides
	page.ConfigOverrideKeys = overrideKeys

	return page, nil
}

func parseRepositorySettings(values []string) (*bool, []string, error) {
	if len(values) == 1 && (values[0] == "custom" || values[0] == "none") {
		value := values[0] == "custom"
		return &value, nil, nil
	}

	keys := make([]string, 0, len(values))
	for _, setting := range values {
		if setting == allFilter && len(values) == 1 {
			continue
		}
		if !panelConfigKey(setting) {
			return nil, nil, fmt.Errorf("invalid repository setting")
		}
		if !slices.Contains(keys, setting) {
			keys = append(keys, setting)
		}
	}

	return nil, keys, nil
}

func parseRepositoryFileStatuses(values []string) ([]storage.RepositoryFileStatus, error) {
	statuses := make([]storage.RepositoryFileStatus, 0, len(values))
	for _, value := range values {
		if value == allFilter && len(values) == 1 {
			continue
		}
		status := storage.RepositoryFileStatus(value)
		switch status {
		case storage.RepositoryFileMissing,
			storage.RepositoryFileValid,
			storage.RepositoryFileInvalid,
			storage.RepositoryFileBypassed:
		default:
			return nil, fmt.Errorf("invalid repository file status")
		}
		if !slices.Contains(statuses, status) {
			statuses = append(statuses, status)
		}
	}

	return statuses, nil
}

func panelConfigKey(key string) bool {
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
