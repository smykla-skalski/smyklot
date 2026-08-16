package github_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

var _ = Describe("Pull request labels [Unit]", func() {
	var server *httptest.Server

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	Describe("RemoveLabel", func() {
		// The organization's labels are kind/*, area/* and ci/*. Interpolated
		// raw, `kind/bug` addressed /issues/42/labels/kind/bug - a different
		// endpoint - and the label survived. Nothing reported it, because the
		// callers discard the error.
		DescribeTable("escapes the label so it stays one path segment",
			func(label, encoded string) {
				var gotPath string

				server = httptest.NewServer(http.HandlerFunc(
					func(w http.ResponseWriter, r *http.Request) {
						Expect(r.Method).To(Equal(http.MethodDelete))
						gotPath = r.URL.EscapedPath()
						_, _ = w.Write([]byte(`[]`))
					}))

				client, err := github.NewClient("test-token", server.URL)
				Expect(err).NotTo(HaveOccurred())

				Expect(client.RemoveLabel(
					context.Background(), "acme", "web", 42, label,
				)).To(Succeed())

				Expect(gotPath).To(Equal(
					fmt.Sprintf("/repos/acme/web/issues/42/labels/%s", encoded),
				))
			},
			Entry("a slash", "kind/bug", "kind%2Fbug"),
			Entry("a colon, which always worked", "smyklot:pending:ci", "smyklot:pending:ci"),
			Entry("a space", "help wanted", "help%20wanted"),
			Entry("a question mark", "why?", "why%3F"),
			Entry("a hash", "c#", "c%23"),
		)
	})

	Describe("GetLabels", func() {
		It("follows pagination rather than reporting the first page as the whole set", func() {
			var pages int

			server = httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					pages++

					if r.URL.Query().Get("page") == "1" {
						w.Header().Set(
							"Link",
							fmt.Sprintf(`<%s/repos/acme/web/issues/42/labels?page=2>; rel="next"`,
								server.URL),
						)
						_ = json.NewEncoder(w).Encode([]map[string]any{
							{"name": "kind/bug"},
						})

						return
					}

					_ = json.NewEncoder(w).Encode([]map[string]any{
						{"name": "area/ci"},
					})
				}))

			client, err := github.NewClient("test-token", server.URL)
			Expect(err).NotTo(HaveOccurred())

			labels, err := client.GetLabels(context.Background(), "acme", "web", 42)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).To(Equal([]string{"kind/bug", "area/ci"}))
			Expect(pages).To(Equal(2))
		})

		It("asks for a full page rather than GitHub's default of thirty", func() {
			var perPage string

			server = httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					perPage = r.URL.Query().Get("per_page")
					_, _ = w.Write([]byte(`[]`))
				}))

			client, err := github.NewClient("test-token", server.URL)
			Expect(err).NotTo(HaveOccurred())

			_, err = client.GetLabels(context.Background(), "acme", "web", 42)
			Expect(err).NotTo(HaveOccurred())
			Expect(perPage).To(Equal("100"))
		})
	})

	Describe("AddLabel", func() {
		// GitHub documents two accepted bodies for this endpoint, the bare
		// array and {"labels": [...]}. The hand-rolled client sent the object;
		// go-github sends the array. Both work, and pinning which one we send
		// is the point of the spec - so the next person to see the wire change
		// finds it recorded rather than surprising.
		It("sends the label as the documented bare array", func() {
			var body []string

			server = httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					Expect(r.Method).To(Equal(http.MethodPost))
					Expect(r.URL.Path).To(Equal("/repos/acme/web/issues/42/labels"))
					Expect(json.NewDecoder(r.Body).Decode(&body)).To(Succeed())
					_, _ = w.Write([]byte(`[]`))
				}))

			client, err := github.NewClient("test-token", server.URL)
			Expect(err).NotTo(HaveOccurred())

			Expect(client.AddLabel(
				context.Background(), "acme", "web", 42, "kind/bug",
			)).To(Succeed())

			Expect(body).To(Equal([]string{"kind/bug"}))
		})
	})
})
