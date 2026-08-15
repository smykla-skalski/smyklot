package github_test

import (
	"context"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

var _ = Describe("GetPullRequestState [Unit]", func() {
	It("reads reconciliation state without fetching reviews", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			Expect(request.Method).To(Equal(http.MethodGet))
			Expect(request.URL.Path).To(Equal("/repos/owner/repo/pulls/42"))
			_, _ = w.Write([]byte(`{
                "state":"open", "merged":false, "draft":true,
                "head":{"sha":"abc123"}, "base":{"ref":"main"},
                "labels":[{"name":"smyklot:pending:ci:squash"}]
            }`))
		}))
		DeferCleanup(server.Close)
		client, err := github.NewClient("token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		state, err := client.GetPullRequestState(context.Background(), "owner", "repo", 42)
		Expect(err).NotTo(HaveOccurred())
		Expect(state).To(Equal(github.PullRequestState{
			Number: 42, Open: true, Draft: true, HeadSHA: "abc123", BaseBranch: "main",
			Labels: []string{"smyklot:pending:ci:squash"},
		}))
	})

	It("rejects an incomplete response", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"state":"open"}`))
		}))
		DeferCleanup(server.Close)
		client, err := github.NewClient("token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		_, err = client.GetPullRequestState(context.Background(), "owner", "repo", 42)
		Expect(err).To(MatchError(ContainSubstring("no head SHA")))
	})
})
