package storage

import "time"

// ElevationLifetime is the absolute lifetime of one Root installation grant.
const ElevationLifetime = 15 * time.Minute

// ElevationEndReason explains why a Root installation grant stopped.
type ElevationEndReason string

const (
	ElevationEnded   ElevationEndReason = "ended"
	ElevationExpired ElevationEndReason = "expired"
	ElevationRevoked ElevationEndReason = "revoked"
)

// ElevationGrant starts one session-bound Root installation grant.
type ElevationGrant struct {
	ID               string
	SessionTokenHash string
	RootAccountID    string
	TargetID         string
	Reason           *string
	StartedAt        time.Time
}

// Elevation is one auditable Root installation grant.
type Elevation struct {
	ID               string
	SessionTokenHash string
	RootAccountID    string
	TargetID         string
	Reason           *string
	StartedAt        time.Time
	ExpiresAt        time.Time
	EndedAt          *time.Time
	EndReason        *ElevationEndReason
}

// ActiveAt reports whether the absolute grant window is still open.
func (elevation Elevation) ActiveAt(now time.Time) bool {
	return elevation.EndedAt == nil && now.Before(elevation.ExpiresAt)
}

// NotificationPageRequest selects one page of security notifications.
type NotificationPageRequest struct {
	Offset int
	Limit  int
}

// SecurityNotification is one immutable Owner notice for an elevated write.
type SecurityNotification struct {
	ID           int64
	RecipientID  string
	TargetID     string
	Target       Account
	Actor        Account
	ElevationID  string
	AuditEventID int64
	Action       string
	Reason       *string
	CreatedAt    time.Time
	ReadAt       *time.Time
}

// NotificationPage is one page plus the account's unread total.
type NotificationPage struct {
	Items      []SecurityNotification
	NextOffset int
	Total      int
	Unread     int
}

// RootOverview is the application-wide operational state shown to Root users.
type RootOverview struct {
	InstallationCount      int
	RepositoryCount        int
	EnabledRepositoryCount int
	OwnershipFresh         int
	OwnershipStale         int
	OwnershipPending       int
	OwnershipError         int
	ActiveElevations       int
	UnreadSecurityEvents   int
	RecentFailures         []RootFailure
}

// RootFailure adds installation identity to a delivery failure.
type RootFailure struct {
	Failure DeliveryFailure
	Target  Account
}
