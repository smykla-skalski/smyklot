// Package permissions provides CODEOWNERS-based authorization for Smyklot.
//
// It validates user permissions by parsing .github/CODEOWNERS files and
// checking if users have approval rights for repository changes.
package permissions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// GitHubClient defines the interface for GitHub API operations needed by Checker
type GitHubClient interface {
	IsTeamMember(ctx context.Context, org, teamSlug, username string) (bool, error)
}

// Checker validates user permissions based on CODEOWNERS files
//
// It reads .github/CODEOWNERS and honours two things: the global owners named
// by the * pattern, who may approve anything, and team ownership
// (@org/team-name), resolved through the GitHub API when a client is supplied.
//
// Path-specific ownership is not implemented. A CODEOWNERS line naming a path
// is parsed and then ignored, so nobody gains permission from one and nobody
// loses permission they would otherwise have.
type Checker struct {
	rootApprovers []string
	githubClient  GitHubClient
}

// NewCheckerFromContent creates a new permission checker from CODEOWNERS content
//
// This is useful when the CODEOWNERS content is fetched from an API
// rather than read from the filesystem.
//
// The githubClient parameter is optional. If provided, the checker will support
// team membership validation. If nil, team ownership will be treated as individual
// usernames (backward compatible behavior).
//
// Returns an error if the content cannot be parsed.
func NewCheckerFromContent(content string, githubClient GitHubClient) (*Checker, error) {
	checker := &Checker{
		rootApprovers: []string{},
		githubClient:  githubClient,
	}

	if content == "" {
		return checker, nil
	}

	codeowners, err := ParseCodeownersContent(content)
	if err != nil {
		// Fail-closed: return error if CODEOWNERS cannot be parsed
		// This prevents privilege escalation via corrupted CODEOWNERS files
		return nil, NewCheckerError(ErrParseFailed, "content", err)
	}

	checker.rootApprovers = codeowners.GetGlobalOwners()

	return checker, nil
}

// NewChecker creates a new permission checker for the given repository path
//
// The checker loads the .github/CODEOWNERS file if it exists. If no
// CODEOWNERS file exists, all permission checks will return false.
//
// The githubClient parameter is optional. If provided, the checker will support
// team membership validation. If nil, team ownership will be treated as individual
// usernames (backward compatible behavior).
//
// Returns an error if:
//   - The repository path is empty
//   - The repository path does not exist
func NewChecker(repoPath string, githubClient GitHubClient) (*Checker, error) {
	if repoPath == "" {
		return nil, ErrEmptyRepoPath
	}

	// Check if repository path exists
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		return nil, NewCheckerError(ErrRepoPathNotExist, repoPath, err)
	}

	checker := &Checker{
		rootApprovers: []string{},
		githubClient:  githubClient,
	}

	// Try to load .github/CODEOWNERS file
	codeownersPath := filepath.Join(repoPath, ".github", "CODEOWNERS")
	if _, err := os.Stat(codeownersPath); err == nil {
		codeowners, err := ParseCodeownersFile(codeownersPath)
		if err != nil {
			// Fail-closed: return error if CODEOWNERS cannot be parsed
			// This prevents privilege escalation via corrupted CODEOWNERS files
			return nil, err
		}
		checker.rootApprovers = codeowners.GetGlobalOwners()
	}

	return checker, nil
}

// isTeamMember checks if a user is a member of a team via GitHub API
func (c *Checker) isTeamMember(approver, username string) (bool, error) {
	if c.githubClient == nil {
		return false, nil
	}

	parts := strings.SplitN(approver, "/", 2)
	if len(parts) != 2 {
		return false, nil
	}

	org, teamSlug := parts[0], parts[1]
	return c.githubClient.IsTeamMember(context.Background(), org, teamSlug, username)
}

// CanApprove reports whether a user may approve changes.
//
// The path argument is accepted and ignored: approval is not scoped to the
// files a pull request touches, so a global owner may approve any of them. The
// parameter is kept so that adding scope later does not change every call site.
//
// A user matches by being a global owner, or by belonging to a team named as
// one - checked against the GitHub API when a client was supplied, and by
// string comparison when it was not.
//
// An empty username is never an approver.
func (c *Checker) CanApprove(username, _ string) (bool, error) {
	if username == "" {
		return false, nil
	}

	// Global owners, and any team they belong to
	for _, approver := range c.rootApprovers {
		// Check if approver is a team (contains '/')
		if strings.Contains(approver, "/") {
			isMember, err := c.isTeamMember(approver, username)
			if err != nil {
				return false, err
			}
			if isMember {
				return true, nil
			}
			continue
		}

		// Individual user ownership: exact match
		if approver == username {
			return true, nil
		}
	}

	return false, nil
}

// GetApprovers returns everyone named as a global owner.
//
// Teams are returned as they were written, not expanded into their members:
// membership is resolved per user at check time, and expanding it here would
// mean an API call for every team on every read.
func (c *Checker) GetApprovers() []string {
	return c.rootApprovers
}
