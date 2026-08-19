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

// userSuggestionsResponse carries logins offered while somebody types one. It is
// a bare list rather than a page: the server has already cut it to what a reader
// can take in, and there is no second page of a completion.
type userSuggestionsResponse struct {
	Items []accountResponse `json:"items"`
}

type viewerResponse struct {
	Account     accountResponse         `json:"account"`
	SystemRole  storage.SystemRole      `json:"system_role"`
	Status      storage.PanelUserStatus `json:"status"`
	TargetCount int                     `json:"target_count"`
}

type capabilityResponse struct {
	Read              bool `json:"read"`
	Write             bool `json:"write"`
	ManageTargetUsers bool `json:"manage_target_users"`
}

type panelUserResponse struct {
	Account      accountResponse         `json:"account"`
	SystemRole   storage.SystemRole      `json:"system_role"`
	Status       storage.PanelUserStatus `json:"status"`
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
	Role             *storage.InstallationRole `json:"role"`
	Suspended        bool                      `json:"suspended"`
	SuspensionReason *string                   `json:"suspension_reason,omitempty"`
	Revision         int64                     `json:"revision"`
	UpdatedAt        *time.Time                `json:"updated_at,omitempty"`
	EffectiveRole    storage.InstallationRole  `json:"effective_role"`
	Source           storage.AccessSource      `json:"source"`
	Capabilities     capabilityResponse        `json:"capabilities"`
}

type targetResponse struct {
	ID                                  string                          `json:"id"`
	InstallationID                      string                          `json:"installation_id"`
	Type                                storage.TargetKind              `json:"type"`
	Account                             accountResponse                 `json:"account"`
	RepositoryDefaultEnabled            bool                            `json:"repository_default_enabled"`
	PendingCIModeDefault                storage.PendingCIMode           `json:"pending_ci_mode_default"`
	PendingCIBranchPatternsDefault      storage.PendingCIBranchPatterns `json:"pending_ci_branch_patterns_default"`
	PendingCIQuietPeriodSecondsOverride *int64                          `json:"pending_ci_quiet_period_seconds_override"`
	// What this installation would use if it set nothing: the value the running
	// service resolved, never null. A panel that only knew "nothing is set here"
	// had to invent a prefill, and invented the same one on every deployment.
	PendingCIQuietPeriodSecondsInherited int64                        `json:"pending_ci_quiet_period_seconds_inherited"`
	PathIndexIntervalSecondsOverride     *int64                       `json:"path_index_interval_seconds_override"`
	PathIndexIntervalSecondsInherited    int64                        `json:"path_index_interval_seconds_inherited"`
	PendingCIPermissions                 pendingCIPermissionsResponse `json:"pending_ci_permissions"`
	ConfigPatch                          config.Patch                 `json:"config_patch"`
	InheritedConfig                      config.Config                `json:"inherited_config"`
	EffectiveConfig                      config.Config                `json:"effective_config"`
	ConfigSources                        map[string]config.Source     `json:"config_sources"`
	Revision                             int64                        `json:"revision"`
	RepositoryCounts                     repositoryCountsResponse     `json:"repository_counts"`
	EffectiveRole                        storage.InstallationRole     `json:"effective_role"`
	AccessSource                         storage.AccessSource         `json:"access_source"`
	Capabilities                         capabilityResponse           `json:"capabilities"`
	SuspensionReason                     *string                      `json:"suspension_reason,omitempty"`
}

type pendingCIPermissionsResponse struct {
	ChecksWrite         bool `json:"checks_write"`
	AdministrationWrite bool `json:"administration_write"`
	MergeQueuesRead     bool `json:"merge_queues_read"`
	CommitStatusesRead  bool `json:"commit_statuses_read"`
}

type pendingCIGateResponse struct {
	DesiredMode   storage.PendingCIMode          `json:"desired_mode"`
	EffectiveMode storage.PendingCIEffectiveMode `json:"effective_mode"`
	Readiness     storage.PendingCIReadiness     `json:"readiness"`
	Reason        string                         `json:"reason"`
	AppID         *int64                         `json:"app_id,omitempty"`
	RulesetID     *int64                         `json:"ruleset_id,omitempty"`
}

// The repository tallies, named the way every other field on this wire is named.
//
// storage.RepositoryCounts used to go out as itself. It carries no struct tags -
// nothing below the port needs any - so it marshalled under its Go field names, and
// the panel, reading the lower-case names it uses everywhere else, got undefined for
// each of them. Two undefined numbers added together are what put "of NaN enabled"
// on the Root console's installations table. The development fixture spelled them
// the way the panel reads them, so the page was only ever wrong against the service.
type repositoryCountsResponse struct {
	Total    int `json:"total"`
	Enabled  int `json:"enabled"`
	Disabled int `json:"disabled"`
}

func newRepositoryCountsResponse(counts storage.RepositoryCounts) repositoryCountsResponse {
	return repositoryCountsResponse{
		Total:    counts.Total,
		Enabled:  counts.Enabled,
		Disabled: counts.Disabled,
	}
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
	PendingCIMode       storage.PendingCIMode        `json:"pending_ci_mode"`
	PendingCIModeSource string                       `json:"pending_ci_mode_source"`
	ConfigOverrideCount int                          `json:"config_override_count"`
	ConfigFileStatus    storage.RepositoryFileStatus `json:"config_file_status"`
	UpdatedAt           time.Time                    `json:"updated_at"`
}

type repositoryDetailResponse struct {
	Repository                           repositorySummaryResponse        `json:"repository"`
	ConfigPatch                          config.Patch                     `json:"config_patch"`
	InheritedConfig                      config.Config                    `json:"inherited_config"`
	EffectiveConfig                      config.Config                    `json:"effective_config"`
	ConfigSources                        map[string]config.Source         `json:"config_sources"`
	ConfigFilePatch                      config.Patch                     `json:"config_file_patch"`
	ConfigFileError                      *string                          `json:"config_file_error,omitempty"`
	ConfigFilePath                       string                           `json:"config_file_path,omitempty"`
	ConfigFileSuperseded                 []string                         `json:"config_file_superseded,omitempty"`
	ConfigMigration                      storage.ConfigMigrationState     `json:"config_migration"`
	ConfigMigrationPR                    *int                             `json:"config_migration_pr,omitempty"`
	IgnoreRepositoryFile                 bool                             `json:"ignore_repository_file"`
	PendingCIModeOverride                *storage.PendingCIMode           `json:"pending_ci_mode_override"`
	PendingCIModeInherited               storage.PendingCIMode            `json:"pending_ci_mode_inherited"`
	PendingCIBranchPatternsOverride      *storage.PendingCIBranchPatterns `json:"pending_ci_branch_patterns_override"`
	PendingCIBranchPatternsInherited     storage.PendingCIBranchPatterns  `json:"pending_ci_branch_patterns_inherited"`
	PendingCIQuietPeriodSecondsOverride  *int64                           `json:"pending_ci_quiet_period_seconds_override"`
	PendingCIQuietPeriodSecondsInherited int64                            `json:"pending_ci_quiet_period_seconds_inherited"`
	PathIndexIntervalSecondsOverride     *int64                           `json:"path_index_interval_seconds_override"`
	PathIndexIntervalSecondsInherited    int64                            `json:"path_index_interval_seconds_inherited"`
	PendingCIGate                        *pendingCIGateResponse           `json:"pending_ci_gate,omitempty"`
	Revision                             int64                            `json:"revision"`
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
		Account:     accountDTO(user.Account),
		SystemRole:  user.SystemRole,
		Status:      user.Status,
		TargetCount: targetCount,
	}
}

func panelUserDTO(user storage.PanelUser, manageable bool) panelUserResponse {
	return panelUserResponse{
		Account: accountDTO(user.Account), SystemRole: user.SystemRole, Status: user.Status,
		BanReason: user.BanReason, BannedAt: user.BannedAt, LastLoginAt: user.LastLoginAt,
		Revision: user.Revision, CreatedAt: user.CreatedAt,
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
	}
}

// targetDTO renders one installation.
//
// It takes the whole of the running service's settings rather than its bot
// config alone, because two of the durations below cascade through this level:
// what an installation inherits is what the process resolved, and a panel told
// only "nothing is set here" has to invent a number to prefill - which is how
// every page below Root came to show one hour whatever the deployment ran.
func targetDTO(
	runtime RuntimeValues,
	target storage.Target,
	access storage.TargetAccess,
) targetResponse {
	process := runtime.BotConfig
	inherited := config.Resolve(process)
	resolved := config.Resolve(process, config.Layer{
		Source: config.SourceTarget,
		Patch:  target.ConfigPatch,
	})

	return targetResponse{
		ID:                                  target.ID,
		InstallationID:                      target.InstallationID,
		Type:                                target.Kind,
		Account:                             accountDTO(target.Account),
		RepositoryDefaultEnabled:            target.RepositoryDefaultEnabled,
		PendingCIModeDefault:                target.PendingCIModeDefault,
		PendingCIBranchPatternsDefault:      target.PendingCIBranchPatternsDefault,
		PendingCIQuietPeriodSecondsOverride: durationSecondsDTO(target.PendingCIQuietPeriodOverride),
		PendingCIQuietPeriodSecondsInherited: int64(
			runtime.PendingCIQuietPeriod / time.Second,
		),
		PathIndexIntervalSecondsOverride: durationSecondsDTO(target.PathIndexIntervalOverride),
		PathIndexIntervalSecondsInherited: int64(
			runtime.PathIndexInterval / time.Second,
		),
		PendingCIPermissions: pendingCIPermissionsResponse{
			ChecksWrite:         target.Grants("checks"),
			AdministrationWrite: target.Grants("administration"),
			MergeQueuesRead:     target.CanRead("merge_queues"),
			CommitStatusesRead:  target.CanRead("statuses"),
		},
		ConfigPatch:      target.ConfigPatch,
		InheritedConfig:  inherited.Values,
		EffectiveConfig:  resolved.Values,
		ConfigSources:    resolved.Sources,
		Revision:         target.Revision,
		RepositoryCounts: newRepositoryCountsResponse(target.RepositoryCounts),
		EffectiveRole:    access.Role,
		AccessSource:     access.Source,
		Capabilities:     capabilitiesDTO(access.Capabilities),
		SuspensionReason: access.SuspensionReason,
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
	mode := target.PendingCIModeDefault
	modeSource := "target"
	if repository.PendingCIModeOverride != nil {
		mode = *repository.PendingCIModeOverride
		modeSource = "repository"
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
		PendingCIMode:       mode,
		PendingCIModeSource: modeSource,
		ConfigOverrideCount: patchSize(repository.ConfigPatch),
		ConfigFileStatus:    repository.ConfigFileStatus,
		UpdatedAt:           repository.UpdatedAt,
	}
}

// repositoryDetailDTO renders one repository.
//
// Like `targetDTO`, it takes the whole of the running service's settings: a
// duration this repository inherits is resolved through every level above it,
// so a repository under an installation that sets nothing inherits what the
// process runs with rather than nothing at all.
func repositoryDetailDTO(
	runtime RuntimeValues,
	target storage.Target,
	repository storage.Repository,
) repositoryDetailResponse {
	process := runtime.BotConfig
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
		Repository:                          repositorySummaryDTO(target, repository),
		ConfigPatch:                         repository.ConfigPatch,
		InheritedConfig:                     inherited.Values,
		EffectiveConfig:                     resolved.Values,
		ConfigSources:                       resolved.Sources,
		ConfigFilePatch:                     repository.ConfigFilePatch,
		ConfigFileError:                     repository.ConfigFileError,
		ConfigFilePath:                      repository.ConfigFilePath,
		ConfigFileSuperseded:                repository.ConfigFileSuperseded,
		ConfigMigration:                     migrationState(repository.ConfigMigration),
		ConfigMigrationPR:                   repository.ConfigMigrationPR,
		IgnoreRepositoryFile:                repository.IgnoreRepositoryFile,
		PendingCIModeOverride:               repository.PendingCIModeOverride,
		PendingCIModeInherited:              target.PendingCIModeDefault,
		PendingCIBranchPatternsOverride:     repository.PendingCIBranchPatternsOverride,
		PendingCIBranchPatternsInherited:    target.PendingCIBranchPatternsDefault,
		PendingCIQuietPeriodSecondsOverride: durationSecondsDTO(repository.PendingCIQuietPeriodOverride),
		PendingCIQuietPeriodSecondsInherited: inheritedSecondsDTO(
			target.PendingCIQuietPeriodOverride, runtime.PendingCIQuietPeriod,
		),
		PathIndexIntervalSecondsOverride: durationSecondsDTO(repository.PathIndexIntervalOverride),
		PathIndexIntervalSecondsInherited: inheritedSecondsDTO(
			target.PathIndexIntervalOverride, runtime.PathIndexInterval,
		),
		PendingCIGate: pendingCIGateDTO(repository.PendingCIGate),
		Revision:      repository.Revision,
	}
}

func durationSecondsDTO(value *time.Duration) *int64 {
	if value == nil {
		return nil
	}
	seconds := int64(*value / time.Second)

	return &seconds
}

// inheritedSecondsDTO is what a level would use if it set nothing: the nearest
// level above that did, or what the running service resolved.
//
// Never null, which is the whole point. The panel used to be told only whether
// the level above had an override, so a repository under an installation that
// set nothing had to invent its prefill - and invented the same hour whether
// the deployment ran with fifteen minutes or a day.
func inheritedSecondsDTO(above *time.Duration, process time.Duration) int64 {
	if above != nil {
		return int64(*above / time.Second)
	}

	return int64(process / time.Second)
}

func pendingCIGateDTO(gate *storage.PendingCIRepositoryGate) *pendingCIGateResponse {
	if gate == nil {
		return nil
	}

	return &pendingCIGateResponse{
		DesiredMode: gate.DesiredMode, EffectiveMode: gate.EffectiveMode,
		Readiness: gate.Readiness, Reason: gate.Reason,
		AppID: gate.AppID, RulesetID: gate.RulesetID,
	}
}

// migrationState fills in the state a repository written before this column
// existed carries, so the panel never has to read an empty string as a state.
func migrationState(state storage.ConfigMigrationState) storage.ConfigMigrationState {
	if state == "" {
		return storage.ConfigMigrationNone
	}

	return state
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
		items = append(items, failureDTO(failure))
	}

	return pageResponse[failureResponse]{
		Items: items, NextCursor: offsetCursor(page.NextOffset), Total: page.Total,
	}
}

func failureDTO(failure storage.DeliveryFailure) failureResponse {
	return failureResponse{
		ID: strconv.FormatInt(failure.ID, 10), DeliveryID: failure.DeliveryID,
		RepositoryFullName: failure.RepositoryFullName, Event: failure.Event,
		Stage: failure.Stage, Reason: failure.Reason,
		Retryable: failure.Retryable, OccurredAt: failure.OccurredAt,
	}
}

func offsetCursor(value int) *string {
	if value == 0 {
		return nil
	}
	formatted := strconv.Itoa(value)

	return &formatted
}

// patchSize is how many settings a layer speaks to, which the panel shows as
// the override count beside a repository.
//
// This used to enumerate the fields, and it had drifted: Runner was missing, so
// a repository overriding only its runner counted as overriding nothing. Asking
// the patch is what stops that happening again to the next field added.
func patchSize(patch config.Patch) int {
	return len(patch.SetKeys())
}

var auditHistoryOrders = []storage.HistoryOrder{
	storage.HistoryNewest,
	storage.HistoryOldest,
	storage.HistoryActorAscending,
	storage.HistoryActorDescending,
	storage.HistoryTargetAscending,
	storage.HistoryTargetDescending,
	storage.HistoryChangeAscending,
	storage.HistoryChangeDescending,
}

var failureHistoryOrders = []storage.HistoryOrder{
	storage.HistoryNewest,
	storage.HistoryOldest,
	storage.HistoryStatusAscending,
	storage.HistoryStatusDescending,
	storage.HistoryRepositoryAscending,
	storage.HistoryRepositoryDescending,
}

func parseHistoryPage(
	values url.Values,
	allowedOrders ...storage.HistoryOrder,
) (storage.HistoryPageRequest, error) {
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
	order := storage.HistoryOrder(values.Get("sort"))
	if order == "" {
		order = storage.HistoryNewest
	}
	if !slices.Contains(allowedOrders, order) {
		return storage.HistoryPageRequest{}, fmt.Errorf("invalid history sort order")
	}
	page.Order = order

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
	case storage.RepositoryNameDescending,
		storage.RepositoryFileAscending,
		storage.RepositoryFileDescending,
		storage.RepositoryOverridesAscending,
		storage.RepositoryOverridesDescending,
		storage.RepositoryNewest,
		storage.RepositoryOldest:
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
