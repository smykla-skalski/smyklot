package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
)

// RepoConfigPaths are the places a repository's own configuration may live, in
// the order they are looked for.
//
// The first match wins, so a repository that has both a TOML file and the
// legacy YAML one is read as TOML and told about the other. The legacy path is
// last for exactly that reason: it is what a repository migrates away from.
var RepoConfigPaths = []string{
	".smyklot.toml",
	".smyklot/config.toml",
	".github/.smyklot.toml",
	repoConfigPath,
}

// LegacyRepoConfigPath is the file a repository configured before TOML.
const LegacyRepoConfigPath = repoConfigPath

// FindRepoConfig returns the repository's configuration file.
//
// preferred is the path an operator set in the panel, and is looked at first
// when it is not empty. A repository with no configuration file at all gets a
// zero RepoConfig back rather than an error.
//
// This costs one request per candidate until something is found, and most
// repositories have none - which is why nothing calls it on a schedule. The
// service asks only when the default branch has moved, and reuses the answer
// until it moves again. Probing every candidate on every sweep tick would cost
// an organization of two hundred repositories twelve thousand requests an hour
// against a budget of five thousand.
func (c *Client) FindRepoConfig(
	ctx context.Context,
	owner, repo, preferred string,
) (RepoConfig, error) {
	for _, path := range candidatePaths(preferred) {
		content, err := c.getFileContent(ctx, owner, repo, path, maxRepoConfigSize)
		if err != nil {
			return RepoConfig{}, err
		}

		if content != nil {
			return RepoConfig{Path: path, Content: content}, nil
		}
	}

	return RepoConfig{}, nil
}

// candidatePaths puts an operator's chosen path first and never looks at any
// path twice, so naming one of the standard paths in the panel costs nothing
// rather than costing a duplicate request.
func candidatePaths(preferred string) []string {
	preferred = strings.TrimSpace(strings.TrimPrefix(preferred, "/"))
	if preferred == "" {
		return RepoConfigPaths
	}

	if slices.Contains(RepoConfigPaths, preferred) {
		paths := make([]string, 0, len(RepoConfigPaths))
		paths = append(paths, preferred)

		for _, path := range RepoConfigPaths {
			if path != preferred {
				paths = append(paths, path)
			}
		}

		return paths
	}

	return append([]string{preferred}, RepoConfigPaths...)
}

// DefaultBranchHead is the SHA at the head of the repository's default branch.
//
// It exists to be a cache validator. Reading a configuration file means one
// request per candidate path, and the answer can only change when the default
// branch does - so the service asks this one cheap question per repository per
// tick and re-reads nothing until the answer changes. "No file anywhere" is
// cached against the SHA exactly like a file that was found.
//
// Keying on the SHA is also stricter than the age-based cache it replaces: an
// answer is reused only while the content provably cannot have changed, where a
// thirty-second window could serve a rolled-back runner setting for thirty
// seconds after the merge that changed it.
//
// A repository with no commits has no head, and reports an empty SHA rather
// than an error - it has no configuration file either.
func (c *Client) DefaultBranchHead(ctx context.Context, owner, repo string) (string, error) {
	// The list endpoint defaults to the repository's default branch, so this
	// needs neither the branch name nor a second request to learn it.
	path := fmt.Sprintf("/repos/%s/%s/commits?per_page=1", owner, repo)

	commits, err := doJSON[[]struct {
		SHA string `json:"sha"`
	}](ctx, c, http.MethodGet, path, nil)
	if err != nil {
		// GitHub answers a repository with no commits with 409, which is a
		// repository that cannot have a configuration file rather than a
		// failure to read one.
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict {
			return "", nil
		}

		return "", err
	}

	if len(commits) == 0 {
		return "", nil
	}

	return commits[0].SHA, nil
}
