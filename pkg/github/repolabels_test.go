package github_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

var _ = Describe("Repository labels [Unit]", func() {
	var (
		server   *httptest.Server
		mu       sync.Mutex
		requests []*http.Request
		bodies   []string
	)

	BeforeEach(func() {
		requests = nil
		bodies = nil
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	// record keeps what went over the wire, because what these methods send is
	// the whole of what they do.
	record := func(handler http.HandlerFunc) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)

			mu.Lock()
			requests = append(requests, r)
			bodies = append(bodies, string(body))
			mu.Unlock()

			handler(w, r)
		}))
	}

	client := func() *github.Client {
		GinkgoHelper()

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		return client
	}

	Describe("listing", func() {
		It("reads a label's colour and description, not only its name", func() {
			server = record(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w,
					`[{"name":"bug","color":"d73a4a","description":"Something broken"}]`)
			})

			labels, err := client().ListRepositoryLabels(context.Background(), "acme", "web")
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).To(Equal([]github.RepositoryLabel{{
				Name: "bug", Color: "d73a4a", Description: "Something broken",
			}}))
		})

		// GitHub's default page is 30 and an organization's label set is
		// routinely larger. An unpaginated read reports the rest as missing, a
		// sync then creates them, GitHub answers 422 for a label that already
		// exists, and the repository loses every label after it
		It("reads past the first page", func() {
			server = record(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("page") == "2" {
					_, _ = io.WriteString(w, `[{"name":"second-page"}]`)

					return
				}

				w.Header().Set("Link",
					fmt.Sprintf(`<%s/repos/acme/web/labels?page=2>; rel="next"`, server.URL))
				_, _ = io.WriteString(w, `[{"name":"first-page"}]`)
			})

			labels, err := client().ListRepositoryLabels(context.Background(), "acme", "web")
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).To(HaveLen(2))
			Expect(labels[1].Name).To(Equal("second-page"))
		})
	})

	Describe("creating", func() {
		It("sends the name, colour and description", func() {
			server = record(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusCreated)
				_, _ = io.WriteString(w, `{"name":"bug"}`)
			})

			Expect(client().CreateRepositoryLabel(context.Background(), "acme", "web",
				github.RepositoryLabel{Name: "bug", Color: "d73a4a", Description: "Broken"},
			)).To(Succeed())

			var sent map[string]any
			Expect(json.Unmarshal([]byte(bodies[0]), &sent)).To(Succeed())
			Expect(sent).To(HaveKeyWithValue("name", "bug"))
			Expect(sent).To(HaveKeyWithValue("color", "d73a4a"))
			Expect(sent).To(HaveKeyWithValue("description", "Broken"))
		})

		// An empty description here is somebody asking for it to be empty. What
		// a label should say is decided before this is called, so omitting the
		// key would make "clear this" impossible to express
		It("sends an empty description rather than omitting it", func() {
			server = record(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusCreated)
				_, _ = io.WriteString(w, `{}`)
			})

			Expect(client().CreateRepositoryLabel(context.Background(), "acme", "web",
				github.RepositoryLabel{Name: "bug", Color: "d73a4a"},
			)).To(Succeed())

			var sent map[string]any
			Expect(json.Unmarshal([]byte(bodies[0]), &sent)).To(Succeed())
			Expect(sent).To(HaveKeyWithValue("description", ""))
		})
	})

	Describe("updating", func() {
		It("renames as well as recolours", func() {
			server = record(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, `{}`)
			})

			Expect(client().UpdateRepositoryLabel(context.Background(), "acme", "web", "Bug",
				github.RepositoryLabel{Name: "bug", Color: "d73a4a"},
			)).To(Succeed())

			Expect(requests[0].Method).To(Equal(http.MethodPatch))
			Expect(requests[0].URL.Path).To(Equal("/repos/acme/web/labels/Bug"))

			// new_name, which is how the update endpoint spells "call it this"
			var sent map[string]any
			Expect(json.Unmarshal([]byte(bodies[0]), &sent)).To(Succeed())
			Expect(sent).To(HaveKeyWithValue("new_name", "bug"))
			Expect(sent).To(HaveKeyWithValue("color", "d73a4a"))
		})

		// An organization's labels are exactly the shape that breaks an
		// unescaped path: `kind/bug` addresses a different endpoint entirely
		It("escapes a label whose name contains a slash", func() {
			server = record(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, `{}`)
			})

			Expect(client().UpdateRepositoryLabel(context.Background(), "acme", "web", "kind/bug",
				github.RepositoryLabel{Name: "kind/bug", Color: "d73a4a"},
			)).To(Succeed())

			// EscapedPath keeps the encoding the client sent; Path would show
			// it already decoded and the assertion would pass either way
			Expect(requests[0].URL.EscapedPath()).
				To(Equal("/repos/acme/web/labels/kind%2Fbug"))
		})
	})

	Describe("deleting", func() {
		It("addresses the label by name, escaped", func() {
			server = record(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})

			Expect(client().DeleteRepositoryLabel(
				context.Background(), "acme", "web", "area/panel",
			)).To(Succeed())

			Expect(requests[0].Method).To(Equal(http.MethodDelete))
			Expect(requests[0].URL.EscapedPath()).
				To(Equal("/repos/acme/web/labels/area%2Fpanel"))
		})

		It("reports a refusal rather than swallowing it", func() {
			server = record(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = io.WriteString(w, `{"message":"Resource not accessible by integration"}`)
			})

			err := client().DeleteRepositoryLabel(context.Background(), "acme", "web", "bug")
			Expect(err).To(HaveOccurred())
			Expect(strings.ToLower(err.Error())).To(ContainSubstring("403"))
		})
	})
})
