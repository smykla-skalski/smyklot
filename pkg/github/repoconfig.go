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

	// One loop covers both cases. When the chosen path is not one of the
	// standard ones the filter copies them all through, which is the same list
	// a plain concatenation would have produced.
	paths := make([]string, 0, len(RepoConfigPaths)+1)
	paths = append(paths, preferred)

	for _, path := range RepoConfigPaths {
		if path != preferred {
			paths = append(paths, path)
		}
	}

	return paths
}

// configRoots are the entries at the repository root that a configuration file
// can be reached through: the file itself, and the two directories one may sit
// in.
var configRoots = []string{".smyklot.toml", ".smyklot", ".github"}

// RepoConfigFingerprint identifies the state of everything a configuration file
// could be read from, in one request.
//
// It exists to be a cache validator. Reading the configuration means a request
// per candidate path, and the answer can only change when one of those paths
// does - so the service asks this one cheap question per repository per tick
// and re-reads nothing until the answer changes. "No file anywhere" is
// fingerprinted exactly like a file that was found.
//
// It fingerprints the root entries rather than the head commit, and that is the
// whole point. The head moves on every commit, so on a repository anyone is
// working in it would report a change every tick and re-probe every candidate
// path - six requests where there used to be one, for exactly the repositories
// this bot exists to serve. A blob SHA and two tree SHAs move only when
// something that could be the configuration moves.
//
// A repository with no commits, or one Smyklot cannot read, fingerprints as
// empty, which never compares equal - so it is re-read rather than assumed.
func (c *Client) RepoConfigFingerprint(ctx context.Context, owner, repo string) (string, error) {
	path := fmt.Sprintf("/repos/%s/%s/contents", owner, repo)

	entries, err := doJSON[[]struct {
		Name string `json:"name"`
		SHA  string `json:"sha"`
	}](ctx, c, http.MethodGet, path, nil)
	if err != nil {
		// An empty repository answers 404 here and has no configuration file.
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return "", nil
		}

		return "", err
	}

	found := make(map[string]string, len(configRoots))

	for _, entry := range entries {
		if slices.Contains(configRoots, entry.Name) {
			found[entry.Name] = entry.SHA
		}
	}

	// Rendered in a fixed order over a fixed set, so the same repository always
	// fingerprints the same way and an absent entry is distinct from a present
	// one rather than just missing from the middle of a string.
	var builder strings.Builder

	for _, name := range configRoots {
		builder.WriteString(name)
		builder.WriteByte('=')
		builder.WriteString(found[name])
		builder.WriteByte(';')
	}

	return builder.String(), nil
}
