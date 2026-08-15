package main

import (
	"context"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
)

type pendingCIControlStore interface {
	Get(context.Context, int64) (pendingci.Request, error)
	CheckNow(context.Context, pendingci.CheckNowRequest) (pendingci.Request, error)
	Finish(context.Context, pendingci.FinishRequest) (pendingci.Request, error)
}

// pendingCIControl applies panel transitions through the repository boundary
// shared with merge and cleanup effects.
type pendingCIControl struct {
	store       pendingCIControlStore
	coordinator pendingCIExclusive
	wake        func()
}

func newPendingCIControl(
	store pendingCIControlStore,
	coordinator pendingCIExclusive,
	wake func(),
) *pendingCIControl {
	return &pendingCIControl{store: store, coordinator: coordinator, wake: wake}
}

func (control *pendingCIControl) CheckNow(
	ctx context.Context,
	change pendingci.CheckNowRequest,
) (pendingci.Request, error) {
	return control.change(ctx, change.ID, func() (pendingci.Request, error) {
		return control.store.CheckNow(ctx, change)
	})
}

func (control *pendingCIControl) Cancel(
	ctx context.Context,
	change pendingci.FinishRequest,
) (pendingci.Request, error) {
	return control.change(ctx, change.ID, func() (pendingci.Request, error) {
		return control.store.Finish(ctx, change)
	})
}

func (control *pendingCIControl) change(
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
