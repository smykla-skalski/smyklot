package storage

import (
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

// SaveInstallationSettingsRequest is one atomic installation-scoped settings
// save. Every resource shares the same actor, authorization proof, and clock.
type SaveInstallationSettingsRequest struct {
	TargetID         string
	ActorAccountID   string
	ElevationID      *string
	SessionTokenHash string
	ChangedAt        time.Time
	Target           *InstallationTargetSettingsChange
	Repositories     []InstallationRepositorySettingsChange
	SyncConfigs      []InstallationSyncConfigChange
	SyncOverrides    []InstallationSyncOverrideChange
}

// InstallationSyncConfigChange replaces one installation Sync kind. Document
// is exact JSON text because its bytes participate in the Sync digest.
type InstallationSyncConfigChange struct {
	Kind             orgsync.Kind
	Enabled          bool
	Document         []byte
	ExpectedRevision int64
}

// InstallationSyncOverrideChange replaces one repository's answer for one
// Sync kind. An empty Document is stored canonically as an empty JSON object.
type InstallationSyncOverrideChange struct {
	RepositoryID     string
	Kind             orgsync.Kind
	Enabled          *bool
	Document         []byte
	ExpectedRevision int64
}

// InstallationTargetSettingsChange replaces every panel-owned target setting.
type InstallationTargetSettingsChange struct {
	RepositoryDefaultEnabled       bool
	PendingCIModeDefault           PendingCIMode
	PendingCIBranchPatternsDefault PendingCIBranchPatterns
	PendingCIQuietPeriodOverride   *time.Duration
	PathIndexIntervalOverride      *time.Duration
	ConfigPatch                    config.Patch
	ExpectedRevision               int64
	RetunePendingCIQuietPeriod     bool
	DeploymentPendingCIQuietPeriod time.Duration
}

// InstallationRepositorySettingsChange replaces every panel-owned setting for
// one repository. A batch may contain several repositories, each at its own
// optimistic revision.
type InstallationRepositorySettingsChange struct {
	RepositoryID                    string
	EnabledOverride                 *bool
	PendingCIModeOverride           *PendingCIMode
	PendingCIBranchPatternsOverride *PendingCIBranchPatterns
	PendingCIQuietPeriodOverride    *time.Duration
	PathIndexIntervalOverride       *time.Duration
	ConfigPatch                     config.Patch
	IgnoreRepositoryFile            bool
	ExpectedRevision                int64
	RetunePendingCIQuietPeriod      bool
	DeploymentPendingCIQuietPeriod  time.Duration
}

// SaveInstallationSettingsResult is the state after one atomic save.
// CheckpointID is nil when every requested resource already matched storage.
type SaveInstallationSettingsResult struct {
	Target                 *Target
	Repositories           []Repository
	SyncConfigs            []orgsync.Config
	SyncOverrides          []orgsync.RepositoryOverride
	CheckpointID           *int64
	CatalogSettingsChanged bool
	TargetChanged          bool
	ChangedRepositoryIDs   []string
	SyncChanged            bool
}

// TargetSettingsDocument is the complete restorable target-settings payload
// stored on either side of a checkpoint item.
type TargetSettingsDocument struct {
	RepositoryDefaultEnabled       bool                    `json:"repository_default_enabled"`
	PendingCIModeDefault           PendingCIMode           `json:"pending_ci_mode_default"`
	PendingCIBranchPatternsDefault PendingCIBranchPatterns `json:"pending_ci_branch_patterns_default"`
	PendingCIQuietPeriodOverride   *time.Duration          `json:"pending_ci_quiet_period_override"`
	PathIndexIntervalOverride      *time.Duration          `json:"path_index_interval_override"`
	ConfigPatch                    config.Patch            `json:"config_patch"`
}

// RepositorySettingsDocument is the complete restorable repository-settings
// payload stored on either side of a checkpoint item.
type RepositorySettingsDocument struct {
	EnabledOverride                 *bool                    `json:"enabled_override"`
	PendingCIModeOverride           *PendingCIMode           `json:"pending_ci_mode_override"`
	PendingCIBranchPatternsOverride *PendingCIBranchPatterns `json:"pending_ci_branch_patterns_override"`
	PendingCIQuietPeriodOverride    *time.Duration           `json:"pending_ci_quiet_period_override"`
	PathIndexIntervalOverride       *time.Duration           `json:"path_index_interval_override"`
	ConfigPatch                     config.Patch             `json:"config_patch"`
	IgnoreRepositoryFile            bool                     `json:"ignore_repository_file"`
}

// SyncConfigSettingsDocument is the complete restorable state of one Sync
// kind. Document is JSON encoded as a string so checkpoint serialization keeps
// its exact bytes rather than compacting them and changing the Sync digest.
type SyncConfigSettingsDocument struct {
	Enabled  bool   `json:"enabled"`
	Document string `json:"document"`
}

// SyncOverrideSettingsDocument is the complete restorable repository answer
// for one Sync kind. Empty input is represented by the canonical "{}" text.
type SyncOverrideSettingsDocument struct {
	Enabled  *bool  `json:"enabled"`
	Document string `json:"document"`
}
