package github_test

import (
	"context"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

var _ = Describe("GetIssueComment [Unit]", func() {
	It("reads the mutable command fields", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			Expect(request.Method).To(Equal(http.MethodGet))
			Expect(request.URL.Path).To(Equal("/repos/owner/repo/issues/comments/101"))
			_, _ = w.Write([]byte(`{
				"id":101, "body":"/squash after ci",
				"updated_at":"2026-08-15T12:00:00Z",
				"user":{"login":"owner","type":"User"}
			}`))
		}))
		DeferCleanup(server.Close)
		client, err := github.NewClient("token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		comment, err := client.GetIssueComment(
			context.Background(), "owner", "repo", 101,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(comment.ID).To(Equal(int64(101)))
		Expect(comment.Body).To(Equal("/squash after ci"))
		Expect(comment.UpdatedAt).To(Equal("2026-08-15T12:00:00Z"))
		Expect(comment.User.Login).To(Equal("owner"))
		Expect(comment.User.Type).To(Equal("User"))
	})

	It("rejects an incomplete response", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"id":101}`))
		}))
		DeferCleanup(server.Close)
		client, err := github.NewClient("token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		_, err = client.GetIssueComment(context.Background(), "owner", "repo", 101)
		Expect(err).To(MatchError(ContainSubstring("incomplete issue comment response")))
	})
})
