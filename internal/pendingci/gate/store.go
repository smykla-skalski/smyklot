package gate

import (
	"context"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

// Store is the part of the durable store the runtime reads and writes
// directly.
//
// It is small because most of what this package persists goes through one of
// the narrower ports beside it - leaseStore, ControlStore, handoffStore and the
// rest, each declared where its one caller lives. What is left is the handful
// of reads the runtime makes on its own account.
type Store interface {
	GetArmed(context.Context, string, int) (pendingci.Request, error)
	Wake(context.Context, pendingci.WakeRequest) (bool, error)
	WakeByHead(context.Context, pendingci.WakeHeadRequest) (int64, error)
	Reauthorize(context.Context, pendingci.ReauthorizeRequest) (*pendingci.Request, error)
	DrainLegacy(context.Context, pendingci.LegacyDrainRequest) (pendingci.LegacyDrainResult, error)
	HasPendingCleanup(context.Context, pendingci.CleanupFilter) (bool, error)

	GetCheckSlot(context.Context, int64) (pendingci.CheckSlot, error)
	GetCheckSlotByHead(context.Context, string, string) (pendingci.CheckSlot, error)

	GetPendingCIRepositoryGate(context.Context, string) (storage.PendingCIRepositoryGate, error)

	// The two rows the panel writes: whether an account has the bot switched
	// on, and whether one repository overrides it.
	GetTarget(context.Context, string) (storage.Target, error)
	GetRepository(context.Context, string, string) (storage.Repository, error)
}
