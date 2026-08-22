package github_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

var _ = Describe("GitHub Client [Unit]", func() {
	var server *httptest.Server

	BeforeEach(func() {
		// Server will be set up in each test
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	Describe("NewClient", func() {
		It("should create a new client with token", func() {
			client, err := github.NewClient("test-token", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
		})

		It("should create a new client with custom base URL", func() {
			client, err := github.NewClient("test-token", "https://api.github.com")
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
		})

		It("should return error for empty token", func() {
			_, err := github.NewClient("", "")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(MatchRegexp(`(?i)empty.*token`))
		})
	})

	Describe("GetUser", func() {
		It("resolves the canonical login and stable identity", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Method).To(Equal(http.MethodGet))
				Expect(r.URL.Path).To(Equal("/users/SomeUser"))
				Expect(r.Header.Get("Authorization")).To(Equal("token test-token"))
				name := "Some User"
				avatar := "https://avatars.example/42"
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id": 42, "login": "someuser", "name": name, "avatar_url": avatar,
				})
			}))
			client, err := github.NewClient("test-token", server.URL)
			Expect(err).NotTo(HaveOccurred())

			user, err := client.GetUser(context.Background(), " SomeUser ")
			Expect(err).NotTo(HaveOccurred())
			Expect(user.ID).To(Equal(int64(42)))
			Expect(user.Login).To(Equal("someuser"))
			Expect(user.Name).To(HaveValue(Equal("Some User")))
			Expect(user.AvatarURL).To(HaveValue(Equal("https://avatars.example/42")))
		})
	})

	Describe("AddReaction", func() {
		Context("when adding reaction to a comment", func() {
			It("should add success reaction", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.Method).To(Equal("POST"))
					Expect(r.URL.Path).To(Equal("/repos/owner/repo/issues/comments/123/reactions"))
					Expect(r.Header.Get("Authorization")).To(Equal("token test-token"))

					var body map[string]string
					err := json.NewDecoder(r.Body).Decode(&body)
					Expect(err).NotTo(HaveOccurred())
					Expect(body["content"]).To(Equal("+1"))

					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]interface{}{
						"id":      1,
						"content": "+1",
					})
				}))

				client, err := github.NewClient("test-token", server.URL)
				Expect(err).NotTo(HaveOccurred())

				err = client.AddReaction(context.Background(), "owner", "repo", 123, github.ReactionPlusOne)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should add error reaction", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var body map[string]string
					_ = json.NewDecoder(r.Body).Decode(&body)
					Expect(body["content"]).To(Equal("-1"))

					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]interface{}{
						"id":      1,
						"content": "-1",
					})
				}))

				client, err := github.NewClient("test-token", server.URL)
				Expect(err).NotTo(HaveOccurred())

				err = client.AddReaction(context.Background(), "owner", "repo", 456, github.ReactionMinusOne)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should handle API errors", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusUnauthorized)
					_ = json.NewEncoder(w).Encode(map[string]string{
						"message": "Bad credentials",
					})
				}))

				client, err := github.NewClient("test-token", server.URL)
				Expect(err).NotTo(HaveOccurred())

				err = client.AddReaction(context.Background(), "owner", "repo", 123, github.ReactionPlusOne)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("401"))
			})
		})
	})

	Describe("RemoveReactionByUser", func() {
		It("preserves matching reactions from other users", func() {
			var deleted []string
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					Expect(r.URL.Path).To(Equal("/repos/owner/repo/issues/comments/123/reactions"))
					_ = json.NewEncoder(w).Encode([]map[string]any{
						{"id": 1, "content": "eyes", "user": map[string]any{"login": "smyklot[bot]"}},
						{"id": 2, "content": "eyes", "user": map[string]any{"login": "reviewer"}},
						{"id": 3, "content": "+1", "user": map[string]any{"login": "smyklot[bot]"}},
					})
				case http.MethodDelete:
					deleted = append(deleted, r.URL.Path)
					w.WriteHeader(http.StatusNoContent)
				default:
					Fail("unexpected request method: " + r.Method)
				}
			}))
			client, err := github.NewClient("test-token", server.URL)
			Expect(err).NotTo(HaveOccurred())

			err = client.RemoveReactionByUser(
				context.Background(), "owner", "repo", 123,
				github.ReactionEyes, "smyklot[bot]",
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(deleted).To(Equal([]string{
				"/repos/owner/repo/issues/comments/123/reactions/1",
			}))
		})
	})

	// GitHub answers thirty comments and the bot's own are the newest, so an
	// unpaginated read finds none of them on exactly the busy pull requests
	// where cleanup and the pending-CI reaction swap have work to do
	Describe("GetPRComments", func() {
		It("reads past the first page", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal("/repos/owner/repo/issues/7/comments"))

				if r.URL.Query().Get("page") == "2" {
					_ = json.NewEncoder(w).Encode([]map[string]any{
						{"id": 2, "body": "second", "user": map[string]any{"login": "smyklot[bot]"}},
					})

					return
				}

				w.Header().Set("Link",
					fmt.Sprintf(`<%s/repos/owner/repo/issues/7/comments?page=2>; rel="next"`, server.URL))
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{"id": 1, "body": "first", "user": map[string]any{"login": "reviewer"}},
				})
			}))
			client, err := github.NewClient("test-token", server.URL)
			Expect(err).NotTo(HaveOccurred())

			comments, err := client.GetPRComments(context.Background(), "owner", "repo", 7)
			Expect(err).NotTo(HaveOccurred())
			Expect(comments).To(HaveLen(2))
			Expect(comments[1].ID).To(Equal(int64(2)))
			Expect(comments[1].User.Login).To(Equal("smyklot[bot]"))
		})
	})

	Describe("PostComment", func() {
		Context("when posting a comment on a PR", func() {
			It("should post a comment successfully", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.Method).To(Equal("POST"))
					Expect(r.URL.Path).To(Equal("/repos/owner/repo/issues/1/comments"))
					Expect(r.Header.Get("Authorization")).To(Equal("token test-token"))

					var body map[string]string
					err := json.NewDecoder(r.Body).Decode(&body)
					Expect(err).NotTo(HaveOccurred())
					Expect(body["body"]).To(Equal("Test comment"))

					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]interface{}{
						"id":   123,
						"body": "Test comment",
					})
				}))

				client, err := github.NewClient("test-token", server.URL)
				Expect(err).NotTo(HaveOccurred())

				err = client.PostComment(context.Background(), "owner", "repo", 1, "Test comment")
				Expect(err).NotTo(HaveOccurred())
			})

			It("should handle empty comment body", func() {
				client, err := github.NewClient("test-token", "")
				Expect(err).NotTo(HaveOccurred())

				err = client.PostComment(context.Background(), "owner", "repo", 1, "")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(MatchRegexp(`(?i)empty.*comment`))
			})

			It("should handle API errors", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusForbidden)
					_ = json.NewEncoder(w).Encode(map[string]string{
						"message": "Forbidden",
					})
				}))

				client, err := github.NewClient("test-token", server.URL)
				Expect(err).NotTo(HaveOccurred())

				err = client.PostComment(context.Background(), "owner", "repo", 1, "Test comment")
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("ApprovePR", func() {
		Context("when approving a pull request", func() {
			It("should approve PR successfully", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.Method).To(Equal("POST"))
					Expect(r.URL.Path).To(Equal("/repos/owner/repo/pulls/1/reviews"))
					Expect(r.Header.Get("Authorization")).To(Equal("token test-token"))

					var body map[string]string
					err := json.NewDecoder(r.Body).Decode(&body)
					Expect(err).NotTo(HaveOccurred())
					Expect(body["event"]).To(Equal("APPROVE"))

					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(map[string]interface{}{
						"id":    1,
						"state": "APPROVED",
					})
				}))

				client, err := github.NewClient("test-token", server.URL)
				Expect(err).NotTo(HaveOccurred())

				err = client.ApprovePR(context.Background(), "owner", "repo", 1)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should handle API errors", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusUnprocessableEntity)
					_ = json.NewEncoder(w).Encode(map[string]string{
						"message": "Pull request already approved",
					})
				}))

				client, err := github.NewClient("test-token", server.URL)
				Expect(err).NotTo(HaveOccurred())

				err = client.ApprovePR(context.Background(), "owner", "repo", 1)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("MergePR", func() {
		Context("when merging a pull request", func() {
			It("should merge PR successfully", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.Method).To(Equal("PUT"))
					Expect(r.URL.Path).To(Equal("/repos/owner/repo/pulls/1/merge"))
					Expect(r.Header.Get("Authorization")).To(Equal("token test-token"))

					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(map[string]interface{}{
						"sha":     "abc123",
						"merged":  true,
						"message": "Pull Request successfully merged",
					})
				}))

				client, err := github.NewClient("test-token", server.URL)
				Expect(err).NotTo(HaveOccurred())

				err = client.MergePR(context.Background(), "owner", "repo", 1, github.MergeMethodMerge)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should handle merge conflicts", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusConflict)
					_ = json.NewEncoder(w).Encode(map[string]string{
						"message": "Merge conflict",
					})
				}))

				client, err := github.NewClient("test-token", server.URL)
				Expect(err).NotTo(HaveOccurred())

				err = client.MergePR(context.Background(), "owner", "repo", 1, github.MergeMethodMerge)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("409"))
			})

			It("should handle PR not mergeable", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusMethodNotAllowed)
					_ = json.NewEncoder(w).Encode(map[string]string{
						"message": "Pull Request is not mergeable",
					})
				}))

				client, err := github.NewClient("test-token", server.URL)
				Expect(err).NotTo(HaveOccurred())

				err = client.MergePR(context.Background(), "owner", "repo", 1, github.MergeMethodMerge)
				Expect(err).To(HaveOccurred())
			})

			It("should reject a successful response that did not merge", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(map[string]interface{}{
						"merged":  false,
						"message": "Head branch was modified",
					})
				}))

				client, err := github.NewClient("test-token", server.URL)
				Expect(err).NotTo(HaveOccurred())
				err = client.MergePR(context.Background(), "owner", "repo", 1, github.MergeMethodMerge)
				Expect(err).To(MatchError(ContainSubstring("Head branch was modified")))
			})

			It("should send the expected head SHA for a checked merge", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var body map[string]interface{}
					Expect(json.NewDecoder(r.Body).Decode(&body)).To(Succeed())
					Expect(body).To(Equal(map[string]interface{}{
						"merge_method": "squash",
						"sha":          "abc123",
					}))
					_ = json.NewEncoder(w).Encode(map[string]interface{}{"merged": true})
				}))

				client, err := github.NewClient("test-token", server.URL)
				Expect(err).NotTo(HaveOccurred())
				err = client.MergePRAtHead(
					context.Background(), "owner", "repo", 1, github.MergeMethodSquash, "abc123",
				)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should not replay an exact-head merge after a server error", func() {
				attempts := 0
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					attempts++
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"message":"server error"}`))
				}))

				client, err := github.NewClient("test-token", server.URL)
				Expect(err).NotTo(HaveOccurred())
				err = client.MergePRAtHead(
					context.Background(), "owner", "repo", 1, github.MergeMethodSquash, "abc123",
				)
				Expect(err).To(HaveOccurred())
				Expect(attempts).To(Equal(1))
			})
		})
	})

	Describe("GetPRInfo", func() {
		Context("when getting PR information", func() {
			It("should get PR info successfully", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					defer GinkgoRecover()

					Expect(r.Method).To(Equal("GET"))
					Expect(r.Header.Get("Authorization")).To(Equal("token test-token"))

					// Handle both PR info and reviews requests
					switch r.URL.Path {
					case "/repos/owner/repo/pulls/1":
						w.WriteHeader(http.StatusOK)
						_ = json.NewEncoder(w).Encode(map[string]interface{}{
							"number":    1,
							"state":     "open",
							"mergeable": true,
							"title":     "Test PR",
							"body":      "Test description",
							"user": map[string]interface{}{
								"login": "testuser",
							},
						})
					case "/repos/owner/repo/pulls/1/reviews":
						w.WriteHeader(http.StatusOK)
						_ = json.NewEncoder(w).Encode([]map[string]interface{}{
							{
								"state": "APPROVED",
								"user": map[string]interface{}{
									"login": "reviewer1",
								},
							},
						})
					default:
						Fail("unexpected request path: " + r.URL.Path)
					}
				}))

				client, err := github.NewClient("test-token", server.URL)
				Expect(err).NotTo(HaveOccurred())

				info, err := client.GetPRInfo(context.Background(), "owner", "repo", 1)
				Expect(err).NotTo(HaveOccurred())
				Expect(info).NotTo(BeNil())
				Expect(info.Number).To(Equal(1))
				Expect(info.State).To(Equal("open"))
				Expect(info.Mergeable).To(BeTrue())
				Expect(info.Title).To(Equal("Test PR"))
				Expect(info.Author).To(Equal("testuser"))
				Expect(info.ApprovedBy).To(ConsistOf("reviewer1"))
			})

			It("should handle PR not found", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound)
					_ = json.NewEncoder(w).Encode(map[string]string{
						"message": "Not Found",
					})
				}))

				client, err := github.NewClient("test-token", server.URL)
				Expect(err).NotTo(HaveOccurred())

				_, err = client.GetPRInfo(context.Background(), "owner", "repo", 999)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("404"))
			})

			It("should parse mergeable_state field", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					defer GinkgoRecover()

					Expect(r.Method).To(Equal("GET"))

					switch r.URL.Path {
					case "/repos/owner/repo/pulls/1":
						w.WriteHeader(http.StatusOK)
						_ = json.NewEncoder(w).Encode(map[string]interface{}{
							"number":          1,
							"state":           "open",
							"mergeable":       false,
							"mergeable_state": "blocked",
							"title":           "Test PR",
							"user": map[string]interface{}{
								"login": "testuser",
							},
						})
					case "/repos/owner/repo/pulls/1/reviews":
						w.WriteHeader(http.StatusOK)
						_ = json.NewEncoder(w).Encode([]map[string]interface{}{})
					default:
						Fail("unexpected request path: " + r.URL.Path)
					}
				}))

				client, err := github.NewClient("test-token", server.URL)
				Expect(err).NotTo(HaveOccurred())

				info, err := client.GetPRInfo(context.Background(), "owner", "repo", 1)
				Expect(err).NotTo(HaveOccurred())
				Expect(info.Mergeable).To(BeFalse())
				Expect(info.MergeableState).To(Equal(github.MergeableStateBlocked))
			})

			It("should handle all mergeable_state values", func() {
				testCases := []struct {
					state    string
					expected github.MergeableState
				}{
					{"clean", github.MergeableStateClean},
					{"dirty", github.MergeableStateDirty},
					{"blocked", github.MergeableStateBlocked},
					{"unstable", github.MergeableStateUnstable},
					{"unknown", github.MergeableStateUnknown},
				}

				for _, tc := range testCases {
					server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						switch r.URL.Path {
						case "/repos/owner/repo/pulls/1":
							w.WriteHeader(http.StatusOK)
							_ = json.NewEncoder(w).Encode(map[string]interface{}{
								"number":          1,
								"mergeable_state": tc.state,
							})
						case "/repos/owner/repo/pulls/1/reviews":
							w.WriteHeader(http.StatusOK)
							_ = json.NewEncoder(w).Encode([]map[string]interface{}{})
						}
					}))

					client, err := github.NewClient("test-token", server.URL)
					Expect(err).NotTo(HaveOccurred())

					info, err := client.GetPRInfo(context.Background(), "owner", "repo", 1)
					Expect(err).NotTo(HaveOccurred())
					Expect(info.MergeableState).To(Equal(tc.expected), "failed for state: "+tc.state)

					server.Close()
				}
			})
		})
	})

	Describe("IsTeamMember", func() {
		Context("when checking team membership", func() {
			It("should return true when user is a team member", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.Method).To(Equal("GET"))
					Expect(r.URL.Path).To(Equal("/orgs/test-org/teams/test-team/memberships/testuser"))
					Expect(r.Header.Get("Authorization")).To(Equal("token test-token"))

					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(map[string]interface{}{
						"state": "active",
						"role":  "member",
					})
				}))

				client, err := github.NewClient("test-token", server.URL)
				Expect(err).NotTo(HaveOccurred())

				isMember, err := client.IsTeamMember(context.Background(), "test-org", "test-team", "testuser")
				Expect(err).NotTo(HaveOccurred())
				Expect(isMember).To(BeTrue())
			})

			It("should return false when user is not a team member", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound)
					_ = json.NewEncoder(w).Encode(map[string]string{
						"message": "Not Found",
					})
				}))

				client, err := github.NewClient("test-token", server.URL)
				Expect(err).NotTo(HaveOccurred())

				isMember, err := client.IsTeamMember(context.Background(), "test-org", "test-team", "nonmember")
				Expect(err).NotTo(HaveOccurred())
				Expect(isMember).To(BeFalse())
			})

			It("should return false when team membership is pending", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(map[string]interface{}{
						"state": "pending",
						"role":  "member",
					})
				}))

				client, err := github.NewClient("test-token", server.URL)
				Expect(err).NotTo(HaveOccurred())

				isMember, err := client.IsTeamMember(context.Background(), "test-org", "test-team", "pendinguser")
				Expect(err).NotTo(HaveOccurred())
				Expect(isMember).To(BeFalse())
			})

			It("should handle API errors", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusForbidden)
					_ = json.NewEncoder(w).Encode(map[string]string{
						"message": "Forbidden",
					})
				}))

				client, err := github.NewClient("test-token", server.URL)
				Expect(err).NotTo(HaveOccurred())

				_, err = client.IsTeamMember(context.Background(), "test-org", "test-team", "testuser")
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("GetPRHeadRef", func() {
		Context("when getting PR head ref", func() {
			It("should return head SHA successfully", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.Method).To(Equal("GET"))
					Expect(r.URL.Path).To(Equal("/repos/owner/repo/pulls/42"))
					Expect(r.Header.Get("Authorization")).To(Equal("token test-token"))

					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(map[string]interface{}{
						"head": map[string]interface{}{
							"sha": "abc123def456",
						},
					})
				}))

				client, err := github.NewClient("test-token", server.URL)
				Expect(err).NotTo(HaveOccurred())

				sha, err := client.GetPRHeadRef(context.Background(), "owner", "repo", 42)
				Expect(err).NotTo(HaveOccurred())
				Expect(sha).To(Equal("abc123def456"))
			})

			It("should handle PR not found", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound)
					_ = json.NewEncoder(w).Encode(map[string]string{
						"message": "Not Found",
					})
				}))

				client, err := github.NewClient("test-token", server.URL)
				Expect(err).NotTo(HaveOccurred())

				_, err = client.GetPRHeadRef(context.Background(), "owner", "repo", 999)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("404"))
			})

			It("should handle missing head SHA in response", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(map[string]interface{}{
						"head": map[string]interface{}{},
					})
				}))

				client, err := github.NewClient("test-token", server.URL)
				Expect(err).NotTo(HaveOccurred())

				_, err = client.GetPRHeadRef(context.Background(), "owner", "repo", 42)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no head SHA"))
			})
		})
	})

	Describe("Error Handling", func() {
		Context("when handling various error conditions", func() {
			It("should handle network errors", func() {
				// An address nothing is listening on, rather than a hostname
				// nothing resolves: a name that does not exist costs seconds to
				// establish on a machine with mDNS, and resolves to a landing
				// page on any network running a wildcard DNS, which would make
				// this pass for the wrong reason.
				dead := httptest.NewServer(http.NotFoundHandler())
				address := dead.URL
				dead.Close()

				client, err := github.NewClient("test-token", address)
				Expect(err).NotTo(HaveOccurred())

				err = client.AddReaction(context.Background(), "owner", "repo", 1, github.ReactionPlusOne)
				Expect(err).To(HaveOccurred())
			})

			It("should handle malformed JSON responses", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("invalid json"))
				}))

				client, err := github.NewClient("test-token", server.URL)
				Expect(err).NotTo(HaveOccurred())

				_, err = client.GetPRInfo(context.Background(), "owner", "repo", 1)
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
