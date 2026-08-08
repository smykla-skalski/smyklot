package githubapp_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/pkg/githubapp"
)

// testPrivateKey is generated once for the suite - RSA key generation is slow
// enough that doing it per spec dominates the run
var testPrivateKey []byte

var _ = BeforeSuite(func() {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	testPrivateKey = pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
})

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
			_, err := githubapp.NewTokenStore("", testPrivateKey, "")
			Expect(err).To(MatchError(githubapp.ErrNoAppID))
		})

		It("should reject a missing private key", func() {
			_, err := githubapp.NewTokenStore("Iv1.test", nil, "")
			Expect(err).To(MatchError(githubapp.ErrNoPrivateKey))
		})

		It("should reject a private key that is not a key", func() {
			_, err := githubapp.NewTokenStore("Iv1.test", []byte("not a key"), "")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("AppToken", func() {
		It("should return a JWT for the App itself", func() {
			store, err := githubapp.NewTokenStore("Iv1.test", testPrivateKey, server.URL)
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
			store, err := githubapp.NewTokenStore("Iv1.test", testPrivateKey, server.URL)
			Expect(err).NotTo(HaveOccurred())

			token, err := store.InstallationToken(987)
			Expect(err).NotTo(HaveOccurred())
			Expect(token).To(Equal("token-for/app/installations/987/access_tokens"))
		})

		// One process serves many installations, so the store must key on the
		// installation rather than hold a single token
		It("should keep installations apart", func() {
			store, err := githubapp.NewTokenStore("Iv1.test", testPrivateKey, server.URL)
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
			store, err := githubapp.NewTokenStore("Iv1.test", testPrivateKey, server.URL)
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

			store, err := githubapp.NewTokenStore("Iv1.test", testPrivateKey, failing.URL)
			Expect(err).NotTo(HaveOccurred())

			_, err = store.InstallationToken(987)
			Expect(err).To(MatchError(ContainSubstring("404")))
		})

		It("should be safe to call concurrently", func() {
			store, err := githubapp.NewTokenStore("Iv1.test", testPrivateKey, server.URL)
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
