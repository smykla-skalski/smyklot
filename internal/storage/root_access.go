package storage

import "time"

// RootPanelUserOrder controls application-wide user ordering.
type RootPanelUserOrder string

const (
	RootPanelUserNameAscending  RootPanelUserOrder = "name_asc"
	RootPanelUserNameDescending RootPanelUserOrder = "name_desc"
	RootPanelUserRoleAscending  RootPanelUserOrder = "role_asc"
	RootPanelUserRoleDescending RootPanelUserOrder = "role_desc"
	RootPanelUserLoginNewest    RootPanelUserOrder = "login_newest"
	RootPanelUserLoginOldest    RootPanelUserOrder = "login_oldest"
)

// RootPanelUserPageRequest selects one application-wide account page.
type RootPanelUserPageRequest struct {
	Offset      int
	Limit       int
	Order       RootPanelUserOrder
	Query       string
	SystemRoles []SystemRole
	Statuses    []PanelUserStatus
}

// RootPanelUser adds installation relationship counts to one account.
type RootPanelUser struct {
	User                      PanelUser
	OwnedInstallationCount    int
	AssignedInstallationCount int
}

// RootPanelUserPage is one application-wide account page.
type RootPanelUserPage struct {
	Items      []RootPanelUser
	NextOffset int
	Total      int
}

// SystemRoleChange changes one non-Super-Root account's system role.
type SystemRoleChange struct {
	AccountID        string
	ActorAccountID   string
	SystemRole       SystemRole
	ExpectedRevision int64
	ChangedAt        time.Time
}
