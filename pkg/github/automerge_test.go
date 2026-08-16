package github_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

// autoMergeStub answers the pull-request lookup EnableAutoMerge needs and then
// hands the GraphQL mutation whatever body the spec is testing.
func autoMergeStub(graphqlStatus int, graphqlBody map[string]any, seen *[]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if seen != nil {
			*seen = append(*seen, r.Method+" "+r.URL.Path)
		}

		if r.URL.Path == "/graphql" {
			Expect(r.Method).To(Equal(http.MethodPost))
			Expect(r.Header.Get("Authorization")).To(Equal("token test-token"))

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(graphqlStatus)
			_ = json.NewEncoder(w).Encode(graphqlBody)

			return
		}

		Expect(r.URL.Path).To(Equal("/repos/acme/web/pulls/42"))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"number":  42,
			"node_id": "PR_kwDOABCD",
		})
	}))
}

var _ = Describe("EnableAutoMerge [Unit]", func() {
	var server *httptest.Server

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	It("succeeds when GitHub accepts the mutation", func() {
		var seen []string
		server = autoMergeStub(http.StatusOK, map[string]any{
			"data": map[string]any{
				"enablePullRequestAutoMerge": map[string]any{"clientMutationId": nil},
			},
		}, &seen)

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		Expect(client.EnableAutoMerge(
			context.Background(), "acme", "web", 42, github.MergeMethodSquash,
		)).To(Succeed())

		Expect(seen).To(Equal([]string{
			"GET /repos/acme/web/pulls/42",
			"POST /graphql",
		}))
	})

	// The bug this file exists for. GraphQL answers a refused mutation with 200
	// and an errors array, so a client that branches on status alone reports
	// success - and Smyklot then posts "auto-merge enabled" on a pull request
	// where nothing was enabled.
	It("fails when GitHub refuses the mutation with HTTP 200", func() {
		server = autoMergeStub(http.StatusOK, map[string]any{
			"data": nil,
			"errors": []map[string]any{{
				"type":    "UNPROCESSABLE",
				"message": "Pull request Auto merge is not allowed for this repository",
			}},
		}, nil)

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		err = client.EnableAutoMerge(
			context.Background(), "acme", "web", 42, github.MergeMethodSquash,
		)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("Auto merge is not allowed"))
		Expect(err.Error()).To(ContainSubstring("UNPROCESSABLE"))
	})

	It("reports every message GitHub sent, not only the first", func() {
		server = autoMergeStub(http.StatusOK, map[string]any{
			"errors": []map[string]any{
				{"message": "first problem"},
				{"message": "second problem"},
			},
		}, nil)

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		err = client.EnableAutoMerge(
			context.Background(), "acme", "web", 42, github.MergeMethodMerge,
		)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("first problem"))
		Expect(err.Error()).To(ContainSubstring("second problem"))
	})

	It("fails on a transport-level error status", func() {
		server = autoMergeStub(http.StatusBadGateway, map[string]any{}, nil)

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		err = client.EnableAutoMerge(
			context.Background(), "acme", "web", 42, github.MergeMethodRebase,
		)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("502"))
	})

	It("never puts the token in the error it returns", func() {
		server = autoMergeStub(http.StatusOK, map[string]any{
			"errors": []map[string]any{{"message": "refused"}},
		}, nil)

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		err = client.EnableAutoMerge(
			context.Background(), "acme", "web", 42, github.MergeMethodSquash,
		)

		Expect(err).To(HaveOccurred())
		Expect(strings.Contains(err.Error(), "test-token")).To(
			BeFalse(), "an APIError reaches the log and the failures endpoint",
		)
	})
})
