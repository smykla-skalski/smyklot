// Package storagetest holds the storage expectations every engine must meet.
//
// The specs live here rather than beside one adapter so that parity between
// engines is something the suite proves, not something a second implementation
// is trusted to preserve. An engine supplies a Harness and runs the same specs.
package storagetest

import (
	"context"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

// Harness is what an engine supplies so the shared specs can run against it.
type Harness struct {
	// Open returns an empty, migrated store. It is called once per spec, and
	// the spec closes what it returns.
	Open func(ctx context.Context) storage.Store

	// RejectSecurityNotifications makes the next write to the notification
	// table fail, so a spec can prove that an elevated write rolls back whole
	// rather than leaving the change without its notifications. Engines break
	// a write differently, so each supplies its own way.
	RejectSecurityNotifications func(ctx context.Context)
}
