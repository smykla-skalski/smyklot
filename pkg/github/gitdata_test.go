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

var _ = Describe("Git data [Unit]", func() {
	var (
		server *httptest.Server
		bodies map[string]json.RawMessage
	)

	BeforeEach(func() {
		bodies = map[string]json.RawMessage{}
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	// record answers each endpoint with a canned reply and keeps what was sent,
	// because what these methods put on the wire is the whole of what they do.
	record := func(answers map[string]string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	Describe("GetRef", func() {
		It("resolves a branch to its commit", func() {
			server = record(map[string]string{
				"/git/ref/heads/main": `{"object":{"sha":"abc123"}}`,
			})

			Expect(client().GetRef(
				context.Background(), "acme", "web", "heads/main",
			)).To(Equal("abc123"))
		})

		// "Is this branch there" is a question, and 404 is one of its answers.
		// Reading it as a failure is what would make the migration retry
		// forever against a repository that simply has no such branch.
		It("reads an absent reference as an empty answer", func() {
			server = record(nil)

			sha, err := client().GetRef(context.Background(), "acme", "web", "heads/absent")
			Expect(err).NotTo(HaveOccurred())
			Expect(sha).To(BeEmpty())
		})
	})

	Describe("GetCommit", func() {
		// The tree because a commit is not one, and the message because it is
		// what tells Smyklot's own commit on a branch from anybody else's.
		It("reads the tree and the message", func() {
			server = record(map[string]string{
				"/git/commits/c1": `{"tree":{"sha":"t1"},"message":"chore: move it"}`,
			})

			Expect(client().GetCommit(context.Background(), "acme", "web", "c1")).
				To(Equal(github.Commit{Tree: "t1", Message: "chore: move it"}))
		})
	})

	Describe("CreateTree", func() {
		// The API spells a deletion as an explicit null sha. An omitted key
		// leaves the path alone and an empty string asks for an object named
		// "", so this is the one entry whose exact bytes matter.
		It("sends a null object for a path being deleted", func() {
			server = record(map[string]string{"/git/trees": `{"sha":"tree1"}`})

			_, err := client().CreateTree(
				context.Background(), "acme", "web", "base1",
				[]github.TreeChange{
					{Path: ".smyklot.toml", Blob: "blob1"},
					{Path: ".github/smyklot.yaml"},
				},
			)
			Expect(err).NotTo(HaveOccurred())

			var sent struct {
				BaseTree string `json:"base_tree"`
				Tree     []struct {
					Path string  `json:"path"`
					SHA  *string `json:"sha"`
				} `json:"tree"`
			}
			Expect(json.Unmarshal(bodies["/repos/acme/web/git/trees"], &sent)).To(Succeed())

			Expect(sent.BaseTree).To(Equal("base1"))
			Expect(sent.Tree).To(HaveLen(2))
			Expect(sent.Tree[0].SHA).To(HaveValue(Equal("blob1")))
			Expect(sent.Tree[1].Path).To(Equal(".github/smyklot.yaml"))
			Expect(sent.Tree[1].SHA).To(BeNil())
		})
	})

	Describe("CreateRef", func() {
		It("sends the fully qualified reference", func() {
			server = record(map[string]string{"/git/refs": `{"object":{"sha":"c1"}}`})

			Expect(client().CreateRef(
				context.Background(), "acme", "web", "heads/smyklot/x", "c1",
			)).To(Succeed())

			var sent struct {
				Ref string `json:"ref"`
				SHA string `json:"sha"`
			}
			Expect(json.Unmarshal(bodies["/repos/acme/web/git/refs"], &sent)).To(Succeed())
			Expect(sent.Ref).To(Equal("refs/heads/smyklot/x"))
			Expect(sent.SHA).To(Equal("c1"))
		})
	})

	Describe("FindPullRequestByHead", func() {
		// Whatever state, because the answer decides whether to propose
		// something again. Listing only the open ones would read a refusal as
		// "nobody has asked yet" and ask forever.
		It("reports a pull request somebody closed without merging", func() {
			server = record(map[string]string{
				"/pulls": `[{"number":12,"state":"closed","merged":false}]`,
			})

			pull, err := client().FindPullRequestByHead(
				context.Background(), "acme", "web", "smyklot/x",
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(pull).NotTo(BeNil())
			Expect(pull.Number).To(Equal(12))
			Expect(pull.State).To(Equal("closed"))
			Expect(pull.Merged).To(BeFalse())
		})

		It("reports nothing for a branch nobody opened one from", func() {
			server = record(map[string]string{"/pulls": `[]`})

			pull, err := client().FindPullRequestByHead(
				context.Background(), "acme", "web", "smyklot/x",
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(pull).To(BeNil())
		})
	})
})
