package bot

import (
	"context"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

// Exclusive serializes work on one repository.
//
// A command arriving while another is still running on the same repository
// would read the state the first one is halfway through writing, so the second
// waits. Who serializes is the service's business; that it happens is this
// package's.
type Exclusive interface {
	Exclusive(context.Context, string, func() error) error
}

// PendingCIChecks is the check run a repository sees while it waits for CI.
//
// The three Ensure methods each name the state the check should be left in -
// passing before anything is armed, blocking once it is, and action-required
// when the authorization has to be renewed - rather than exposing the one
// method underneath them that takes a desired state. A caller that could
// describe any state could describe a wrong one.
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

	// CheckSlot reads one slot back.
	//
	// This used to be reached as checks.store.GetCheckSlot - through the
	// unexported field of another type, which compiled only because both lived
	// in package main. It is a method here so that the store the checks
	// service holds stays the checks service's own.
	CheckSlot(ctx context.Context, id int64) (pendingci.CheckSlot, error)
}
