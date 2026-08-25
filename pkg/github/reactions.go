package github

import (
	"context"
	"fmt"
	"net/http"

	gogithub "github.com/google/go-github/v90/github"
)

// AddReaction adds an emoji reaction to a comment
//
// The reaction parameter is one of the eight ReactionType constants; GitHub
// rejects anything else.
func (c *Client) AddReaction(
	ctx context.Context,
	owner, repo string,
	commentID int,
	reaction ReactionType,
) error {
	_, err := c.AddReactionState(ctx, owner, repo, commentID, reaction)

	return err
}

// AddReactionState adds an emoji reaction and returns GitHub's durable state.
func (c *Client) AddReactionState(
	ctx context.Context,
	owner, repo string,
	commentID int,
	reaction ReactionType,
) (Reaction, error) {
	path := fmt.Sprintf("/repos/%s/%s/issues/comments/%d/reactions", owner, repo, commentID)

	created, _, err := c.gh.Reactions.CreateIssueCommentReaction(
		ctx, owner, repo, int64(commentID), string(reaction),
	)
	if err != nil {
		return Reaction{}, wrapError(ErrAPIRequest, http.MethodPost, path, err)
	}

	return convertReaction(created), nil
}

// RemoveCommentReaction deletes one exact reaction from a comment.
func (c *Client) RemoveCommentReaction(
	ctx context.Context,
	owner, repo string,
	commentID int,
	reactionID int64,
) error {
	path := fmt.Sprintf("/repos/%s/%s/issues/comments/%d/reactions/%d", owner, repo, commentID, reactionID)
	_, err := c.gh.Reactions.DeleteIssueCommentReaction(
		ctx, owner, repo, int64(commentID), reactionID,
	)

	return wrapError(ErrAPIRequest, http.MethodDelete, path, err)
}

// RemoveReactionByUser deletes every matching reaction one account left on a
// comment.
//
// Every page is read before anything is deleted. The version this replaces read
// one page and stopped, so a comment carrying more than thirty reactions kept
// the ones that had spilled onto the second - and the bot's own cleanup then
// left its marks behind on exactly the busy pull requests where they matter.
func (c *Client) RemoveReactionByUser(
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

		if item.GetUser().GetLogin() != username {
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
//
//nolint:dupl // paginate-and-convert is the idiom every list read here follows
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
		reactions = append(reactions, convertReaction(item))
	}

	return reactions
}

func convertReaction(item *gogithub.Reaction) Reaction {
	return Reaction{
		ID:        item.GetID(),
		Type:      ReactionType(item.GetContent()),
		User:      item.GetUser().GetLogin(),
		CreatedAt: item.GetCreatedAt().Time,
	}
}
