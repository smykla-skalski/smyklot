package webhook_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

const (
	testInstallation = 4242
	testRepoID       = 31337
	testOwner        = "owner"
	testRepo         = "repository"
	testPRNumber     = 7
	testCommentID    = 900
	testAuthor       = "author"
	testUpdatedAt    = "2026-01-01T00:00:00Z"
	testSecret       = "s3cr3t"
)

type issueComment struct {
	Action        string
	Body          string
	AuthorType    string
	UpdatedAt     string
	CommentID     int64
	IsPullRequest bool
}

func issueCommentPayload(event issueComment) []byte {
	if event.AuthorType == "" {
		event.AuthorType = "User"
	}
	if event.UpdatedAt == "" {
		event.UpdatedAt = testUpdatedAt
	}
	if event.CommentID == 0 {
		event.CommentID = testCommentID
	}

	pullRequest := "null"
	if event.IsPullRequest {
		pullRequest = fmt.Sprintf(
			`{"url": "https://api.github.com/repos/%s/%s/pulls/%d"}`,
			testOwner, testRepo, testPRNumber,
		)
	}

	return fmt.Appendf(nil, `{
		"action": %q,
		"comment": {
			"id": %d,
			"body": %q,
			"updated_at": %q,
			"user": {"login": %q, "type": %q}
		},
		"issue": {"number": %d, "pull_request": %s},
		"repository": {
			"id": %d,
			"name": %q,
			"full_name": "%s/%s",
			"owner": {"login": %q}
		},
		"installation": {"id": %d}
	}`,
		event.Action,
		event.CommentID, event.Body, event.UpdatedAt,
		testAuthor, event.AuthorType,
		testPRNumber, pullRequest,
		testRepoID, testRepo, testOwner, testRepo, testOwner,
		testInstallation,
	)
}

func comment(body string) []byte {
	return issueCommentPayload(issueComment{
		Action: webhook.ActionCreated, Body: body, IsPullRequest: true,
	})
}

func signed(event, deliveryID string, body []byte) *http.Request {
	mac := hmac.New(sha256.New, []byte(testSecret))
	mac.Write(body)

	request := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	request.Header.Set(webhook.EventHeader, event)
	request.Header.Set(webhook.DeliveryHeader, deliveryID)
	request.Header.Set(webhook.SignatureHeader, "sha256="+hex.EncodeToString(mac.Sum(nil)))

	return request
}
