package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

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
	for page := 1; page <= maxPages; page++ {
		raw, path, err := c.pendingCIReactionsPage(
			ctx, owner, repo, commentID, page,
		)
		if err != nil {
			return false, err
		}
		for _, reaction := range parseReactions(raw) {
			if reaction.User == username && reaction.Type == reactionType {
				return true, nil
			}
		}
		if len(raw) < pageSize {
			return false, nil
		}
		if page == maxPages {
			return false, NewAPIError(
				ErrIncompletePagination, 0, http.MethodGet, path,
				fmt.Errorf("reaction list still has a full page after %d pages", page),
			)
		}
	}

	return false, nil
}

func (c *Client) removeCommentReactionsByUser(
	ctx context.Context,
	owner, repo string,
	commentID int,
	username string,
	reactionType ReactionType,
) error {
	for page := 1; page <= maxPages; page++ {
		raw, path, err := c.pendingCIReactionsPage(
			ctx, owner, repo, commentID, page,
		)
		if err != nil {
			return err
		}
		for _, reaction := range raw {
			if err := c.removeMatchingCommentReaction(
				ctx, owner, repo, commentID, username, reactionType, reaction,
			); err != nil {
				return err
			}
		}
		if len(raw) < pageSize {
			return nil
		}
		if page == maxPages {
			return NewAPIError(
				ErrIncompletePagination, 0, http.MethodGet, path,
				fmt.Errorf("reaction list still has a full page after %d pages", page),
			)
		}
	}

	return nil
}

func (c *Client) removeMatchingCommentReaction(
	ctx context.Context,
	owner, repo string,
	commentID int,
	username string,
	reactionType ReactionType,
	raw map[string]interface{},
) error {
	parsed := parseReactions([]map[string]interface{}{raw})
	if len(parsed) != 1 || parsed[0].User != username || parsed[0].Type != reactionType {
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
