package bot

import (
	"context"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

type Exclusive interface {
	Exclusive(context.Context, string, func() error) error
}

type PendingCIChecks interface {
	EnsureBaseline(
		ctx context.Context,
		target storage.Target,
		repository storage.Repository,
		pullRequest int,
		headSHA string,
	) (pendingci.CheckSlot, error)

	EnsureAuthorized(
		ctx context.Context,
		target storage.Target,
		repository storage.Repository,
		pullRequest int,
		headSHA string,
		method pendingci.MergeMethod,
		requester string,
	) (pendingci.CheckSlot, error)

	EnsureReauthorization(
		ctx context.Context,
		target storage.Target,
		repository storage.Repository,
		pullRequest int,
		headSHA string,
	) (pendingci.CheckSlot, error)

	CheckSlot(ctx context.Context, id int64) (pendingci.CheckSlot, error)
}
