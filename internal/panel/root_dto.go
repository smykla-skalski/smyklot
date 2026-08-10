package panel

import (
	"strconv"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
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
	RepositoryCounts storage.RepositoryCounts `json:"repository_counts"`
	Ownership        ownershipResponse        `json:"ownership"`
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

func rootInstallationDTO(target storage.Target, now time.Time) rootInstallationResponse {
	return rootInstallationResponse{
		ID: target.ID, InstallationID: target.InstallationID, Type: target.Kind,
		Account: accountDTO(target.Account), Available: target.Available,
		RepositoryCounts: target.RepositoryCounts,
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
