package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

var ErrBypassActorNotFound = errors.New("GitHub could not resolve this actor")

// BypassActorIdentity is display metadata, separate from the ruleset authority.
type BypassActorIdentity struct {
	ActorID            int64   `json:"actor_id"`
	ActorType          string  `json:"actor_type"`
	Name               string  `json:"name"`
	Slug               string  `json:"slug"`
	AvatarURL          *string `json:"avatar_url"`
	InstallationStatus string  `json:"installation_status,omitempty"`
}

type bypassActorNode struct {
	Type       string `json:"__typename"`
	DatabaseID int64  `json:"databaseId"`
	Name       string `json:"name"`
	Slug       string `json:"slug"`
	Login      string `json:"login"`
	LogoURL    string `json:"logoUrl"`
	AvatarURL  string `json:"avatarUrl"`
	Privacy    string `json:"privacy"`
}

const (
	bypassOwnerKey = "owner"
	bypassNameKey  = "name"
)

// #nosec G101 -- This is a GraphQL field selection, not a credential.
const bypassActorFields = `__typename
 ... on App { databaseId name slug logoUrl }
 ... on Team { databaseId name slug avatarUrl }
 ... on User { databaseId name login avatarUrl }`

func (node bypassActorNode) identity() (BypassActorIdentity, bool) {
	kind, slug, avatar := node.Type, node.Slug, node.AvatarURL
	if kind == "App" {
		kind, avatar = "Integration", node.LogoURL
	}
	if kind == "User" {
		slug = node.Login
	}
	name := node.Name
	if name == "" {
		name = slug
	}
	if node.DatabaseID <= 0 || name == "" {
		return BypassActorIdentity{}, false
	}
	var picture *string
	if parsed, err := url.Parse(avatar); err == nil && parsed.Scheme == "https" && parsed.Host != "" {
		picture = &avatar
	}
	return BypassActorIdentity{ActorID: node.DatabaseID, ActorType: kind, Name: name, Slug: slug, AvatarURL: picture}, true
}

// AppSlug reads the authenticated app's public name using an App JWT.
func (c *Client) AppSlug(ctx context.Context) (string, error) {
	app, err := doJSON[struct {
		Slug string `json:"slug"`
	}](ctx, c, http.MethodGet, "/app", nil)
	if err != nil {
		return "", err
	}
	if app.Slug == "" {
		return "", errors.New("GitHub returned an app without a slug")
	}
	return app.Slug, nil
}

// ResolveBypassActor resolves a name to its stable identity. App logos come
// from App.logoUrl, never from the app owner's unrelated profile picture.
func (c *Client) ResolveBypassActor(ctx context.Context, owner, kind, name string) (BypassActorIdentity, error) {
	var node bypassActorNode
	switch kind {
	case "Integration":
		app, err := doJSON[struct {
			NodeID string `json:"node_id"`
		}](ctx, c, http.MethodGet, "/apps/"+url.PathEscape(name), nil)
		if err != nil {
			return BypassActorIdentity{}, err
		}
		if app.NodeID == "" {
			return BypassActorIdentity{}, errors.New("GitHub returned an app without a node id")
		}
		var data struct {
			Node bypassActorNode `json:"node"`
		}
		if err := c.graphql(ctx, `query($id:ID!){node(id:$id){`+bypassActorFields+`}}`, map[string]any{"id": app.NodeID}, &data); err != nil {
			return BypassActorIdentity{}, err
		}
		node = data.Node
	case "Team":
		var data struct {
			Organization struct{ Team bypassActorNode }
		}
		if err := c.graphql(ctx, `query($owner:String!,$name:String!){organization(login:$owner){team(slug:$name){__typename databaseId name slug avatarUrl privacy}}}`, map[string]any{bypassOwnerKey: owner, bypassNameKey: name}, &data); err != nil {
			return BypassActorIdentity{}, err
		}
		node = data.Organization.Team
	case "User":
		var data struct{ User bypassActorNode }
		if err := c.graphql(ctx, `query($name:String!){user(login:$name){__typename databaseId name login avatarUrl}}`, map[string]any{bypassNameKey: name}, &data); err != nil {
			return BypassActorIdentity{}, err
		}
		node = data.User
	default:
		return BypassActorIdentity{}, errors.New("this actor type does not have a directory")
	}
	identity, ok := node.identity()
	if !ok || identity.ActorType != kind || node.Privacy == "SECRET" {
		return BypassActorIdentity{}, ErrBypassActorNotFound
	}
	return identity, nil
}

type bypassPageInfo struct {
	HasNextPage bool
	EndCursor   string
}
type bypassActorConnection struct {
	Nodes []struct {
		Actor                    bypassActorNode
		RepositoryRoleDatabaseID *int64
		RepositoryRoleName       string
	}
	PageInfo bypassPageInfo
}

// ListRepositoryBypassIdentities reads actual node IDs and metadata from the
// rulesets GitHub returned. Both connections are paged independently.
func (c *Client) ListRepositoryBypassIdentities(ctx context.Context, owner, repository string) ([]BypassActorIdentity, error) {
	var identities []BypassActorIdentity
	var after any
	for range maxPages {
		var data struct {
			Repository *struct {
				Rulesets struct {
					Nodes []struct {
						ID           string
						BypassActors bypassActorConnection
					}
					PageInfo bypassPageInfo
				}
			}
		}
		query := `query($owner:String!,$repo:String!,$after:String){repository(owner:$owner,name:$repo){rulesets(first:100,after:$after){nodes{id bypassActors(first:100){nodes{repositoryRoleDatabaseId repositoryRoleName actor{` + bypassActorFields + `}} pageInfo{hasNextPage endCursor}}} pageInfo{hasNextPage endCursor}}}}`
		if err := c.graphql(ctx, query, map[string]any{bypassOwnerKey: owner, "repo": repository, "after": after}, &data); err != nil {
			return nil, err
		}
		if data.Repository == nil {
			return nil, errors.New("repository is unavailable")
		}
		for _, rule := range data.Repository.Rulesets.Nodes {
			actors, err := c.remainingBypassIdentities(ctx, rule.ID, rule.BypassActors)
			if err != nil {
				return nil, err
			}
			identities = append(identities, actors...)
		}
		page := data.Repository.Rulesets.PageInfo
		if !page.HasNextPage {
			return identities, nil
		}
		if page.EndCursor == "" || page.EndCursor == after {
			return nil, errors.New("ruleset pagination did not advance")
		}
		after = page.EndCursor
	}
	return nil, fmt.Errorf("ruleset directory exceeded %d pages", maxPages)
}

func (c *Client) remainingBypassIdentities(ctx context.Context, id string, page bypassActorConnection) ([]BypassActorIdentity, error) {
	var result []BypassActorIdentity
	var after string
	for range maxPages {
		for _, row := range page.Nodes {
			if identity, ok := row.Actor.identity(); ok {
				result = append(result, identity)
			} else if row.RepositoryRoleDatabaseID != nil && *row.RepositoryRoleDatabaseID > 0 && row.RepositoryRoleName != "" {
				result = append(result, BypassActorIdentity{ActorID: *row.RepositoryRoleDatabaseID, ActorType: "RepositoryRole", Name: row.RepositoryRoleName})
			}
		}
		if !page.PageInfo.HasNextPage {
			return result, nil
		}
		if page.PageInfo.EndCursor == "" || page.PageInfo.EndCursor == after {
			return nil, errors.New("bypass pagination did not advance")
		}
		after = page.PageInfo.EndCursor
		var data struct {
			Node *struct{ BypassActors bypassActorConnection }
		}
		query := `query($id:ID!,$after:String!){node(id:$id){...on RepositoryRuleset{bypassActors(first:100,after:$after){nodes{repositoryRoleDatabaseId repositoryRoleName actor{` + bypassActorFields + `}} pageInfo{hasNextPage endCursor}}}}}`
		if err := c.graphql(ctx, query, map[string]any{"id": id, "after": after}, &data); err != nil {
			return nil, err
		}
		if data.Node == nil {
			return nil, errors.New("ruleset is unavailable")
		}
		page = data.Node.BypassActors
	}
	return nil, fmt.Errorf("bypass directory exceeded %d pages", maxPages)
}
