package storage

import (
	"encoding/json"
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

// SystemRole is an account-wide Smyklot administration role, modeled
// separately from installation access.
type SystemRole string

const (
	SystemRoleNone      SystemRole = "none"
	SystemRoleRoot      SystemRole = "root"
	SystemRoleSuperRoot SystemRole = "super_root"
)

// IsRoot reports whether the role can enter Root administration.
func (role SystemRole) IsRoot() bool {
	return role == SystemRoleRoot || role == SystemRoleSuperRoot
}

// InstallationRole is a user's authorization level within one installation.
type InstallationRole string

const (
	InstallationRoleNone   InstallationRole = "none"
	InstallationRoleViewer InstallationRole = "viewer"
	InstallationRoleEditor InstallationRole = "editor"
	InstallationRoleAdmin  InstallationRole = "admin"
	InstallationRoleOwner  InstallationRole = "owner"
)

// PanelUserStatus is the account-wide lifecycle of a panel user.
type PanelUserStatus string

const (
	PanelUserActive  PanelUserStatus = "active"
	PanelUserBanned  PanelUserStatus = "banned"
	PanelUserRemoved PanelUserStatus = "removed"
)

// PanelUserOrder controls how user-management pages are ordered.
type PanelUserOrder string

const (
	PanelUserNameAscending  PanelUserOrder = "name_asc"
	PanelUserNameDescending PanelUserOrder = "name_desc"
	PanelUserRoleAscending  PanelUserOrder = "role_asc"
	PanelUserRoleDescending PanelUserOrder = "role_desc"
	PanelUserUpdatedNewest  PanelUserOrder = "updated_newest"
	PanelUserUpdatedOldest  PanelUserOrder = "updated_oldest"
	PanelUserLoginNewest    PanelUserOrder = "login_newest"
	PanelUserLoginOldest    PanelUserOrder = "login_oldest"
)

// PanelUserListState includes the installation-only suspended state alongside
// account-wide lifecycle states used by user-management filters.
type PanelUserListState string

const (
	PanelUserListActive    PanelUserListState = "active"
	PanelUserListBanned    PanelUserListState = "banned"
	PanelUserListSuspended PanelUserListState = "suspended"
)

// PanelUserPageRequest selects one filtered, numbered user-management page.
type PanelUserPageRequest struct {
	Offset int
	Limit  int
	Order  PanelUserOrder
	Query  string
	Roles  []InstallationRole
	States []PanelUserListState
}

// TargetPanelUserPage is one installation-scoped user page.
type TargetPanelUserPage struct {
	Items      []TargetPanelUser
	NextOffset int
	Total      int
}

// PanelUser is one persisted panel identity and its account-wide lifecycle.
type PanelUser struct {
	Account     Account
	SystemRole  SystemRole
	Status      PanelUserStatus
	BanReason   *string
	BannedAt    *time.Time
	RemovedAt   *time.Time
	LastLoginAt *time.Time
	Revision    int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// PanelUserCreate activates a known provider identity and records its actor.
type PanelUserCreate struct {
	AccountID      string
	ActorAccountID string
	ChangedAt      time.Time
}

// PanelUserChange atomically replaces one user's account-wide lifecycle.
type PanelUserChange struct {
	AccountID        string
	ActorAccountID   string
	Status           PanelUserStatus
	BanReason        *string
	ExpectedRevision int64
	ChangedAt        time.Time
}

// InvitationStatus is the lifecycle of one single-use access invitation.
type InvitationStatus string

const (
	InvitationPending  InvitationStatus = "pending"
	InvitationAccepted InvitationStatus = "accepted"
	InvitationDeclined InvitationStatus = "declined"
	InvitationRevoked  InvitationStatus = "revoked"
	InvitationExpired  InvitationStatus = "expired"
)

// InvitationOrder controls invitation-management page ordering.
type InvitationOrder string

const (
	InvitationCreatedNewest  InvitationOrder = "created_newest"
	InvitationCreatedOldest  InvitationOrder = "created_oldest"
	InvitationExpirySoonest  InvitationOrder = "expiry_soonest"
	InvitationExpiryLatest   InvitationOrder = "expiry_latest"
	InvitationNameAscending  InvitationOrder = "name_asc"
	InvitationNameDescending InvitationOrder = "name_desc"
	InvitationRoleAscending  InvitationOrder = "role_asc"
	InvitationRoleDescending InvitationOrder = "role_desc"
)

// InvitationPageRequest selects one filtered invitation-management page.
type InvitationPageRequest struct {
	Offset   int
	Limit    int
	Order    InvitationOrder
	Query    string
	Roles    []InstallationRole
	Statuses []InvitationStatus
}

// InvitationPage is one page of identity-locked invitations.
type InvitationPage struct {
	Items      []Invitation
	NextOffset int
	Total      int
}

// AccessDecision is one immutable role, lifecycle, or invitation decision.
type AccessDecision struct {
	ID        int64
	TargetID  *string
	Actor     Account
	Action    string
	Summary   string
	CreatedAt time.Time
}

// Invitation is an identity-locked offer of a system or installation role.
type Invitation struct {
	ID       string
	Account  Account
	TargetID *string
	// TargetName, TargetLogin and TargetKind describe the installation the offer
	// is scoped to, and are all nil for a system-role offer. The login and kind
	// are what let a reader check the scope against GitHub itself: a display name
	// alone names an organisation without identifying it.
	TargetName  *string
	TargetLogin *string
	TargetKind  *string
	Role        *InstallationRole
	SystemRole  *SystemRole
	Status      InvitationStatus
	ExpiresAt   time.Time
	CreatedBy   Account
	CreatedAt   time.Time
	RespondedAt *time.Time
}

// InvitationCreate creates a new token and invalidates earlier pending offers
// for the same identity and scope.
type InvitationCreate struct {
	ID               string
	TokenHash        string
	AccountID        string
	TargetID         *string
	Role             *InstallationRole
	SystemRole       *SystemRole
	ElevationID      *string
	SessionTokenHash string
	ExpiresAt        time.Time
	CreatedByAccount string
	CreatedAt        time.Time

	// AcknowledgeDeclined lets an offer through to an identity that turned the last one
	// down. Declining is an answer, so asking again is a decision the caller has to make
	// on purpose rather than by pressing the same button twice.
	AcknowledgeDeclined bool
}

// InvitationReissue replaces a pending or expired invitation token.
type InvitationReissue struct {
	ID               string
	TokenHash        string
	ElevationID      *string
	SessionTokenHash string
	ExpiresAt        time.Time
	CreatedByAccount string
	CreatedAt        time.Time
}

// InvitationRevoke invalidates an invitation without deleting its audit trail.
type InvitationRevoke struct {
	ID               string
	ActorAccountID   string
	ElevationID      *string
	SessionTokenHash string
	RevokedAt        time.Time
}

// InvitationResponse accepts or declines an invitation as its named identity.
type InvitationResponse struct {
	TokenHash string
	AccountID string
	Accept    bool
	At        time.Time
}

// TargetPanelUser combines an account identity with one installation policy.
type TargetPanelUser struct {
	User     PanelUser
	Override *TargetAccessOverride
	Access   TargetAccess
}

// AccessSource identifies which policy decided an installation role.
type AccessSource string

const (
	AccessSourceOwner     AccessSource = "owner"
	AccessSourceTarget    AccessSource = "target"
	AccessSourceSuspended AccessSource = "suspended"
	AccessSourceRoot      AccessSource = "root"
	AccessSourceElevation AccessSource = "elevation"
	AccessSourceDenied    AccessSource = "denied"
)

// AccessCapabilities contains server-authoritative actions for one role.
type AccessCapabilities struct {
	Read              bool
	Write             bool
	ManageTargetUsers bool
}

// EffectiveCapabilities returns the fixed capability set for a resolved role.
func EffectiveCapabilities(role InstallationRole) AccessCapabilities {
	capabilities := AccessCapabilities{}
	switch role {
	case InstallationRoleOwner:
		capabilities.Read = true
		capabilities.Write = true
		capabilities.ManageTargetUsers = true
	case InstallationRoleAdmin:
		capabilities.Read = true
		capabilities.Write = true
		capabilities.ManageTargetUsers = true
	case InstallationRoleEditor:
		capabilities.Read = true
		capabilities.Write = true
	case InstallationRoleViewer:
		capabilities.Read = true
	}

	return capabilities
}

// TargetAccess is the effective authorization for one user and installation.
type TargetAccess struct {
	Role             InstallationRole
	Source           AccessSource
	Root             bool
	SuspensionReason *string
	Capabilities     AccessCapabilities
}

// TargetAccessOverride is one persisted installation role and suspension.
// A nil Role means no explicit access. Suspension remains an independent
// overlay so restoring access retains the previous role.
type TargetAccessOverride struct {
	TargetID         string
	AccountID        string
	Role             *InstallationRole
	Suspended        bool
	SuspensionReason *string
	Revision         int64
	UpdatedAt        time.Time
}

// TargetAccessChange atomically replaces one installation override and audit.
type TargetAccessChange struct {
	TargetID         string
	SubjectAccountID string
	ActorAccountID   string
	ElevationID      *string
	SessionTokenHash string
	Role             *InstallationRole
	Suspended        bool
	SuspensionReason *string
	ExpectedRevision int64
	ChangedAt        time.Time
}

// Session is a panel session. TokenHash is a digest of the cookie token.
type Session struct {
	TokenHash    string
	AccountID    string
	CreatedAt    time.Time
	ExpiresAt    time.Time
	RevokedAt    *time.Time
	RevokeCode   *string
	RevokeReason *string
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

// DeliveryHealth summarizes retained webhook failures for one installation.
type DeliveryHealth struct {
	Failed        int
	LastFailureAt *time.Time
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
	DeliveryHealth           DeliveryHealth
	Ownership                TargetOwnership
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

// ConfigMigrationState is how far Smyklot has got with moving a repository's
// configuration file to TOML.
type ConfigMigrationState string

const (
	// ConfigMigrationNone is a repository nobody has asked yet.
	ConfigMigrationNone ConfigMigrationState = "none"

	// ConfigMigrationProposed is a pull request waiting on the repository.
	ConfigMigrationProposed ConfigMigrationState = "proposed"

	// ConfigMigrationDeclined is a pull request somebody closed without
	// merging. It is durable and never expires: asking again would be the bot
	// arguing with a decision a person already made.
	ConfigMigrationDeclined ConfigMigrationState = "declined"

	// ConfigMigrationBlocked is GitHub refusing the work - most often an App
	// that was never granted write access to this repository. Durable for a
	// different reason than a refusal: finding out costs seven requests, and a
	// permission nobody has granted will not appear because the bot asked
	// again twelve times an hour.
	ConfigMigrationBlocked ConfigMigrationState = "blocked"
)

// RepositoryConfigMigration records what came of proposing the move to TOML.
type RepositoryConfigMigration struct {
	TargetID     string
	RepositoryID string
	State        ConfigMigrationState

	// PullRequest is the proposal, so the panel can link to the thing it is
	// describing rather than to a search for it.
	PullRequest *int

	// ActorAccountID is who decided, when anyone did. The sweep leaves it
	// unset: it observed a pull request rather than choosing anything, and
	// there is no synthetic account to attribute that to.
	ActorAccountID *string

	// ChangedAt stamps the audit entry an actor earns.
	ChangedAt time.Time
}

// Repository is a catalog entry plus its panel-owned controls.
type Repository struct {
	ID                   string
	TargetID             string
	Name                 string
	FullName             string
	Private              bool
	DefaultBranch        string
	Available            bool
	EnabledOverride      *bool
	ConfigPatch          config.Patch
	IgnoreRepositoryFile bool
	ConfigFileStatus     RepositoryFileStatus
	ConfigFilePatch      config.Patch
	ConfigFileError      *string

	// ConfigFilePath is the file the configuration was read from, empty when
	// the repository has none. Discovery looks in four places plus a
	// panel-chosen one, so the status alone no longer says which file it is
	// describing.
	ConfigFilePath string

	// ConfigFileSuperseded are the other paths that also hold a configuration
	// file and were passed over. Nothing reads them; they are here to be shown
	// to a repository that migrated and left the old file behind.
	ConfigFileSuperseded []string

	// ConfigMigration is how far the move to TOML has got.
	ConfigMigration ConfigMigrationState

	// ConfigMigrationPR is the proposal, when there has been one.
	ConfigMigrationPR *int

	Revision  int64
	UpdatedAt time.Time
}

// RepositorySnapshot is GitHub-owned catalog state. Reconciliation must not
// overwrite any local control omitted here.
type RepositorySnapshot struct {
	ID            string
	Name          string
	FullName      string
	Private       bool
	DefaultBranch string
}

// InstallationSnapshot is a complete view of one installation's currently
// available repositories.
type InstallationSnapshot struct {
	TargetID       string
	InstallationID string
	Kind           TargetKind
	Account        Account
	Repositories   []RepositorySnapshot
	Ownership      OwnershipSnapshot
	SyncedAt       time.Time
}

// TargetSettingsChange atomically changes target defaults and records audit.
type TargetSettingsChange struct {
	TargetID                 string
	ActorAccountID           string
	ElevationID              *string
	SessionTokenHash         string
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
	ElevationID          *string
	SessionTokenHash     string
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
	Path         string
	Superseded   []string
	ObservedAt   time.Time
}

// RepositoryOrder controls how repository catalog pages are ordered.
type RepositoryOrder string

const (
	RepositoryNameAscending       RepositoryOrder = "name_asc"
	RepositoryNameDescending      RepositoryOrder = "name_desc"
	RepositoryFileAscending       RepositoryOrder = "file_asc"
	RepositoryFileDescending      RepositoryOrder = "file_desc"
	RepositoryOverridesAscending  RepositoryOrder = "overrides_asc"
	RepositoryOverridesDescending RepositoryOrder = "overrides_desc"
	RepositoryNewest              RepositoryOrder = "newest"
	RepositoryOldest              RepositoryOrder = "oldest"
)

// RepositoryPageRequest selects one filtered page of available repositories.
//
// Offset counts rows from the start of the ordered result instead of seeking
// from a row boundary, because the panel loads consecutive windows into one
// virtualized list. The cost is drift: rows written between two fetches shift
// the window, so a later window can repeat a row already shown or skip one.
// Nothing is lost from storage, only from that one rendered list.
// FailurePageRequest and HistoryPageRequest page the same way.
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
	Payload            []byte
	ClaimedAt          time.Time
}

// DeliveryWork is one durable payload leased to an executor. Attempt starts at
// one and increases each time an expired or explicitly retried lease is taken.
type DeliveryWork struct {
	ID                 int64
	ClaimKey           string
	DeliveryID         string
	TargetID           string
	RepositoryID       *string
	RepositoryFullName string
	Event              string
	Payload            []byte
	Attempt            int
}

// DeliveryLeaseResult contains either ready work or the next instant at which
// the queue should ask again. Both fields are nil when the inbox is empty.
type DeliveryLeaseResult struct {
	Work        *DeliveryWork
	AvailableAt *time.Time
}

// DeliveryRetryChange returns leased work to the durable inbox after a
// transient execution failure.
type DeliveryRetryChange struct {
	ClaimID int64
	Stage   string
	Reason  string
	RetryAt time.Time
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
	HistoryNewest               HistoryOrder = "newest"
	HistoryOldest               HistoryOrder = "oldest"
	HistoryActorAscending       HistoryOrder = "actor_asc"
	HistoryActorDescending      HistoryOrder = "actor_desc"
	HistoryTargetAscending      HistoryOrder = "target_asc"
	HistoryTargetDescending     HistoryOrder = "target_desc"
	HistoryChangeAscending      HistoryOrder = "change_asc"
	HistoryChangeDescending     HistoryOrder = "change_desc"
	HistoryStatusAscending      HistoryOrder = "status_asc"
	HistoryStatusDescending     HistoryOrder = "status_desc"
	HistoryRepositoryAscending  HistoryOrder = "repository_asc"
	HistoryRepositoryDescending HistoryOrder = "repository_desc"
)

// HistoryPageRequest is an offset-based page request. See
// RepositoryPageRequest for why the panel pages by offset and what drifts as a
// result.
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

// AuditChange limits audit history by the kind of configuration mutation.
type AuditChange string

const (
	AuditChangeAll        AuditChange = "all"
	AuditChangeEnablement AuditChange = "enablement"
	AuditChangeRepository AuditChange = "repository"
	AuditChangeAccount    AuditChange = "account"
)

// AuditPageRequest adds mutation scope to common history controls.
type AuditPageRequest struct {
	HistoryPageRequest
	Scope  AuditScope
	Change AuditChange
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

// Preferences is one account's synced panel preference document. Revision 0
// with an empty Values map is the first-class "never stored" state.
type Preferences struct {
	AccountID string
	Values    map[string]json.RawMessage
	Revision  int64
	UpdatedAt time.Time
}

// PreferenceChange merges per-key preference values into an account's
// document. A JSON null (or nil) value deletes the key.
type PreferenceChange struct {
	AccountID string
	Changes   map[string]json.RawMessage
	ChangedAt time.Time
}
