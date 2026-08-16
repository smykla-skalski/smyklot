package github

import (
	"context"
	"fmt"
	"net/http"
)

// GetOpenPRs retrieves every open pull request in a repository.
//
// The result stays a slice of raw maps: poll.go reads fields out of it that no
// typed model here covers, and turning it into a struct is a separate change
// with its own blast radius. What moved is the transport underneath.
func (c *Client) GetOpenPRs(ctx context.Context, owner, repo string) ([]map[string]interface{}, error) {
	var openPRs []map[string]interface{}

	page := 1

	for range maxPages {
		path := fmt.Sprintf(
			"/repos/%s/%s/pulls?state=open&per_page=%d&page=%d",
			owner,
			repo,
			pageSize,
			page,
		)

		response, resp, err := doJSONPage[[]map[string]interface{}](
			ctx, c, http.MethodGet, path, nil,
		)
		if err != nil {
			return nil, err
		}

		for _, pr := range response {
			if state, ok := pr["state"].(string); ok && state == "open" {
				openPRs = append(openPRs, pr)
			}
		}

		// Two signals, and both have to say stop.
		//
		// GitHub's Link header is the authoritative one, and it is what lets a
		// full last page end the walk without a wasted request. But an
		// endpoint or a proxy that sends no Link header at all would then look
		// like a single page, silently truncating the list - so a full page
		// with no next link still advances, which is the rule this code used
		// before and the one the specs pin.
		switch {
		case resp != nil && resp.NextPage > 0:
			page = resp.NextPage
		case len(response) < pageSize:
			return openPRs, nil
		default:
			page++
		}
	}

	return nil, NewAPIError(
		ErrIncompletePagination,
		0,
		http.MethodGet,
		fmt.Sprintf("/repos/%s/%s/pulls", owner, repo),
		fmt.Errorf("pull request list still has pages after %d items", len(openPRs)),
	)
}
