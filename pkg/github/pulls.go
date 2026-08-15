package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// GetOpenPRs retrieves every open pull request in a repository.
func (c *Client) GetOpenPRs(ctx context.Context, owner, repo string) ([]map[string]interface{}, error) {
	var openPRs []map[string]interface{}

	for page := 1; page <= maxPages; page++ {
		path := fmt.Sprintf(
			"/repos/%s/%s/pulls?state=open&per_page=%d&page=%d",
			owner,
			repo,
			pageSize,
			page,
		)
		data, err := c.makeRequestWithRetry(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, err
		}

		var response []map[string]interface{}
		if err := json.Unmarshal(data, &response); err != nil {
			return nil, NewAPIError(ErrResponseParse, 0, http.MethodGet, path, err)
		}

		for _, pr := range response {
			if state, ok := pr["state"].(string); ok && state == "open" {
				openPRs = append(openPRs, pr)
			}
		}
		if len(response) < pageSize {
			return openPRs, nil
		}
		if page == maxPages {
			return nil, NewAPIError(
				ErrIncompletePagination,
				0,
				http.MethodGet,
				path,
				fmt.Errorf("pull request list still has a full page after %d items", len(openPRs)),
			)
		}
	}

	return openPRs, nil
}
