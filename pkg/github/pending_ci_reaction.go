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
			found, err := c.commentHasReactionByUser(
				ctx, owner, repo, int(commentID), username, reactionType,
			)
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
	data, err := c.makeRequestWithRetry(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, path, err
	}
	var comments []map[string]interface{}
	if err := json.Unmarshal(data, &comments); err != nil {
		return nil, path, NewAPIError(ErrResponseParse, 0, http.MethodGet, path, err)
	}

	return comments, path, nil
}

func (c *Client) commentHasReactionByUser(
	ctx context.Context,
	owner, repo string,
	commentID int,
	username string,
	reactionType ReactionType,
) (bool, error) {
	for page := 1; page <= maxPages; page++ {
		path := fmt.Sprintf(
			"/repos/%s/%s/issues/comments/%d/reactions?per_page=%d&page=%d",
			owner, repo, commentID, pageSize, page,
		)
		data, err := c.makeRequestWithRetry(ctx, http.MethodGet, path, nil)
		if err != nil {
			return false, err
		}
		var raw []map[string]interface{}
		if err := json.Unmarshal(data, &raw); err != nil {
			return false, NewAPIError(ErrResponseParse, 0, http.MethodGet, path, err)
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
