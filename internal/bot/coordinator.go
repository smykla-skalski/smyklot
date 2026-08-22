package bot

import (
	"context"
	"fmt"
	"slices"
	"sync"
)

const CatalogCoordinatorKey = "pending-ci:catalog"

// Coordinator serializes the few repository operations that cross
// durable state and GitHub side effects. Policy, persistence, transport, and
// API adapters remain independent and meet only at this boundary.
type Coordinator struct {
	mu    sync.Mutex
	gates map[string]*repositoryGate
}

type repositoryGate struct {
	ready chan struct{}
	users int
}

func ExclusiveRepositories(
	ctx context.Context,
	coordinator Exclusive,
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

func NewCoordinator() *Coordinator {
	return &Coordinator{gates: make(map[string]*repositoryGate)}
}

func (coordinator *Coordinator) Exclusive(
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

func (coordinator *Coordinator) join(
	repositoryID string,
) *repositoryGate {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()

	gate := coordinator.gates[repositoryID]
	if gate == nil {
		gate = &repositoryGate{ready: make(chan struct{}, 1)}
		gate.ready <- struct{}{}
		coordinator.gates[repositoryID] = gate
	}
	gate.users++

	return gate
}

func (coordinator *Coordinator) leave(
	repositoryID string,
	gate *repositoryGate,
) {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()

	gate.users--
	if gate.users == 0 && coordinator.gates[repositoryID] == gate {
		delete(coordinator.gates, repositoryID)
	}
}
