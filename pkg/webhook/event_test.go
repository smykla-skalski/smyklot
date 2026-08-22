package webhook_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

// delivery renders a body shaped like GitHub's, trimmed to the fields the
// parser reads.
func delivery(action, body, authorType string, isPR bool, updatedAt string) []byte {
	return issueCommentPayload(issueComment{
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
			delivery("created", "/approve", "User", true, "2026-08-08T10:00:00Z"),
		)
		Expect(err).NotTo(HaveOccurred())

		Expect(event.Action).To(Equal("created"))
		Expect(event.Comment.ID).To(Equal(int64(testCommentID)))
		Expect(event.Comment.Body).To(Equal("/approve"))
		Expect(event.Comment.User.Login).To(Equal(testAuthor))
		Expect(event.Issue.Number).To(Equal(testPRNumber))
		Expect(event.Repository.Owner.Login).To(Equal(testOwner))
		Expect(event.Repository.ID).To(Equal(int64(testRepoID)))
		Expect(event.Repository.Name).To(Equal(testRepo))
		Expect(event.Installation.ID).To(Equal(int64(testInstallation)))
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
				delivery(action, "/approve", authorType, isPR, "2026-08-08T10:00:00Z"),
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
