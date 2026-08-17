package github_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

var _ = Describe("Repository rulesets [Unit]", func() {
	var (
		server   *httptest.Server
		mu       sync.Mutex
		requests []*http.Request
		bodies   []string
	)

	BeforeEach(func() {
		requests = nil
		bodies = nil
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	record := func(handler http.HandlerFunc) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)

			mu.Lock()
			requests = append(requests, r)
			bodies = append(bodies, string(body))
			mu.Unlock()

			handler(w, r)
		}))
	}

	client := func() *github.Client {
		GinkgoHelper()

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		return client
	}

	// sent reads back what a write put on the wire, which for these methods is
	// the whole of what they did.
	sent := func(index int) map[string]any {
		GinkgoHelper()

		var body map[string]any
		Expect(json.Unmarshal([]byte(bodies[index]), &body)).To(Succeed())

		return body
	}

	// rule finds one entry of GitHub's rules array by its type. The array is
	// the wire shape and the struct is not, so every write spec has to look at
	// it the way GitHub does.
	rule := func(body map[string]any, kind string) map[string]any {
		GinkgoHelper()

		rules, ok := body["rules"].([]any)
		Expect(ok).To(BeTrue(), "the body carries no rules array")

		for _, entry := range rules {
			found, ok := entry.(map[string]any)
			Expect(ok).To(BeTrue())

			if found["type"] == kind {
				return found
			}
		}

		return nil
	}

	Describe("listing", func() {
		It("reads a ruleset's identity and the level it is defined at", func() {
			server = record(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, `[
					{"id":1,"name":"main","target":"branch",
					 "enforcement":"active","source_type":"Repository"},
					{"id":2,"name":"org-wide","target":"branch",
					 "enforcement":"evaluate","source_type":"Organization"}
				]`)
			})

			rulesets, err := client().ListRepositoryRulesets(
				context.Background(), "acme", "web")
			Expect(err).NotTo(HaveOccurred())

			Expect(rulesets).To(Equal([]github.RulesetSummary{
				{
					ID: 1, Name: "main", Target: "branch",
					Enforcement: "active", Source: github.RulesetSourceRepository,
				},
				{
					ID: 2, Name: "org-wide", Target: "branch",
					Enforcement: "evaluate", Source: github.RulesetSourceOrganization,
				},
			}))
		})

		// An inherited ruleset is not this repository's to write and not its to
		// delete, and a repository-level one created beside it is a second set
		// of rules enforced on the same refs. Without the parents in the answer
		// neither fact is knowable
		It("asks for the rulesets the repository inherits as well as its own", func() {
			server = record(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, `[]`)
			})

			_, err := client().ListRepositoryRulesets(context.Background(), "acme", "web")
			Expect(err).NotTo(HaveOccurred())

			Expect(requests[0].URL.Query().Get("includes_parents")).To(Equal("true"))
		})

		// The tool this replaces read one page. Past thirty rulesets it missed
		// entries, created a second ruleset with a name it already managed -
		// which GitHub permits - and from then on updated whichever came back
		// first
		It("reads past the first page", func() {
			server = record(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("page") == "2" {
					_, _ = io.WriteString(w, `[{"id":2,"name":"second-page"}]`)

					return
				}

				w.Header().Set("Link",
					fmt.Sprintf(`<%s/repos/acme/web/rulesets?page=2>; rel="next"`, server.URL))
				_, _ = io.WriteString(w, `[{"id":1,"name":"first-page"}]`)
			})

			rulesets, err := client().ListRepositoryRulesets(
				context.Background(), "acme", "web")
			Expect(err).NotTo(HaveOccurred())
			Expect(rulesets).To(HaveLen(2))
			Expect(rulesets[1].Name).To(Equal("second-page"))
		})

		// The source decides whether sync may write to a ruleset. Reading an
		// unlabelled one as inherited would leave a repository's own ruleset
		// unmanageable for ever with nothing on screen to say why
		It("reads a ruleset with no source as the repository's own", func() {
			server = record(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, `[{"id":1,"name":"main"}]`)
			})

			rulesets, err := client().ListRepositoryRulesets(
				context.Background(), "acme", "web")
			Expect(err).NotTo(HaveOccurred())
			Expect(rulesets[0].Source).To(Equal(github.RulesetSourceRepository))
			Expect(rulesets[0].Source.Inherited()).To(BeFalse())
		})
	})

	Describe("reading one whole", func() {
		// The listing carries identity and nothing else. Comparing
		// configuration against a summary would find every rule absent and
		// answer that every repository needs changing, for ever
		It("reads the rules, the conditions and the bypass actors", func() {
			server = record(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, `{
					"id": 7, "name": "main", "target": "branch",
					"enforcement": "active",
					"conditions": {"ref_name": {
						"include": ["refs/heads/main"], "exclude": ["refs/heads/tmp/*"]}},
					"bypass_actors": [
						{"actor_id": 5, "actor_type": "OrganizationAdmin",
						 "bypass_mode": "always"}],
					"rules": [
						{"type": "deletion"},
						{"type": "non_fast_forward"},
						{"type": "required_linear_history"},
						{"type": "required_signatures"},
						{"type": "creation"},
						{"type": "update",
						 "parameters": {"update_allows_fetch_and_merge": true}},
						{"type": "pull_request", "parameters": {
							"required_approving_review_count": 2,
							"dismiss_stale_reviews_on_push": true,
							"require_code_owner_review": true,
							"require_last_push_approval": true,
							"required_review_thread_resolution": true,
							"allowed_merge_methods": ["squash"]}},
						{"type": "required_status_checks", "parameters": {
							"strict_required_status_checks_policy": true,
							"do_not_enforce_on_create": true,
							"required_status_checks": [
								{"context": "build", "integration_id": 42}]}},
						{"type": "code_scanning", "parameters": {
							"code_scanning_tools": [{
								"tool": "CodeQL",
								"alerts_threshold": "errors",
								"security_alerts_threshold": "high_or_higher"}]}}
					]
				}`)
			})

			ruleset, err := client().GetRepositoryRuleset(
				context.Background(), "acme", "web", 7)
			Expect(err).NotTo(HaveOccurred())

			Expect(ruleset).To(Equal(github.RepositoryRuleset{
				Name: "main", Target: "branch", Enforcement: "active",
				Conditions: github.RulesetConditions{
					IncludeRefs: []string{"refs/heads/main"},
					ExcludeRefs: []string{"refs/heads/tmp/*"},
				},
				BypassActors: []github.RulesetBypassActor{{
					ActorID: 5, ActorType: "OrganizationAdmin", Mode: "always",
				}},
				Rules: github.RulesetRules{
					Creation:              true,
					Deletion:              true,
					NonFastForward:        true,
					RequiredLinearHistory: true,
					RequiredSignatures:    true,
					Update: &github.RulesetUpdateRule{
						AllowsFetchAndMerge: true,
					},
					PullRequest: &github.RulesetPullRequestRule{
						RequiredApprovingReviewCount:   2,
						DismissStaleReviewsOnPush:      true,
						RequireCodeOwnerReview:         true,
						RequireLastPushApproval:        true,
						RequiredReviewThreadResolution: true,
						AllowedMergeMethods:            []string{"squash"},
					},
					RequiredStatusChecks: &github.RulesetStatusChecksRule{
						Strict:               true,
						DoNotEnforceOnCreate: true,
						Checks: []github.RulesetStatusCheck{{
							Context: "build", IntegrationID: 42,
						}},
					},
					CodeScanning: &github.RulesetCodeScanningRule{
						Tools: []github.RulesetCodeScanningTool{{
							Tool:                    "CodeQL",
							AlertsThreshold:         "errors",
							SecurityAlertsThreshold: "high_or_higher",
						}},
					},
				},
			}))
		})

		// A rule GitHub does not name is a rule that is not enforced. Reading
		// an absent one as anything else would make an already-matching
		// repository look like it needed changing on every tick
		It("reads a ruleset that enforces nothing", func() {
			server = record(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w,
					`{"id":7,"name":"quiet","target":"branch",
					  "enforcement":"disabled","rules":[]}`)
			})

			ruleset, err := client().GetRepositoryRuleset(
				context.Background(), "acme", "web", 7)
			Expect(err).NotTo(HaveOccurred())

			Expect(ruleset.Rules).To(Equal(github.RulesetRules{}))
			Expect(ruleset.Conditions).To(Equal(github.RulesetConditions{}))
		})

		// A ruleset is replaced whole, so a rule this version has no field for
		// is a rule a replacement removes. Without reading it, the plan
		// somebody approves cannot mention what it destroys - and GitHub adds
		// rule types faster than this will follow them
		It("names the rules it is enforcing that this cannot express", func() {
			server = record(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, `{
					"id": 7, "name": "main", "target": "branch",
					"enforcement": "active",
					"rules": [
						{"type": "deletion"},
						{"type": "merge_queue", "parameters": {
							"check_response_timeout_minutes": 60,
							"grouping_strategy": "ALLGREEN",
							"max_entries_to_build": 5,
							"max_entries_to_merge": 5,
							"merge_method": "SQUASH",
							"min_entries_to_merge": 1,
							"min_entries_to_merge_wait_minutes": 5}},
						{"type": "commit_message_pattern", "parameters": {
							"operator": "starts_with", "pattern": "feat"}}
					]
				}`)
			})

			ruleset, err := client().GetRepositoryRuleset(
				context.Background(), "acme", "web", 7)
			Expect(err).NotTo(HaveOccurred())

			// In a fixed order, because a plan of the same state has to read
			// the same way twice
			Expect(ruleset.OtherRules).To(Equal([]string{
				"merge_queue", "commit_message_pattern",
			}))

			// And what it does model is still read
			Expect(ruleset.Rules.Deletion).To(BeTrue())
		})

		// Not a rule of its own but a parameter of one that is modelled, and
		// dropped by a replacement exactly the same way
		It("names required reviewers it cannot express", func() {
			server = record(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, `{
					"id": 7, "name": "main", "target": "branch",
					"enforcement": "active",
					"rules": [{"type": "pull_request", "parameters": {
						"required_approving_review_count": 1,
						"required_reviewers": [
							{"file_patterns": ["*.go"], "minimum_approvals": 1,
							 "reviewer": {"id": 3, "type": "Team"}}]}}]
				}`)
			})

			ruleset, err := client().GetRepositoryRuleset(
				context.Background(), "acme", "web", 7)
			Expect(err).NotTo(HaveOccurred())

			Expect(ruleset.OtherRules).To(Equal([]string{"pull_request.required_reviewers"}))
		})

		// The ordinary case, and the one that keeps the plan quiet about it
		It("names nothing where it can express everything", func() {
			server = record(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w,
					`{"id":7,"name":"main","target":"branch","enforcement":"active",
					  "rules":[{"type":"deletion"},{"type":"non_fast_forward"}]}`)
			})

			ruleset, err := client().GetRepositoryRuleset(
				context.Background(), "acme", "web", 7)
			Expect(err).NotTo(HaveOccurred())

			Expect(ruleset.OtherRules).To(BeEmpty())
		})

		It("reports a ruleset it cannot read", func() {
			server = record(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"message":"Not Found"}`)
			})

			_, err := client().GetRepositoryRuleset(context.Background(), "acme", "web", 7)
			Expect(err).To(MatchError(github.ErrAPIRequest))
		})
	})

	Describe("creating", func() {
		It("sends the whole ruleset in the shape GitHub takes", func() {
			server = record(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusCreated)
				_, _ = io.WriteString(w, `{"id":7}`)
			})

			Expect(client().CreateRepositoryRuleset(
				context.Background(), "acme", "web", github.RepositoryRuleset{
					Name: "main", Target: "branch", Enforcement: "active",
					Conditions: github.RulesetConditions{
						IncludeRefs: []string{"refs/heads/main"},
					},
					BypassActors: []github.RulesetBypassActor{{
						ActorID: 1197525, ActorType: "Integration", Mode: "always",
					}},
					Rules: github.RulesetRules{
						Deletion: true,
						PullRequest: &github.RulesetPullRequestRule{
							RequiredApprovingReviewCount: 1,
							AllowedMergeMethods:          []string{"squash"},
						},
					},
				})).To(Succeed())

			Expect(requests[0].Method).To(Equal(http.MethodPost))
			Expect(requests[0].URL.Path).To(Equal("/repos/acme/web/rulesets"))

			body := sent(0)
			Expect(body).To(HaveKeyWithValue("name", "main"))
			Expect(body).To(HaveKeyWithValue("target", "branch"))
			Expect(body).To(HaveKeyWithValue("enforcement", "active"))

			Expect(rule(body, "deletion")).NotTo(BeNil())
			Expect(rule(body, "pull_request")).To(HaveKeyWithValue("parameters",
				HaveKeyWithValue("required_approving_review_count", float64(1))))

			// The one nobody asked for. A rule the configuration does not name
			// is a rule that must not be enforced, and sending it would enforce
			// something nobody reviewed
			Expect(rule(body, "required_signatures")).To(BeNil())
		})

		// GitHub refuses a null where it expects a list, and a ruleset that
		// applies to every ref is spelled with empty lists rather than by
		// leaving the object out. The tool this replaces got this right for
		// include and exclude and wrong for status-check contexts
		It("never sends a null where GitHub wants a list", func() {
			server = record(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusCreated)
				_, _ = io.WriteString(w, `{"id":7}`)
			})

			Expect(client().CreateRepositoryRuleset(
				context.Background(), "acme", "web", github.RepositoryRuleset{
					Name: "everything", Target: "branch", Enforcement: "active",
				})).To(Succeed())

			body := sent(0)
			Expect(body["conditions"]).To(HaveKeyWithValue("ref_name", And(
				HaveKeyWithValue("include", BeEmpty()),
				HaveKeyWithValue("exclude", BeEmpty()),
			)))
			Expect(body["bypass_actors"]).To(BeEmpty())
			Expect(body["bypass_actors"]).NotTo(BeNil())
		})

		// Parsed by the tool this replaces and then never read, so a branch
		// created from a protected one could not exist until checks nobody had
		// run had passed
		It("sends do_not_enforce_on_create", func() {
			server = record(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusCreated)
				_, _ = io.WriteString(w, `{"id":7}`)
			})

			Expect(client().CreateRepositoryRuleset(
				context.Background(), "acme", "web", github.RepositoryRuleset{
					Name: "main", Target: "branch", Enforcement: "active",
					Rules: github.RulesetRules{
						RequiredStatusChecks: &github.RulesetStatusChecksRule{
							Checks:               []github.RulesetStatusCheck{{Context: "build"}},
							DoNotEnforceOnCreate: true,
						},
					},
				})).To(Succeed())

			Expect(rule(sent(0), "required_status_checks")).To(HaveKeyWithValue("parameters",
				HaveKeyWithValue("do_not_enforce_on_create", true)))
		})

		// Zero is not an App: it is nobody having pinned the check. Sending it
		// would ask GitHub to require a report from integration nought
		It("omits an integration nobody pinned the check to", func() {
			server = record(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusCreated)
				_, _ = io.WriteString(w, `{"id":7}`)
			})

			Expect(client().CreateRepositoryRuleset(
				context.Background(), "acme", "web", github.RepositoryRuleset{
					Name: "main", Target: "branch", Enforcement: "active",
					Rules: github.RulesetRules{
						RequiredStatusChecks: &github.RulesetStatusChecksRule{
							Checks: []github.RulesetStatusCheck{{Context: "build"}},
						},
					},
				})).To(Succeed())

			checks, ok := rule(sent(0), "required_status_checks")["parameters"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(checks["required_status_checks"]).To(ConsistOf(
				Not(HaveKey("integration_id"))))
		})
	})

	Describe("updating", func() {
		It("replaces the ruleset at its id", func() {
			server = record(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, `{"id":7}`)
			})

			Expect(client().UpdateRepositoryRuleset(
				context.Background(), "acme", "web", 7, github.RepositoryRuleset{
					Name: "main", Target: "branch", Enforcement: "active",
				})).To(Succeed())

			Expect(requests[0].Method).To(Equal(http.MethodPut))
			Expect(requests[0].URL.Path).To(Equal("/repos/acme/web/rulesets/7"))
			Expect(sent(0)).To(HaveKeyWithValue("name", "main"))
		})

		It("reports a refusal rather than reporting success", func() {
			server = record(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = io.WriteString(w, `{"message":"Invalid request"}`)
			})

			Expect(client().UpdateRepositoryRuleset(
				context.Background(), "acme", "web", 7, github.RepositoryRuleset{Name: "main"},
			)).To(MatchError(github.ErrAPIRequest))
		})
	})

	Describe("deleting", func() {
		It("removes the ruleset at its id", func() {
			server = record(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})

			Expect(client().DeleteRepositoryRuleset(
				context.Background(), "acme", "web", 7)).To(Succeed())

			Expect(requests[0].Method).To(Equal(http.MethodDelete))
			Expect(requests[0].URL.Path).To(Equal("/repos/acme/web/rulesets/7"))
		})
	})
})
