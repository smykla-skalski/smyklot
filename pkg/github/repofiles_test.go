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

	// GitHub declines to list a very large tree in one answer, and the walk
	// that reads one directory at a time costs a request per directory - five
	// thousand of them for a repository the size of Linux. Truncation is a
	// property of a RESPONSE though, so a subtree of a tree too large to list
	// is usually listable whole, and the division only has to go where it was
	// actually refused. Measured on torvalds/linux: 95,056 files in 26
	// requests, where the single truncated read reports 67,614.
	Describe("ListWholeRepositoryTree", func() {
		// Answers by tree and by whether the read asked for the whole thing,
		// which the shared recorder cannot do: the division reads one address
		// twice and needs a different answer each time.
		divided := func(whole, level map[string]string) *httptest.Server {
			return httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					requests = append(requests, r.URL.RequestURI())
					at := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]

					answers := level
					if r.URL.Query().Get("recursive") == "1" {
						answers = whole
					}
					if answer, known := answers[at]; known {
						_, _ = io.WriteString(w, answer)

						return
					}

					w.WriteHeader(http.StatusNotFound)
					_, _ = io.WriteString(w, `{"message":"Not Found"}`)
				}))
		}

		It("reads a listing GitHub finishes in one request", func() {
			server = divided(map[string]string{
				"main": `{"tree":[{"path":"README.md","type":"blob",
					"mode":"100644","sha":"b1","size":7}],"truncated":false}`,
			}, nil)

			tree, err := client().ListWholeRepositoryTree(
				context.Background(), "acme", "web", "main")

			Expect(err).NotTo(HaveOccurred())
			Expect(tree.Truncated).To(BeFalse())
			Expect(tree.Entries).To(HaveKey("README.md"))
			Expect(requests).To(HaveLen(1))
		})

		// The point of the whole thing: the root will not be listed, so one
		// level of it is read and each subdirectory is asked whole. Nothing is
		// dropped and no directory that answered is read again.
		It("divides where it was refused and keeps every path", func() {
			server = divided(
				map[string]string{
					"main": `{"tree":[],"truncated":true}`,
					"d1": `{"tree":[{"path":"guide.md","type":"blob",
						"mode":"100644","sha":"b2","size":8}],"truncated":false}`,
					"d2": `{"tree":[{"path":"deep/one.md","type":"blob",
						"mode":"100644","sha":"b3","size":4}],"truncated":false}`,
				},
				map[string]string{
					"main": `{"tree":[
						{"path":"README.md","type":"blob","mode":"100644","sha":"b1","size":7},
						{"path":"docs","type":"tree","mode":"040000","sha":"d1"},
						{"path":"src","type":"tree","mode":"040000","sha":"d2"}]}`,
				})

			tree, err := client().ListWholeRepositoryTree(
				context.Background(), "acme", "web", "main")

			Expect(err).NotTo(HaveOccurred())
			Expect(tree.Truncated).To(BeFalse())
			// Every entry from the root's own level, and every entry of each
			// subtree under the name it sits at.
			Expect(tree.Entries).To(HaveKey("README.md"))
			Expect(tree.Entries).To(HaveKey("docs"))
			Expect(tree.Entries).To(HaveKey("docs/guide.md"))
			Expect(tree.Entries).To(HaveKey("src/deep/one.md"))
			// The root whole, the root level, and one per subdirectory. The
			// subdirectories answered, so neither is divided again.
			Expect(requests).To(HaveLen(4))
		})

		// A subtree that is itself too large is divided the same way, which is
		// what makes this a rule rather than one special case at the root.
		It("divides a subtree that is refused as well", func() {
			server = divided(
				map[string]string{
					"main": `{"tree":[],"truncated":true}`,
					"d1":   `{"tree":[],"truncated":true}`,
					"d2": `{"tree":[{"path":"one.md","type":"blob",
						"mode":"100644","sha":"b1","size":4}],"truncated":false}`,
				},
				map[string]string{
					"main": `{"tree":[{"path":"docs","type":"tree",
						"mode":"040000","sha":"d1"}]}`,
					"d1": `{"tree":[{"path":"deep","type":"tree",
						"mode":"040000","sha":"d2"}]}`,
				})

			tree, err := client().ListWholeRepositoryTree(
				context.Background(), "acme", "web", "main")

			Expect(err).NotTo(HaveOccurred())
			Expect(tree.Entries).To(HaveKey("docs/deep/one.md"))
		})

		// The one truncation the division cannot answer: a single directory
		// holding more entries than one response carries. There is nothing left
		// to divide, so what comes back is a partial list - and it used to be
		// recorded as a complete one, which is the reading a path finder then
		// tells somebody every missing file does not exist on.
		It("reports a level GitHub truncates as well", func() {
			server = divided(
				map[string]string{
					"main": `{"tree":[],"truncated":true}`,
					"d1": `{"tree":[{"path":"guide.md","type":"blob",
						"mode":"100644","sha":"b2","size":8}],"truncated":false}`,
				},
				map[string]string{
					"main": `{"tree":[{"path":"docs","type":"tree",
						"mode":"040000","sha":"d1"}],"truncated":true}`,
				})

			tree, err := client().ListWholeRepositoryTree(
				context.Background(), "acme", "web", "main")

			Expect(err).NotTo(HaveOccurred())
			Expect(tree.Truncated).To(BeTrue())
			// And what it did read is kept: a partial list is worth having,
			// said to be partial.
			Expect(tree.Entries).To(HaveKey("docs/guide.md"))
		})

		// A division reads the subtrees of a level it was refused, and the
		// budget above it can run out part way through. Ranging the level's map
		// meant which of them were reached first was Go's map iteration order,
		// so the same repository listed twice came back holding different files
		// - a difference nobody could reproduce and nothing could explain, in
		// the one case the caller is already being told is incomplete.
		//
		// Asserted through the order the subtrees are asked for, which is what
		// the descent actually decides, and read twice because one run of a map
		// range looks perfectly ordered often enough to pass.
		It("descends a refused level in name order", func() {
			server = divided(
				map[string]string{
					"main": `{"tree":[],"truncated":true}`,
					"t1":   `{"tree":[{"path":"a.md","type":"blob","mode":"100644","sha":"b1"}]}`,
					"t2":   `{"tree":[{"path":"b.md","type":"blob","mode":"100644","sha":"b2"}]}`,
					"t3":   `{"tree":[{"path":"c.md","type":"blob","mode":"100644","sha":"b3"}]}`,
					"t4":   `{"tree":[{"path":"d.md","type":"blob","mode":"100644","sha":"b4"}]}`,
					"t5":   `{"tree":[{"path":"e.md","type":"blob","mode":"100644","sha":"b5"}]}`,
					"t6":   `{"tree":[{"path":"f.md","type":"blob","mode":"100644","sha":"b6"}]}`,
				},
				// Listed in an order that is neither the names' nor the SHAs',
				// and six of them: with three, one run of a randomised map range
				// comes out sorted often enough to pass by luck.
				map[string]string{
					"main": `{"tree":[
						{"path":"zeta","type":"tree","mode":"040000","sha":"t6"},
						{"path":"alpha","type":"tree","mode":"040000","sha":"t1"},
						{"path":"omega","type":"tree","mode":"040000","sha":"t4"},
						{"path":"delta","type":"tree","mode":"040000","sha":"t2"},
						{"path":"sigma","type":"tree","mode":"040000","sha":"t5"},
						{"path":"kappa","type":"tree","mode":"040000","sha":"t3"}],
						"truncated":false}`,
				})

			descents := func() []string {
				requests = nil
				_, err := client().ListWholeRepositoryTree(
					context.Background(), "acme", "web", "main")
				Expect(err).NotTo(HaveOccurred())

				asked := []string{}
				for _, one := range requests {
					for _, subtree := range []string{"t1", "t2", "t3", "t4", "t5", "t6"} {
						if strings.Contains(one, "/git/trees/"+subtree) {
							asked = append(asked, subtree)
						}
					}
				}

				return asked
			}

			// Name order of the DIRECTORIES - alpha, delta, kappa, omega,
			// sigma, zeta - which the SHAs above are numbered to spell out.
			wanted := []string{"t1", "t2", "t3", "t4", "t5", "t6"}
			Expect(descents()).To(Equal(wanted))
			Expect(descents()).To(Equal(wanted))
		})

		It("reads a repository with no commits as an empty tree", func() {
			server = divided(nil, nil)

			tree, err := client().ListWholeRepositoryTree(
				context.Background(), "acme", "web", "main")

			Expect(err).NotTo(HaveOccurred())
			Expect(tree.Entries).To(BeEmpty())
			Expect(tree.Missing).To(BeTrue())
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
