package github

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	issueEventLabeled        = "labeled"
	issueEventConvertToDraft = "convert_to_draft"
)

type pullRequestIssueEvent struct {
	ID        int64                      `json:"id"`
	Event     string                     `json:"event"`
	CreatedAt time.Time                  `json:"created_at"`
	Label     pullRequestIssueEventLabel `json:"label"`
	Actor     pullRequestIssueEventActor `json:"actor"`
}

type pullRequestIssueEventLabel struct {
	Name string `json:"name"`
}

type pullRequestIssueEventActor struct {
	Login string `json:"login"`
}

// LatestPullRequestDraftTransition returns the newest durable transition that
// converted the pull request to draft. Callers compare this GitHub timestamp
// with the authorization event that may publish or merge the pull request.
func (c *Client) LatestPullRequestDraftTransition(
	ctx context.Context,
	owner, repository string,
	pullRequest int,
) (time.Time, bool, error) {
	events, path, err := c.pullRequestIssueEvents(ctx, owner, repository, pullRequest)
	if err != nil {
		return time.Time{}, false, err
	}

	var latest pullRequestIssueEvent
	for _, event := range events {
		if event.Event != issueEventConvertToDraft {
			continue
		}
		if event.ID <= 0 || event.CreatedAt.IsZero() {
			return time.Time{}, false, NewAPIError(
				ErrResponseParse, 0, http.MethodGet, path,
				fmt.Errorf("incomplete %q issue event", event.Event),
			)
		}
		if issueEventAfter(event, latest) {
			latest = event
		}
	}
	if latest.ID == 0 {
		return time.Time{}, false, nil
	}

	return latest.CreatedAt.UTC(), true, nil
}

// PullRequestDraftedAfterLabel reports whether a draft transition happened
// after the current occurrence of a pending-CI label was added. The label
// event is the durable authorization boundary for the legacy Action runner.
func (c *Client) PullRequestDraftedAfterLabel(
	ctx context.Context,
	owner, repository string,
	pullRequest int,
	label string,
	botUsername string,
) (bool, error) {
	events, path, err := c.pullRequestIssueEvents(ctx, owner, repository, pullRequest)
	if err != nil {
		return false, err
	}

	var authorization pullRequestIssueEvent
	for _, event := range events {
		matchingLabel := event.Event == issueEventLabeled && event.Label.Name == label
		if (event.Event == issueEventConvertToDraft || matchingLabel) &&
			(event.ID <= 0 || event.CreatedAt.IsZero()) {
			return false, NewAPIError(
				ErrResponseParse, 0, http.MethodGet, path,
				fmt.Errorf("incomplete %q issue event", event.Event),
			)
		}
		if matchingLabel && event.Actor.Login == "" {
			return false, NewAPIError(
				ErrResponseParse, 0, http.MethodGet, path,
				fmt.Errorf("incomplete actor for %q issue event", event.Event),
			)
		}
		if matchingLabel && strings.EqualFold(event.Actor.Login, botUsername) &&
			issueEventAfter(event, authorization) {
			authorization = event
		}
	}
	if authorization.ID == 0 || authorization.CreatedAt.IsZero() {
		return false, NewAPIError(
			ErrResponseParse, 0, http.MethodGet, path,
			fmt.Errorf("no labeled event found for %q", label),
		)
	}

	for _, event := range events {
		if event.Event == issueEventConvertToDraft && issueEventAfter(event, authorization) {
			return true, nil
		}
	}

	return false, nil
}

func (c *Client) pullRequestIssueEvents(
	ctx context.Context,
	owner, repository string,
	pullRequest int,
) ([]pullRequestIssueEvent, string, error) {
	basePath := fmt.Sprintf("/repos/%s/%s/issues/%d/events", owner, repository, pullRequest)
	events := make([]pullRequestIssueEvent, 0, pageSize)
	page := 1
	for range maxPages {
		path := fmt.Sprintf("%s?per_page=%d&page=%d", basePath, pageSize, page)
		items, response, err := doJSONPage[[]pullRequestIssueEvent](
			ctx, c, http.MethodGet, path, nil,
		)
		if err != nil {
			return nil, basePath, err
		}
		events = append(events, items...)
		switch {
		case response != nil && response.NextPage > 0:
			page = response.NextPage
		case len(items) < pageSize:
			return events, basePath, nil
		default:
			page++
		}
	}

	return nil, basePath, NewAPIError(
		ErrIncompletePagination, 0, http.MethodGet, basePath,
		fmt.Errorf("issue event list still has pages after %d items", len(events)),
	)
}

func issueEventAfter(candidate, boundary pullRequestIssueEvent) bool {
	if boundary.ID == 0 {
		return true
	}
	if candidate.CreatedAt.Equal(boundary.CreatedAt) {
		return candidate.ID > boundary.ID
	}

	return candidate.CreatedAt.After(boundary.CreatedAt)
}
