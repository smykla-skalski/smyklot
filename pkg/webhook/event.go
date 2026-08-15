// Package webhook parses and de-duplicates GitHub webhook deliveries.
//
// Signature verification is not reimplemented here - github.com/jferrl/
// go-githubauth/webhook already does it in constant time and ships the header
// constants this package re-exports.
package webhook

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/jferrl/go-githubauth/webhook"
)

// Header names GitHub sets on every delivery.
const (
	// SignatureHeader carries the HMAC-SHA256 of the raw body
	SignatureHeader = webhook.SignatureHeader

	// EventHeader carries the event name, such as issue_comment
	EventHeader = webhook.EventHeader

	// DeliveryHeader carries the delivery identifier, used for tracing
	DeliveryHeader = webhook.DeliveryHeader
)

// Event names this service acts on.
const (
	// EventIssueComment is the only event that carries a command
	EventIssueComment = "issue_comment"
	EventCheckRun     = "check_run"
	EventCheckSuite   = "check_suite"
	EventStatus       = "status"
	EventPullRequest  = "pull_request"

	// EventPing is what GitHub sends when a webhook is first configured
	EventPing = "ping"
)

// Actions of the issue_comment event.
const (
	ActionCreated = "created"
	ActionEdited  = "edited"
	ActionDeleted = "deleted"
)

// userTypeBot is the type GitHub gives a comment written by an App.
const userTypeBot = "Bot"

// IssueCommentEvent is the part of an issue_comment delivery this service uses.
//
// The payload carries far more than this; anything not needed to decide and
// execute a command is deliberately left unparsed.
type IssueCommentEvent struct {
	Action string `json:"action"`

	Comment struct {
		ID        int64  `json:"id"`
		Body      string `json:"body"`
		UpdatedAt string `json:"updated_at"`
		User      struct {
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"user"`
	} `json:"comment"`

	Issue struct {
		Number int `json:"number"`

		// PullRequest is present only when the issue is a pull request. GitHub
		// delivers issue_comment for both, and only pull requests are ours
		PullRequest *struct {
			URL string `json:"url"`
		} `json:"pull_request"`
	} `json:"issue"`

	Repository struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		FullName string `json:"full_name"`
		Owner    struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`

	// Installation identifies which installation to mint a token for. It is
	// what lets one process serve every repository the App is installed on
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
}

// ParseIssueComment decodes an issue_comment delivery body.
func ParseIssueComment(body []byte) (*IssueCommentEvent, error) {
	var event IssueCommentEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformedPayload, err)
	}

	if event.Installation.ID == 0 {
		return nil, ErrNoInstallation
	}

	if event.Repository.ID == 0 || event.Repository.Owner.Login == "" || event.Repository.Name == "" {
		return nil, ErrNoRepository
	}
	if event.Repository.FullName == "" {
		event.Repository.FullName = event.Repository.Owner.Login + "/" + event.Repository.Name
	}

	return &event, nil
}

// Actionable reports whether a delivery can produce any action at all.
//
// Three kinds never can, and every one of them arrives constantly:
//
//   - an action other than created, edited, or deleted
//   - a comment on an issue that is not a pull request
//   - a comment the bot itself wrote, which would otherwise let the bot's own
//     feedback trigger it again
func (e *IssueCommentEvent) Actionable() bool {
	switch e.Action {
	case ActionCreated, ActionEdited, ActionDeleted:
	default:
		return false
	}

	if e.Issue.PullRequest == nil {
		return false
	}

	return e.Comment.User.Type != userTypeBot
}

// ContentKey identifies the event content when no delivery GUID is available.
//
// The service normally uses X-GitHub-Delivery, which GitHub guarantees remains
// stable across redelivery. This fallback cannot distinguish content that
// cycles within GitHub's second-granularity updated_at timestamp.
func (e *IssueCommentEvent) ContentKey() string {
	bodyDigest := sha256.Sum256([]byte(e.Comment.Body))

	return fmt.Sprintf(
		"%s:%s:%s:%d:%s:%x",
		EventIssueComment,
		e.Action,
		e.Repository.FullName,
		e.Comment.ID,
		e.Comment.UpdatedAt,
		bodyDigest,
	)
}

// SourceSequence orders actions that GitHub reports with the same comment
// timestamp. An edit supersedes creation; deletion supersedes both.
func (e *IssueCommentEvent) SourceSequence() int {
	switch e.Action {
	case ActionCreated:
		return 1
	case ActionEdited:
		return 2
	case ActionDeleted:
		return 3
	default:
		return 0
	}
}
