package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	gogithub "github.com/google/go-github/v90/github"
)

// AddLabel adds a label to a pull request
func (c *Client) AddLabel(ctx context.Context, owner, repo string, prNumber int, label string) error {
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/labels", owner, repo, prNumber)

	_, _, err := c.gh.Issues.AddLabelsToIssue(ctx, owner, repo, prNumber, []string{label})

	return wrapError(ErrAPIRequest, http.MethodPost, path, err)
}

// RemoveLabel removes a label from a pull request
//
// The label name is a path segment, so it has to be escaped. It used to be
// interpolated raw, which was survivable only because every label Smyklot
// managed spelled itself `smyklot:pending:ci` and a colon happens to be legal
// there. A slash addressed a different endpoint entirely, a `#` or a `?`
// truncated the path, and the organization's own label set is `kind/*`,
// `area/*` and `ci/*` - so this goes live the moment label sync manages them.
//
// The escaping happens here rather than being left to the library, because
// go-github interpolates the label into its path with the same bare Sprintf the
// hand-rolled client used. Passing it an already-escaped segment is what makes
// the request correct; it parses the result and preserves the encoding.
func (c *Client) RemoveLabel(ctx context.Context, owner, repo string, prNumber int, label string) error {
	escaped := url.PathEscape(label)
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/labels/%s", owner, repo, prNumber, escaped)

	_, err := c.gh.Issues.RemoveLabelForIssue(ctx, owner, repo, prNumber, escaped)

	return wrapError(ErrAPIRequest, http.MethodDelete, path, err)
}

// GetLabels retrieves all labels from a pull request
//
// Paginated, unlike the request it replaces: GitHub's default page is 30, and a
// pull request carrying more labels than that silently reported a short list.
func (c *Client) GetLabels(ctx context.Context, owner, repo string, prNumber int) ([]string, error) {
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/labels", owner, repo, prNumber)

	raw, err := paginate(ctx, path,
		func(ctx context.Context, opts *gogithub.ListOptions) ([]*gogithub.Label, *gogithub.Response, error) {
			return c.gh.Issues.ListLabelsByIssue(ctx, owner, repo, prNumber, opts)
		})
	if err != nil {
		return nil, err
	}

	labels := make([]string, 0, len(raw))

	for _, label := range raw {
		if name := label.GetName(); name != "" {
			labels = append(labels, name)
		}
	}

	return labels, nil
}
