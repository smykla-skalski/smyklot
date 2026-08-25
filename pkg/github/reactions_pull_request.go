package github

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"

	gogithub "github.com/google/go-github/v90/github"
)

const reactionScanAttempts = 3

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
	return findReaction(
		c.pullRequestReactionPager(ctx, owner, repo, pullRequest),
		username,
		reactionType,
		"pull request reaction",
	)
}

// AddPullRequestReaction adds a reaction to the pull request itself.
func (c *Client) AddPullRequestReaction(
	ctx context.Context,
	owner, repo string,
	pullRequest int,
	reactionType ReactionType,
) error {
	_, err := c.AddPullRequestReactionState(
		ctx, owner, repo, pullRequest, reactionType,
	)

	return err
}

// AddPullRequestReactionState adds a reaction and returns its durable state.
func (c *Client) AddPullRequestReactionState(
	ctx context.Context,
	owner, repo string,
	pullRequest int,
	reactionType ReactionType,
) (Reaction, error) {
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/reactions", owner, repo, pullRequest)
	created, _, err := c.gh.Reactions.CreateIssueReaction(
		ctx, owner, repo, pullRequest, string(reactionType),
	)
	if err != nil {
		return Reaction{}, wrapError(ErrAPIRequest, http.MethodPost, path, err)
	}

	return convertReaction(created), nil
}

// RemovePullRequestReaction removes one exact reaction from the pull request.
func (c *Client) RemovePullRequestReaction(
	ctx context.Context,
	owner, repo string,
	pullRequest int,
	reactionID int64,
) error {
	path := fmt.Sprintf(
		"/repos/%s/%s/issues/%d/reactions/%d", owner, repo, pullRequest, reactionID,
	)
	_, err := c.gh.Reactions.DeleteIssueReaction(
		ctx, owner, repo, pullRequest, reactionID,
	)

	return wrapError(ErrAPIRequest, http.MethodDelete, path, err)
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
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/reactions", owner, repo, pullRequest)
	reaction, found, err := locateReaction(
		c.pullRequestReactionPager(ctx, owner, repo, pullRequest),
		username,
		reactionType,
		"pull request reaction",
	)
	if err != nil || !found {
		return err
	}
	_, err = c.gh.Reactions.DeleteIssueReaction(
		ctx, owner, repo, pullRequest, reaction.GetID(),
	)

	return wrapError(ErrAPIRequest, http.MethodDelete, path, err)
}

type reactionPage struct {
	items []*gogithub.Reaction
	next  int
	path  string
}

type (
	reactionPager   func(int) (reactionPage, error)
	reactionFetcher func(
		*gogithub.ListReactionOptions,
	) ([]*gogithub.Reaction, *gogithub.Response, error)
)

type reactionScan struct {
	match       *gogithub.Reaction
	fingerprint [sha256.Size]byte
	pages       int
	path        string
}

func (c *Client) pullRequestReactionPager(
	ctx context.Context,
	owner, repo string,
	pullRequest int,
) reactionPager {
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/reactions", owner, repo, pullRequest)

	return newReactionPager(
		path,
		func(opts *gogithub.ListReactionOptions) (
			[]*gogithub.Reaction,
			*gogithub.Response,
			error,
		) {
			return c.gh.Reactions.ListIssueReactions(
				ctx, owner, repo, pullRequest, opts,
			)
		},
	)
}

func newReactionPager(
	path string,
	fetch reactionFetcher,
) reactionPager {
	return func(page int) (reactionPage, error) {
		items, response, err := fetch(&gogithub.ListReactionOptions{
			ListOptions: gogithub.ListOptions{Page: page, PerPage: pageSize},
		})
		if err != nil {
			return reactionPage{},
				wrapError(decodeOp(response, err), http.MethodGet, path, err)
		}

		return reactionPage{
			items: items, next: nextPage(response), path: path,
		}, nil
	}
}

func findReaction(
	page reactionPager,
	username string,
	reactionType ReactionType,
	subject string,
) (bool, error) {
	_, found, err := locateReaction(page, username, reactionType, subject)

	return found, err
}

func locateReaction(
	page reactionPager,
	username string,
	reactionType ReactionType,
	subject string,
) (*gogithub.Reaction, bool, error) {
	var previous [sha256.Size]byte
	hasPrevious := false
	lastPath := ""
	for range reactionScanAttempts {
		scan, err := scanReactions(page, username, reactionType, subject)
		if err != nil {
			return nil, false, err
		}
		if scan.match != nil {
			return scan.match, true, nil
		}
		if scan.pages == 1 || hasPrevious && scan.fingerprint == previous {
			return nil, false, nil
		}
		previous = scan.fingerprint
		hasPrevious = true
		lastPath = scan.path
	}

	return nil, false, reactionMutationError(subject, lastPath)
}

func scanReactions(
	page reactionPager,
	username string,
	reactionType ReactionType,
	subject string,
) (reactionScan, error) {
	digest := sha256.New()
	number := 1
	for pageCount := 1; pageCount <= maxPages; pageCount++ {
		current, err := page(number)
		if err != nil {
			return reactionScan{}, err
		}
		for _, reaction := range current.items {
			if reactionMatches(reaction, username, reactionType) {
				return reactionScan{match: reaction}, nil
			}
			_, _ = fmt.Fprintf(
				digest,
				"%d\x00%s\x00%s\x00",
				reaction.GetID(),
				reaction.GetContent(),
				reaction.GetUser().GetLogin(),
			)
		}
		if current.next > 0 {
			number = current.next

			continue
		}
		if len(current.items) < pageSize {
			var fingerprint [sha256.Size]byte
			copy(fingerprint[:], digest.Sum(nil))

			return reactionScan{
				fingerprint: fingerprint, pages: pageCount, path: current.path,
			}, nil
		}
		if pageCount == maxPages {
			return reactionScan{},
				reactionPaginationError(subject, current.path, pageCount)
		}
		number++
	}

	return reactionScan{}, nil
}

func reactionMatches(
	reaction *gogithub.Reaction,
	username string,
	reactionType ReactionType,
) bool {
	return reaction.GetUser().GetLogin() == username &&
		reaction.GetContent() == string(reactionType)
}

func reactionPaginationError(subject, path string, page int) error {
	return NewAPIError(
		ErrIncompletePagination,
		0,
		http.MethodGet,
		path,
		fmt.Errorf("%s list still has a full page after %d pages", subject, page),
	)
}

func reactionMutationError(subject, path string) error {
	return NewAPIError(
		ErrIncompletePagination,
		0,
		http.MethodGet,
		path,
		fmt.Errorf("%s list changed during pagination", subject),
	)
}

func nextPage(response *gogithub.Response) int {
	if response == nil {
		return 0
	}

	return response.NextPage
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
	comments, err := c.pullRequestComments(ctx, owner, repo, pullRequest)
	if err != nil {
		return err
	}
	for _, comment := range comments {
		if err := c.removeOneCommentReaction(
			ctx, owner, repo, comment.GetID(), username, reactionType,
		); err != nil {
			return err
		}
	}

	return nil
}

func (c *Client) pullRequestComments(
	ctx context.Context,
	owner, repo string,
	pullRequest int,
) ([]*gogithub.IssueComment, error) {
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/comments", owner, repo, pullRequest)

	return paginate(
		ctx,
		path,
		func(
			ctx context.Context,
			opts *gogithub.ListOptions,
		) ([]*gogithub.IssueComment, *gogithub.Response, error) {
			return c.gh.Issues.ListComments(
				ctx,
				owner,
				repo,
				pullRequest,
				&gogithub.IssueListCommentsOptions{ListOptions: *opts},
			)
		},
	)
}

func (c *Client) commentReactionPager(
	ctx context.Context,
	owner, repo string,
	commentID int64,
) reactionPager {
	path := fmt.Sprintf("/repos/%s/%s/issues/comments/%d/reactions", owner, repo, commentID)

	return newReactionPager(
		path,
		func(opts *gogithub.ListReactionOptions) (
			[]*gogithub.Reaction,
			*gogithub.Response,
			error,
		) {
			return c.gh.Reactions.ListIssueCommentReactions(
				ctx, owner, repo, commentID, opts,
			)
		},
	)
}

func (c *Client) removeOneCommentReaction(
	ctx context.Context,
	owner, repo string,
	commentID int64,
	username string,
	reactionType ReactionType,
) error {
	path := fmt.Sprintf("/repos/%s/%s/issues/comments/%d/reactions", owner, repo, commentID)
	reaction, found, err := locateReaction(
		c.commentReactionPager(ctx, owner, repo, commentID),
		username,
		reactionType,
		"reaction",
	)
	if err != nil || !found {
		return err
	}
	_, err = c.gh.Reactions.DeleteIssueCommentReaction(
		ctx, owner, repo, commentID, reaction.GetID(),
	)

	return wrapError(ErrAPIRequest, http.MethodDelete, path, err)
}
