// Package githubtest builds the GitHub payloads and credentials the test
// suites stand in for.
//
// The suites that need these live in different packages - cmd/github-action
// exercises the entry points, pkg/github the client, pkg/webhook the parser -
// so a Go-internal _test file cannot be shared between them. Without this
// package each one grows its own copy of the same wire format, and the copies
// drift.
package githubtest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"sync"
)

// ContentsResponse renders the contents API's answer for a file.
//
// GitHub base64-encodes file content in this endpoint, and the client decodes
// it, so a stub that skips the encoding tests nothing.
func ContentsResponse(content string) string {
	payload, err := json.Marshal(map[string]string{
		"content":  base64.StdEncoding.EncodeToString([]byte(content)),
		"encoding": "base64",
	})
	if err != nil {
		panic(err)
	}

	return string(payload)
}

// IssueComment describes the delivery to render.
type IssueComment struct {
	// CommentID identifies one mutable source comment.
	CommentID int64

	// Action is created, edited, or deleted
	Action string

	// Body is the comment text, which is what carries a command
	Body string

	// AuthorType is User for a person and Bot for an App, which decides
	// whether the bot would be answering itself
	AuthorType string

	// UpdatedAt distinguishes an edit from a redelivery of the same comment
	UpdatedAt string

	// IsPullRequest is false for a comment on a plain issue, which no command
	// applies to
	IsPullRequest bool
}

// Defaults for a delivery, so a spec states only what it is actually about.
const (
	DefaultCommentID     = 555
	DefaultPRNumber      = 42
	DefaultRepoOwner     = "smykla-skalski"
	DefaultRepoName      = "smyklot"
	DefaultRepoID        = 123456
	DefaultAuthor        = "someone"
	DefaultInstallation  = 987
	DefaultUpdatedAt     = "2026-08-08T10:00:00Z"
	DefaultAuthorTypeVal = "User"
)

// IssueCommentPayload renders an issue_comment delivery body the way GitHub
// sends one, trimmed to the fields the service reads.
func IssueCommentPayload(event IssueComment) []byte {
	if event.AuthorType == "" {
		event.AuthorType = DefaultAuthorTypeVal
	}

	if event.UpdatedAt == "" {
		event.UpdatedAt = DefaultUpdatedAt
	}
	if event.CommentID == 0 {
		event.CommentID = DefaultCommentID
	}

	pullRequest := "null"
	if event.IsPullRequest {
		pullRequest = fmt.Sprintf(
			`{"url": "https://api.github.com/repos/%s/%s/pulls/%d"}`,
			DefaultRepoOwner, DefaultRepoName, DefaultPRNumber,
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
		DefaultAuthor, event.AuthorType,
		DefaultPRNumber, pullRequest,
		DefaultRepoID, DefaultRepoName, DefaultRepoOwner, DefaultRepoName, DefaultRepoOwner,
		DefaultInstallation,
	)
}

// Command renders the common case: a person commenting a command on a PR.
func Command(body string) []byte {
	return IssueCommentPayload(IssueComment{
		Action:        "created",
		Body:          body,
		IsPullRequest: true,
	})
}

var (
	privateKeyOnce sync.Once
	privateKeyPEM  []byte
)

// AppPrivateKey returns a PEM-encoded RSA key for signing App JWTs.
//
// Generated once per process: RSA key generation is slow enough that doing it
// per spec dominates a suite's run time.
func AppPrivateKey() []byte {
	privateKeyOnce.Do(func() {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}

		privateKeyPEM = pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(key),
		})
	})

	return privateKeyPEM
}
