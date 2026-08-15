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
) ([]pendingci.Request, error) {
	var requests []pendingci.Request
	err := handoff.coordinator.Exclusive(ctx, repositoryID, func() error {
		var transitionErr error
		requests, transitionErr = handoff.store.CancelRepository(
			ctx,
			pendingci.CancelRepositoryRequest{
				RepositoryID: repositoryID, Reason: reason, CancelledAt: cancelledAt,
			},
		)

		return transitionErr
	})
	if err != nil {
		return nil, err
	}
	if len(requests) > 0 {
		handoff.wake()
	}

	return requests, nil
}
