package github_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

var _ = Describe("Repository files [Unit]", func() {
	var (
		server   *httptest.Server
		requests []string
		methods  map[string]string
		bodies   map[string]json.RawMessage
	)

	BeforeEach(func() {
		requests = nil
		methods = map[string]string{}
		bodies = map[string]json.RawMessage{}
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	record := func(answers map[string]string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests = append(requests, r.URL.RequestURI())
			methods[r.URL.Path] = r.Method

			if r.Body != nil {
				body, _ := io.ReadAll(r.Body)
				if len(body) > 0 {
					bodies[r.URL.Path] = body
				}
			}

			for suffix, answer := range answers {
				if strings.HasSuffix(r.URL.Path, suffix) {
					_, _ = io.WriteString(w, answer)

					return
				}
			}

			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"message":"Not Found"}`)
		}))
	}

	client := func() *github.Client {
		GinkgoHelper()

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		return client
	}

	Describe("ListRepositoryTree", func() {
		// The directories are kept, and so is every mode. A path holding
		// anything but an ordinary file cannot be written to without
		// destroying what is there, and git does that without a word.
		It("reads every path, and what git records at each of them", func() {
			server = record(map[string]string{
				"/git/trees/main": `{"sha":"t1","tree":[
					{"path":".github","type":"tree","mode":"040000","sha":"d1"},
					{"path":".github/ci.yaml","type":"blob","mode":"100644","sha":"b1","size":42},
					{"path":"README.md","type":"blob","mode":"100644","sha":"b2","size":7},
					{"path":"latest","type":"blob","mode":"120000","sha":"b3","size":9}
				],"truncated":false}`,
			})

			tree, err := client().ListRepositoryTree(
				context.Background(), "acme", "web", "main")

			Expect(err).NotTo(HaveOccurred())
			Expect(tree.Truncated).To(BeFalse())
			Expect(tree.Entries).To(Equal(map[string]github.TreeEntry{
				".github":         {Mode: "040000", Blob: "d1"},
				".github/ci.yaml": {Mode: "100644", Blob: "b1", Size: 42},
				"README.md":       {Mode: "100644", Blob: "b2", Size: 7},
				"latest":          {Mode: "120000", Blob: "b3", Size: 9},
			}))

			Expect(tree.Entries["README.md"].OrdinaryFile()).To(BeTrue())
			Expect(tree.Entries[".github"].OrdinaryFile()).To(BeFalse())
			Expect(tree.Entries["latest"].OrdinaryFile()).To(BeFalse())
		})

		It("asks for the whole tree rather than one level of it", func() {
			server = record(map[string]string{"/git/trees/main": `{"tree":[]}`})

			_, err := client().ListRepositoryTree(context.Background(), "acme", "web", "main")

			Expect(err).NotTo(HaveOccurred())
			Expect(requests).To(ConsistOf(ContainSubstring("recursive=1")))
		})

		// This endpoint takes exactly one path segment where a branch name can
		// hold several, so a repository whose default branch is `release/main`
		// asked for a route GitHub does not have and was answered 404 - which
		// reads as a repository with nothing in it.
		It("escapes a ref that carries a slash", func() {
			server = record(map[string]string{"/git/trees": `{"tree":[]}`})

			_, err := client().ListRepositoryTree(
				context.Background(), "acme", "web", "release/main")

			Expect(err).NotTo(HaveOccurred())
			Expect(requests).To(ConsistOf(ContainSubstring("/git/trees/release%2Fmain")))
		})

		// The difference between "this repository does not have that file" and
		// "GitHub did not say" is the difference between creating a file and
		// overwriting one, so the answer carries which it was.
		It("reports a listing GitHub declined to finish", func() {
			server = record(map[string]string{
				"/git/trees/main": `{"tree":[{"path":"a","type":"blob","sha":"b"}],
					"truncated":true}`,
			})

			tree, err := client().ListRepositoryTree(
				context.Background(), "acme", "web", "main")

			Expect(err).NotTo(HaveOccurred())
			Expect(tree.Truncated).To(BeTrue())
		})

		// An empty repository is a repository, and every managed file is
		// missing from it.
		It("reads a repository with no commits as an empty tree", func() {
			server = record(nil)

			tree, err := client().ListRepositoryTree(
				context.Background(), "acme", "web", "main")

			Expect(err).NotTo(HaveOccurred())
			Expect(tree.Entries).To(BeEmpty())
		})
	})

	// A listing and a level walk answer the same question, so they answer it
	// in the same type. Held apart, the two came to different conclusions
	// about a retired path whose parent had become a file.
	Describe("RepositoryTree.At", func() {
		held := github.RepositoryTree{Entries: map[string]github.TreeEntry{
			".github":         {Mode: "040000", Blob: "d1"},
			".github/ci.yaml": {Mode: "100644", Blob: "b1", Size: 42},
			"latest":          {Mode: "120000", Blob: "b3", Size: 9},
			"vendor":          {Mode: "160000", Blob: "c1"},
			"notes":           {Mode: "100644", Blob: "b4", Size: 3},
		}}

		DescribeTable("says what a repository holds at a path",
			func(filePath string, expected github.TreePath) {
				Expect(held.At(filePath)).To(Equal(expected))
			},

			Entry("an ordinary file", ".github/ci.yaml", github.TreePath{
				Entry: github.TreeEntry{Mode: "100644", Blob: "b1", Size: 42}, Found: true,
			}),
			Entry("a directory", ".github", github.TreePath{
				Entry: github.TreeEntry{Mode: "040000", Blob: "d1"}, Found: true,
			}),
			Entry("a symbolic link", "latest", github.TreePath{
				Entry: github.TreeEntry{Mode: "120000", Blob: "b3", Size: 9}, Found: true,
			}),
			Entry("a submodule", "vendor", github.TreePath{
				Entry: github.TreeEntry{Mode: "160000", Blob: "c1"}, Found: true,
			}),
			Entry("nothing at all", "CONTRIBUTING.md", github.TreePath{}),
			Entry("nothing at all, several levels down", "a/b/c.md", github.TreePath{}),

			// The path does not exist and cannot: git puts a blob or a tree at
			// a name, never both, so writing here replaces the file above.
			Entry("under a file", "notes/today.md", github.TreePath{Blocked: "notes"}),
			Entry("under a file, several levels down",
				"notes/2026/today.md", github.TreePath{Blocked: "notes"}),
			Entry("under a link", "latest/ci.yaml", github.TreePath{Blocked: "latest"}),
			Entry("under a submodule", "vendor/go.mod", github.TreePath{Blocked: "vendor"}),
		)

		// Whichever way it was read. The service compares the two against one
		// repository, and a difference between them is a repository answered
		// differently depending on how large it is.
		It("agrees with the level walk about a path under a file", func() {
			server = record(map[string]string{
				"/git/trees/main": `{"tree":[{"path":"notes","type":"blob",` +
					`"mode":"100644","sha":"b4"}]}`,
			})

			walked, err := client().ResolveTreePaths(
				context.Background(), "acme", "web", "main", []string{"notes/today.md"})

			Expect(err).NotTo(HaveOccurred())
			Expect(walked["notes/today.md"]).To(Equal(held.At("notes/today.md")))
		})
	})

	// Only reached where a whole-tree listing came back truncated, and exact
	// where that listing cannot be. Asking the contents API instead would
	// answer 404 for a path whose parent is a file, which reads as "nothing is
	// there" - and that is what turns a write into a create that takes the
	// parent out.
	Describe("ResolveTreePaths", func() {
		// One level per segment, so the fixtures are keyed by the tree each
		// request asks for rather than by the path.
		levels := func(root, docs string) map[string]string {
			return map[string]string{
				"/git/trees/main": root,
				"/git/trees/d1":   docs,
			}
		}

		resolve := func(paths ...string) map[string]github.TreePath {
			GinkgoHelper()

			found, err := client().ResolveTreePaths(
				context.Background(), "acme", "web", "main", paths)
			Expect(err).NotTo(HaveOccurred())

			return found
		}

		It("reads what git holds at a nested path", func() {
			server = record(levels(
				`{"tree":[{"path":"docs","type":"tree","mode":"040000","sha":"d1"}]}`,
				`{"tree":[{"path":"guide.md","type":"blob","mode":"100644",`+
					`"sha":"b1","size":12}]}`,
			))

			found := resolve("docs/guide.md")["docs/guide.md"]

			Expect(found.Found).To(BeTrue())
			Expect(found.Blocked).To(BeEmpty())
			Expect(found.Entry).To(Equal(github.TreeEntry{
				Mode: "100644", Blob: "b1", Size: 12,
			}))
		})

		It("reports a path nothing is at", func() {
			server = record(levels(
				`{"tree":[{"path":"docs","type":"tree","mode":"040000","sha":"d1"}]}`,
				`{"tree":[]}`,
			))

			found := resolve("docs/guide.md")["docs/guide.md"]

			Expect(found.Found).To(BeFalse())
			Expect(found.Blocked).To(BeEmpty())
		})

		It("names a directory on the way there that is a file", func() {
			server = record(map[string]string{
				"/git/trees/main": `{"tree":[{"path":"docs","type":"blob",` +
					`"mode":"100644","sha":"b1","size":4}]}`,
			})

			found := resolve("docs/guide.md")["docs/guide.md"]

			Expect(found.Found).To(BeFalse())
			Expect(found.Blocked).To(Equal("docs"))
		})

		It("reads a repository with no commits as holding nothing", func() {
			server = record(nil)

			Expect(resolve("README.md")["README.md"].Found).To(BeFalse())
		})

		// A level that stops early cannot say a path is absent, and absent is
		// the answer that becomes a create.
		It("refuses to read absence out of a level GitHub cut short", func() {
			server = record(map[string]string{
				"/git/trees/main": `{"tree":[{"path":"other","type":"blob",` +
					`"mode":"100644","sha":"b1"}],"truncated":true}`,
			})

			_, err := client().ResolveTreePaths(
				context.Background(), "acme", "web", "main", []string{"README.md"})

			Expect(err).To(HaveOccurred())
		})

		// Every managed path goes through the root, and most of them through
		// the same directory after it. Reading one path at a time read the
		// root once per path.
		It("reads each level once however many paths pass through it", func() {
			server = record(levels(
				`{"tree":[{"path":"docs","type":"tree","mode":"040000","sha":"d1"}]}`,
				`{"tree":[{"path":"one.md","type":"blob","mode":"100644","sha":"b1"},`+
					`{"path":"two.md","type":"blob","mode":"100644","sha":"b2"}]}`,
			))

			found := resolve("docs/one.md", "docs/two.md")

			Expect(found).To(HaveLen(2))
			Expect(found["docs/one.md"].Entry.Blob).To(Equal("b1"))
			Expect(found["docs/two.md"].Entry.Blob).To(Equal("b2"))

			Expect(requests).To(ConsistOf(
				ContainSubstring("/git/trees/main"), ContainSubstring("/git/trees/d1")))
		})
	})

	Describe("EditPullRequest", func() {
		It("rewrites the title and the body", func() {
			server = record(map[string]string{"/pulls/7": `{"number":7}`})

			Expect(client().EditPullRequest(
				context.Background(), "acme", "web", 7, "New title", "New body",
			)).To(Succeed())

			Expect(methods["/repos/acme/web/pulls/7"]).To(Equal(http.MethodPatch))
			Expect(bodies["/repos/acme/web/pulls/7"]).To(MatchJSON(
				`{"title":"New title","body":"New body"}`))
		})
	})

})
