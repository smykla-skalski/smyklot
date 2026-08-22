package gate

import (
	"context"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
)

type ControlStore interface {
	Get(context.Context, int64) (pendingci.Request, error)
	CheckNow(context.Context, pendingci.CheckNowRequest) (pendingci.Request, error)
	Finish(context.Context, pendingci.FinishRequest) (pendingci.Request, error)
}

// Control applies panel transitions through the repository boundary
// shared with merge and cleanup effects.
type Control struct {
	store       ControlStore
	coordinator bot.Exclusive
	wake        func()
}

func newControl(
	store ControlStore,
	coordinator bot.Exclusive,
	wake func(),
) *Control {
	return &Control{store: store, coordinator: coordinator, wake: wake}
}

func (control *Control) CheckNow(
	ctx context.Context,
	change pendingci.CheckNowRequest,
) (pendingci.Request, error) {
	return control.change(ctx, change.ID, func() (pendingci.Request, error) {
		return control.store.CheckNow(ctx, change)
	})
}

func (control *Control) Cancel(
	ctx context.Context,
	change pendingci.FinishRequest,
) (pendingci.Request, error) {
	return control.change(ctx, change.ID, func() (pendingci.Request, error) {
		return control.store.Finish(ctx, change)
	})
}

func (control *Control) Wake() {
	control.wake()
}

func (control *Control) Exclusive(
	ctx context.Context,
	repositoryIDs []string,
	operation func() error,
) error {
	return bot.ExclusiveRepositories(ctx, control.coordinator, repositoryIDs, operation)
}

func (control *Control) ExclusiveCatalog(
	ctx context.Context,
	operation func() error,
) error {
	return control.coordinator.Exclusive(ctx, bot.CatalogCoordinatorKey, operation)
}

func (control *Control) change(
	ctx context.Context,
	id int64,
	transition func() (pendingci.Request, error),
) (pendingci.Request, error) {
	current, err := control.store.Get(ctx, id)
	if err != nil {
		return pendingci.Request{}, fmt.Errorf("read pending CI control target: %w", err)
	}

	var changed pendingci.Request
	err = control.coordinator.Exclusive(ctx, current.RepositoryID, func() error {
		var transitionErr error
		changed, transitionErr = transition()

		return transitionErr
	})
	if err != nil {
		return pendingci.Request{}, err
	}
	control.wake()

	return changed, nil
}
