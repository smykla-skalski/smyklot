package github

import (
	"context"
	"fmt"
	"net/http"

	gogithub "github.com/google/go-github/v90/github"
)

// AddReaction adds an emoji reaction to a comment
//
// The reaction parameter should be one of the ReactionType constants
// (ReactionSuccess, ReactionError, ReactionWarning, ReactionEyes).
func (c *Client) AddReaction(
	ctx context.Context,
	owner, repo string,
	commentID int,
	reaction ReactionType,
) error {
	path := fmt.Sprintf("/repos/%s/%s/issues/comments/%d/reactions", owner, repo, commentID)

	_, _, err := c.gh.Reactions.CreateIssueCommentReaction(
		ctx, owner, repo, int64(commentID), string(reaction),
	)

	return wrapError(ErrAPIRequest, http.MethodPost, path, err)
}

// RemoveReaction removes an emoji reaction from a comment
//
// This retrieves all reactions on the comment and deletes matching ones,
// whoever left them. RemoveReactionByUser is the narrower form.
func (c *Client) RemoveReaction(
	ctx context.Context,
	owner, repo string,
	commentID int,
	reaction ReactionType,
) error {
	return c.removeCommentReactions(ctx, owner, repo, commentID, reaction, "")
}

// RemoveReactionByUser removes a reaction left by one account.
func (c *Client) RemoveReactionByUser(
	ctx context.Context,
	owner, repo string,
	commentID int,
	reaction ReactionType,
	username string,
) error {
	return c.removeCommentReactions(ctx, owner, repo, commentID, reaction, username)
}

// removeCommentReactions deletes every matching reaction on a comment.
//
// An empty username matches any author.
//
// Every page is read before anything is deleted. The version this replaces read
// one page and stopped, so a comment carrying more than thirty reactions kept
// the ones that had spilled onto the second - and the bot's own cleanup then
// left its marks behind on exactly the busy pull requests where they matter.
func (c *Client) removeCommentReactions(
	ctx context.Context,
	owner, repo string,
	commentID int,
	reaction ReactionType,
	username string,
) error {
	path := fmt.Sprintf("/repos/%s/%s/issues/comments/%d/reactions", owner, repo, commentID)

	reactions, err := c.listCommentReactions(ctx, owner, repo, commentID)
	if err != nil {
		return err
	}

	for _, item := range reactions {
		if item.GetContent() != string(reaction) {
			continue
		}

		if username != "" && item.GetUser().GetLogin() != username {
			continue
		}

		_, err := c.gh.Reactions.DeleteIssueCommentReaction(
			ctx, owner, repo, int64(commentID), item.GetID(),
		)
		if err != nil {
			return wrapError(ErrAPIRequest, http.MethodDelete, path, err)
		}
	}

	return nil
}

// GetPRReactions retrieves all reactions on a pull request's own body.
//
// These are the reactions on the description, not on any of its comments.
func (c *Client) GetPRReactions(ctx context.Context, owner, repo string, prNumber int) ([]Reaction, error) {
	op := fmt.Sprintf("/repos/%s/%s/issues/%d/reactions", owner, repo, prNumber)

	raw, err := paginate(ctx, op,
		func(ctx context.Context, opts *gogithub.ListOptions) ([]*gogithub.Reaction, *gogithub.Response, error) {
			return c.gh.Reactions.ListIssueReactions(
				ctx, owner, repo, prNumber, &gogithub.ListReactionOptions{ListOptions: *opts},
			)
		})
	if err != nil {
		return nil, err
	}

	return convertReactions(raw), nil
}

// GetCommentReactions retrieves all reactions on a comment.
func (c *Client) GetCommentReactions(
	ctx context.Context,
	owner, repo string,
	commentID int,
) ([]Reaction, error) {
	raw, err := c.listCommentReactions(ctx, owner, repo, commentID)
	if err != nil {
		return nil, err
	}

	return convertReactions(raw), nil
}

func (c *Client) listCommentReactions(
	ctx context.Context,
	owner, repo string,
	commentID int,
) ([]*gogithub.Reaction, error) {
	op := fmt.Sprintf("/repos/%s/%s/issues/comments/%d/reactions", owner, repo, commentID)

	return paginate(ctx, op,
		func(ctx context.Context, opts *gogithub.ListOptions) ([]*gogithub.Reaction, *gogithub.Response, error) {
			return c.gh.Reactions.ListIssueCommentReactions(
				ctx, owner, repo, int64(commentID), &gogithub.ListReactionOptions{ListOptions: *opts},
			)
		})
}

func convertReactions(raw []*gogithub.Reaction) []Reaction {
	reactions := make([]Reaction, 0, len(raw))

	for _, item := range raw {
		reactions = append(reactions, Reaction{
			Type: ReactionType(item.GetContent()),
			User: item.GetUser().GetLogin(),
		})
	}

	return reactions
}
