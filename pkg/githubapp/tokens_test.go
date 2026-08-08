package githubapp_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/githubtest"
	"github.com/smykla-skalski/smyklot/pkg/githubapp"
)

// tokenServer stands in for GitHub's installation token endpoint, recording
// which installations were asked for
type tokenServer struct {
	mu       sync.Mutex
	requests []string
}

func (s *tokenServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.requests = append(s.requests, r.URL.Path)

	expiry := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)

	w.WriteHeader(http.StatusCreated)
	_, _ = fmt.Fprintf(w, `{"token": "token-for%s", "expires_at": %q}`, r.URL.Path, expiry)
}

func (s *tokenServer) paths() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]string(nil), s.requests...)
}

var _ = Describe("TokenStore [Unit]", func() {
	var (
		server *httptest.Server
		github *tokenServer
	)

	BeforeEach(func() {
		github = &tokenServer{}
		server = httptest.NewServer(github)
		DeferCleanup(server.Close)
	})

	Describe("NewTokenStore", func() {
		It("should reject a missing identifier", func() {
			_, err := githubapp.NewTokenStore("", githubtest.AppPrivateKey(), "", 0)
			Expect(err).To(MatchError(githubapp.ErrNoAppID))
		})

		It("should reject a missing private key", func() {
			_, err := githubapp.NewTokenStore("Iv1.test", nil, "", 0)
			Expect(err).To(MatchError(githubapp.ErrNoPrivateKey))
		})

		It("should reject a private key that is not a key", func() {
			_, err := githubapp.NewTokenStore("Iv1.test", []byte("not a key"), "", 0)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("AppToken", func() {
		It("should return a JWT for the App itself", func() {
			store, err := githubapp.NewTokenStore("Iv1.test", githubtest.AppPrivateKey(), server.URL, 0)
			Expect(err).NotTo(HaveOccurred())

			token, err := store.AppToken()
			Expect(err).NotTo(HaveOccurred())

			// A JWT is three dot-separated segments; the installation endpoint
			// is not involved
			Expect(strings.Split(token, ".")).To(HaveLen(3))
			Expect(github.paths()).To(BeEmpty())
		})
	})

	Describe("InstallationToken", func() {
		It("should mint a token for the installation it is asked about", func() {
			store, err := githubapp.NewTokenStore("Iv1.test", githubtest.AppPrivateKey(), server.URL, 0)
			Expect(err).NotTo(HaveOccurred())

			token, err := store.InstallationToken(987)
			Expect(err).NotTo(HaveOccurred())
			Expect(token).To(Equal("token-for/app/installations/987/access_tokens"))
		})

		// One process serves many installations, so the store must key on the
		// installation rather than hold a single token
		It("should keep installations apart", func() {
			store, err := githubapp.NewTokenStore("Iv1.test", githubtest.AppPrivateKey(), server.URL, 0)
			Expect(err).NotTo(HaveOccurred())

			first, err := store.InstallationToken(111)
			Expect(err).NotTo(HaveOccurred())

			second, err := store.InstallationToken(222)
			Expect(err).NotTo(HaveOccurred())

			Expect(first).NotTo(Equal(second))
			Expect(github.paths()).To(Equal([]string{
				"/app/installations/111/access_tokens",
				"/app/installations/222/access_tokens",
			}))
		})

		// Without caching, every webhook delivery would spend a round trip
		// minting a token that is still valid
		It("should reuse a token that has not expired", func() {
			store, err := githubapp.NewTokenStore("Iv1.test", githubtest.AppPrivateKey(), server.URL, 0)
			Expect(err).NotTo(HaveOccurred())

			for range 5 {
				_, err := store.InstallationToken(987)
				Expect(err).NotTo(HaveOccurred())
			}

			Expect(github.paths()).To(HaveLen(1))
		})

		It("should surface a minting failure", func() {
			failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"message": "Not Found"}`))
			}))
			DeferCleanup(failing.Close)

			store, err := githubapp.NewTokenStore("Iv1.test", githubtest.AppPrivateKey(), failing.URL, 0)
			Expect(err).NotTo(HaveOccurred())

			_, err = store.InstallationToken(987)
			Expect(err).To(MatchError(ContainSubstring("404")))
		})

		// go-githubauth mints with context.Background() and no overall client
		// timeout, so without one of ours a GitHub that accepts the connection
		// and then goes quiet would hold this worker for good
		It("should give up on a GitHub that never answers", func() {
			blocked := make(chan struct{})

			hanging := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				select {
				case <-blocked:
				case <-r.Context().Done():
				}
			}))
			DeferCleanup(func() {
				close(blocked)
				hanging.Close()
			})

			store, err := githubapp.NewTokenStore("Iv1.test", githubtest.AppPrivateKey(), hanging.URL, 200*time.Millisecond)
			Expect(err).NotTo(HaveOccurred())

			done := make(chan error, 1)

			go func() {
				defer GinkgoRecover()

				_, err := store.InstallationToken(987)
				done <- err
			}()

			// The bound is what matters, not its exact value; a store without
			// one never sends here at all
			Eventually(done, 5*time.Second).Should(Receive(HaveOccurred()))
		})

		It("should be safe to call concurrently", func() {
			store, err := githubapp.NewTokenStore("Iv1.test", githubtest.AppPrivateKey(), server.URL, 0)
			Expect(err).NotTo(HaveOccurred())

			var wg sync.WaitGroup

			for i := range 20 {
				wg.Add(1)

				go func() {
					defer wg.Done()
					defer GinkgoRecover()

					_, err := store.InstallationToken(int64(i%4) + 1)
					Expect(err).NotTo(HaveOccurred())
				}()
			}

			wg.Wait()

			Expect(github.paths()).To(HaveLen(4))
		})
	})
})
