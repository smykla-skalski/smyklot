package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// graphqlPath is where GitHub answers GraphQL.
//
// This is a sibling of the REST root, not a child of it, which is why it is
// joined to baseURL directly rather than going through the REST client.
// Enterprise splits the two further apart still - REST at /api/v3/ and GraphQL
// at /api/graphql - but Smyklot documents Enterprise as unsupported, so the
// simple join is the honest one.
const graphqlPath = "/graphql"

// graphqlError is one entry of the errors array GitHub returns.
type graphqlError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// graphqlResponse is the envelope every GraphQL answer arrives in.
type graphqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []graphqlError  `json:"errors"`
}

// graphql performs a GraphQL request and reports what GitHub actually said.
//
// This is the only hand-written request left in the package, kept rather than
// pulling in a second GitHub library for a single mutation.
//
// The reason it exists at all is the failure it catches. GraphQL answers a
// rejected mutation with HTTP 200 and an errors array, so a caller that
// branches on status alone reads every rejection as a success. That is how
// Smyklot came to post "auto-merge enabled" on pull requests where the mutation
// had been refused - for a repository with auto-merge disabled, an unprotected
// branch, or a permission the App was never granted.
func (c *Client) graphql(ctx context.Context, query string, variables map[string]any, out any) error {
	payload, err := json.Marshal(map[string]any{
		"query":     query,
		"variables": variables,
	})
	if err != nil {
		return NewAPIError(ErrAPIRequest, 0, http.MethodPost, graphqlPath, err)
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.baseURL+graphqlPath, bytes.NewReader(payload),
	)
	if err != nil {
		return NewAPIError(ErrAPIRequest, 0, http.MethodPost, graphqlPath, err)
	}

	// Authorization and User-Agent come from the transport.
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return NewAPIError(ErrAPIRequest, 0, http.MethodPost, graphqlPath, err)
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	var envelope graphqlResponse
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		// A transport-level failure has no envelope to read. Report the status
		// rather than the decode error, which would describe the symptom.
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return NewAPIError(
				ErrAPIRequest, resp.StatusCode, http.MethodPost, graphqlPath,
				fmt.Errorf("status code %d", resp.StatusCode),
			)
		}

		return NewAPIError(ErrResponseParse, resp.StatusCode, http.MethodPost, graphqlPath, err)
	}

	if len(envelope.Errors) > 0 {
		return NewAPIError(
			ErrAPIRequest, resp.StatusCode, http.MethodPost, graphqlPath,
			fmt.Errorf("%s", joinGraphQLErrors(envelope.Errors)),
		)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return NewAPIError(
			ErrAPIRequest, resp.StatusCode, http.MethodPost, graphqlPath,
			fmt.Errorf("status code %d", resp.StatusCode),
		)
	}

	if out == nil || len(envelope.Data) == 0 {
		return nil
	}

	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return NewAPIError(ErrResponseParse, resp.StatusCode, http.MethodPost, graphqlPath, err)
	}

	return nil
}

// joinGraphQLErrors renders every message GitHub sent, not just the first.
// A rejected mutation often explains itself in one entry and names the offending
// field in another.
func joinGraphQLErrors(errs []graphqlError) string {
	messages := make([]string, 0, len(errs))

	for _, item := range errs {
		switch {
		case item.Message != "" && item.Type != "":
			messages = append(messages, item.Type+": "+item.Message)
		case item.Message != "":
			messages = append(messages, item.Message)
		case item.Type != "":
			messages = append(messages, item.Type)
		}
	}

	if len(messages) == 0 {
		return "GraphQL request failed"
	}

	return strings.Join(messages, "; ")
}
