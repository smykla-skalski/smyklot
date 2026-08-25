package github_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

var _ = Describe("MarkPullRequestReadyForReview [Unit]", func() {
	It("sends the parameterized mutation once", func() {
		var requests []string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests = append(requests, r.Method+" "+r.URL.Path)
			Expect(r.Header.Get("Authorization")).To(Equal("token test-token"))
			if r.URL.Path != "/graphql" {
				_ = json.NewEncoder(w).Encode(map[string]any{"node_id": "PR_node"})
				return
			}

			var body struct {
				Query     string         `json:"query"`
				Variables map[string]any `json:"variables"`
			}
			Expect(json.NewDecoder(r.Body).Decode(&body)).To(Succeed())
			Expect(body.Query).To(ContainSubstring("markPullRequestReadyForReview"))
			Expect(body.Variables).To(Equal(map[string]any{"pullRequestId": "PR_node"}))
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"markPullRequestReadyForReview": map[string]any{"clientMutationId": nil},
			}})
		}))
		defer server.Close()

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())
		Expect(client.MarkPullRequestReadyForReview(
			context.Background(), "acme", "web", 42,
		)).To(Succeed())
		Expect(requests).To(Equal([]string{
			"GET /repos/acme/web/pulls/42", "POST /graphql",
		}))
	})

	It("verifies an ambiguous response without replaying the mutation", func() {
		var pullReads, mutations atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/graphql" {
				mutations.Add(1)
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte(`{"message":"response lost"}`))
				return
			}

			read := pullReads.Add(1)
			if read == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{"node_id": "PR_node"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"state": "open", "draft": false,
				"head": map[string]any{"sha": "head"},
			})
		}))
		defer server.Close()

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())
		Expect(client.MarkPullRequestReadyForReview(
			context.Background(), "acme", "web", 42,
		)).To(Succeed())
		Expect(mutations.Load()).To(Equal(int32(1)))
		Expect(pullReads.Load()).To(Equal(int32(2)))
	})

	It("returns GitHub's error when verification still sees a draft", func() {
		var pullReads atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/graphql" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"errors": []map[string]any{{"type": "FORBIDDEN", "message": "refused"}},
				})
				return
			}
			if pullReads.Add(1) == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{"node_id": "PR_node"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"state": "open", "draft": true,
				"head": map[string]any{"sha": "head"},
			})
		}))
		defer server.Close()

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())
		err = client.MarkPullRequestReadyForReview(context.Background(), "acme", "web", 42)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("FORBIDDEN: refused"))
		Expect(strings.Contains(err.Error(), "test-token")).To(BeFalse())
	})

	It("fails before mutation when the REST response has no node id", func() {
		var mutations atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/graphql" {
				mutations.Add(1)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"number": 42})
		}))
		defer server.Close()

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())
		err = client.MarkPullRequestReadyForReview(context.Background(), "acme", "web", 42)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no node_id"))
		Expect(mutations.Load()).To(BeZero())
	})
})
