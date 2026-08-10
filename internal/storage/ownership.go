package storage

import "time"

// OwnershipFreshFor is the maximum age of a GitHub-derived Owner snapshot.
const OwnershipFreshFor = 15 * time.Minute

// OwnershipSource identifies how Smyklot derives installation Owners.
type OwnershipSource string

const (
	OwnershipSourcePersonal          OwnershipSource = "personal"
	OwnershipSourceOrganizationAdmin OwnershipSource = "organization_admin"
)

// OwnershipStatus is the last result of synchronizing installation Owners.
type OwnershipStatus string

const (
	OwnershipStatusFresh             OwnershipStatus = "fresh"
	OwnershipStatusPermissionPending OwnershipStatus = "permission_pending"
	OwnershipStatusError             OwnershipStatus = "error"
)

// TargetOwnership summarizes persisted ownership health for one installation.
type TargetOwnership struct {
	Source     OwnershipSource
	Status     OwnershipStatus
	Detail     *string
	SyncedAt   time.Time
	OwnerCount int
}

// FreshAt reports whether regular-panel access may trust this snapshot.
func (ownership TargetOwnership) FreshAt(now time.Time) bool {
	return ownership.Status == OwnershipStatusFresh && ownership.OwnerCount > 0 &&
		!ownership.SyncedAt.Before(now.Add(-OwnershipFreshFor))
}

// OwnershipSnapshot replaces the GitHub-derived Owners for one installation.
type OwnershipSnapshot struct {
	Source   OwnershipSource
	Status   OwnershipStatus
	Detail   *string
	Owners   []Account
	SyncedAt time.Time
}
