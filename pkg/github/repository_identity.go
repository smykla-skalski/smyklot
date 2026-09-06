package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// GetRepositoryByID resolves a repository's current name and default branch
// without trusting a cached name that may have been claimed by another repository.
func (c *Client) GetRepositoryByID(ctx context.Context, id int64) (Repository, error) {
	if id <= 0 {
		return Repository{}, errors.New("repository identity must be positive")
	}
	repository, _, err := c.gh.Repositories.GetByID(ctx, id)
	if err != nil {
		return Repository{}, wrapError(ErrAPIRequest, http.MethodGet, fmt.Sprintf("/repositories/%d", id), err)
	}
	if repository.GetID() != id {
		return Repository{}, errors.New("GitHub returned a different repository identity")
	}
	return Repository{
		ID: repository.GetID(), Owner: repository.GetOwner().GetLogin(), Name: repository.GetName(),
		FullName: repository.GetFullName(), DefaultBranch: repository.GetDefaultBranch(), Private: repository.GetPrivate(),
	}, nil
}
