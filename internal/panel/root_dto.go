package panel

import (
	"math"
	"strconv"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

// The three states the panel colours a dependency, and the words its stylesheet
// keys on.
const (
	rootServiceHealthy     = "healthy"
	rootServiceDegraded    = "degraded"
	rootServiceUnavailable = "unavailable"
)

type ownershipResponse struct {
	Source     storage.OwnershipSource `json:"source"`
	Status     storage.OwnershipStatus `json:"status"`
	Detail     *string                 `json:"detail,omitempty"`
	SyncedAt   time.Time               `json:"synced_at"`
	OwnerCount int                     `json:"owner_count"`
	Stale      bool                    `json:"stale"`
}

type rootInstallationResponse struct {
	ID               string                   `json:"id"`
	InstallationID   string                   `json:"installation_id"`
	Type             storage.TargetKind       `json:"type"`
	Account          accountResponse          `json:"account"`
	Available        bool                     `json:"available"`
	OwnedByViewer    bool                     `json:"owned_by_viewer"`
	RepositoryCounts repositoryCountsResponse `json:"repository_counts"`
	DeliveryHealth   deliveryHealthResponse   `json:"delivery_health"`
	Ownership        ownershipResponse        `json:"ownership"`
}

type deliveryHealthResponse struct {
	Failed        int        `json:"failed"`
	LastFailureAt *time.Time `json:"last_failure_at,omitempty"`
}

type elevationResponse struct {
	ID        string     `json:"id"`
	TargetID  string     `json:"target_id"`
	Reason    *string    `json:"reason,omitempty"`
	StartedAt time.Time  `json:"started_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}

type securityNotificationResponse struct {
	ID           string          `json:"id"`
	Installation accountResponse `json:"installation"`
	Actor        accountResponse `json:"actor"`
	ElevationID  string          `json:"elevation_id"`
	AuditEventID string          `json:"audit_event_id"`
	Action       string          `json:"action"`
	Reason       *string         `json:"reason,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	ReadAt       *time.Time      `json:"read_at,omitempty"`
}

type notificationPageResponse struct {
	Items      []securityNotificationResponse `json:"items"`
	NextCursor *string                        `json:"next_cursor"`
	Total      int                            `json:"total"`
	Unread     int                            `json:"unread"`
}

type rootServiceResponse struct {
	Status        string                 `json:"status"`
	Version       string                 `json:"version"`
	ServiceHost   string                 `json:"service_host"`
	UptimeSeconds int64                  `json:"uptime_seconds"`
	Storage       string                 `json:"storage"`
	Database      databaseStatusResponse `json:"database"`
}

type databaseConnectionsResponse struct {
	Open       int     `json:"open"`
	InUse      int     `json:"in_use"`
	Idle       int     `json:"idle"`
	Max        int     `json:"max"`
	WaitCount  int64   `json:"wait_count"`
	WaitMillis float64 `json:"wait_ms"`
}

type databaseStatusResponse struct {
	State         string                      `json:"state"`
	Engine        string                      `json:"engine"`
	Version       string                      `json:"version"`
	SchemaVersion int                         `json:"schema_version"`
	SizeBytes     int64                       `json:"size_bytes"`
	LatencyMillis float64                     `json:"latency_ms"`
	Detail        string                      `json:"detail,omitempty"`
	Connections   databaseConnectionsResponse `json:"connections"`
}

func databaseStatusDTO(status storage.DatabaseStatus) databaseStatusResponse {
	return databaseStatusResponse{
		State:         databaseState(status),
		Engine:        status.Engine,
		Version:       status.Version,
		SchemaVersion: status.SchemaVersion,
		SizeBytes:     status.SizeBytes,
		LatencyMillis: millisecondsDTO(status.Latency),
		Detail:        status.Error,
		Connections: databaseConnectionsResponse{
			Open: status.Connections.Open, InUse: status.Connections.InUse,
			Idle: status.Connections.Idle, Max: status.Connections.Max,
			WaitCount:  status.Connections.WaitCount,
			WaitMillis: millisecondsDTO(status.Connections.WaitDuration),
		},
	}
}

// databaseState is the panel's reading of a status, and the only place that
// decides what colour a database gets.
//
// Pool pressure is deliberately not part of it. A pool that is momentarily
// full is a busy instant rather than a fault, and a light that went amber on
// one would teach an operator to stop reading it. The counts are shown as
// numbers instead, where WaitCount records what a sample cannot: whether
// anyone has ever had to queue for a connection.
func databaseState(status storage.DatabaseStatus) string {
	switch {
	case !status.Reachable:
		return rootServiceUnavailable
	case status.Error != "":
		return rootServiceDegraded
	default:
		return rootServiceHealthy
	}
}

// millisecondsDTO rounds a duration to hundredths of a millisecond: finer than
// a database round trip is worth reporting to, and coarse enough that the
// number does not churn every time the page is read.
func millisecondsDTO(value time.Duration) float64 {
	return math.Round(float64(value.Microseconds())/10) / 100
}

type rootCatalogResponse struct {
	Installations       int `json:"installations"`
	Repositories        int `json:"repositories"`
	EnabledRepositories int `json:"enabled_repositories"`
}

type rootOwnershipSummaryResponse struct {
	Fresh             int `json:"fresh"`
	Stale             int `json:"stale"`
	PermissionPending int `json:"permission_pending"`
	Error             int `json:"error"`
}

type rootFailureResponse struct {
	Installation accountResponse `json:"installation"`
	Failure      failureResponse `json:"failure"`
}

type rootOverviewResponse struct {
	Service              rootServiceResponse          `json:"service"`
	Catalog              rootCatalogResponse          `json:"catalog"`
	Ownership            rootOwnershipSummaryResponse `json:"ownership"`
	ActiveElevations     int                          `json:"active_elevations"`
	UnreadSecurityEvents int                          `json:"unread_security_events"`
	RecentFailures       []rootFailureResponse        `json:"recent_failures"`
	PendingCI            pendingCIQueueResponse       `json:"pending_ci"`
}

type pendingCIQueueResponse struct {
	Active   []pendingCIResponse `json:"active"`
	Deferred []pendingCIResponse `json:"deferred"`
	Recent   []pendingCIResponse `json:"recent"`
}

type pendingCIResponse struct {
	ID                 string                `json:"id"`
	RepositoryFullName string                `json:"repository_full_name"`
	PullRequest        int                   `json:"pull_request"`
	HeadSHA            string                `json:"head_sha"`
	MergeMethod        pendingci.MergeMethod `json:"merge_method"`
	RequiredChecksOnly bool                  `json:"required_checks_only"`
	Requester          string                `json:"requester"`
	Lifecycle          pendingci.Lifecycle   `json:"lifecycle"`
	Schedule           pendingci.Schedule    `json:"schedule"`
	NextCheckAt        time.Time             `json:"next_check_at"`
	NextCheckTrigger   pendingci.Trigger     `json:"next_check_trigger"`
	LastObservedState  string                `json:"last_observed_state"`
	Reason             string                `json:"reason"`
	RequestedAt        time.Time             `json:"requested_at"`
	UpdatedAt          time.Time             `json:"updated_at"`
	FinishedAt         *time.Time            `json:"finished_at,omitempty"`
	CleanupPending     bool                  `json:"cleanup_pending"`
	CleanupError       string                `json:"cleanup_error,omitempty"`
	Revision           int64                 `json:"revision"`
}

type pendingCIEventResponse struct {
	ID         string              `json:"id"`
	Kind       pendingci.EventKind `json:"kind"`
	Trigger    pendingci.Trigger   `json:"trigger"`
	EventName  string              `json:"event_name,omitempty"`
	EventKey   string              `json:"event_key,omitempty"`
	DeliveryID string              `json:"delivery_id,omitempty"`
	State      string              `json:"state,omitempty"`
	Summary    string              `json:"summary"`
	CreatedAt  time.Time           `json:"created_at"`
}

type pendingCIDetailResponse struct {
	Request pendingCIResponse        `json:"request"`
	Events  []pendingCIEventResponse `json:"events"`
}

type rootAuditResponse struct {
	ID                     string                `json:"id"`
	Category               storage.AuditCategory `json:"category"`
	TargetID               *string               `json:"target_id,omitempty"`
	Installation           *accountResponse      `json:"installation,omitempty"`
	Actor                  accountResponse       `json:"actor"`
	Subject                *accountResponse      `json:"subject,omitempty"`
	ElevationID            *string               `json:"elevation_id,omitempty"`
	SyncConfigCheckpointID *string               `json:"sync_config_checkpoint_id,omitempty"`
	Action                 string                `json:"action"`
	Summary                string                `json:"summary"`
	CreatedAt              time.Time             `json:"created_at"`
}

type rootPanelUserResponse struct {
	Account               accountResponse         `json:"account"`
	SystemRole            storage.SystemRole      `json:"system_role"`
	Status                storage.PanelUserStatus `json:"status"`
	BanReason             *string                 `json:"ban_reason,omitempty"`
	BannedAt              *time.Time              `json:"banned_at,omitempty"`
	RemovedAt             *time.Time              `json:"removed_at,omitempty"`
	LastLoginAt           *time.Time              `json:"last_login_at,omitempty"`
	Revision              int64                   `json:"revision"`
	OwnedInstallations    int                     `json:"owned_installations"`
	AssignedInstallations int                     `json:"assigned_installations"`
	Manageable            bool                    `json:"manageable"`
	CanManageSystemRole   bool                    `json:"can_manage_system_role"`
}

func rootInstallationDTO(
	target storage.Target,
	now time.Time,
	ownedByViewer bool,
) rootInstallationResponse {
	return rootInstallationResponse{
		ID: target.ID, InstallationID: target.InstallationID, Type: target.Kind,
		Account: accountDTO(target.Account), Available: target.Available,
		OwnedByViewer:    ownedByViewer,
		RepositoryCounts: newRepositoryCountsResponse(target.RepositoryCounts),
		DeliveryHealth: deliveryHealthResponse{
			Failed: target.DeliveryHealth.Failed, LastFailureAt: target.DeliveryHealth.LastFailureAt,
		},
		Ownership: ownershipResponse{
			Source: target.Ownership.Source, Status: target.Ownership.Status,
			Detail: target.Ownership.Detail, SyncedAt: target.Ownership.SyncedAt,
			OwnerCount: target.Ownership.OwnerCount, Stale: !target.Ownership.FreshAt(now),
		},
	}
}

func elevationDTO(elevation storage.Elevation) elevationResponse {
	return elevationResponse{
		ID: elevation.ID, TargetID: elevation.TargetID, Reason: elevation.Reason,
		StartedAt: elevation.StartedAt, ExpiresAt: elevation.ExpiresAt, EndedAt: elevation.EndedAt,
	}
}

func notificationPageDTO(page storage.NotificationPage) notificationPageResponse {
	items := make([]securityNotificationResponse, 0, len(page.Items))
	for _, notification := range page.Items {
		items = append(items, securityNotificationDTO(notification))
	}

	return notificationPageResponse{
		Items: items, NextCursor: offsetCursor(page.NextOffset),
		Total: page.Total, Unread: page.Unread,
	}
}

func securityNotificationDTO(notification storage.SecurityNotification) securityNotificationResponse {
	return securityNotificationResponse{
		ID:           strconv.FormatInt(notification.ID, 10),
		Installation: accountDTO(notification.Target), Actor: accountDTO(notification.Actor),
		ElevationID:  notification.ElevationID,
		AuditEventID: strconv.FormatInt(notification.AuditEventID, 10),
		Action:       notification.Action, Reason: notification.Reason,
		CreatedAt: notification.CreatedAt, ReadAt: notification.ReadAt,
	}
}

func rootOverviewDTO(
	overview storage.RootOverview,
	database storage.DatabaseStatus,
	activeQueue, deferredQueue, recent []pendingci.Request,
	cfg Config,
	startedAt, now time.Time,
) rootOverviewResponse {
	failures := make([]rootFailureResponse, 0, len(overview.RecentFailures))
	for _, item := range overview.RecentFailures {
		failures = append(failures, rootFailureResponse{
			Installation: accountDTO(item.Target),
			Failure:      failureDTO(item.Failure),
		})
	}
	uptime := max(int64(now.Sub(startedAt).Seconds()), 0)
	databaseStatus := databaseStatusDTO(database)

	return rootOverviewResponse{
		Service: rootServiceResponse{
			Status: rootServiceHealthy, Version: cfg.Version, ServiceHost: cfg.ServiceHost,
			UptimeSeconds: uptime,
			// The summary word and the detail below it come from one reading,
			// so the card cannot say healthy over a panel that says otherwise.
			Storage: databaseStatus.State, Database: databaseStatus,
		},
		Catalog: rootCatalogResponse{
			Installations: overview.InstallationCount, Repositories: overview.RepositoryCount,
			EnabledRepositories: overview.EnabledRepositoryCount,
		},
		Ownership: rootOwnershipSummaryResponse{
			Fresh: overview.OwnershipFresh, Stale: overview.OwnershipStale,
			PermissionPending: overview.OwnershipPending, Error: overview.OwnershipError,
		},
		ActiveElevations:     overview.ActiveElevations,
		UnreadSecurityEvents: overview.UnreadSecurityEvents,
		RecentFailures:       failures,
		PendingCI: pendingCIQueueResponse{
			Active: pendingCIQueueDTO(activeQueue), Deferred: pendingCIQueueDTO(deferredQueue),
			Recent: pendingCIQueueDTO(recent),
		},
	}
}

func pendingCIQueueDTO(requests []pendingci.Request) []pendingCIResponse {
	items := make([]pendingCIResponse, 0, len(requests))
	for _, request := range requests {
		items = append(items, pendingCIResponse{
			ID: strconv.FormatInt(request.ID, 10), RepositoryFullName: request.RepositoryFullName,
			PullRequest: request.PullRequest, HeadSHA: request.HeadSHA,
			MergeMethod: request.MergeMethod, RequiredChecksOnly: request.RequiredChecksOnly,
			Requester: request.Requester, Lifecycle: request.Lifecycle, Schedule: request.Schedule,
			NextCheckAt: request.NextCheckAt, NextCheckTrigger: request.NextCheckTrigger,
			LastObservedState: request.LastObservedState, Reason: request.Reason,
			RequestedAt: request.RequestedAt, UpdatedAt: request.UpdatedAt,
			FinishedAt: request.FinishedAt, CleanupPending: request.CleanupPending,
			CleanupError: request.CleanupError,
			Revision:     request.Revision,
		})
	}

	return items
}

func pendingCIEventsDTO(events []pendingci.Event) []pendingCIEventResponse {
	items := make([]pendingCIEventResponse, 0, len(events))
	for _, event := range events {
		items = append(items, pendingCIEventResponse{
			ID: strconv.FormatInt(event.ID, 10), Kind: event.Kind, Trigger: event.Trigger,
			EventName: event.EventName, EventKey: event.EventKey,
			DeliveryID: event.DeliveryID, State: event.State,
			Summary: event.Summary, CreatedAt: event.CreatedAt,
		})
	}

	return items
}

func rootAuditPageDTO(page storage.RootAuditPage) pageResponse[rootAuditResponse] {
	items := make([]rootAuditResponse, 0, len(page.Items))
	for _, event := range page.Items {
		item := rootAuditResponse{
			ID: strconv.FormatInt(event.ID, 10), Category: event.Category,
			TargetID: event.TargetID, Actor: accountDTO(event.Actor), ElevationID: event.ElevationID,
			SyncConfigCheckpointID: stringID(event.SyncConfigCheckpointID),
			Action:                 event.Action, Summary: event.Summary, CreatedAt: event.CreatedAt,
		}
		if event.Target != nil {
			target := accountDTO(*event.Target)
			item.Installation = &target
		}
		if event.Subject != nil {
			subject := accountDTO(*event.Subject)
			item.Subject = &subject
		}
		items = append(items, item)
	}

	return pageResponse[rootAuditResponse]{
		Items: items, NextCursor: offsetCursor(page.NextOffset), Total: page.Total,
	}
}

func rootFailurePageDTO(page storage.RootFailurePage) pageResponse[rootFailureResponse] {
	items := make([]rootFailureResponse, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, rootFailureResponse{
			Installation: accountDTO(item.Target), Failure: failureDTO(item.Failure),
		})
	}

	return pageResponse[rootFailureResponse]{
		Items: items, NextCursor: offsetCursor(page.NextOffset), Total: page.Total,
	}
}

func rootPanelUserPageDTO(
	page storage.RootPanelUserPage,
	actor storage.Account,
	actorRole storage.SystemRole,
) pageResponse[rootPanelUserResponse] {
	items := make([]rootPanelUserResponse, 0, len(page.Items))
	for _, item := range page.Items {
		user := item.User
		otherAccount := actor.ID != user.Account.ID
		items = append(items, rootPanelUserResponse{
			Account: accountDTO(user.Account), SystemRole: user.SystemRole, Status: user.Status,
			BanReason: user.BanReason, BannedAt: user.BannedAt, RemovedAt: user.RemovedAt,
			LastLoginAt: user.LastLoginAt, Revision: user.Revision,
			OwnedInstallations:    item.OwnedInstallationCount,
			AssignedInstallations: item.AssignedInstallationCount,
			Manageable:            otherAccount && user.SystemRole == storage.SystemRoleNone,
			CanManageSystemRole: otherAccount && actorRole == storage.SystemRoleSuperRoot &&
				user.SystemRole != storage.SystemRoleSuperRoot && user.Status == storage.PanelUserActive,
		})
	}

	return pageResponse[rootPanelUserResponse]{
		Items: items, NextCursor: offsetCursor(page.NextOffset), Total: page.Total,
	}
}
