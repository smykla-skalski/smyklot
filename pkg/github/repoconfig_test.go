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

	// Every candidate is asked for, not just those up to the first hit, so a
	// repository carrying two configuration files can be told which one is in
	// charge. The fingerprint cache is what makes that affordable: the service
	// asks only when something a file could live in has moved.
	It("asks for every candidate even once it has an answer", func() {
		var asked []string

		server = serveOnly(".smyklot.toml", "quiet_success = true\n", &asked)

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		found, err := client.FindRepoConfig(context.Background(), "acme", "web", "")
		Expect(err).NotTo(HaveOccurred())
		Expect(found.Path).To(Equal(".smyklot.toml"))
		Expect(found.Superseded).To(BeEmpty())
		Expect(asked).To(Equal(github.RepoConfigPaths))
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

		// The file it migrated away from is still there, still believed by
		// whoever wrote it to be in charge, and read by nothing
		Expect(found.Superseded).To(Equal([]string{".github/smyklot.yaml"}))
	})

	// Reporting must not take a repository offline over a file it is not even
	// using. Before anything is found the read has failed and stays fail-closed
	It("keeps the file it found when a later candidate cannot be read", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/contents/.smyklot.toml") {
				_, _ = w.Write([]byte(githubtest.ContentsResponse("quiet_success = true\n")))

				return
			}

			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"Resource not accessible by integration"}`))
		}))

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		found, err := client.FindRepoConfig(context.Background(), "acme", "web", "")
		Expect(err).NotTo(HaveOccurred())
		Expect(found.Path).To(Equal(".smyklot.toml"))
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
			Expect(asked[0]).To(Equal("config/smyklot.toml"))
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

	// What is watched has to cover what is searched, or a file changes and the
	// cache goes on serving the answer from before it did. Nothing sets a
	// preferred path yet, and the first thing to do so would otherwise have
	// found this out in production
	It("watches the root of every path it searches", func() {
		var asked []string

		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			asked = append(asked, r.URL.Path)
			_, _ = w.Write([]byte(`[
				{"name":".smyklot.toml","sha":"a"},
				{"name":".smyklot","sha":"b"},
				{"name":".github","sha":"c"},
				{"name":"config","sha":"d"}
			]`))
		}))

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		standard, err := client.RepoConfigFingerprint(context.Background(), "acme", "web", "")
		Expect(err).NotTo(HaveOccurred())
		Expect(standard).NotTo(ContainSubstring("config="))

		chosen, err := client.RepoConfigFingerprint(
			context.Background(), "acme", "web", "config/smyklot.toml",
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(chosen).To(ContainSubstring("config=d"))
		Expect(chosen).NotTo(Equal(standard))
	})

	// The file decides what may be run, so it is read from whatever GitHub
	// serves by default - the default branch - and never from a ref a caller
	// names. Sending one would let a pull request widen allowed_commands for
	// its own pull request, which is a privilege escalation with a one-line
	// diff and no test standing in its way until now.
	It("never asks for a configuration file at a ref anybody chose", func() {
		var queries []string

		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			queries = append(queries, r.URL.RawQuery)
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Not Found"}`))
		}))

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		_, err = client.FindRepoConfig(context.Background(), "acme", "web", "")
		Expect(err).NotTo(HaveOccurred())

		_, err = client.RepoConfigFingerprint(context.Background(), "acme", "web", "")
		Expect(err).NotTo(HaveOccurred())

		Expect(queries).NotTo(BeEmpty())
		for _, query := range queries {
			Expect(query).To(BeEmpty())
		}
	})

	It("pins migration reads to the resolved default-branch commit", func() {
		var queries []string

		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			queries = append(queries, r.URL.Query().Get("ref"))
			if strings.HasSuffix(r.URL.Path, "/contents/.github/smyklot.yaml") {
				_, _ = w.Write([]byte(githubtest.ContentsResponse("quiet_success: true\n")))

				return
			}
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Not Found"}`))
		}))

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		found, err := client.FindRepoConfigAtCommit(
			context.Background(), "acme", "web", "", "base-commit",
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(found.Path).To(Equal(".github/smyklot.yaml"))
		Expect(queries).To(HaveLen(len(github.RepoConfigPaths)))
		Expect(queries).To(ConsistOf(
			"base-commit", "base-commit", "base-commit", "base-commit",
		))
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

	Describe("RepoConfigFingerprint", func() {
		// One request, and it must be sensitive to what a configuration file
		// could live in and to nothing else. Fingerprinting the head commit
		// instead would report a change on every commit, and re-probe every
		// candidate path on every sweep tick for any repository anyone works in.
		It("reads the roots a configuration file can live in, in one request", func() {
			var asked []string

			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				asked = append(asked, r.URL.Path)
				_, _ = w.Write([]byte(`[
					{"name":"README.md","sha":"aaa"},
					{"name":".github","sha":"bbb"},
					{"name":"main.go","sha":"ccc"}
				]`))
			}))

			client, err := github.NewClient("test-token", server.URL)
			Expect(err).NotTo(HaveOccurred())

			fingerprint, err := client.RepoConfigFingerprint(context.Background(), "acme", "web", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(fingerprint).To(ContainSubstring(".github=bbb"))
			Expect(fingerprint).NotTo(ContainSubstring("aaa"))
			Expect(fingerprint).NotTo(ContainSubstring("ccc"))
			Expect(asked).To(Equal([]string{"/repos/acme/web/contents"}))
		})

		fingerprintOf := func(body string) string {
			GinkgoHelper()

			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			DeferCleanup(server.Close)

			client, err := github.NewClient("test-token", server.URL)
			Expect(err).NotTo(HaveOccurred())

			fingerprint, err := client.RepoConfigFingerprint(context.Background(), "acme", "web", "")
			Expect(err).NotTo(HaveOccurred())

			return fingerprint
		}

		// The whole point: a commit that touches nothing a configuration file
		// could be in must not look like a change.
		It("does not change when an unrelated file does", func() {
			before := fingerprintOf(`[{"name":"main.go","sha":"one"},{"name":".github","sha":"gh"}]`)
			after := fingerprintOf(`[{"name":"main.go","sha":"two"},{"name":".github","sha":"gh"}]`)

			Expect(after).To(Equal(before))
		})

		DescribeTable("changes when something it could be in does",
			func(body string) {
				Expect(fingerprintOf(body)).NotTo(Equal(
					fingerprintOf(`[{"name":".github","sha":"gh"}]`),
				))
			},
			Entry("the .github tree moved", `[{"name":".github","sha":"moved"}]`),
			Entry("a root file appeared", `[{"name":".github","sha":"gh"},{"name":".smyklot.toml","sha":"x"}]`),
			Entry("a directory appeared", `[{"name":".github","sha":"gh"},{"name":".smyklot","sha":"y"}]`),
			Entry("everything went away", `[]`),
		)

		// An empty repository has no configuration file. Reporting a failure
		// would make the sweep retry it forever.
		It("reads an empty repository as having no roots", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"message":"This repository is empty."}`))
			}))

			client, err := github.NewClient("test-token", server.URL)
			Expect(err).NotTo(HaveOccurred())

			fingerprint, err := client.RepoConfigFingerprint(context.Background(), "acme", "web", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(fingerprint).To(BeEmpty())
		})

		It("surfaces any other failure", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"message":"nope"}`))
			}))

			client, err := github.NewClient("test-token", server.URL)
			Expect(err).NotTo(HaveOccurred())

			_, err = client.RepoConfigFingerprint(context.Background(), "acme", "web", "")
			Expect(err).To(HaveOccurred())
		})

	})
})
