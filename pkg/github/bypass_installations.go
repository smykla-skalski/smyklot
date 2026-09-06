package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	gogithub "github.com/google/go-github/v91/github"
)

// AppInstallationAccess is an organization's installation inventory. A selected
// repository installation does not reveal which repositories another app can
// access, so callers must not interpret it as repository-level confirmation.
type AppInstallationAccess struct {
	AppID               int64      `json:"app_id"`
	Slug                string     `json:"app_slug"`
	RepositorySelection string     `json:"repository_selection"`
	SuspendedAt         *time.Time `json:"suspended_at"`
}

func (c *Client) ListOrganizationAppInstallations(ctx context.Context, organization string) ([]AppInstallationAccess, error) {
	items := []AppInstallationAccess{}
	for page := 1; page <= maxPages; page++ {
		path := fmt.Sprintf("/orgs/%s/installations?per_page=%d&page=%d", url.PathEscape(organization), pageSize, page)
		response, err := doJSON[struct {
			TotalCount    *int                    `json:"total_count"`
			Installations []AppInstallationAccess `json:"installations"`
		}](ctx, c, http.MethodGet, path, nil)
		if err != nil {
			return nil, err
		}
		if response.TotalCount == nil || *response.TotalCount < 0 || response.Installations == nil {
			return nil, fmt.Errorf("installation inventory is incomplete")
		}
		for _, item := range response.Installations {
			if item.AppID <= 0 {
				return nil, fmt.Errorf("installation inventory has an invalid app identity")
			}
		}
		items = append(items, response.Installations...)
		if len(items) >= *response.TotalCount {
			return items, nil
		}
		if len(response.Installations) == 0 {
			return nil, fmt.Errorf("installation inventory ended before its reported count")
		}
	}
	return nil, fmt.Errorf("installation inventory exceeded %d pages", maxPages)
}

func (c *Client) ListBypassTeams(ctx context.Context, organization string) ([]BypassActorIdentity, error) {
	teams, err := paginate(ctx, "/orgs/"+url.PathEscape(organization)+"/teams",
		func(ctx context.Context, options *gogithub.ListOptions) ([]*gogithub.Team, *gogithub.Response, error) {
			return c.gh.Teams.ListTeams(ctx, organization, options)
		})
	if err != nil {
		return nil, err
	}
	items := make([]BypassActorIdentity, 0, len(teams))
	for _, team := range teams {
		// GitHub excludes secret teams from ruleset bypass eligibility.
		if team.GetPrivacy() == "secret" {
			continue
		}
		items = append(items, BypassActorIdentity{ActorID: team.GetID(), ActorType: "Team", Name: team.GetName(), Slug: team.GetSlug()})
	}
	return items, nil
}
