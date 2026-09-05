// Package storage defines the persistence capabilities used by Smyklot's
// panel and long-running service.
package storage

import (
	"context"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

// AuthStore persists short-lived authentication records and panel identities.
type AuthStore interface {
	UpsertAccount(context.Context, Account) error
	GetAccount(context.Context, string) (Account, error)
	ReconcileSuperRoot(context.Context, string, time.Time) error
	ActivateDerivedOwner(context.Context, string, time.Time) (bool, error)
	CreateSession(context.Context, Session, int) error
	GetSession(context.Context, string, time.Time) (Session, error)
	// ExtendSession moves a live session's expiry out, never in, and never
	// revives one that has already ended.
	ExtendSession(ctx context.Context, tokenHash string, expiresAt, now time.Time) error
	DeleteSession(context.Context, string, ElevationEndReason, time.Time) error
	RevokeAccountSessions(context.Context, string, string, string, time.Time) ([]string, error)
	DeleteExpiredAuth(context.Context, time.Time) error
}

// AccessStore owns panel-user roles and resolves installation permissions.
type AccessStore interface {
	GetPanelUser(context.Context, string) (PanelUser, error)
	ListPanelUsers(context.Context) ([]PanelUser, error)
	ListRootPanelUserPage(context.Context, RootPanelUserPageRequest) (RootPanelUserPage, error)
	UpdateSystemRole(context.Context, SystemRoleChange) (PanelUser, error)
	ListTargetPanelUsers(context.Context, string, time.Time) ([]TargetPanelUser, error)
	ListTargetPanelUserPage(context.Context, string, time.Time, PanelUserPageRequest) (TargetPanelUserPage, error)
	ListAccessDecisions(context.Context, string, *string, int) ([]AccessDecision, error)
	CreatePanelUser(context.Context, PanelUserCreate) (PanelUser, error)
	UpdatePanelUser(context.Context, PanelUserChange) (PanelUser, error)
	GetTargetAccessOverride(context.Context, string, string) (TargetAccessOverride, error)
	SetTargetAccess(context.Context, TargetAccessChange) (TargetAccessOverride, error)
	ResolveTargetAccess(context.Context, string, string, time.Time) (TargetAccess, error)
	ListTargets(context.Context, string, time.Time) ([]Target, error)
}

// InvitationStore owns identity-locked panel invitations and acceptance.
type InvitationStore interface {
	ListInvitations(context.Context, *string, time.Time) ([]Invitation, error)
	ListInvitationPage(context.Context, *string, time.Time, InvitationPageRequest) (InvitationPage, error)
	ListRootInvitationPage(context.Context, time.Time, InvitationPageRequest) (InvitationPage, error)
	GetInvitation(context.Context, string, time.Time) (Invitation, error)
	GetInvitationByToken(context.Context, string, time.Time) (Invitation, error)
	CreateInvitation(context.Context, InvitationCreate) (Invitation, error)
	ReissueInvitation(context.Context, InvitationReissue) (Invitation, error)
	RevokeInvitation(context.Context, InvitationRevoke) (Invitation, error)
	RespondToInvitation(context.Context, InvitationResponse) (Invitation, error)
}

// CatalogStore persists GitHub-owned installation and repository snapshots.
type CatalogStore interface {
	ReconcileCatalog(context.Context, []InstallationSnapshot) error
	ReconcileInstallation(context.Context, InstallationSnapshot) error
	GetTarget(context.Context, string) (Target, error)
	ListRootTargets(context.Context) ([]Target, error)
	ListRepositories(context.Context, string) ([]Repository, error)
	ListRepositoryPage(context.Context, string, RepositoryPageRequest) (RepositoryPage, error)
	GetRepository(context.Context, string, string) (Repository, error)
}

// ConfigStore owns atomic panel-setting changes and their audit records.
type ConfigStore interface {
	SaveInstallationSettings(context.Context, SaveInstallationSettingsRequest) (SaveInstallationSettingsResult, error)
	InspectInstallationSettingsCheckpoint(
		context.Context,
		SettingsCheckpointRef,
	) (SettingsCheckpointInspection, error)
	InspectInstallationSettingsBaseline(
		context.Context,
		string,
	) (SettingsCheckpointInspection, error)
	RestoreInstallationSettings(
		context.Context,
		RestoreInstallationSettingsRequest,
	) (SaveInstallationSettingsResult, error)
	UpdateRepositoryFileState(context.Context, RepositoryFileState) (bool, error)
	SetRepositoryConfigMigration(context.Context, RepositoryConfigMigration) error
	GetRuntimeSettings(context.Context) (RuntimeSettings, error)
	SaveRuntimeSettings(context.Context, RuntimeSettingsChange) (SaveRuntimeSettingsResult, error)
	InspectRootSettingsCheckpoint(
		context.Context,
		SettingsCheckpointRef,
	) (SettingsCheckpointInspection, error)
	InspectRootSettingsBaseline(context.Context) (SettingsCheckpointInspection, error)
	RestoreRuntimeSettings(
		context.Context,
		RestoreRuntimeSettingsRequest,
	) (SaveRuntimeSettingsResult, error)
}

// PendingCIGateStore owns the desired/effective repository protection
// transition. Saved settings may exist while GitHub permissions are missing;
// only a ready effective mode may accept new merge-after-CI commands.
type PendingCIGateStore interface {
	GetPendingCIRepositoryGate(context.Context, string) (PendingCIRepositoryGate, error)
	ListTargetPendingCIRepositoryGates(context.Context, string) ([]PendingCIRepositoryGate, error)
	ListPendingCIRepositoryGates(context.Context, int) ([]PendingCIRepositoryGate, error)
	UpdatePendingCIRepositoryGate(
		context.Context,
		PendingCIGateChange,
	) (PendingCIRepositoryGate, error)
}

// DeliveryStore owns delivery claims, completion, failure, and retention.
type DeliveryStore interface {
	ClaimDelivery(context.Context, DeliveryClaim) (DeliveryClaimResult, error)
	AbandonDelivery(context.Context, int64) error
	LeaseDelivery(context.Context, time.Time, time.Time) (DeliveryLeaseResult, error)
	RetryDelivery(context.Context, DeliveryRetryChange) error
	CompleteDelivery(context.Context, int64, time.Time) error
	FailDelivery(context.Context, DeliveryFailureChange) error
	RecoverRunningDeliveries(context.Context, time.Time) error
	ListFailures(context.Context, string, FailurePageRequest) (FailurePage, error)
	ListRootFailures(context.Context, FailurePageRequest) (RootFailurePage, error)
	PruneDeliveries(context.Context, time.Time) error
}

// AuditReader reads immutable mutation history.
type AuditReader interface {
	ListAudit(context.Context, string, AuditPageRequest) (AuditPage, error)
	ListRootAudit(context.Context, RootAuditPageRequest) (RootAuditPage, error)
}

// SecurityStore owns Root elevation grants and Owner notifications.
type SecurityStore interface {
	GetRootOverview(context.Context, string, time.Time) (RootOverview, error)
	BeginElevation(context.Context, ElevationGrant) (Elevation, error)
	GetElevation(context.Context, string, string, time.Time) (Elevation, error)
	EndElevation(context.Context, string, string, ElevationEndReason, time.Time) (Elevation, error)
	EndSessionElevations(context.Context, string, ElevationEndReason, time.Time) error
	ListSecurityNotifications(context.Context, string, NotificationPageRequest) (NotificationPage, error)
	MarkSecurityNotificationRead(context.Context, string, int64, time.Time) (SecurityNotification, error)
	// MarkAllSecurityNotificationsRead clears one recipient's whole inbox and answers how
	// many were unread. A count rather than the rows: the caller is a control that empties
	// a list, not one that reads it.
	MarkAllSecurityNotificationsRead(context.Context, string, time.Time) (int, error)
}

// PreferenceStore owns per-account synced panel preferences.
type PreferenceStore interface {
	GetPreferences(context.Context, string) (Preferences, error)
	ApplyPreferences(context.Context, PreferenceChange) (Preferences, error)
}

// Store is the complete persistence capability needed by the service. It does
// not expose SQL handles or transactions to callers.
type Store interface {
	AuthStore
	AccessStore
	InvitationStore
	CatalogStore
	ConfigStore
	PendingCIGateStore
	DeliveryStore
	AuditReader
	SecurityStore
	PreferenceStore
	SampleStore
	pendingci.Store
	pendingci.CheckStore
	orgsync.Store
	workqueue.Store

	Ping(context.Context) error

	// Status describes the database behind the port. It returns no error
	// because a database that will not answer is a status worth reading, not a
	// failure to produce one.
	Status(context.Context) DatabaseStatus

	Close() error
}
