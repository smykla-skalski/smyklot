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
	DeleteExpiredAuth(context.Context, time.Time) error
}

// CatalogStore persists GitHub-owned installation and repository snapshots.
type CatalogStore interface {
	ReconcileCatalog(context.Context, []InstallationSnapshot) error
	ReconcileInstallation(context.Context, InstallationSnapshot) error
	ReplaceAccountAccess(context.Context, string, []string, time.Time) error
	ReplaceOwnerAccess(context.Context, []string, time.Time) error
	GrantOwnerAccess(context.Context, string, time.Time) error
	ListTargets(context.Context, string) ([]Target, error)
	CanAccessTarget(context.Context, string, string) (bool, error)
	GetTarget(context.Context, string) (Target, error)
	ListRepositories(context.Context, string) ([]Repository, error)
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
	CatalogStore
	ConfigStore
	DeliveryStore
	AuditReader

	Ping(context.Context) error
	Close() error
}
