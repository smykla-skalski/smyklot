package github

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"

	gogithub "github.com/google/go-github/v90/github"
)

const pendingCIReactionScanAttempts = 3

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
		c.pendingCIPullRequestReactionPager(ctx, owner, repo, pullRequest),
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
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/reactions", owner, repo, pullRequest)
	_, _, err := c.gh.Reactions.CreateIssueReaction(
		ctx, owner, repo, pullRequest, string(reactionType),
	)

	return wrapError(ErrAPIRequest, http.MethodPost, path, err)
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
	reaction, found, err := locatePendingCIReaction(
		c.pendingCIPullRequestReactionPager(ctx, owner, repo, pullRequest),
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

type pendingCIReactionPage struct {
	items []*gogithub.Reaction
	next  int
	path  string
}

type pendingCIReactionPager func(int) (pendingCIReactionPage, error)
type pendingCIReactionFetcher func(
	*gogithub.ListReactionOptions,
) ([]*gogithub.Reaction, *gogithub.Response, error)

type pendingCIReactionScan struct {
	match       *gogithub.Reaction
	fingerprint [sha256.Size]byte
	pages       int
	path        string
}

func (c *Client) pendingCIPullRequestReactionPager(
	ctx context.Context,
	owner, repo string,
	pullRequest int,
) pendingCIReactionPager {
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/reactions", owner, repo, pullRequest)

	return newPendingCIReactionPager(
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

func newPendingCIReactionPager(
	path string,
	fetch pendingCIReactionFetcher,
) pendingCIReactionPager {
	return func(page int) (pendingCIReactionPage, error) {
		items, response, err := fetch(&gogithub.ListReactionOptions{
			ListOptions: gogithub.ListOptions{Page: page, PerPage: pageSize},
		})
		if err != nil {
			return pendingCIReactionPage{},
				wrapError(decodeOp(response, err), http.MethodGet, path, err)
		}

		return pendingCIReactionPage{
			items: items, next: nextPage(response), path: path,
		}, nil
	}
}

func findPendingCIReaction(
	page pendingCIReactionPager,
	username string,
	reactionType ReactionType,
	subject string,
) (bool, error) {
	_, found, err := locatePendingCIReaction(page, username, reactionType, subject)

	return found, err
}

func locatePendingCIReaction(
	page pendingCIReactionPager,
	username string,
	reactionType ReactionType,
	subject string,
) (*gogithub.Reaction, bool, error) {
	var previous [sha256.Size]byte
	hasPrevious := false
	lastPath := ""
	for range pendingCIReactionScanAttempts {
		scan, err := scanPendingCIReactions(page, username, reactionType, subject)
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

	return nil, false, pendingCIReactionMutationError(subject, lastPath)
}

func scanPendingCIReactions(
	page pendingCIReactionPager,
	username string,
	reactionType ReactionType,
	subject string,
) (pendingCIReactionScan, error) {
	digest := sha256.New()
	number := 1
	for pageCount := 1; pageCount <= maxPages; pageCount++ {
		current, err := page(number)
		if err != nil {
			return pendingCIReactionScan{}, err
		}
		for _, reaction := range current.items {
			if pendingCIReactionMatches(reaction, username, reactionType) {
				return pendingCIReactionScan{match: reaction}, nil
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

			return pendingCIReactionScan{
				fingerprint: fingerprint, pages: pageCount, path: current.path,
			}, nil
		}
		if pageCount == maxPages {
			return pendingCIReactionScan{},
				pendingCIReactionPaginationError(subject, current.path, pageCount)
		}
		number++
	}

	return pendingCIReactionScan{}, nil
}

func pendingCIReactionMatches(
	reaction *gogithub.Reaction,
	username string,
	reactionType ReactionType,
) bool {
	return reaction.GetUser().GetLogin() == username &&
		reaction.GetContent() == string(reactionType)
}

func pendingCIReactionPaginationError(subject, path string, page int) error {
	return NewAPIError(
		ErrIncompletePagination,
		0,
		http.MethodGet,
		path,
		fmt.Errorf("%s list still has a full page after %d pages", subject, page),
	)
}

func pendingCIReactionMutationError(subject, path string) error {
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

// HasPullRequestCommentReaction reports whether one user left a specific
// reaction on any pull-request comment.
func (c *Client) HasPullRequestCommentReaction(
	ctx context.Context,
	owner, repo string,
	pullRequest int,
	username string,
	reactionType ReactionType,
) (bool, error) {
	comments, err := c.pendingCIPullRequestComments(ctx, owner, repo, pullRequest)
	if err != nil {
		return false, err
	}
	for _, comment := range comments {
		found, err := findPendingCIReaction(
			c.pendingCICommentReactionPager(ctx, owner, repo, comment.GetID()),
			username,
			reactionType,
			"reaction",
		)
		if err != nil || found {
			return found, err
		}
	}

	return false, nil
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
	comments, err := c.pendingCIPullRequestComments(ctx, owner, repo, pullRequest)
	if err != nil {
		return err
	}
	for _, comment := range comments {
		if err := c.removePendingCICommentReaction(
			ctx, owner, repo, comment.GetID(), username, reactionType,
		); err != nil {
			return err
		}
	}

	return nil
}

func (c *Client) pendingCIPullRequestComments(
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

func (c *Client) pendingCICommentReactionPager(
	ctx context.Context,
	owner, repo string,
	commentID int64,
) pendingCIReactionPager {
	path := fmt.Sprintf("/repos/%s/%s/issues/comments/%d/reactions", owner, repo, commentID)

	return newPendingCIReactionPager(
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

func (c *Client) removePendingCICommentReaction(
	ctx context.Context,
	owner, repo string,
	commentID int64,
	username string,
	reactionType ReactionType,
) error {
	path := fmt.Sprintf("/repos/%s/%s/issues/comments/%d/reactions", owner, repo, commentID)
	reaction, found, err := locatePendingCIReaction(
		c.pendingCICommentReactionPager(ctx, owner, repo, commentID),
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
