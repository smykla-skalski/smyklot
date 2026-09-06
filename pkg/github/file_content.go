package github

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

type fileContentResponse struct {
	Type     string  `json:"type"`
	Encoding string  `json:"encoding"`
	Content  *string `json:"content"`
	Size     *int    `json:"size"`
	SHA      string  `json:"sha"`
}

// GetFileContent reads the default branch, or the caller's resolved ref. Only
// a missing path returns nil. An empty file returns a non-nil empty slice.
// GitHub omits inline content for large files; read their immutable blob rather
// than confusing encoding=none with an empty file or following a download URL.
func (c *Client) GetFileContent(
	ctx context.Context,
	owner, repo, filePath, ref string,
	maxSize int,
) ([]byte, error) {
	if maxSize < 0 || maxSize > 100<<20 {
		return nil, errors.New("file size limit must be between zero and 100 MiB")
	}
	path := fmt.Sprintf("/repos/%s/%s/contents/%s", owner, repo, filePath)
	if ref != "" {
		path += "?" + url.Values{"ref": []string{ref}}.Encode()
	}
	response, err := c.readFileResponse(ctx, path, maxSize)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		return nil, err
	}
	if response.Type != "" && response.Type != "file" {
		return nil, fileResponseError(path, "path is not a regular file")
	}
	if response.Encoding == "none" {
		return c.readContentBlob(ctx, owner, repo, response, maxSize)
	}
	return decodeFileResponse(path, response, maxSize)
}

func (c *Client) readContentBlob(
	ctx context.Context,
	owner, repo string,
	metadata fileContentResponse,
	maxSize int,
) ([]byte, error) {
	path := fmt.Sprintf("/repos/%s/%s/git/blobs/%s", owner, repo, metadata.SHA)
	if _, err := hex.DecodeString(metadata.SHA); err != nil ||
		(len(metadata.SHA) != 40 && len(metadata.SHA) != 64) || metadata.Size == nil {
		return nil, fileResponseError(path, "large file response needs a blob id and size")
	}
	response, err := c.readFileResponse(ctx, path, maxSize)
	if err != nil {
		// Unlike the original contents lookup, a missing referenced blob is a
		// failed read, never evidence that the configuration file was deleted.
		return nil, err
	}
	if response.SHA != metadata.SHA || response.Size == nil || *response.Size != *metadata.Size {
		return nil, fileResponseError(path, "blob does not match the file metadata")
	}
	return decodeFileResponse(path, response, maxSize)
}

func (c *Client) readFileResponse(ctx context.Context, path string, maxSize int) (fileContentResponse, error) {
	request, err := newAPIRequest(ctx, c, http.MethodGet, path, nil)
	if err != nil {
		return fileContentResponse{}, err
	}
	// Base64, escaped line breaks and metadata need room, but the response is
	// bounded before JSON or base64 allocation, not after an unbounded decode.
	body := boundedFileBody{limit: 2*maxSize + (64 << 10)}
	response, err := c.gh.Do(request, &body)
	if err != nil {
		return fileContentResponse{}, wrapError(decodeOp(response, err), http.MethodGet, path, err)
	}
	var file fileContentResponse
	if err := json.Unmarshal(body.buffer.Bytes(), &file); err != nil {
		return fileContentResponse{}, NewAPIError(ErrResponseParse, 0, http.MethodGet, path, err)
	}
	if file.Size != nil && (*file.Size < 0 || *file.Size > maxSize) {
		return fileContentResponse{}, fileResponseError(path, "file too large or size invalid")
	}
	return file, nil
}

func decodeFileResponse(path string, response fileContentResponse, maxSize int) ([]byte, error) {
	if response.Content == nil {
		return nil, fileResponseError(path, "no content field in response")
	}
	if response.Encoding != "base64" {
		return nil, fileResponseError(path, "unsupported file content encoding")
	}
	decoded, err := base64.StdEncoding.DecodeString(*response.Content)
	if err != nil {
		return nil, NewAPIError(ErrResponseParse, 0, http.MethodGet, path, err)
	}
	if len(decoded) > maxSize {
		return nil, fileResponseError(path, "file too large")
	}
	if response.Size != nil && len(decoded) != *response.Size {
		return nil, fileResponseError(path, "file content does not match its size")
	}
	return decoded, nil
}

func fileResponseError(path, reason string) error {
	return NewAPIError(ErrResponseParse, 0, http.MethodGet, path, errors.New(reason))
}

type boundedFileBody struct {
	buffer bytes.Buffer
	limit  int
}

func (body *boundedFileBody) Write(data []byte) (int, error) {
	if len(data) > body.limit-body.buffer.Len() {
		return 0, errors.New("file response exceeds its size limit")
	}
	return body.buffer.Write(data)
}
