package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// HasPullRequestReaction reports whether one user left a specific reaction on
// the pull request itself. Unlike a comment reaction, this survives deletion
// of the command comment and can therefore fence service-owned work safely.
func (c *Client) HasPullRequestReaction(
	ctx context.Context,
	owner, repo string,
	pullRequest int,
	username string,
	reactionType ReactionType,
) (bool, error) {
	return findPendingCIReaction(
		func(page int) ([]map[string]interface{}, string, error) {
			return c.pendingCIPullRequestReactionsPage(
				ctx, owner, repo, pullRequest, page,
			)
		},
		username, reactionType, "pull request reaction",
	)
}

// AddPullRequestReaction adds a reaction to the pull request itself.
func (c *Client) AddPullRequestReaction(
	ctx context.Context,
	owner, repo string,
	pullRequest int,
	reactionType ReactionType,
) error {
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/reactions", owner, repo, pullRequest)
	_, err := c.makeRequest(ctx, http.MethodPost, path, map[string]string{
		"content": string(reactionType),
	})

	return err
}

// RemovePullRequestReactionByUser removes one user's matching reaction from
// the pull request itself.
func (c *Client) RemovePullRequestReactionByUser(
	ctx context.Context,
	owner, repo string,
	pullRequest int,
	username string,
	reactionType ReactionType,
) error {
	return removePendingCIReactions(
		func(page int) ([]map[string]interface{}, string, error) {
			return c.pendingCIPullRequestReactionsPage(
				ctx, owner, repo, pullRequest, page,
			)
		},
		func(raw map[string]interface{}) error {
			return c.removeMatchingPullRequestReaction(
				ctx, owner, repo, pullRequest, username, reactionType, raw,
			)
		},
		"pull request reaction",
	)
}

func (c *Client) pendingCIPullRequestReactionsPage(
	ctx context.Context,
	owner, repo string,
	pullRequest, page int,
) ([]map[string]interface{}, string, error) {
	path := fmt.Sprintf(
		"/repos/%s/%s/issues/%d/reactions?per_page=%d&page=%d",
		owner, repo, pullRequest, pageSize, page,
	)
	raw, err := c.pendingCIObjectsPage(ctx, path)

	return raw, path, err
}

func (c *Client) removeMatchingPullRequestReaction(
	ctx context.Context,
	owner, repo string,
	pullRequest int,
	username string,
	reactionType ReactionType,
	raw map[string]interface{},
) error {
	if !pendingCIReactionMatches(raw, username, reactionType) {
		return nil
	}
	id, ok := raw["id"].(float64)
	if !ok {
		return nil
	}
	path := fmt.Sprintf(
		"/repos/%s/%s/issues/%d/reactions/%d",
		owner, repo, pullRequest, int(id),
	)
	_, err := c.makeRequest(ctx, http.MethodDelete, path, nil)

	return err
}

type pendingCIReactionPager func(int) ([]map[string]interface{}, string, error)
type pendingCIReactionRemover func(map[string]interface{}) error

func findPendingCIReaction(
	page pendingCIReactionPager,
	username string,
	reactionType ReactionType,
	subject string,
) (bool, error) {
	for number := 1; number <= maxPages; number++ {
		raw, path, err := page(number)
		if err != nil {
			return false, err
		}
		for _, reaction := range raw {
			if pendingCIReactionMatches(reaction, username, reactionType) {
				return true, nil
			}
		}
		if len(raw) < pageSize {
			return false, nil
		}
		if number == maxPages {
			return false, pendingCIReactionPaginationError(subject, path, number)
		}
	}

	return false, nil
}

func removePendingCIReactions(
	page pendingCIReactionPager,
	remove pendingCIReactionRemover,
	subject string,
) error {
	for number := 1; number <= maxPages; number++ {
		raw, path, err := page(number)
		if err != nil {
			return err
		}
		for _, reaction := range raw {
			if err := remove(reaction); err != nil {
				return err
			}
		}
		if len(raw) < pageSize {
			return nil
		}
		if number == maxPages {
			return pendingCIReactionPaginationError(subject, path, number)
		}
	}

	return nil
}

func pendingCIReactionMatches(
	raw map[string]interface{},
	username string,
	reactionType ReactionType,
) bool {
	parsed := parseReactions([]map[string]interface{}{raw})

	return len(parsed) == 1 && parsed[0].User == username && parsed[0].Type == reactionType
}

func pendingCIReactionPaginationError(subject, path string, page int) error {
	return NewAPIError(
		ErrIncompletePagination, 0, http.MethodGet, path,
		fmt.Errorf("%s list still has a full page after %d pages", subject, page),
	)
}

// HasPullRequestCommentReaction reports whether one user left a specific
// reaction on any pull-request comment.
func (c *Client) HasPullRequestCommentReaction(
	ctx context.Context,
	owner, repo string,
	pullRequest int,
	username string,
	reactionType ReactionType,
) (bool, error) {
	return c.visitPullRequestComments(
		ctx, owner, repo, pullRequest,
		func(commentID int) (bool, error) {
			return c.commentHasReactionByUser(
				ctx, owner, repo, commentID, username, reactionType,
			)
		},
	)
}

// RemovePullRequestCommentReactionsByUser removes one user's matching
// reactions from every comment on a pull request.
func (c *Client) RemovePullRequestCommentReactionsByUser(
	ctx context.Context,
	owner, repo string,
	pullRequest int,
	username string,
	reactionType ReactionType,
) error {
	_, err := c.visitPullRequestComments(
		ctx, owner, repo, pullRequest,
		func(commentID int) (bool, error) {
			return false, c.removeCommentReactionsByUser(
				ctx, owner, repo, commentID, username, reactionType,
			)
		},
	)

	return err
}

type pendingCICommentVisitor func(int) (bool, error)

func (c *Client) visitPullRequestComments(
	ctx context.Context,
	owner, repo string,
	pullRequest int,
	visit pendingCICommentVisitor,
) (bool, error) {
	for page := 1; page <= maxPages; page++ {
		comments, path, err := c.pendingCICommentsPage(
			ctx, owner, repo, pullRequest, page,
		)
		if err != nil {
			return false, err
		}
		for _, comment := range comments {
			commentID, ok := comment["id"].(float64)
			if !ok {
				continue
			}
			found, err := visit(int(commentID))
			if err != nil || found {
				return found, err
			}
		}
		if len(comments) < pageSize {
			return false, nil
		}
		if page == maxPages {
			return false, NewAPIError(
				ErrIncompletePagination, 0, http.MethodGet, path,
				fmt.Errorf("comment list still has a full page after %d pages", page),
			)
		}
	}

	return false, nil
}

func (c *Client) pendingCICommentsPage(
	ctx context.Context,
	owner, repo string,
	pullRequest, page int,
) ([]map[string]interface{}, string, error) {
	path := fmt.Sprintf(
		"/repos/%s/%s/issues/%d/comments?per_page=%d&page=%d",
		owner, repo, pullRequest, pageSize, page,
	)
	comments, err := c.pendingCIObjectsPage(ctx, path)

	return comments, path, err
}

func (c *Client) commentHasReactionByUser(
	ctx context.Context,
	owner, repo string,
	commentID int,
	username string,
	reactionType ReactionType,
) (bool, error) {
	return findPendingCIReaction(
		func(page int) ([]map[string]interface{}, string, error) {
			return c.pendingCIReactionsPage(ctx, owner, repo, commentID, page)
		},
		username, reactionType, "reaction",
	)
}

func (c *Client) removeCommentReactionsByUser(
	ctx context.Context,
	owner, repo string,
	commentID int,
	username string,
	reactionType ReactionType,
) error {
	return removePendingCIReactions(
		func(page int) ([]map[string]interface{}, string, error) {
			return c.pendingCIReactionsPage(ctx, owner, repo, commentID, page)
		},
		func(raw map[string]interface{}) error {
			return c.removeMatchingCommentReaction(
				ctx, owner, repo, commentID, username, reactionType, raw,
			)
		},
		"reaction",
	)
}

func (c *Client) removeMatchingCommentReaction(
	ctx context.Context,
	owner, repo string,
	commentID int,
	username string,
	reactionType ReactionType,
	raw map[string]interface{},
) error {
	if !pendingCIReactionMatches(raw, username, reactionType) {
		return nil
	}
	reactionID, ok := raw["id"].(float64)
	if !ok {
		return nil
	}
	path := fmt.Sprintf(
		"/repos/%s/%s/issues/comments/%d/reactions/%d",
		owner, repo, commentID, int(reactionID),
	)
	_, err := c.makeRequestWithRetry(ctx, http.MethodDelete, path, nil)

	return err
}

func (c *Client) pendingCIReactionsPage(
	ctx context.Context,
	owner, repo string,
	commentID, page int,
) ([]map[string]interface{}, string, error) {
	path := fmt.Sprintf(
		"/repos/%s/%s/issues/comments/%d/reactions?per_page=%d&page=%d",
		owner, repo, commentID, pageSize, page,
	)
	raw, err := c.pendingCIObjectsPage(ctx, path)

	return raw, path, err
}

func (c *Client) pendingCIObjectsPage(
	ctx context.Context,
	path string,
) ([]map[string]interface{}, error) {
	data, err := c.makeRequestWithRetry(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var raw []map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, NewAPIError(ErrResponseParse, 0, http.MethodGet, path, err)
	}

	return raw, nil
}
