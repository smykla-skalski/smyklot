package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type CheckRunAction struct {
	Label       string `json:"label"`
	Description string `json:"description"`
	Identifier  string `json:"identifier"`
}

type CheckRunOutput struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

type CheckRunWrite struct {
	Name        string
	HeadSHA     string
	ExternalID  string
	Status      string
	Conclusion  string
	Output      CheckRunOutput
	Actions     []CheckRunAction
	StartedAt   time.Time
	CompletedAt time.Time
}

type CheckRun struct {
	ID         int64
	Name       string
	HeadSHA    string
	ExternalID string
	Status     string
	Conclusion string
	HTMLURL    string
	AppID      int64
}

func (c *Client) CreateCheckRun(
	ctx context.Context,
	owner, repo string,
	write CheckRunWrite,
) (CheckRun, error) {
	path := fmt.Sprintf("/repos/%s/%s/check-runs", owner, repo)
	raw, err := doJSON[checkRunWriteResponse](
		withoutRetry(ctx),
		c,
		http.MethodPost,
		path,
		checkRunPayload(write, true),
	)
	if err != nil {
		return CheckRun{}, err
	}

	return raw.checkRun(), nil
}

func (c *Client) UpdateCheckRun(
	ctx context.Context,
	owner, repo string,
	id int64,
	write CheckRunWrite,
) (CheckRun, error) {
	path := fmt.Sprintf("/repos/%s/%s/check-runs/%d", owner, repo, id)
	raw, err := doJSON[checkRunWriteResponse](
		ctx,
		c,
		http.MethodPatch,
		path,
		checkRunPayload(write, false),
	)
	if err != nil {
		return CheckRun{}, err
	}

	return raw.checkRun(), nil
}

func (c *Client) ListCheckRunsForRef(
	ctx context.Context,
	owner, repo, ref string,
) ([]CheckRun, error) {
	var runs []CheckRun
	for page := 1; page <= maxPages; page++ {
		path := fmt.Sprintf(
			"/repos/%s/%s/commits/%s/check-runs?filter=all&per_page=%d&page=%d",
			owner,
			repo,
			url.PathEscape(ref),
			pageSize,
			page,
		)
		response, err := doJSON[struct {
			TotalCount *int                    `json:"total_count"`
			CheckRuns  []checkRunWriteResponse `json:"check_runs"`
		}](ctx, c, http.MethodGet, path, nil)
		if err != nil {
			return nil, err
		}
		for _, raw := range response.CheckRuns {
			runs = append(runs, raw.checkRun())
		}
		complete, err := paginationComplete(
			len(response.CheckRuns), len(runs), response.TotalCount, page,
		)
		if err != nil {
			return nil, NewAPIError(ErrIncompletePagination, 0, http.MethodGet, path, err)
		}
		if complete {
			return runs, nil
		}
	}

	return nil, NewAPIError(ErrIncompletePagination, 0, http.MethodGet, "check-runs", nil)
}

type checkRunWriteResponse struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	HeadSHA    string `json:"head_sha"`
	ExternalID string `json:"external_id"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	HTMLURL    string `json:"html_url"`
	App        struct {
		ID int64 `json:"id"`
	} `json:"app"`
}

func (response checkRunWriteResponse) checkRun() CheckRun {
	return CheckRun{
		ID: response.ID, Name: response.Name, HeadSHA: response.HeadSHA,
		ExternalID: response.ExternalID, Status: response.Status,
		Conclusion: response.Conclusion, HTMLURL: response.HTMLURL, AppID: response.App.ID,
	}
}

func checkRunPayload(write CheckRunWrite, includeHead bool) map[string]any {
	actions := write.Actions
	if actions == nil {
		actions = []CheckRunAction{}
	}
	payload := map[string]any{
		"name": write.Name, "external_id": write.ExternalID,
		"status": write.Status, "output": write.Output, "actions": actions,
	}
	if includeHead {
		payload["head_sha"] = write.HeadSHA
	}
	if !write.StartedAt.IsZero() {
		payload["started_at"] = write.StartedAt.UTC().Format(time.RFC3339)
	}
	if write.Conclusion != "" {
		payload["conclusion"] = write.Conclusion
	}
	if !write.CompletedAt.IsZero() {
		payload["completed_at"] = write.CompletedAt.UTC().Format(time.RFC3339)
	}

	return payload
}
