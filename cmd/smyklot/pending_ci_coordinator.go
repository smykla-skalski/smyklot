package main

import (
	"context"
	"fmt"
	"slices"
	"sync"
)

type pendingCIExclusive interface {
	Exclusive(context.Context, string, func() error) error
}

const pendingCICatalogCoordinatorKey = "pending-ci:catalog"

// pendingCICoordinator serializes the few repository operations that cross
// durable state and GitHub side effects. Policy, persistence, transport, and
// API adapters remain independent and meet only at this boundary.
type pendingCICoordinator struct {
	mu    sync.Mutex
	gates map[string]*pendingCIRepositoryGate
}

type pendingCIRepositoryGate struct {
	ready chan struct{}
	users int
}

func exclusivePendingCIRepositories(
	ctx context.Context,
	coordinator pendingCIExclusive,
	repositoryIDs []string,
	operation func() error,
) error {
	if coordinator == nil {
		return operation()
	}
	ids := slices.Clone(repositoryIDs)
	slices.Sort(ids)
	ids = slices.Compact(ids)

	var acquire func(int) error
	acquire = func(index int) error {
		if index == len(ids) {
			return operation()
		}

		return coordinator.Exclusive(ctx, ids[index], func() error {
			return acquire(index + 1)
		})
	}

	return acquire(0)
}

func newPendingCICoordinator() *pendingCICoordinator {
	return &pendingCICoordinator{gates: make(map[string]*pendingCIRepositoryGate)}
}

func (coordinator *pendingCICoordinator) Exclusive(
	ctx context.Context,
	repositoryID string,
	operation func() error,
) error {
	if repositoryID == "" {
		return fmt.Errorf("coordinate pending CI: repository identity is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	gate := coordinator.join(repositoryID)
	select {
	case <-ctx.Done():
		coordinator.leave(repositoryID, gate)

		return ctx.Err()
	case <-gate.ready:
	}
	if err := ctx.Err(); err != nil {
		gate.ready <- struct{}{}
		coordinator.leave(repositoryID, gate)

		return err
	}
	defer func() {
		gate.ready <- struct{}{}
		coordinator.leave(repositoryID, gate)
	}()

	return operation()
}

func (coordinator *pendingCICoordinator) join(
	repositoryID string,
) *pendingCIRepositoryGate {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()

	gate := coordinator.gates[repositoryID]
	if gate == nil {
		gate = &pendingCIRepositoryGate{ready: make(chan struct{}, 1)}
		gate.ready <- struct{}{}
		coordinator.gates[repositoryID] = gate
	}
	gate.users++

	return gate
}

func (coordinator *pendingCICoordinator) leave(
	repositoryID string,
	gate *pendingCIRepositoryGate,
) {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()

	gate.users--
	if gate.users == 0 && coordinator.gates[repositoryID] == gate {
		delete(coordinator.gates, repositoryID)
	}
}
