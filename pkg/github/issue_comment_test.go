package github_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

var _ = Describe("GetIssueComment [Unit]", func() {
	var server *httptest.Server

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	// updated_at is an opaque revision token, not a time. Smyklot compares it
	// byte-for-byte against the value a webhook payload carried to decide
	// whether a command comment changed under it. Parsing it into a time and
	// formatting it back loses sub-second digits, which reads as "the comment
	// changed" and makes the bot act a second time.
	DescribeTable("returns updated_at exactly as GitHub spelled it",
		func(updatedAt string) {
			server = httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					Expect(r.Method).To(Equal(http.MethodGet))
					Expect(r.URL.Path).To(Equal("/repos/acme/web/issues/comments/101"))
					_, _ = fmt.Fprintf(w, `{
						"id": 101,
						"body": "/approve",
						"updated_at": %q,
						"user": {"login": "someone", "type": "User"}
					}`, updatedAt)
				}))

			client, err := github.NewClient("test-token", server.URL)
			Expect(err).NotTo(HaveOccurred())

			comment, err := client.GetIssueComment(context.Background(), "acme", "web", 101)
			Expect(err).NotTo(HaveOccurred())
			Expect(comment.UpdatedAt).To(Equal(updatedAt))
		},
		Entry("whole seconds", "2026-08-15T12:00:00Z"),
		Entry("nanosecond precision", "2026-08-15T12:00:00.123456789Z"),
		Entry("trailing zeros a Nano format would trim", "2026-08-15T12:00:00.100Z"),
		Entry("milliseconds", "2026-08-15T12:00:00.500Z"),
	)

	It("carries the author and its type", func() {
		server = httptest.NewServer(http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{
					"id": 101, "body": "lgtm", "updated_at": "2026-08-15T12:00:00Z",
					"user": {"login": "smyklot[bot]", "type": "Bot"}
				}`))
			}))

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		comment, err := client.GetIssueComment(context.Background(), "acme", "web", 101)
		Expect(err).NotTo(HaveOccurred())
		Expect(comment.User.Login).To(Equal("smyklot[bot]"))
		Expect(comment.User.Type).To(Equal("Bot"))
	})

	It("refuses a response missing the fields staleness is decided on", func() {
		server = httptest.NewServer(http.HandlerFunc(
			func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"id": 101, "body": "lgtm"}`))
			}))

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		_, err = client.GetIssueComment(context.Background(), "acme", "web", 101)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("incomplete issue comment"))
	})

	It("reports a 404 as an error rather than an empty comment", func() {
		server = httptest.NewServer(http.HandlerFunc(
			func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"message":"Not Found"}`))
			}))

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		_, err = client.GetIssueComment(context.Background(), "acme", "web", 101)
		Expect(err).To(HaveOccurred())

		var apiErr *github.APIError
		Expect(err).To(BeAssignableToTypeOf(apiErr))
	})
})
