package github_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

var _ = Describe("GitHub pull request lists [Unit]", func() {
	It("paginates every open pull request", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			Expect(r.URL.Query().Get("state")).To(Equal("open"))
			page, err := strconv.Atoi(r.URL.Query().Get("page"))
			Expect(err).NotTo(HaveOccurred())
			count := 100
			if page == 2 {
				count = 1
			}
			prs := make([]map[string]interface{}, 0, count)
			for index := 0; index < count; index++ {
				prs = append(prs, map[string]interface{}{
					"number": (page-1)*100 + index + 1,
					"state":  "open",
				})
			}
			_ = json.NewEncoder(w).Encode(prs)
		}))
		defer server.Close()

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())
		prs, err := client.GetOpenPRs(context.Background(), "owner", "repo")
		Expect(err).NotTo(HaveOccurred())
		Expect(prs).To(HaveLen(101))
	})

	// Two signals end the walk, and the implementation requires both to agree.
	// The spec above exercises the short-page one, which is all a stub without
	// Link headers can say. This one proves the header is genuinely read: it
	// points somewhere a "page + 1" walk would never go.
	It("follows the page the Link header names, not the next one in sequence", func() {
		var visited []int

		var server *httptest.Server

		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			page, err := strconv.Atoi(r.URL.Query().Get("page"))
			Expect(err).NotTo(HaveOccurred())
			visited = append(visited, page)

			if page == 1 {
				w.Header().Set(
					"Link",
					`<`+server.URL+`/repos/owner/repo/pulls?page=7>; rel="next"`,
				)
			}

			count := 100
			if page != 1 {
				count = 1
			}

			prs := make([]map[string]interface{}, 0, count)
			for index := range count {
				prs = append(prs, map[string]interface{}{
					"number": page*1000 + index,
					"state":  "open",
				})
			}
			_ = json.NewEncoder(w).Encode(prs)
		}))
		defer server.Close()

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		prs, err := client.GetOpenPRs(context.Background(), "owner", "repo")
		Expect(err).NotTo(HaveOccurred())
		Expect(prs).To(HaveLen(101))
		Expect(visited).To(Equal([]int{1, 7}))
	})

	It("skips pull requests the server reports as not open", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"number": 1, "state": "open"},
				{"number": 2, "state": "closed"},
			})
		}))
		defer server.Close()

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		prs, err := client.GetOpenPRs(context.Background(), "owner", "repo")
		Expect(err).NotTo(HaveOccurred())
		Expect(prs).To(HaveLen(1))
		Expect(prs[0]).To(HaveKeyWithValue("number", float64(1)))
	})
})
