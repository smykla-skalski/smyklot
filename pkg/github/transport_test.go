package github_test

import (
	"context"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

var _ = Describe("Request headers [Unit]", func() {
	var (
		server *httptest.Server
		seen   http.Header
	)

	BeforeEach(func() {
		seen = nil
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seen = r.Header.Clone()
			_, _ = w.Write([]byte(`{"id":42,"login":"someone"}`))
		}))
	})

	AfterEach(func() {
		server.Close()
	})

	// GitHub rejects the token scheme on app-level endpoints such as GET /app,
	// so the two constructors are not interchangeable. Bearer would in fact be
	// accepted for both, which is exactly why this needs a spec: nothing else
	// would notice the distinction collapsing.
	It("sends the token scheme for an installation client", func() {
		client, err := github.NewClient("installation-token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		_, err = client.GetUser(context.Background(), "someone")
		Expect(err).NotTo(HaveOccurred())
		Expect(seen.Get("Authorization")).To(Equal("token installation-token"))
	})

	It("sends the bearer scheme for an app client", func() {
		client, err := github.NewAppClient("app-jwt", server.URL)
		Expect(err).NotTo(HaveOccurred())

		Expect(client.Ping(context.Background())).To(Succeed())
		Expect(seen.Get("Authorization")).To(Equal("Bearer app-jwt"))
	})

	It("identifies itself", func() {
		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		_, err = client.GetUser(context.Background(), "someone")
		Expect(err).NotTo(HaveOccurred())
		Expect(seen.Get("User-Agent")).To(Equal("smyklot-github-app"))
	})

	It("asks for the JSON GitHub answers with", func() {
		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		_, err = client.GetUser(context.Background(), "someone")
		Expect(err).NotTo(HaveOccurred())
		Expect(seen.Get("Accept")).To(ContainSubstring("+json"))
	})
})
