package storage

import "time"

// AuditCategory identifies one kind of application-wide Root event.
type AuditCategory string

const (
	AuditCategoryConfiguration AuditCategory = "configuration"
	AuditCategoryAccess        AuditCategory = "access"
	AuditCategoryOwnership     AuditCategory = "ownership"
	AuditCategoryElevation     AuditCategory = "elevation"
	AuditCategoryNotification  AuditCategory = "notification"
	AuditCategoryRuntime       AuditCategory = "runtime"
)

// AppAuditEvent is one normalized application-wide audit event.
type AppAuditEvent struct {
	ID          int64
	Category    AuditCategory
	Target      *Account
	Actor       Account
	Subject     *Account
	ElevationID *string
	Action      string
	Summary     string
	CreatedAt   time.Time
}

// RootAuditPageRequest selects one filtered app-wide audit window.
type RootAuditPageRequest struct {
	HistoryPageRequest
	Categories []AuditCategory
	TargetID   *string
}

// RootAuditPage is one page of normalized app-wide audit events.
type RootAuditPage struct {
	Items      []AppAuditEvent
	NextOffset int
	Total      int
}

// RootFailurePage is one application-wide page of delivery failures.
type RootFailurePage struct {
	Items      []RootFailure
	NextOffset int
	Total      int
}
