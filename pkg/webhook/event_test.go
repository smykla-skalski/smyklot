package webhook_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/githubtest"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

// issueCommentPayload renders a delivery body shaped like GitHub's, trimmed to
// the fields the parser reads
func issueCommentPayload(action, body, authorType string, isPR bool, updatedAt string) []byte {
	return githubtest.IssueCommentPayload(githubtest.IssueComment{
		Action:        action,
		Body:          body,
		AuthorType:    authorType,
		UpdatedAt:     updatedAt,
		IsPullRequest: isPR,
	})
}

var _ = Describe("ParseIssueComment [Unit]", func() {
	It("should read every field the executor needs", func() {
		event, err := webhook.ParseIssueComment(
			issueCommentPayload("created", "/approve", "User", true, "2026-08-08T10:00:00Z"),
		)
		Expect(err).NotTo(HaveOccurred())

		Expect(event.Action).To(Equal("created"))
		Expect(event.Comment.ID).To(Equal(int64(555)))
		Expect(event.Comment.Body).To(Equal("/approve"))
		Expect(event.Comment.User.Login).To(Equal("someone"))
		Expect(event.Issue.Number).To(Equal(42))
		Expect(event.Repository.Owner.Login).To(Equal("smykla-skalski"))
		Expect(event.Repository.ID).To(Equal(int64(githubtest.DefaultRepoID)))
		Expect(event.Repository.Name).To(Equal("smyklot"))
		Expect(event.Installation.ID).To(Equal(int64(987)))
	})

	It("should reject a malformed body", func() {
		_, err := webhook.ParseIssueComment([]byte("not json"))
		Expect(err).To(MatchError(webhook.ErrMalformedPayload))
	})

	// Without an installation there is nothing to mint a token for, so the
	// delivery is unusable rather than merely uninteresting
	It("should reject a delivery with no installation", func() {
		_, err := webhook.ParseIssueComment([]byte(`{
			"action": "created",
			"repository": {"name": "smyklot", "owner": {"login": "smykla-skalski"}}
		}`))
		Expect(err).To(MatchError(webhook.ErrNoInstallation))
	})

	It("should reject a delivery with no repository", func() {
		_, err := webhook.ParseIssueComment([]byte(`{
			"action": "created",
			"installation": {"id": 987}
		}`))
		Expect(err).To(MatchError(webhook.ErrNoRepository))
	})
})

var _ = Describe("IssueCommentEvent.Actionable [Unit]", func() {
	DescribeTable("should decide whether a delivery can produce any action",
		func(action, authorType string, isPR, expected bool) {
			event, err := webhook.ParseIssueComment(
				issueCommentPayload(action, "/approve", authorType, isPR, "2026-08-08T10:00:00Z"),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(event.Actionable()).To(Equal(expected))
		},
		Entry("created on a PR by a user", "created", "User", true, true),
		Entry("edited on a PR by a user", "edited", "User", true, true),
		Entry("deleted on a PR by a user", "deleted", "User", true, true),
		// The bot's own feedback comments arrive as deliveries too; acting on
		// them would loop
		Entry("written by the bot", "created", "Bot", true, false),
		Entry("on an issue rather than a PR", "created", "User", false, false),
		Entry("an action nothing handles", "pinned", "User", true, false),
	)
})

var _ = Describe("IssueCommentEvent.IdempotencyKey [Unit]", func() {
	key := func(action, body, updatedAt string) string {
		GinkgoHelper()

		event, err := webhook.ParseIssueComment(
			issueCommentPayload(action, body, "User", true, updatedAt),
		)
		Expect(err).NotTo(HaveOccurred())

		return event.IdempotencyKey()
	}

	// The whole reason the key is derived from the comment rather than from
	// X-GitHub-Delivery: a redelivery must land on the same key whatever GitHub
	// does with the delivery identifier
	It("should be stable across a redelivery of the same event", func() {
		Expect(key("created", "/approve", "2026-08-08T10:00:00Z")).
			To(Equal(key("created", "/approve", "2026-08-08T10:00:00Z")))
	})

	It("should change when the comment is edited", func() {
		Expect(key("edited", "/merge", "2026-08-08T10:05:00Z")).
			NotTo(Equal(key("created", "/approve", "2026-08-08T10:00:00Z")))
	})

	// Deleting a comment is a separate event from creating it, and both are
	// reported
	It("should distinguish a deletion from the creation it follows", func() {
		Expect(key("deleted", "/approve", "2026-08-08T10:00:00Z")).
			NotTo(Equal(key("created", "/approve", "2026-08-08T10:00:00Z")))
	})

	It("should name the repository and the comment", func() {
		Expect(key("created", "/approve", "2026-08-08T10:00:00Z")).
			To(Equal("issue_comment:created:smykla-skalski/smyklot:555:2026-08-08T10:00:00Z"))
	})
})
