// Package github provides a GitHub API client for Smyklot operations.
//
// It supports PR operations (approve, merge, info), comment posting, and
// emoji reactions through the GitHub REST API v3.
package github

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	gogithub "github.com/google/go-github/v90/github"
)

const (
	defaultBaseURL      = "https://api.github.com"
	userAgent           = "smyklot-github-app"
	maxIdleConns        = 100
	maxIdleConnsPerHost = 10
	idleConnTimeout     = 90 * time.Second
	maxCodeownersSize   = 1024 * 1024 // 1MB
	maxRepoConfigSize   = 64 * 1024   // 64KB

	// schemeToken authenticates as a user or an App installation
	schemeToken = "token"

	// schemeBearer authenticates as the App itself, with a JWT. GitHub rejects
	// the "token" scheme on app-level endpoints such as GET /app/installations
	schemeBearer = "Bearer"

	// pageSize is the maximum GitHub allows on the paginated endpoints this
	// client reads
	pageSize = 100

	// maxPages bounds a pagination loop so a misbehaving endpoint cannot spin
	// forever. At 100 items per page this covers GitHub's documented
	// 100,000-repository organization and account limit. Reaching the bound
	// without proving the snapshot complete returns an error.
	maxPages = 1000
)

// sharedTransport is the connection pool every client draws on.
//
// A Transport is safe for concurrent use and owns the keep-alive pool, so one
// per process means a client built for a single call reuses a connection
// somebody else opened. A Transport per client would hand every short-lived
// client an empty pool to fill and then abandon, and the abandoned one holds
// its idle connections until they time out on their own.
var sharedTransport = &http.Transport{
	MaxIdleConns:        maxIdleConns,
	MaxIdleConnsPerHost: maxIdleConnsPerHost,
	IdleConnTimeout:     idleConnTimeout,
}

// Client is a GitHub API client
//
// The exported surface is the whole contract: nothing outside this package
// names go-github, and a depguard rule in .golangci.yml keeps it that way. That
// is what let the transport underneath change without touching the hundred call
// sites above it, and what keeps a second client from growing back one
// convenient import at a time.
type Client struct {
	httpClient *http.Client
	gh         *gogithub.Client
	token      string
	baseURL    string
	authScheme string
}

// NewClient creates a new GitHub API client
//
// The token parameter is required and must not be empty. The baseURL parameter
// is optional; if empty, the default GitHub API URL will be used.
func NewClient(token, baseURL string) (*Client, error) {
	return newClient(token, baseURL, schemeToken)
}

// NewAppClient creates a client authenticated as the GitHub App itself.
//
// The jwt parameter is a GitHub App JWT, not an installation token. Use this
// only for app-level endpoints such as ListInstallations; every repository
// operation needs an installation token and NewClient.
func NewAppClient(jwt, baseURL string) (*Client, error) {
	return newClient(jwt, baseURL, schemeBearer)
}

func newClient(token, baseURL, authScheme string) (*Client, error) {
	if token == "" {
		return nil, ErrEmptyToken
	}

	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	// Every path this client builds starts with a slash, so a trailing one
	// would produce a double slash in the request URL
	baseURL = strings.TrimSuffix(baseURL, "/")

	// No Timeout here: it would bound the whole RoundTrip, and retry now lives
	// inside the transport. The deadline is applied per attempt instead, in
	// retryTransport.attempt.
	httpClient := &http.Client{
		Transport: authTransport{
			base: retryTransport{base: sharedTransport},

			scheme: authScheme,
			token:  token,
		},
	}

	gh, err := newGoGitHub(httpClient, baseURL)
	if err != nil {
		return nil, NewAPIError(ErrAPIRequest, 0, "", "", err)
	}

	return &Client{
		httpClient: httpClient,
		gh:         gh,
		token:      token,
		baseURL:    baseURL,
		authScheme: authScheme,
	}, nil
}

// Reaction operations live in reactions.go.

// PostComment posts a comment on a pull request
//
// The body parameter must not be empty.
func (c *Client) PostComment(ctx context.Context, owner, repo string, prNumber int, body string) error {
	if body == "" {
		return ErrEmptyComment
	}

	path := fmt.Sprintf("/repos/%s/%s/issues/%d/comments", owner, repo, prNumber)

	_, _, err := c.gh.Issues.CreateComment(ctx, owner, repo, prNumber, &gogithub.IssueComment{
		Body: gogithub.Ptr(body),
	})

	return wrapError(ErrAPIRequest, http.MethodPost, path, err)
}

// Review operations live in reviews.go.

// EnableAutoMerge enables auto-merge for a pull request
//
// This will automatically merge the PR when all required checks pass.
// Uses GraphQL API as auto-merge is not available in REST API.
func (c *Client) EnableAutoMerge(
	ctx context.Context,
	owner, repo string,
	prNumber int,
	method MergeMethod,
) error {
	// Get PR node ID first (required for GraphQL)
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, prNumber)

	pull, _, err := c.gh.PullRequests.Get(ctx, owner, repo, prNumber)
	if err != nil {
		return wrapError(ErrAPIRequest, http.MethodGet, path, err)
	}

	nodeID := pull.GetNodeID()
	if nodeID == "" {
		return NewAPIError(
			ErrResponseParse,
			0,
			http.MethodGet,
			path,
			errors.New("no node_id in response"),
		)
	}

	// Map merge method to GraphQL enum
	var gqlMethod string
	switch method {
	case MergeMethodMerge:
		gqlMethod = "MERGE"
	case MergeMethodSquash:
		gqlMethod = "SQUASH"
	case MergeMethodRebase:
		gqlMethod = "REBASE"
	default:
		gqlMethod = "MERGE"
	}

	// Enable auto-merge via GraphQL (using parameterized query to prevent injection)
	const mutation = `mutation($pullRequestId: ID!, $mergeMethod: PullRequestMergeMethod!) {
			enablePullRequestAutoMerge(input: {pullRequestId: $pullRequestId, mergeMethod: $mergeMethod}) {
				clientMutationId
			}
		}`

	return c.graphql(ctx, mutation, map[string]any{
		"pullRequestId": nodeID,
		"mergeMethod":   gqlMethod,
	}, nil)
}

// Label operations live in labels.go.

// GetCodeowners fetches the CODEOWNERS file content from the repository
//
// Returns the decoded content of .github/CODEOWNERS file.
// Returns empty string (not error) if file doesn't exist (404).
func (c *Client) GetCodeowners(ctx context.Context, owner, repo string) (string, error) {
	decoded, err := c.getFileContent(ctx, owner, repo, ".github/CODEOWNERS", maxCodeownersSize)
	if err != nil {
		return "", err
	}

	return string(decoded), nil
}

// getFileContent reads a file through the contents API.
//
// Returns nil content (not an error) when the file does not exist, so callers
// can treat "no such file" as "nothing configured".
func (c *Client) getFileContent(
	ctx context.Context,
	owner, repo, filePath string,
	maxSize int,
) ([]byte, error) {
	return c.getFileContentAtRef(ctx, owner, repo, filePath, "", maxSize)
}

func (c *Client) getFileContentAtRef(
	ctx context.Context,
	owner, repo, filePath, ref string,
	maxSize int,
) ([]byte, error) {
	path := fmt.Sprintf("/repos/%s/%s/contents/%s", owner, repo, filePath)
	if ref != "" {
		path += "?" + url.Values{"ref": []string{ref}}.Encode()
	}

	response, err := doJSON[map[string]interface{}](ctx, c, http.MethodGet, path, nil)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return nil, nil
		}

		return nil, err
	}

	content, ok := response["content"].(string)
	if !ok {
		return nil, NewAPIError(
			ErrResponseParse,
			0,
			"GET",
			path,
			fmt.Errorf("no content field in response"),
		)
	}

	// GitHub API returns base64-encoded content, decode it
	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(content, "\n", ""))
	if err != nil {
		return nil, NewAPIError(ErrResponseParse, 0, "GET", path, err)
	}

	// Validate decoded content size to prevent memory exhaustion
	if len(decoded) > maxSize {
		return nil, NewAPIError(
			ErrResponseParse,
			0,
			"GET",
			path,
			fmt.Errorf("%s too large: %d bytes (max: %d)", filePath, len(decoded), maxSize),
		)
	}

	return decoded, nil
}

// Ping reports whether the GitHub API answers and accepts these credentials.
//
// Requires a client created with NewAppClient. GET /app is the cheapest call
// that proves both: it returns the App this JWT belongs to, and it fails on a
// key that has been revoked or a clock that has drifted too far.
//
// GET /rate_limit would look like the better choice, being exempt from the
// limit it reports, and it is what this used to send. GitHub answers it 401 for
// an App JWT, whatever the key - only an installation or user token gets a
// reading. A probe built on it can never report ready.
//
// Sent without the retry every other call gets: a readiness probe wants the
// current answer, not a patient one.
func (c *Client) Ping(ctx context.Context) error {
	return doRequest(withoutRetry(ctx), c, http.MethodGet, "/app", nil)
}

// GetUser resolves a GitHub login to its stable numeric identity.
func (c *Client) GetUser(ctx context.Context, login string) (User, error) {
	login = strings.TrimSpace(login)
	if login == "" {
		return User{}, errors.New("GitHub login must not be empty")
	}
	path := "/users/" + url.PathEscape(login)
	response, err := doJSON[struct {
		ID        int64   `json:"id"`
		Login     string  `json:"login"`
		Name      *string `json:"name"`
		AvatarURL *string `json:"avatar_url"`
	}](ctx, c, http.MethodGet, path, nil)
	if err != nil {
		return User{}, err
	}

	return User{
		ID: response.ID, Login: response.Login, Name: response.Name, AvatarURL: response.AvatarURL,
	}, nil
}

// ListInstallations retrieves every installation of the GitHub App.
//
// Requires a client created with NewAppClient - this endpoint accepts only a
// GitHub App JWT.
func (c *Client) ListInstallations(ctx context.Context) ([]Installation, error) {
	raw, err := paginate(ctx, "/app/installations",
		func(ctx context.Context, opts *gogithub.ListOptions) ([]*gogithub.Installation, *gogithub.Response, error) {
			return c.gh.Apps.ListInstallations(ctx, opts)
		})
	if err != nil {
		return nil, err
	}

	installations := make([]Installation, 0, len(raw))

	for _, item := range raw {
		// A suspended installation cannot mint a token, so polling it only
		// produces errors
		if item.SuspendedAt != nil {
			continue
		}

		installations = append(installations, Installation{
			ID:          item.GetID(),
			AccountID:   item.GetAccount().GetID(),
			Account:     item.GetAccount().GetLogin(),
			AccountType: item.GetAccount().GetType(),
			AvatarURL:   item.GetAccount().GetAvatarURL(),
			Permissions: installationPermissions(item.GetPermissions()),
		})
	}

	return installations, nil
}

// installationPermissions reads the permissions an installation granted.
//
// Only the ones Smyklot acts on. go-github models every permission GitHub has
// as its own field, and carrying all of them would mean a map nothing reads and
// a line to maintain each time GitHub adds one.
func installationPermissions(granted *gogithub.InstallationPermissions) map[string]string {
	if granted == nil {
		return nil
	}

	permissions := map[string]string{}
	for name, level := range map[string]string{
		"administration": granted.GetAdministration(),
		"contents":       granted.GetContents(),
		"issues":         granted.GetIssues(),
		"pull_requests":  granted.GetPullRequests(),

		// Contents is not enough for one directory. GitHub keeps workflow files
		// behind this and refuses the push that writes one without it, so an
		// installation that granted it and had it dropped here would be told
		// for ever that it had not.
		"workflows": granted.GetWorkflows(),
	} {
		if level != "" {
			permissions[name] = level
		}
	}

	return permissions
}

// ListInstallationRepos retrieves every repository the installation can reach.
//
// Requires a client holding an installation token.
func (c *Client) ListInstallationRepos(ctx context.Context) ([]Repository, error) {
	var repos []Repository

	for page := 1; page <= maxPages; page++ {
		path := fmt.Sprintf("/installation/repositories?per_page=%d&page=%d", pageSize, page)

		// Unlike most list endpoints this one wraps its results in an object
		response, err := doJSON[struct {
			TotalCount   *int `json:"total_count"`
			Repositories []struct {
				ID            int64  `json:"id"`
				Name          string `json:"name"`
				FullName      string `json:"full_name"`
				Private       bool   `json:"private"`
				DefaultBranch string `json:"default_branch"`
				Owner         struct {
					Login string `json:"login"`
				} `json:"owner"`
			} `json:"repositories"`
		}](ctx, c, http.MethodGet, path, nil)
		if err != nil {
			return nil, err
		}

		for _, item := range response.Repositories {
			repos = append(repos, Repository{
				ID:            item.ID,
				Owner:         item.Owner.Login,
				Name:          item.Name,
				FullName:      item.FullName,
				Private:       item.Private,
				DefaultBranch: item.DefaultBranch,
			})
		}

		if len(response.Repositories) < pageSize {
			if response.TotalCount != nil && len(repos) < *response.TotalCount {
				return nil, NewAPIError(
					ErrIncompletePagination,
					0,
					"GET",
					path,
					fmt.Errorf("received %d of %d repositories", len(repos), *response.TotalCount),
				)
			}

			return repos, nil
		}
		if page == maxPages {
			if response.TotalCount != nil && len(repos) >= *response.TotalCount {
				return repos, nil
			}

			return nil, NewAPIError(
				ErrIncompletePagination,
				0,
				"GET",
				path,
				fmt.Errorf("repository list still incomplete after %d items", len(repos)),
			)
		}
	}

	return repos, nil
}

// ListOrganizationAdmins retrieves every organization member with the admin
// role. Requires an installation token with read-only organization Members
// permission.
func (c *Client) ListOrganizationAdmins(ctx context.Context, organization string) ([]User, error) {
	return c.listOrganizationMembers(ctx, organization, "admin")
}

// ListOrganizationMembers retrieves every member of the organization, whatever
// their role. Requires the same read-only organization Members permission as
// ListOrganizationAdmins.
//
// This is the roster the panel completes logins against. GitHub's user search is
// not one of the endpoints an installation token may call, and the panel holds
// no other credential - it reads the signed-in person's profile once and throws
// the OAuth token away rather than keeping one per session. The people who can
// be added to an installation are its organization's members anyway, so the
// narrower list is also the more useful one.
func (c *Client) ListOrganizationMembers(ctx context.Context, organization string) ([]User, error) {
	return c.listOrganizationMembers(ctx, organization, "")
}

func (c *Client) listOrganizationMembers(
	ctx context.Context,
	organization, role string,
) ([]User, error) {
	organization = strings.TrimSpace(organization)
	if organization == "" {
		return nil, errors.New("GitHub organization must not be empty")
	}

	op := fmt.Sprintf("/orgs/%s/members", url.PathEscape(organization))

	raw, err := paginate(ctx, op,
		func(ctx context.Context, opts *gogithub.ListOptions) ([]*gogithub.User, *gogithub.Response, error) {
			return c.gh.Organizations.ListMembers(ctx, organization, &gogithub.ListMembersOptions{
				Role: role, ListOptions: *opts,
			})
		})
	if err != nil {
		return nil, err
	}

	members := make([]User, 0, len(raw))
	for _, item := range raw {
		members = append(members, User{
			ID:        item.GetID(),
			Login:     item.GetLogin(),
			AvatarURL: stringPointer(item.GetAvatarURL()),
		})
	}

	return members, nil
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}

	return &value
}

// GetPRInfo retrieves information about a pull request
//
// Returns a PRInfo struct with details about the PR including number, state,
// mergeable status, author, and approvers.
func (c *Client) GetPRInfo(ctx context.Context, owner, repo string, prNumber int) (*PRInfo, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, prNumber)

	response, err := doJSON[map[string]interface{}](ctx, c, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	info := &PRInfo{
		Number: prNumber,
	}

	if state, ok := response["state"].(string); ok {
		info.State = state
	}

	if mergeable, ok := response["mergeable"].(bool); ok {
		info.Mergeable = mergeable
	}

	if mergeableState, ok := response["mergeable_state"].(string); ok {
		info.MergeableState = MergeableState(mergeableState)
	}

	if title, ok := response["title"].(string); ok {
		info.Title = title
	}

	if body, ok := response["body"].(string); ok {
		info.Body = body
	}

	if user, ok := response["user"].(map[string]interface{}); ok {
		if login, ok := user["login"].(string); ok {
			info.Author = login
		}
	}

	if base, ok := response["base"].(map[string]interface{}); ok {
		if ref, ok := base["ref"].(string); ok {
			info.BaseBranch = ref
		}
	}

	// Populate ApprovedBy field
	info.ApprovedBy = c.getApprovers(ctx, owner, repo, prNumber)

	return info, nil
}

// GetPRComments retrieves all comments on a pull request
//
// Returns a slice of comment data including ID, user, and body.
func (c *Client) GetPRComments(
	ctx context.Context,
	owner, repo string,
	prNumber int,
) ([]map[string]interface{}, error) {
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/comments", owner, repo, prNumber)

	return doJSON[[]map[string]interface{}](ctx, c, http.MethodGet, path, nil)
}

// DeleteComment deletes a comment from a pull request
func (c *Client) DeleteComment(ctx context.Context, owner, repo string, commentID int) error {
	path := fmt.Sprintf("/repos/%s/%s/issues/comments/%d", owner, repo, commentID)

	_, err := c.gh.Issues.DeleteComment(ctx, owner, repo, int64(commentID))

	return wrapError(ErrAPIRequest, http.MethodDelete, path, err)
}

// UpdatePendingCIReaction finds comments with the bot's "eyes" reaction and replaces with "+1"
//
// This is used after a pending-ci merge succeeds to update the visual feedback.
// It searches all comments on the PR, finds ones with "eyes" reaction from the bot,
// removes the "eyes" reaction, and adds a "+1" (thumbs up) reaction.
func (c *Client) UpdatePendingCIReaction(
	ctx context.Context,
	owner, repo string,
	prNumber int,
	botUsername string,
) error {
	// Get all comments on the PR
	comments, err := c.GetPRComments(ctx, owner, repo, prNumber)
	if err != nil {
		return err
	}

	// Check each comment for bot's "eyes" reaction
	for _, comment := range comments {
		commentIDFloat, ok := comment["id"].(float64)
		if !ok {
			continue
		}

		commentID := int(commentIDFloat)

		// Get reactions for this comment
		reactions, err := c.GetCommentReactions(ctx, owner, repo, commentID)
		if err != nil {
			continue // Skip comments we can't get reactions for
		}

		// Check if bot has an "eyes" reaction on this comment
		hasBotEyesReaction := false

		for _, reaction := range reactions {
			if reaction.User == botUsername && reaction.Type == ReactionPendingCI {
				hasBotEyesReaction = true

				break
			}
		}

		if hasBotEyesReaction {
			// Remove the "eyes" reaction
			_ = c.RemoveReactionByUser(ctx, owner, repo, commentID, ReactionPendingCI, botUsername)

			// Add "+1" (thumbs up) reaction
			_ = c.AddReaction(ctx, owner, repo, commentID, ReactionSuccess)
		}
	}

	return nil
}

// HasWritePermission checks if the user has write/admin permission to the repository
func (c *Client) HasWritePermission(ctx context.Context, owner, repo, username string) (bool, error) {
	path := fmt.Sprintf("/repos/%s/%s/collaborators/%s/permission", owner, repo, username)

	response, err := doJSON[map[string]interface{}](ctx, c, http.MethodGet, path, nil)
	if err != nil {
		// If user is not a collaborator, return false (not an error)
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return false, nil
		}

		return false, err
	}

	permission, ok := response["permission"].(string)
	if !ok {
		return false, NewAPIError(
			ErrResponseParse,
			0,
			"GET",
			path,
			fmt.Errorf("no permission field in response"),
		)
	}

	// admin and write permissions allow approving/merging
	return permission == "admin" || permission == "write", nil
}

// IsTeamMember checks if a user is a member of a team
//
// Returns true if the user is an active member of the team (org/team-slug format).
// Returns false if the user is not a member or membership is pending.
func (c *Client) IsTeamMember(ctx context.Context, org, teamSlug, username string) (bool, error) {
	path := fmt.Sprintf("/orgs/%s/teams/%s/memberships/%s", org, teamSlug, username)

	response, err := doJSON[map[string]interface{}](ctx, c, http.MethodGet, path, nil)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) {
			// 404 means user is not a member
			if apiErr.StatusCode == http.StatusNotFound {
				return false, nil
			}
			// 403 likely means insufficient permissions (missing read:org or members:read)
			if apiErr.StatusCode == http.StatusForbidden {
				return false, fmt.Errorf("insufficient permissions to check team membership (need read:org or members:read scope): %w", err)
			}
		}

		return false, err
	}

	// Check if membership is active (not pending)
	state, ok := response["state"].(string)
	if !ok {
		return false, NewAPIError(
			ErrResponseParse,
			0,
			"GET",
			path,
			fmt.Errorf("no state field in response"),
		)
	}

	return state == "active", nil
}

// IsMergeQueueEnabled checks if merge queue is enabled for a branch
func (c *Client) IsMergeQueueEnabled(ctx context.Context, owner, repo, branch string) (bool, error) {
	path := fmt.Sprintf("/repos/%s/%s/branches/%s/protection", owner, repo, branch)

	protection, err := doJSON[map[string]interface{}](ctx, c, http.MethodGet, path, nil)
	if err != nil {
		// 404 means branch protection not enabled
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return false, nil
		}

		return false, err
	}

	// Check if merge queue is enabled
	if mergeQueue, ok := protection["required_pull_request_reviews"].(map[string]interface{}); ok {
		if enabled, ok := mergeQueue["require_last_push_approval"].(bool); ok && enabled {
			return true, nil
		}
	}

	// Also check for the merge_queue field directly
	if mergeQueue, ok := protection["merge_queue"].(map[string]interface{}); ok {
		if enabled, ok := mergeQueue["enabled"].(bool); ok {
			return enabled, nil
		}
	}

	return false, nil
}

// GetPRHeadRef retrieves the head commit SHA of a pull request
//
// Returns the SHA of the latest commit on the PR's head branch.
func (c *Client) GetPRHeadRef(
	ctx context.Context,
	owner, repo string,
	prNumber int,
) (string, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, prNumber)

	// pullRequestStateResponse already models this shape, and reusing it keeps
	// one description of a pull request's head rather than two.
	response, err := doJSON[pullRequestStateResponse](ctx, c, http.MethodGet, path, nil)
	if err != nil {
		return "", err
	}

	if response.Head.SHA == "" {
		return "", NewAPIError(
			ErrResponseParse,
			0,
			"GET",
			path,
			fmt.Errorf("no head SHA in response"),
		)
	}

	return response.Head.SHA, nil
}
