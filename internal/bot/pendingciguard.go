package bot

import (
	"context"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

type PendingCIActivationGuard interface {
	AllowsActivation(context.Context, pendingci.ArtifactKind, string, bool) (bool, error)
}

type PendingCIModeResolver interface {
	PendingCIMode(context.Context, string) (storage.PendingCIMode, error)
}
