package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	gogithub "github.com/google/go-github/v90/github"
)

// RepositoryLabel is a label as the repository has it.
//
// Distinct from the label names the pull-request methods deal in, because those
// answer "which labels are on this thing" and these answer "what labels does
// this repository define" - the second needs the colour and the description to
// say whether anything has drifted.
type RepositoryLabel struct {
	Name        string
	Color       string
	Description string
}

// ListRepositoryLabels reads every label a repository defines.
//
// Paginated. GitHub's default page is 30 and an organization's label set is
// routinely larger, so an unpaginated read would report labels past the first
// page as missing - and a sync would then create them, get a 422 for a label
// that already exists, and abandon the rest of that repository.
func (c *Client) ListRepositoryLabels(
	ctx context.Context,
	owner, repo string,
) ([]RepositoryLabel, error) {
	path := fmt.Sprintf("/repos/%s/%s/labels", owner, repo)

	raw, err := paginate(ctx, path,
		func(ctx context.Context, opts *gogithub.ListOptions) (
			[]*gogithub.Label, *gogithub.Response, error,
		) {
			return c.gh.Issues.ListLabels(ctx, owner, repo, opts)
		})
	if err != nil {
		return nil, err
	}

	labels := make([]RepositoryLabel, 0, len(raw))
	for _, label := range raw {
		labels = append(labels, RepositoryLabel{
			Name:        label.GetName(),
			Color:       label.GetColor(),
			Description: label.GetDescription(),
		})
	}

	return labels, nil
}

// CreateRepositoryLabel defines a label on a repository.
func (c *Client) CreateRepositoryLabel(
	ctx context.Context,
	owner, repo string,
	label RepositoryLabel,
) error {
	path := fmt.Sprintf("/repos/%s/%s/labels", owner, repo)

	_, _, err := c.gh.Issues.CreateLabel(ctx, owner, repo, gogithub.CreateIssueLabelRequest{
		Name:  label.Name,
		Color: gogithub.Ptr(label.Color),

		// A pointer to an empty string, never a nil one. The field carries
		// omitempty, which drops a nil pointer and keeps a pointer to "" - and
		// those mean different things here. What a label should say is decided
		// before this is called: a configuration that declines to say leaves
		// the existing description in the value handed over, so an empty one
		// arriving here is somebody asking for it to be empty.
		Description: gogithub.Ptr(label.Description),
	})

	return wrapError(ErrAPIRequest, http.MethodPost, path, err)
}

// UpdateRepositoryLabel changes a label a repository already has.
//
// name is what the label is called now and label.Name is what it should be
// called, so this renames as well as recolours. GitHub keeps the case it was
// given, so a repository holding "Bug" where configuration says "bug" is a
// rename rather than a second label.
func (c *Client) UpdateRepositoryLabel(
	ctx context.Context,
	owner, repo, name string,
	label RepositoryLabel,
) error {
	// The current name is a path segment, so it is escaped for the same reason
	// RemoveLabel escapes its own: `kind/bug` would otherwise address a
	// different endpoint, and a `#` or `?` would truncate the path. An
	// organization's labels are exactly the `kind/*` shape that breaks it.
	escaped := url.PathEscape(name)
	path := fmt.Sprintf("/repos/%s/%s/labels/%s", owner, repo, escaped)

	_, _, err := c.gh.Issues.UpdateLabel(ctx, owner, repo, escaped,
		gogithub.UpdateIssueLabelRequest{
			// Always sent, even when the name is not changing. GitHub reads
			// new_name as "call it this", and sending the name it already has
			// is a no-op rather than a second request to decide about.
			NewName:     gogithub.Ptr(label.Name),
			Color:       gogithub.Ptr(label.Color),
			Description: gogithub.Ptr(label.Description),
		})

	return wrapError(ErrAPIRequest, http.MethodPatch, path, err)
}

// DeleteRepositoryLabel removes a label from a repository.
//
// This takes the label off every issue and pull request carrying it, which is
// why deletion is off by default and appears in a plan before it ever runs.
func (c *Client) DeleteRepositoryLabel(ctx context.Context, owner, repo, name string) error {
	escaped := url.PathEscape(name)
	path := fmt.Sprintf("/repos/%s/%s/labels/%s", owner, repo, escaped)

	_, err := c.gh.Issues.DeleteLabel(ctx, owner, repo, escaped)

	return wrapError(ErrAPIRequest, http.MethodDelete, path, err)
}
