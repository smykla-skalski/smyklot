package github_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

// pagedServer answers two pages: a full one carrying a next link, then a short
// one. Anything reading a single page sees only the first.
func pagedServer(first, second func(page int) string) *httptest.Server {
	var server *httptest.Server

	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":1}`))

			return
		}

		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page <= 1 {
			w.Header().Set("Link", fmt.Sprintf(`<%s%s?page=2>; rel="next"`, server.URL, r.URL.Path))
			_, _ = w.Write([]byte(first(1)))

			return
		}

		_, _ = w.Write([]byte(second(page)))
	}))

	return server
}

// fullPageOf renders pageSize entries so the walk cannot end on a short page.
func fullPageOf(render func(index int) string) string {
	items := make([]string, 0, 100)
	for index := range 100 {
		items = append(items, render(index))
	}

	return "[" + strings.Join(items, ",") + "]"
}

var _ = Describe("List pagination [Unit]", func() {
	var server *httptest.Server

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	// A busy pull request carries more reactions than one page holds. Reading
	// only the first meant the bot's cleanup left its own marks behind on
	// exactly the pull requests where they are most visible.
	Describe("comment reactions", func() {
		It("reads every page", func() {
			server = pagedServer(
				func(int) string {
					return fullPageOf(func(index int) string {
						return fmt.Sprintf(`{"id":%d,"content":"eyes","user":{"login":"someone"}}`, index+1)
					})
				},
				func(int) string {
					return `[{"id":999,"content":"+1","user":{"login":"late"}}]`
				},
			)

			client, err := github.NewClient("test-token", server.URL)
			Expect(err).NotTo(HaveOccurred())

			reactions, err := client.GetCommentReactions(context.Background(), "acme", "web", 555)
			Expect(err).NotTo(HaveOccurred())
			Expect(reactions).To(HaveLen(101))
			Expect(reactions[100].User).To(Equal("late"))
			Expect(reactions[100].Type).To(Equal(github.ReactionType("+1")))
		})

		It("deletes a match that only appears on a later page", func() {
			var deleted []string

			var srv *httptest.Server

			srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					deleted = append(deleted, r.URL.Path)
					w.WriteHeader(http.StatusNoContent)

					return
				}

				page, _ := strconv.Atoi(r.URL.Query().Get("page"))
				if page <= 1 {
					w.Header().Set(
						"Link", fmt.Sprintf(`<%s%s?page=2>; rel="next"`, srv.URL, r.URL.Path),
					)
					_, _ = w.Write([]byte(fullPageOf(func(index int) string {
						return fmt.Sprintf(`{"id":%d,"content":"+1","user":{"login":"other"}}`, index+1)
					})))

					return
				}

				_, _ = w.Write([]byte(`[{"id":777,"content":"eyes","user":{"login":"smyklot[bot]"}}]`))
			}))

			server = srv

			client, err := github.NewClient("test-token", srv.URL)
			Expect(err).NotTo(HaveOccurred())

			Expect(client.RemoveReactionByUser(
				context.Background(), "acme", "web", 555, github.ReactionEyes, "smyklot[bot]",
			)).To(Succeed())

			Expect(deleted).To(HaveLen(1))
			Expect(deleted[0]).To(HaveSuffix("/reactions/777"))
		})
	})

	// The bot's own approval sitting on the second page survived an unapprove,
	// which is the one outcome DismissReviewByUsername exists to prevent.
	Describe("pull request reviews", func() {
		It("dismisses an approval that only appears on a later page", func() {
			var dismissed []string

			var srv *httptest.Server

			srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/dismissals") {
					dismissed = append(dismissed, r.URL.Path)
					_, _ = w.Write([]byte(`{"id":1,"state":"DISMISSED"}`))

					return
				}

				page, _ := strconv.Atoi(r.URL.Query().Get("page"))
				if page <= 1 {
					w.Header().Set(
						"Link", fmt.Sprintf(`<%s%s?page=2>; rel="next"`, srv.URL, r.URL.Path),
					)
					_, _ = w.Write([]byte(fullPageOf(func(index int) string {
						return fmt.Sprintf(
							`{"id":%d,"state":"COMMENTED","user":{"login":"someone"}}`, index+1,
						)
					})))

					return
				}

				_, _ = w.Write([]byte(
					`[{"id":4242,"state":"APPROVED","user":{"login":"smyklot[bot]"}}]`,
				))
			}))

			server = srv

			client, err := github.NewClient("test-token", srv.URL)
			Expect(err).NotTo(HaveOccurred())

			Expect(client.DismissReviewByUsername(
				context.Background(), "acme", "web", 42, "smyklot[bot]",
			)).To(Succeed())

			Expect(dismissed).To(HaveLen(1))
			Expect(dismissed[0]).To(ContainSubstring("/reviews/4242/dismissals"))
		})

		It("leaves an approval by somebody else alone", func() {
			var dismissed int

			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/dismissals") {
					dismissed++
					_, _ = w.Write([]byte(`{}`))

					return
				}

				_ = json.NewEncoder(w).Encode([]map[string]any{
					{"id": 1, "state": "APPROVED", "user": map[string]any{"login": "a-human"}},
				})
			}))

			client, err := github.NewClient("test-token", server.URL)
			Expect(err).NotTo(HaveOccurred())

			Expect(client.DismissReviewByUsername(
				context.Background(), "acme", "web", 42, "smyklot[bot]",
			)).To(Succeed())

			Expect(dismissed).To(BeZero())
		})
	})
})
