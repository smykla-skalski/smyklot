package github_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/githubtest"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

var _ = Describe("GitHub App Client [Unit]", func() {
	var server *httptest.Server

	AfterEach(func() {
		if server != nil {
			server.Close()
			server = nil
		}
	})

	Describe("NewAppClient", func() {
		// GitHub rejects "token <jwt>" on app-level endpoints, so the scheme
		// is the whole point of this constructor
		It("should authenticate with the Bearer scheme", func() {
			var gotAuth string

			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAuth = r.Header.Get("Authorization")
				_, _ = w.Write([]byte(`[]`))
			}))

			client, err := github.NewAppClient("test-jwt", server.URL)
			Expect(err).NotTo(HaveOccurred())

			_, err = client.ListInstallations(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(gotAuth).To(Equal("Bearer test-jwt"))
		})

		It("should reject an empty JWT", func() {
			_, err := github.NewAppClient("", "")
			Expect(err).To(MatchError(github.ErrEmptyToken))
		})
	})

	Describe("ListInstallations", func() {
		It("should return every installation", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal("/app/installations"))
				Expect(r.URL.Query().Get("per_page")).To(Equal("100"))

				_, _ = w.Write([]byte(`[
					{"id": 111, "account": {"id": 7, "login": "smykla-skalski", "type": "Organization", "avatar_url": "https://avatars.example/7"}},
					{"id": 222, "account": {"id": 8, "login": "someone-else", "type": "User", "avatar_url": "https://avatars.example/8"}}
				]`))
			}))

			client, err := github.NewAppClient("test-jwt", server.URL)
			Expect(err).NotTo(HaveOccurred())

			installations, err := client.ListInstallations(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(installations).To(Equal([]github.Installation{
				{ID: 111, AccountID: 7, Account: "smykla-skalski", AccountType: "Organization", AvatarURL: "https://avatars.example/7"},
				{ID: 222, AccountID: 8, Account: "someone-else", AccountType: "User", AvatarURL: "https://avatars.example/8"},
			}))
		})

		// A suspended installation cannot mint a token, so polling it would
		// only produce errors every sweep
		It("should skip a suspended installation", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`[
					{"id": 111, "account": {"login": "active"}},
					{"id": 222, "account": {"login": "suspended"}, "suspended_at": "2026-01-01T00:00:00Z"}
				]`))
			}))

			client, err := github.NewAppClient("test-jwt", server.URL)
			Expect(err).NotTo(HaveOccurred())

			installations, err := client.ListInstallations(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(installations).To(Equal([]github.Installation{{ID: 111, Account: "active"}}))
		})

		It("should follow pages until a short one", func() {
			var requestedPages []string

			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				page := r.URL.Query().Get("page")
				requestedPages = append(requestedPages, page)

				if page == "1" {
					full := make([]string, 0, 100)
					for i := range 100 {
						full = append(full, fmt.Sprintf(`{"id": %d, "account": {"login": "org%d"}}`, i+1, i+1))
					}
					_, _ = w.Write([]byte("[" + strings.Join(full, ",") + "]"))

					return
				}

				_, _ = w.Write([]byte(`[{"id": 999, "account": {"login": "last"}}]`))
			}))

			client, err := github.NewAppClient("test-jwt", server.URL)
			Expect(err).NotTo(HaveOccurred())

			installations, err := client.ListInstallations(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(requestedPages).To(Equal([]string{"1", "2"}))
			Expect(installations).To(HaveLen(101))
			Expect(installations[100]).To(Equal(github.Installation{ID: 999, Account: "last"}))
		})

		It("should not truncate more than ten thousand installations", func() {
			const installationCount = 10_001
			requestedPages := 0

			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestedPages++
				page := 0
				_, err := fmt.Sscanf(r.URL.Query().Get("page"), "%d", &page)
				Expect(err).NotTo(HaveOccurred())
				start := (page - 1) * 100
				end := min(start+100, installationCount)
				items := make([]map[string]any, 0, end-start)
				for index := start; index < end; index++ {
					items = append(items, map[string]any{
						"id":      index + 1,
						"account": map[string]any{"login": fmt.Sprintf("org-%d", index+1)},
					})
				}
				Expect(json.NewEncoder(w).Encode(items)).To(Succeed())
			}))

			client, err := github.NewAppClient("test-jwt", server.URL)
			Expect(err).NotTo(HaveOccurred())

			installations, err := client.ListInstallations(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(installations).To(HaveLen(installationCount))
			Expect(requestedPages).To(Equal(101))
		})

		It("should return an error when the API fails", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"message": "Bad credentials"}`))
			}))

			client, err := github.NewAppClient("test-jwt", server.URL)
			Expect(err).NotTo(HaveOccurred())

			_, err = client.ListInstallations(context.Background())
			Expect(err).To(MatchError(ContainSubstring("Bad credentials")))
		})
	})

	Describe("ListInstallationRepos", func() {
		// This endpoint wraps its results in an object rather than returning a
		// bare array, unlike every other list this client reads
		It("should unwrap the repositories field", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal("/installation/repositories"))
				Expect(r.Header.Get("Authorization")).To(Equal("token install-token"))

				_, _ = w.Write([]byte(`{
					"total_count": 2,
					"repositories": [
						{"id": 31, "name": "smyklot", "full_name": "smykla-skalski/smyklot", "private": true, "default_branch": "main", "owner": {"login": "smykla-skalski"}},
						{"id": 32, "name": "sai", "full_name": "smykla-skalski/sai", "private": false, "default_branch": "trunk", "owner": {"login": "smykla-skalski"}}
					]
				}`))
			}))

			client, err := github.NewClient("install-token", server.URL)
			Expect(err).NotTo(HaveOccurred())

			repos, err := client.ListInstallationRepos(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(repos).To(Equal([]github.Repository{
				{ID: 31, Owner: "smykla-skalski", Name: "smyklot", FullName: "smykla-skalski/smyklot", Private: true, DefaultBranch: "main"},
				{ID: 32, Owner: "smykla-skalski", Name: "sai", FullName: "smykla-skalski/sai", DefaultBranch: "trunk"},
			}))
		})

		It("should return nothing when the installation has no repositories", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"total_count": 0, "repositories": []}`))
			}))

			client, err := github.NewClient("install-token", server.URL)
			Expect(err).NotTo(HaveOccurred())

			repos, err := client.ListInstallationRepos(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(repos).To(BeEmpty())
		})

		It("should not truncate more than ten thousand repositories", func() {
			const repositoryCount = 10_001
			requestedPages := 0

			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestedPages++
				page := 0
				_, err := fmt.Sscanf(r.URL.Query().Get("page"), "%d", &page)
				Expect(err).NotTo(HaveOccurred())
				start := (page - 1) * 100
				end := min(start+100, repositoryCount)
				items := make([]map[string]any, 0, end-start)
				for index := start; index < end; index++ {
					name := fmt.Sprintf("repo-%d", index+1)
					items = append(items, map[string]any{
						"id":        index + 1,
						"name":      name,
						"full_name": "smykla-skalski/" + name,
						"owner":     map[string]any{"login": "smykla-skalski"},
					})
				}
				Expect(json.NewEncoder(w).Encode(map[string]any{
					"total_count":  repositoryCount,
					"repositories": items,
				})).To(Succeed())
			}))

			client, err := github.NewClient("install-token", server.URL)
			Expect(err).NotTo(HaveOccurred())

			repositories, err := client.ListInstallationRepos(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(repositories).To(HaveLen(repositoryCount))
			Expect(requestedPages).To(Equal(101))
		})

		It("should reject a repository snapshot GitHub reports as incomplete", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"total_count": 1, "repositories": []}`))
			}))

			client, err := github.NewClient("install-token", server.URL)
			Expect(err).NotTo(HaveOccurred())

			_, err = client.ListInstallationRepos(context.Background())
			Expect(err).To(MatchError(ContainSubstring("incomplete GitHub API pagination")))
		})
	})

	Describe("ListOrganizationAdmins", func() {
		It("requests only admins and follows every page", func() {
			var requestedPages []string
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal("/orgs/smykla-skalski/members"))
				Expect(r.URL.Query().Get("role")).To(Equal("admin"))
				Expect(r.URL.Query().Get("per_page")).To(Equal("100"))
				page := r.URL.Query().Get("page")
				requestedPages = append(requestedPages, page)
				if page == "1" {
					items := make([]map[string]any, 100)
					for index := range items {
						items[index] = map[string]any{
							"id": index + 1, "login": fmt.Sprintf("admin-%d", index+1),
						}
					}
					Expect(json.NewEncoder(w).Encode(items)).To(Succeed())

					return
				}
				_, _ = w.Write([]byte(`[{"id": 101, "login": "last", "avatar_url": "https://avatars.example/101"}]`))
			}))

			client, err := github.NewClient("install-token", server.URL)
			Expect(err).NotTo(HaveOccurred())
			admins, err := client.ListOrganizationAdmins(context.Background(), "smykla-skalski")
			Expect(err).NotTo(HaveOccurred())
			Expect(requestedPages).To(Equal([]string{"1", "2"}))
			Expect(admins).To(HaveLen(101))
			avatarURL := "https://avatars.example/101"
			Expect(admins[100]).To(Equal(github.User{
				ID: 101, Login: "last", AvatarURL: &avatarURL,
			}))
		})

		It("preserves the permission response status", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"message":"Resource not accessible by integration"}`))
			}))
			client, err := github.NewClient("install-token", server.URL)
			Expect(err).NotTo(HaveOccurred())
			_, err = client.ListOrganizationAdmins(context.Background(), "smykla-skalski")
			var apiErr *github.APIError
			Expect(errors.As(err, &apiErr)).To(BeTrue())
			Expect(apiErr.StatusCode).To(Equal(http.StatusForbidden))
		})

		It("rejects an empty organization", func() {
			client, err := github.NewClient("install-token", "")
			Expect(err).NotTo(HaveOccurred())
			_, err = client.ListOrganizationAdmins(context.Background(), " ")
			Expect(err).To(MatchError("GitHub organization must not be empty"))
		})
	})

	Describe("GetRepoConfig", func() {
		It("should decode the canonical .yaml file", func() {
			var requestedPaths []string

			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestedPaths = append(requestedPaths, r.URL.Path)
				_, _ = w.Write([]byte(githubtest.ContentsResponse("quiet_success: true\n")))
			}))

			client, err := github.NewClient("test-token", server.URL)
			Expect(err).NotTo(HaveOccurred())

			content, err := client.GetRepoConfig(context.Background(), "owner", "repo")
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(Equal("quiet_success: true\n"))
			Expect(requestedPaths).To(Equal([]string{"/repos/owner/repo/contents/.github/smyklot.yaml"}))
		})

		// Most repositories have no config file, so the miss must not cost more
		// than one request - a second spelling would double it every read
		It("should spend one request on a repository without the file", func() {
			var requestedPaths []string

			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestedPaths = append(requestedPaths, r.URL.Path)

				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"message": "Not Found"}`))
			}))

			client, err := github.NewClient("test-token", server.URL)
			Expect(err).NotTo(HaveOccurred())

			content, err := client.GetRepoConfig(context.Background(), "owner", "repo")
			Expect(err).NotTo(HaveOccurred())
			Expect(content).To(BeNil())
			Expect(requestedPaths).To(Equal([]string{"/repos/owner/repo/contents/.github/smyklot.yaml"}))
		})

		It("should reject a file above the size cap", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(githubtest.ContentsResponse(strings.Repeat("a", 64*1024+1))))
			}))

			client, err := github.NewClient("test-token", server.URL)
			Expect(err).NotTo(HaveOccurred())

			_, err = client.GetRepoConfig(context.Background(), "owner", "repo")
			Expect(err).To(MatchError(ContainSubstring("too large")))
		})

		It("should surface a non-404 API error", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"message": "Resource not accessible by integration"}`))
			}))

			client, err := github.NewClient("test-token", server.URL)
			Expect(err).NotTo(HaveOccurred())

			_, err = client.GetRepoConfig(context.Background(), "owner", "repo")
			Expect(err).To(MatchError(ContainSubstring("Resource not accessible")))
		})
	})

	// GetCodeowners shares its body with GetRepoConfig, so its contract is
	// re-checked here rather than assumed
	Describe("GetCodeowners", func() {
		It("should decode the file", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal("/repos/owner/repo/contents/.github/CODEOWNERS"))
				_, _ = w.Write([]byte(githubtest.ContentsResponse("* @bartsmykla\n")))
			}))

			client, err := github.NewClient("test-token", server.URL)
			Expect(err).NotTo(HaveOccurred())

			content, err := client.GetCodeowners(context.Background(), "owner", "repo")
			Expect(err).NotTo(HaveOccurred())
			Expect(content).To(Equal("* @bartsmykla\n"))
		})

		It("should return an empty string when the file is absent", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"message": "Not Found"}`))
			}))

			client, err := github.NewClient("test-token", server.URL)
			Expect(err).NotTo(HaveOccurred())

			content, err := client.GetCodeowners(context.Background(), "owner", "repo")
			Expect(err).NotTo(HaveOccurred())
			Expect(content).To(BeEmpty())
		})
	})

	Describe("response parsing", func() {
		It("should report malformed JSON", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`not json`))
			}))

			client, err := github.NewAppClient("test-jwt", server.URL)
			Expect(err).NotTo(HaveOccurred())

			_, err = client.ListInstallations(context.Background())
			Expect(err).To(MatchError(github.ErrResponseParse))
		})

		It("should report contents without a content field", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				payload, _ := json.Marshal(map[string]string{"encoding": "base64"})
				_, _ = w.Write(payload)
			}))

			client, err := github.NewClient("test-token", server.URL)
			Expect(err).NotTo(HaveOccurred())

			_, err = client.GetRepoConfig(context.Background(), "owner", "repo")
			Expect(err).To(MatchError(ContainSubstring("no content field")))
		})
	})

	Describe("Ping", func() {
		// GitHub answers /rate_limit 401 for an App JWT, whatever the key, so a
		// probe pointed there can never report ready. Only /app accepts the
		// credentials this client carries
		It("should ask the one endpoint an App JWT is accepted on", func() {
			var gotPath, gotAuth string

			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotAuth = r.Header.Get("Authorization")

				if r.URL.Path != "/app" {
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = w.Write([]byte(`{"message": "Bad credentials"}`))

					return
				}

				_, _ = w.Write([]byte(`{"id": 1197525, "slug": "smyklot"}`))
			}))

			client, err := github.NewAppClient("test-jwt", server.URL)
			Expect(err).NotTo(HaveOccurred())

			Expect(client.Ping(context.Background())).To(Succeed())
			Expect(gotPath).To(Equal("/app"))
			Expect(gotAuth).To(Equal("Bearer test-jwt"))
		})

		It("should fail when the credentials are rejected", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"message": "Bad credentials"}`))
			}))

			client, err := github.NewAppClient("stale-jwt", server.URL)
			Expect(err).NotTo(HaveOccurred())

			Expect(client.Ping(context.Background())).ToNot(Succeed())
		})

		// The retry every other call gets would make a probe wait through the
		// backoff before reporting what it already knows
		It("should not retry a server error", func() {
			var attempts int

			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				attempts++

				w.WriteHeader(http.StatusInternalServerError)
			}))

			client, err := github.NewAppClient("test-jwt", server.URL)
			Expect(err).NotTo(HaveOccurred())

			Expect(client.Ping(context.Background())).ToNot(Succeed())
			Expect(attempts).To(Equal(1))
		})
	})
})
