package main

import (
	"context"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
)

type pendingCIHandoffStore interface {
	CancelRepository(
		context.Context,
		pendingci.CancelRepositoryRequest,
	) ([]pendingci.Request, error)
	HasPendingCleanup(context.Context, pendingci.CleanupFilter) (bool, error)
}

// pendingCIHandoff terminalizes service-owned work before a repository is
// returned to the GitHub Action runner. GitHub artifact cleanup stays with the
// caller that owns the installation client.
type pendingCIHandoff struct {
	store       pendingCIHandoffStore
	coordinator pendingCIExclusive
	wake        func()
}

func (handoff *pendingCIHandoff) CancelRepository(
	ctx context.Context,
	repositoryID string,
	reason string,
	cancelledAt time.Time,
) (bool, error) {
	var cleanupPending bool
	err := handoff.coordinator.Exclusive(ctx, repositoryID, func() error {
		_, transitionErr := handoff.store.CancelRepository(
			ctx,
			pendingci.CancelRepositoryRequest{
				RepositoryID: repositoryID, Reason: reason, CancelledAt: cancelledAt,
			},
		)

		if transitionErr != nil {
			return transitionErr
		}
		cleanupPending, transitionErr = handoff.store.HasPendingCleanup(
			ctx, pendingci.CleanupFilter{RepositoryID: repositoryID},
		)

		return transitionErr
	})
	if err != nil {
		return false, err
	}
	if cleanupPending {
		handoff.wake()
	}

	return cleanupPending, nil
}
