// Package storage defines the persistence capabilities used by Smyklot's
// panel and long-running service.
package storage

import (
	"context"
	"time"
)

// AuthStore persists short-lived authentication records and panel identities.
type AuthStore interface {
	UpsertAccount(context.Context, Account) error
	GetAccount(context.Context, string) (Account, error)
	ClaimOwner(context.Context, string) (bool, error)
	IsOwner(context.Context, string) (bool, error)
	CreateSession(context.Context, Session, int) error
	GetSession(context.Context, string, time.Time) (Session, error)
	DeleteSession(context.Context, string) error
	RevokeAccountSessions(context.Context, string, string, string, time.Time) ([]string, error)
	DeleteExpiredAuth(context.Context, time.Time) error
}

// AccessStore owns panel-user roles and resolves installation permissions.
type AccessStore interface {
	GetPanelUser(context.Context, string) (PanelUser, error)
	ListPanelUsers(context.Context) ([]PanelUser, error)
	ListPanelUserPage(context.Context, PanelUserPageRequest) (PanelUserPage, error)
	ListTargetPanelUsers(context.Context, string) ([]TargetPanelUser, error)
	ListTargetPanelUserPage(context.Context, string, PanelUserPageRequest) (TargetPanelUserPage, error)
	ListAccessDecisions(context.Context, string, *string, int) ([]AccessDecision, error)
	CreatePanelUser(context.Context, PanelUserCreate) (PanelUser, error)
	UpdatePanelUser(context.Context, PanelUserChange) (PanelUser, error)
	GetTargetAccessOverride(context.Context, string, string) (TargetAccessOverride, error)
	SetTargetAccess(context.Context, TargetAccessChange) (TargetAccessOverride, error)
	ResolveTargetAccess(context.Context, string, string) (TargetAccess, error)
	ListTargets(context.Context, string) ([]Target, error)
}

// InvitationStore owns identity-locked panel invitations and acceptance.
type InvitationStore interface {
	ListInvitations(context.Context, *string, time.Time) ([]Invitation, error)
	ListInvitationPage(context.Context, *string, time.Time, InvitationPageRequest) (InvitationPage, error)
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
	ListRepositories(context.Context, string) ([]Repository, error)
	ListRepositoryPage(context.Context, string, RepositoryPageRequest) (RepositoryPage, error)
	GetRepository(context.Context, string, string) (Repository, error)
}

// ConfigStore owns atomic panel-setting changes and their audit records.
type ConfigStore interface {
	UpdateTargetSettings(context.Context, TargetSettingsChange) (Target, error)
	UpdateRepositorySettings(context.Context, RepositorySettingsChange) (Repository, error)
	UpdateRepositoryFileState(context.Context, RepositoryFileState) (bool, error)
}

// DeliveryStore owns delivery claims, completion, failure, and retention.
type DeliveryStore interface {
	ClaimDelivery(context.Context, DeliveryClaim) (DeliveryClaimResult, error)
	AbandonDelivery(context.Context, int64) error
	CompleteDelivery(context.Context, int64, time.Time) error
	FailDelivery(context.Context, DeliveryFailureChange) error
	RecoverRunningDeliveries(context.Context, time.Time) error
	ListFailures(context.Context, string, FailurePageRequest) (FailurePage, error)
	PruneDeliveries(context.Context, time.Time) error
}

// AuditReader reads immutable mutation history.
type AuditReader interface {
	ListAudit(context.Context, string, AuditPageRequest) (AuditPage, error)
}

// Store is the complete persistence capability needed by the service. It does
// not expose SQL handles or transactions to callers.
type Store interface {
	AuthStore
	AccessStore
	InvitationStore
	CatalogStore
	ConfigStore
	DeliveryStore
	AuditReader

	Ping(context.Context) error
	Close() error
}
