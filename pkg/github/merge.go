package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// MergePR merges a pull request using the specified merge method.
func (c *Client) MergePR(ctx context.Context, owner, repo string, prNumber int, method MergeMethod) error {
	return c.mergePR(ctx, owner, repo, prNumber, method, "")
}

// MergePRAtHead merges only if headSHA is still the pull request head.
func (c *Client) MergePRAtHead(
	ctx context.Context,
	owner, repo string,
	prNumber int,
	method MergeMethod,
	headSHA string,
) error {
	if headSHA == "" {
		return errors.New("expected pull request head SHA must not be empty")
	}

	return c.mergePR(ctx, owner, repo, prNumber, method, headSHA)
}

func (c *Client) mergePR(
	ctx context.Context,
	owner, repo string,
	prNumber int,
	method MergeMethod,
	headSHA string,
) error {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/merge", owner, repo, prNumber)
	body := map[string]interface{}{"merge_method": string(method)}
	if headSHA != "" {
		body["sha"] = headSHA
	}

	data, err := c.makeRequestWithRetry(ctx, http.MethodPut, path, body)
	if err != nil {
		return err
	}

	var response struct {
		Merged  bool   `json:"merged"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return NewAPIError(ErrResponseParse, 0, http.MethodPut, path, err)
	}
	if !response.Merged {
		message := response.Message
		if message == "" {
			message = "GitHub reported that the pull request was not merged"
		}

		return NewAPIError(ErrAPIRequest, http.StatusOK, http.MethodPut, path, errors.New(message))
	}

	return nil
}
