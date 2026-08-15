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
})
