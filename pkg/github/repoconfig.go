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

// FindRepoConfig returns the repository's configuration file, and any others
// it passed over.
//
// preferred is the path an operator set in the panel, and is looked at first
// when it is not empty. A repository with no configuration file at all gets a
// zero RepoConfig back rather than an error.
//
// Every candidate is asked for, not just those up to the first hit, so a
// repository carrying two configuration files is told which one is in charge
// rather than left to wonder. That is one request per candidate, and nothing
// calls this on a schedule: the service asks only when the fingerprint of
// everything a configuration file could live in has moved, and reuses the
// answer until it moves again. Probing on every sweep tick instead would cost
// an organization of two hundred repositories twelve thousand requests an hour
// against a budget of five thousand.
func (c *Client) FindRepoConfig(
	ctx context.Context,
	owner, repo, preferred string,
) (RepoConfig, error) {
	var found RepoConfig

	for _, path := range candidatePaths(preferred) {
		content, err := c.getFileContent(ctx, owner, repo, path, maxRepoConfigSize)
		if err != nil {
			// Before anything is found this is the read failing, and the file
			// is fail-closed. After, the answer is already in hand and the
			// remaining requests only decide what to report - so a failure
			// there stops the search rather than taking a repository offline
			// over a file it is not even using.
			if !found.Found() {
				return RepoConfig{}, err
			}

			return found, nil
		}

		switch {
		case content == nil:
		case !found.Found():
			found = RepoConfig{Path: path, Content: content}
		default:
			found.Superseded = append(found.Superseded, path)
		}
	}

	return found, nil
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

// configRoots reports the entries at the repository root that a configuration
// file could be reached through, derived from the paths that are actually
// searched rather than listed a second time.
//
// Derived, because the two must agree: a path searched but not watched is one
// whose edits never invalidate the cache, so the file would change and Smyklot
// would go on reading the old one. Nothing passes a preferred path today, and
// the first thing to do so would otherwise have found that out in production.
func configRoots(preferred string) []string {
	roots := make([]string, 0, len(RepoConfigPaths)+1)

	for _, path := range candidatePaths(preferred) {
		root, _, _ := strings.Cut(path, "/")
		if root != "" && !slices.Contains(roots, root) {
			roots = append(roots, root)
		}
	}

	return roots
}

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
// preferred is the path an operator set in the panel, and is watched alongside
// the standard ones for the same reason it is searched.
//
// A repository with no commits, or one Smyklot cannot read, fingerprints as
// empty, which never compares equal - so it is re-read rather than assumed.
func (c *Client) RepoConfigFingerprint(
	ctx context.Context,
	owner, repo, preferred string,
) (string, error) {
	roots := configRoots(preferred)

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

	found := make(map[string]string, len(roots))

	for _, entry := range entries {
		if slices.Contains(roots, entry.Name) {
			found[entry.Name] = entry.SHA
		}
	}

	// Rendered in a fixed order over a fixed set, so the same repository always
	// fingerprints the same way and an absent entry is distinct from a present
	// one rather than just missing from the middle of a string.
	var builder strings.Builder

	for _, name := range roots {
		builder.WriteString(name)
		builder.WriteByte('=')
		builder.WriteString(found[name])
		builder.WriteByte(';')
	}

	return builder.String(), nil
}
