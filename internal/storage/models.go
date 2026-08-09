package storage

import (
	"time"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

// Account is a GitHub identity known to the panel.
type Account struct {
	ID          string
	Provider    string
	SubjectID   string
	Login       string
	DisplayName string
	AvatarURL   *string
	UpdatedAt   time.Time
}

// Session is a panel session. TokenHash is a digest of the cookie token.
type Session struct {
	TokenHash string
	AccountID string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// DeliveryClaimDisposition explains whether a delivery was accepted, is still
// being processed, or has a retained terminal outcome.
type DeliveryClaimDisposition string

const (
	DeliveryClaimAccepted   DeliveryClaimDisposition = "accepted"
	DeliveryClaimInProgress DeliveryClaimDisposition = "in_progress"
	DeliveryClaimRetained   DeliveryClaimDisposition = "retained"
)

// DeliveryClaimResult identifies a newly accepted attempt. ID is populated
// only when Disposition is DeliveryClaimAccepted.
type DeliveryClaimResult struct {
	ID          int64
	Disposition DeliveryClaimDisposition
}

// TargetKind is the GitHub account type that owns an installation.
type TargetKind string

const (
	TargetOrganization TargetKind = "Organization"
	TargetUser         TargetKind = "User"
)

// RepositoryCounts summarizes a target's currently available repositories.
type RepositoryCounts struct {
	Total    int
	Enabled  int
	Disabled int
}

// Target is one GitHub App installation and its panel-owned settings.
type Target struct {
	ID                       string
	InstallationID           string
	Kind                     TargetKind
	Account                  Account
	Available                bool
	RepositoryDefaultEnabled bool
	ConfigPatch              config.Patch
	Revision                 int64
	UpdatedAt                time.Time
	RepositoryCounts         RepositoryCounts
}

// RepositoryFileStatus is the most recently observed state of the repository
// configuration file.
type RepositoryFileStatus string

const (
	RepositoryFileMissing  RepositoryFileStatus = "missing"
	RepositoryFileValid    RepositoryFileStatus = "valid"
	RepositoryFileInvalid  RepositoryFileStatus = "invalid"
	RepositoryFileBypassed RepositoryFileStatus = "bypassed"
)

// Repository is a catalog entry plus its panel-owned controls.
type Repository struct {
	ID                   string
	TargetID             string
	Name                 string
	FullName             string
	Private              bool
	Available            bool
	EnabledOverride      *bool
	ConfigPatch          config.Patch
	IgnoreRepositoryFile bool
	ConfigFileStatus     RepositoryFileStatus
	ConfigFilePatch      config.Patch
	ConfigFileError      *string
	Revision             int64
	UpdatedAt            time.Time
}

// RepositorySnapshot is GitHub-owned catalog state. Reconciliation must not
// overwrite any local control omitted here.
type RepositorySnapshot struct {
	ID       string
	Name     string
	FullName string
	Private  bool
}

// InstallationSnapshot is a complete view of one installation's currently
// available repositories.
type InstallationSnapshot struct {
	TargetID       string
	InstallationID string
	Kind           TargetKind
	Account        Account
	Repositories   []RepositorySnapshot
	SyncedAt       time.Time
}

// TargetSettingsChange atomically changes target defaults and records audit.
type TargetSettingsChange struct {
	TargetID                 string
	ActorAccountID           string
	RepositoryDefaultEnabled bool
	ConfigPatch              config.Patch
	ExpectedRevision         int64
	ChangedAt                time.Time
}

// RepositorySettingsChange atomically changes repository controls and records
// audit.
type RepositorySettingsChange struct {
	TargetID             string
	RepositoryID         string
	ActorAccountID       string
	EnabledOverride      *bool
	ConfigPatch          config.Patch
	IgnoreRepositoryFile bool
	ExpectedRevision     int64
	ChangedAt            time.Time
}

// RepositoryFileState records the last repository-file read without changing
// panel-owned settings or their revision.
type RepositoryFileState struct {
	TargetID     string
	RepositoryID string
	Status       RepositoryFileStatus
	Patch        config.Patch
	Error        *string
	ObservedAt   time.Time
}

// RepositoryOrder controls how repository catalog pages are ordered.
type RepositoryOrder string

const (
	RepositoryNameAscending  RepositoryOrder = "name_asc"
	RepositoryNameDescending RepositoryOrder = "name_desc"
	RepositoryNewest         RepositoryOrder = "newest"
	RepositoryOldest         RepositoryOrder = "oldest"
)

// RepositoryPageRequest selects one filtered page of available repositories.
type RepositoryPageRequest struct {
	Offset             int
	Limit              int
	Order              RepositoryOrder
	Query              string
	EffectiveEnabled   *bool
	FileStatuses       []RepositoryFileStatus
	HasConfigOverrides *bool
	ConfigOverrideKeys []string
}

// RepositoryPage is one page of the repository catalog.
type RepositoryPage struct {
	Items                    []Repository
	NextOffset               int
	Total                    int
	RepositoryDefaultEnabled bool
}

// AuditEntry is one immutable panel mutation.
type AuditEntry struct {
	ID                 int64
	TargetID           string
	RepositoryID       *string
	RepositoryFullName *string
	Actor              Account
	Action             string
	Summary            string
	CreatedAt          time.Time
}

// DeliveryStatus is the lifecycle of a claimed webhook delivery.
type DeliveryStatus string

const (
	DeliveryRunning   DeliveryStatus = "running"
	DeliverySucceeded DeliveryStatus = "succeeded"
	DeliveryFailed    DeliveryStatus = "failed"
)

// DeliveryClaim is the immutable identity captured when work is accepted.
type DeliveryClaim struct {
	ClaimKey           string
	DeliveryID         string
	TargetID           string
	RepositoryID       *string
	RepositoryFullName string
	Event              string
	ClaimedAt          time.Time
}

// DeliveryFailureChange finishes a delivery with a sanitized failure.
type DeliveryFailureChange struct {
	ClaimID   int64
	Stage     string
	Reason    string
	Retryable bool
	FailedAt  time.Time
}

// DeliveryFailure is a persisted, sanitized failure shown to operators.
type DeliveryFailure struct {
	ID                 int64
	DeliveryID         string
	TargetID           string
	RepositoryFullName string
	Event              string
	Stage              string
	Reason             string
	Retryable          bool
	OccurredAt         time.Time
}

// HistoryOrder controls which end of immutable history is read first.
type HistoryOrder string

const (
	HistoryNewest HistoryOrder = "newest"
	HistoryOldest HistoryOrder = "oldest"
)

// HistoryPageRequest is an offset-based page request.
type HistoryPageRequest struct {
	Offset int
	Limit  int
	Order  HistoryOrder
	Query  string
}

// AuditScope limits audit history to account-wide or repository changes.
type AuditScope string

const (
	AuditAll          AuditScope = "all"
	AuditAccount      AuditScope = "account"
	AuditRepositories AuditScope = "repositories"
)

// AuditPageRequest adds mutation scope to common history controls.
type AuditPageRequest struct {
	HistoryPageRequest
	Scope AuditScope
}

// FailurePageRequest adds retryability filtering to common history controls.
type FailurePageRequest struct {
	HistoryPageRequest
	Retryable *bool
}

// AuditPage is one page of immutable audit entries.
type AuditPage struct {
	Items      []AuditEntry
	NextOffset int
	Total      int
}

// FailurePage is one page of delivery failures.
type FailurePage struct {
	Items      []DeliveryFailure
	NextOffset int
	Total      int
}
