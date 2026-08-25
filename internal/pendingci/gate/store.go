package gate

import (
	"context"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

type Store interface {
	GetArmed(context.Context, string, int) (pendingci.Request, error)
	Wake(context.Context, pendingci.WakeRequest) (bool, error)
	WakeByHead(context.Context, pendingci.WakeHeadRequest) (int64, error)
	RecordDraftTransition(
		context.Context,
		pendingci.DraftTransitionRequest,
	) (pendingci.DraftTransitionResult, error)
	FinishPR(context.Context, pendingci.FinishPRRequest) (*pendingci.Request, error)
	Reauthorize(context.Context, pendingci.ReauthorizeRequest) (*pendingci.Request, error)
	DrainLegacy(context.Context, pendingci.LegacyDrainRequest) (pendingci.LegacyDrainResult, error)
	HasPendingCleanup(context.Context, pendingci.CleanupFilter) (bool, error)

	GetCheckSlot(context.Context, int64) (pendingci.CheckSlot, error)
	GetCheckSlotByHead(context.Context, string, string) (pendingci.CheckSlot, error)

	GetPendingCIRepositoryGate(context.Context, string) (storage.PendingCIRepositoryGate, error)

	GetTarget(context.Context, string) (storage.Target, error)
	GetRepository(context.Context, string, string) (storage.Repository, error)
}
