package apply

import (
	"context"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

type Store interface {
	ListSyncConfigs(context.Context, string) ([]orgsync.Config, error)
	ListSyncRepositoryOverrides(context.Context, string) ([]orgsync.RepositoryOverride, error)
	ListSyncRepositoryState(context.Context, string) ([]orgsync.RepositoryState, error)
	RecordSyncRepositoryState(context.Context, []orgsync.RepositoryState) error

	GetLiveSyncPlan(context.Context, string) (orgsync.Plan, []orgsync.Action, error)
	CreateSyncPlan(context.Context, orgsync.PlanCreate) (orgsync.Plan, error)
	LeaseSyncPlan(context.Context, time.Time, time.Time) (orgsync.PlanLease, error)
	RetrySyncPlan(context.Context, orgsync.PlanRetry) error
	FinishSyncPlan(context.Context, orgsync.PlanOutcome) error
	InvalidateSyncPlans(context.Context, string, time.Time) error
	RecordSyncActionOutcome(context.Context, orgsync.ActionOutcome) error
	RecordSyncAudit(context.Context, orgsync.AuditEntry) error

	ListSyncRepositoryPathScans(context.Context, string) ([]orgsync.RepositoryPathScan, error)
	SetSyncRepositoryPaths(context.Context, orgsync.RepositoryPaths) error
	TouchSyncRepositoryPaths(context.Context, string, time.Time) error
	PruneSyncRepositoryPaths(context.Context, string) (int64, error)

	GetTarget(context.Context, string) (storage.Target, error)
	GetRepository(context.Context, string, string) (storage.Repository, error)
	ListRepositories(context.Context, string) ([]storage.Repository, error)
}
