package github_test

import (
	"context"
	"encoding/base64"
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
		It("reads every file, and what is at each of them", func() {
			server = record(map[string]string{
				"/git/trees/main": `{"sha":"t1","tree":[
					{"path":".github","type":"tree","sha":"d1"},
					{"path":".github/ci.yaml","type":"blob","sha":"b1","size":42},
					{"path":"README.md","type":"blob","sha":"b2","size":7}
				],"truncated":false}`,
			})

			tree, err := client().ListRepositoryTree(
				context.Background(), "acme", "web", "main")

			Expect(err).NotTo(HaveOccurred())
			Expect(tree.Truncated).To(BeFalse())
			Expect(tree.Blobs).To(Equal(map[string]github.TreeBlob{
				".github/ci.yaml": {Blob: "b1", Size: 42},
				"README.md":       {Blob: "b2", Size: 7},
			}))
		})

		It("asks for the whole tree rather than one level of it", func() {
			server = record(map[string]string{"/git/trees/main": `{"tree":[]}`})

			_, err := client().ListRepositoryTree(context.Background(), "acme", "web", "main")

			Expect(err).NotTo(HaveOccurred())
			Expect(requests).To(ConsistOf(ContainSubstring("recursive=1")))
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
			Expect(tree.Blobs).To(BeEmpty())
		})
	})

	Describe("GetRepositoryFile", func() {
		It("reads a file at a ref", func() {
			server = record(map[string]string{
				"/contents/README.md": `{"content":"` +
					base64.StdEncoding.EncodeToString([]byte("hello\n")) + `"}`,
			})

			content, found, err := client().GetRepositoryFile(
				context.Background(), "acme", "web", "main", "README.md")

			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
			Expect(string(content)).To(Equal("hello\n"))
			Expect(requests).To(ConsistOf(ContainSubstring("ref=main")))
		})

		It("reports a file that is not there", func() {
			server = record(nil)

			_, found, err := client().GetRepositoryFile(
				context.Background(), "acme", "web", "main", "README.md")

			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeFalse())
		})

		// A file that exists and is empty is not a file that is absent, and the
		// read underneath answers both with no content.
		It("reports an empty file as one that is there", func() {
			server = record(map[string]string{"/contents/EMPTY": `{"content":""}`})

			content, found, err := client().GetRepositoryFile(
				context.Background(), "acme", "web", "main", "EMPTY")

			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
			Expect(content).To(BeEmpty())
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

	Describe("DeleteRef", func() {
		It("removes the reference", func() {
			server = record(map[string]string{"/git/refs/heads/gone": `{}`})

			Expect(client().DeleteRef(
				context.Background(), "acme", "web", "heads/gone",
			)).To(Succeed())

			Expect(methods["/repos/acme/web/git/refs/heads/gone"]).
				To(Equal(http.MethodDelete))
		})
	})
})
