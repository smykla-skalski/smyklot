package github_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/githubtest"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

var _ = Describe("Repository configuration discovery [Unit]", func() {
	var server *httptest.Server

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	// serveOnly answers the contents API for exactly one path and 404s the rest,
	// recording every path asked for.
	serveOnly := func(present, body string, asked *[]string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := strings.TrimPrefix(r.URL.Path, "/repos/acme/web/contents/")
			*asked = append(*asked, path)

			if present != "" && path == present {
				_, _ = w.Write([]byte(githubtest.ContentsResponse(body)))

				return
			}

			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Not Found"}`))
		}))
	}

	DescribeTable("finds the file wherever a repository put it",
		func(path string) {
			var asked []string

			server = serveOnly(path, "quiet_success = true\n", &asked)

			client, err := github.NewClient("test-token", server.URL)
			Expect(err).NotTo(HaveOccurred())

			found, err := client.FindRepoConfig(context.Background(), "acme", "web", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(found.Found()).To(BeTrue())
			Expect(found.Path).To(Equal(path))
		},
		Entry("repository root", ".smyklot.toml"),
		Entry("its own directory", ".smyklot/config.toml"),
		Entry("under .github", ".github/.smyklot.toml"),
		Entry("the legacy file", ".github/smyklot.yaml"),
	)

	It("stops at the first match rather than reading them all", func() {
		var asked []string

		server = serveOnly(".smyklot.toml", "quiet_success = true\n", &asked)

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		_, err = client.FindRepoConfig(context.Background(), "acme", "web", "")
		Expect(err).NotTo(HaveOccurred())
		Expect(asked).To(Equal([]string{".smyklot.toml"}))
	})

	// TOML wins over the legacy file, so a half-finished migration leaves the
	// repository reading the file it migrated to rather than the one it left.
	It("prefers TOML over the legacy file when a repository has both", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, ".github/.smyklot.toml") ||
				strings.HasSuffix(r.URL.Path, ".github/smyklot.yaml") {
				_, _ = w.Write([]byte(githubtest.ContentsResponse("quiet_success = true\n")))

				return
			}

			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Not Found"}`))
		}))

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		found, err := client.FindRepoConfig(context.Background(), "acme", "web", "")
		Expect(err).NotTo(HaveOccurred())
		Expect(found.Path).To(Equal(".github/.smyklot.toml"))
	})

	Describe("a path an operator chose in the panel", func() {
		It("is looked at first", func() {
			var asked []string

			server = serveOnly("config/smyklot.toml", "quiet_success = true\n", &asked)

			client, err := github.NewClient("test-token", server.URL)
			Expect(err).NotTo(HaveOccurred())

			found, err := client.FindRepoConfig(
				context.Background(), "acme", "web", "config/smyklot.toml",
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(found.Path).To(Equal("config/smyklot.toml"))
			Expect(asked).To(Equal([]string{"config/smyklot.toml"}))
		})

		// Naming a standard path must not make it cost two requests.
		It("is not asked for twice when it is already a standard path", func() {
			var asked []string

			server = serveOnly("", "", &asked)

			client, err := github.NewClient("test-token", server.URL)
			Expect(err).NotTo(HaveOccurred())

			_, err = client.FindRepoConfig(
				context.Background(), "acme", "web", ".github/.smyklot.toml",
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(asked).To(HaveLen(len(github.RepoConfigPaths)))
			Expect(asked[0]).To(Equal(".github/.smyklot.toml"))
			Expect(asked).To(ConsistOf(github.RepoConfigPaths))
		})
	})

	// A repository with no file is the common case, and it is the one whose
	// cost has to be bounded. Four requests is the whole budget; anything that
	// grows this grows it for every repository on every miss.
	It("spends one request per candidate on a repository with no file", func() {
		var asked []string

		server = serveOnly("", "", &asked)

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		found, err := client.FindRepoConfig(context.Background(), "acme", "web", "")
		Expect(err).NotTo(HaveOccurred())
		Expect(found.Found()).To(BeFalse())
		Expect(asked).To(Equal(github.RepoConfigPaths))
	})

	It("surfaces a read failure rather than trying the next path", func() {
		var asked []string

		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			asked = append(asked, r.URL.Path)
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"Resource not accessible by integration"}`))
		}))

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		_, err = client.FindRepoConfig(context.Background(), "acme", "web", "")
		Expect(err).To(HaveOccurred())
		Expect(asked).To(HaveLen(1))
	})

	Describe("DefaultBranchHead", func() {
		It("reads the head of the default branch in one request", func() {
			var asked []string

			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				asked = append(asked, r.URL.Path+"?"+r.URL.RawQuery)
				_, _ = w.Write([]byte(`[{"sha":"abc123"}]`))
			}))

			client, err := github.NewClient("test-token", server.URL)
			Expect(err).NotTo(HaveOccurred())

			head, err := client.DefaultBranchHead(context.Background(), "acme", "web")
			Expect(err).NotTo(HaveOccurred())
			Expect(head).To(Equal("abc123"))
			Expect(asked).To(HaveLen(1))
			Expect(asked[0]).To(Equal("/repos/acme/web/commits?per_page=1"))
		})

		// An empty repository has no head and no configuration file. Reporting
		// a failure would make the sweep retry it forever.
		It("reads an empty repository as having no head", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusConflict)
				_, _ = w.Write([]byte(`{"message":"Git Repository is empty."}`))
			}))

			client, err := github.NewClient("test-token", server.URL)
			Expect(err).NotTo(HaveOccurred())

			head, err := client.DefaultBranchHead(context.Background(), "acme", "web")
			Expect(err).NotTo(HaveOccurred())
			Expect(head).To(BeEmpty())
		})

		It("surfaces any other failure", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"message":"nope"}`))
			}))

			client, err := github.NewClient("test-token", server.URL)
			Expect(err).NotTo(HaveOccurred())

			_, err = client.DefaultBranchHead(context.Background(), "acme", "web")
			Expect(err).To(HaveOccurred())
		})
	})
})
