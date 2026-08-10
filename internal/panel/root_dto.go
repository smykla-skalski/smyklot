package panel

import (
	"strconv"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

const rootServiceHealthy = "healthy"

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
	RepositoryCounts storage.RepositoryCounts `json:"repository_counts"`
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
	Status        string `json:"status"`
	Version       string `json:"version"`
	ServiceHost   string `json:"service_host"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	Storage       string `json:"storage"`
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
}

type rootAuditResponse struct {
	ID           string                `json:"id"`
	Category     storage.AuditCategory `json:"category"`
	Installation *accountResponse      `json:"installation,omitempty"`
	Actor        accountResponse       `json:"actor"`
	Subject      *accountResponse      `json:"subject,omitempty"`
	ElevationID  *string               `json:"elevation_id,omitempty"`
	Action       string                `json:"action"`
	Summary      string                `json:"summary"`
	CreatedAt    time.Time             `json:"created_at"`
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
		RepositoryCounts: target.RepositoryCounts,
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

	return rootOverviewResponse{
		Service: rootServiceResponse{
			Status: rootServiceHealthy, Version: cfg.Version, ServiceHost: cfg.ServiceHost,
			UptimeSeconds: uptime, Storage: rootServiceHealthy,
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
	}
}

func rootAuditPageDTO(page storage.RootAuditPage) pageResponse[rootAuditResponse] {
	items := make([]rootAuditResponse, 0, len(page.Items))
	for _, event := range page.Items {
		item := rootAuditResponse{
			ID: strconv.FormatInt(event.ID, 10), Category: event.Category,
			Actor: accountDTO(event.Actor), ElevationID: event.ElevationID,
			Action: event.Action, Summary: event.Summary, CreatedAt: event.CreatedAt,
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
