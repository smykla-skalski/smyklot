package github_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

var _ = Describe("Secondary rate limits [Unit]", func() {
	var server *httptest.Server

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	// go-github remembers a secondary rate limit and then answers later calls
	// on the same client itself, without touching the network - which also
	// means without passing through this package's retry transport, so its own
	// cap on how long to wait never applies.
	//
	// A client is not per-call: the sweep mints one and reuses it for every
	// repository in an installation. Left unbounded, one request tripping abuse
	// detection fails every repository left in that sweep for as long as GitHub
	// named, which its documentation says can be minutes.
	// Deliberately the slowest spec in the package: proving the window is
	// bounded means waiting for it to pass, and a bound nobody waits out is a
	// bound nobody has checked.
	It("does not let one abuse response silence the client for as long as GitHub asked", func() {
		var calls atomic.Int32

		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)

			// An hour is well past anything this service should honour inline.
			w.Header().Set("Retry-After", "3600")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"You have exceeded a secondary rate limit"}`))
		}))

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		// First call trips the limit.
		Expect(client.AddLabel(ctx, "acme", "web", 42, "kind/bug")).NotTo(Succeed())

		before := calls.Load()
		Expect(before).To(BeNumerically(">=", 1))

		// A second call must still be able to reach the server once the cap has
		// elapsed. Without the cap, go-github answers it from memory for the
		// full hour and the request count never moves again.
		Eventually(func() int32 {
			_ = client.AddLabel(ctx, "acme", "web", 42, "kind/bug")

			return calls.Load()
		}, 15*time.Second, 250*time.Millisecond).Should(BeNumerically(">", before))
	})
})
