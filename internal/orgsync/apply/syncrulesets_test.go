package apply

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

// everyRulesetField is a ruleset with no field left at its zero value.
//
// The fixture the seam is pinned by. It is checked reflectively rather than
// trusted, so adding a field to orgsync.Ruleset fails this test until somebody
// sets it here - and setting it fails the round trip until the conversion
// carries it.
func everyRulesetField() orgsync.Ruleset {
	return orgsync.Ruleset{
		Name:        "main-branch-protection",
		Target:      orgsync.RulesetTargetBranch,
		Enforcement: orgsync.RulesetEnforcementActive,
		Conditions: orgsync.RulesetConditions{
			IncludeRefs: []string{"refs/heads/main"},
			ExcludeRefs: []string{"refs/heads/tmp/*"},
		},
		BypassActors: []orgsync.RulesetBypassActor{{
			ActorID: 5, ActorType: "OrganizationAdmin", Mode: "always",
		}},
		Rules: orgsync.RulesetRules{
			Creation:              true,
			Deletion:              true,
			NonFastForward:        true,
			RequiredLinearHistory: true,
			RequiredSignatures:    true,
			Update:                &orgsync.RulesetUpdateRule{AllowsFetchAndMerge: true},
			PullRequest: &orgsync.RulesetPullRequestRule{
				RequiredApprovingReviewCount:   2,
				DismissStaleReviewsOnPush:      true,
				RequireCodeOwnerReview:         true,
				RequireLastPushApproval:        true,
				RequiredReviewThreadResolution: true,
				AllowedMergeMethods:            []string{"squash"},
			},
			RequiredStatusChecks: &orgsync.RulesetStatusChecksRule{
				Checks: []orgsync.RulesetStatusCheck{{
					Context: "build", IntegrationID: 42,
				}},
				Strict:               true,
				DoNotEnforceOnCreate: true,
			},
			CodeScanning: &orgsync.RulesetCodeScanningRule{
				Tools: []orgsync.RulesetCodeScanningTool{{
					Tool:                    "CodeQL",
					AlertsThreshold:         "errors",
					SecurityAlertsThreshold: "high_or_higher",
				}},
			},
		},
	}
}

// TestARulesetSurvivesTheSeam is what makes two hand-written mirror conversions
// safe rather than hopeful.
//
// Neither package may import the other, so the conversion between the sync
// domain and the GitHub client is written out twice. A field added to one side
// and forgotten in the other compiles perfectly and silently stops being
// synchronized - which is exactly how three of the tool this replaces came to
// be parsed, schema-validated and then dropped on the floor.
//
// Three assertions, one fixture. The fixture leaves nothing at its zero value,
// so it cannot go stale; the client-side check catches a field the domain does
// not have; the round trip catches a field either conversion forgets.
func TestARulesetSurvivesTheSeam(t *testing.T) {
	whole := everyRulesetField()
	noZeroFields(t, "orgsync.Ruleset", reflect.ValueOf(whole))

	sent := asClientRuleset(whole)
	noZeroFields(t, "github.RepositoryRuleset", reflect.ValueOf(sent), readOnlyAtTheSeam...)

	if back := asConfiguredRuleset(sent); !reflect.DeepEqual(back, whole) {
		t.Errorf("a ruleset did not survive the seam:\n sent %+v\n back %+v", whole, back)
	}

	// And the one read-only field stays read-only. A conversion that started
	// filling it would be inventing an observation out of configuration, and
	// the plan would then report a repository dropping rules it never had.
	if sent.OtherRules != nil {
		t.Errorf("OtherRules = %v, wanted nothing: it describes what GitHub has, "+
			"not what configuration asks for", sent.OtherRules)
	}
}

// readOnlyAtTheSeam is what the client type carries that configuration cannot
// express, so a round trip proves nothing about it.
//
// One entry, and it earns its exception: OtherRules names the rules a
// replacement would remove, which only makes sense read from GitHub. Everything
// else on that type crosses in both directions and is held to it above.
var readOnlyAtTheSeam = []string{"github.RepositoryRuleset.OtherRules"}

// noZeroFields refuses a fixture that has left anything unsaid.
//
// Every leaf must differ from its zero, every pointer must be set and every
// slice must hold something, because a field the fixture leaves empty is a
// field the round trip cannot prove anything about.
func noZeroFields(t *testing.T, path string, value reflect.Value, skip ...string) {
	t.Helper()

	if slices.Contains(skip, path) {
		return
	}

	switch value.Kind() {
	case reflect.Struct:
		for index := range value.NumField() {
			noZeroFields(t,
				path+"."+value.Type().Field(index).Name, value.Field(index), skip...)
		}

	case reflect.Pointer:
		if value.IsNil() {
			t.Errorf("%s is nil, so the seam is not proven for anything under it", path)

			return
		}

		noZeroFields(t, path, value.Elem(), skip...)

	case reflect.Slice:
		if value.Len() == 0 {
			t.Errorf("%s is empty, so the seam is not proven for anything in it", path)

			return
		}

		for index := range value.Len() {
			noZeroFields(t,
				fmt.Sprintf("%s[%d]", path, index), value.Index(index), skip...)
		}

	default:
		if value.IsZero() {
			t.Errorf("%s is at its zero value, which proves nothing", path)
		}
	}
}

var _ = Describe("Ruleset sync [Unit]", func() {
	var (
		server   *httptest.Server
		requests []string
	)

	BeforeEach(func() { requests = nil })

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	// serve answers the two read endpoints and records every request, which for
	// the write side is the whole of what happened.
	serve := func(listing string, whole map[int64]string) {
		server = httptest.NewServer(http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				requests = append(requests,
					r.Method+" "+r.URL.Path+" "+string(body))

				for id, answer := range whole {
					if r.URL.Path == fmt.Sprintf("/repos/acme/web/rulesets/%d", id) &&
						r.Method == http.MethodGet {
						_, _ = io.WriteString(w, answer)

						return
					}
				}

				if r.URL.Path == "/repos/acme/web/rulesets" && r.Method == http.MethodGet {
					_, _ = io.WriteString(w, listing)

					return
				}

				_, _ = io.WriteString(w, `{"id":1}`)
			}))
	}

	client := func() *github.Client {
		GinkgoHelper()

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		return client
	}

	named := func(names ...string) orgsync.RulesetConfig {
		config := orgsync.RulesetConfig{}
		for _, name := range names {
			config.Rulesets = append(config.Rulesets, orgsync.Ruleset{Name: name})
		}

		return config
	}

	Describe("reading what a repository has", func() {
		// The listing carries no rules, so the whole object costs a request
		// each. Asking for every ruleset a repository happens to have is a
		// number somebody else decides; asking only for the ones configuration
		// names is a number this configuration decides
		It("reads whole only the rulesets configuration names", func() {
			serve(`[{"id":1,"name":"main","source_type":"Repository"},
			        {"id":2,"name":"legacy","source_type":"Repository"}]`,
				map[int64]string{1: `{"id":1,"name":"main","target":"branch",
				                      "enforcement":"active","rules":[{"type":"deletion"}]}`})

			current, err := readRulesets(
				GinkgoT().Context(), client(), "acme", "web", named("main"))
			Expect(err).NotTo(HaveOccurred())

			Expect(current).To(HaveLen(2))
			Expect(current[0].Defined).NotTo(BeNil())
			Expect(current[0].Defined.Rules.Deletion).To(BeTrue())

			// Named by nobody, so nothing asked what it enforces. Its name and
			// its id are all a removal needs
			Expect(current[1].Name).To(Equal("legacy"))
			Expect(current[1].Defined).To(BeNil())

			Expect(requests).To(HaveLen(2))
		})

		// Not this repository's to change and not its to delete, so reading it
		// whole would be a request spent on an answer nothing can act on
		It("never reads an inherited ruleset whole", func() {
			serve(`[{"id":9,"name":"main","source_type":"Organization"}]`, nil)

			current, err := readRulesets(
				GinkgoT().Context(), client(), "acme", "web", named("main"))
			Expect(err).NotTo(HaveOccurred())

			Expect(current).To(Equal([]orgsync.CurrentRuleset{{
				ID: 9, Name: "main", Inherited: true,
			}}))
			Expect(requests).To(HaveLen(1))
		})

		It("reports a listing it could not read", func() {
			server = httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusForbidden)
					_, _ = io.WriteString(w, `{"message":"Resource not accessible"}`)
				}))

			_, err := readRulesets(
				GinkgoT().Context(), client(), "acme", "web", named("main"))
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("applying one action", func() {
		payload := func(ruleset orgsync.ResolvedRuleset) []byte {
			GinkgoHelper()

			encoded, err := json.Marshal(ruleset)
			Expect(err).NotTo(HaveOccurred())

			return encoded
		}

		apply := func(action orgsync.Action) error {
			return applyRulesetAction(
				GinkgoT().Context(), client(), "acme", "web", action)
		}

		creating := func() orgsync.Action {
			return orgsync.Action{
				Kind:      orgsync.KindRulesets,
				Operation: orgsync.OperationCreate,
				Subject:   "main-branch-protection",
				Payload: payload(orgsync.ResolvedRuleset{
					Ruleset: everyRulesetField(),
				}),
			}
		}

		It("creates a ruleset the repository does not have", func() {
			serve(`[]`, nil)

			Expect(apply(creating())).To(Succeed())

			Expect(requests[1]).To(HavePrefix("POST /repos/acme/web/rulesets "))
			Expect(requests[1]).To(ContainSubstring(`"name":"main-branch-protection"`))
		})

		// A create carries no id because there is nothing yet to carry, and
		// GitHub permits two rulesets of one name - so a name claimed between
		// approval and apply would be answered with a second copy, which is the
		// state nothing downstream can address
		It("refuses to create a name that has been taken since the plan", func() {
			serve(`[{"id":7,"name":"main-branch-protection","source_type":"Repository"}]`, nil)

			Expect(apply(creating())).To(MatchError(errSyncRulesetTaken))

			// Read and refused, and nothing written
			Expect(requests).To(HaveLen(1))
			Expect(requests[0]).To(HavePrefix("GET "))
		})

		// It is the organization's, not the repository's, and the two enforce
		// side by side rather than colliding. Refusing here would leave the
		// repository unable to have its own for ever
		It("creates beside an inherited ruleset of the same name", func() {
			serve(`[{"id":9,"name":"main-branch-protection","source_type":"Organization"}]`, nil)

			Expect(apply(creating())).To(Succeed())

			Expect(requests[1]).To(HavePrefix("POST /repos/acme/web/rulesets "))
		})

		It("replaces the ruleset the plan addressed", func() {
			serve(`[]`, nil)

			Expect(apply(orgsync.Action{
				Kind:      orgsync.KindRulesets,
				Operation: orgsync.OperationUpdate,
				Subject:   "main",
				Payload: payload(orgsync.ResolvedRuleset{
					Ruleset: everyRulesetField(), ID: 7,
				}),
			})).To(Succeed())

			Expect(requests[0]).To(HavePrefix("PUT /repos/acme/web/rulesets/7 "))
		})

		It("removes the ruleset the plan addressed", func() {
			serve(`[]`, nil)

			Expect(apply(orgsync.Action{
				Kind:      orgsync.KindRulesets,
				Operation: orgsync.OperationDelete,
				Subject:   "legacy",
				Payload: payload(orgsync.ResolvedRuleset{
					Ruleset: orgsync.Ruleset{Name: "legacy"}, ID: 9,
				}),
			})).To(Succeed())

			Expect(requests[0]).To(HavePrefix("DELETE /repos/acme/web/rulesets/9 "))
		})

		// Resolving it by name here would write to whatever holds the name at
		// apply time, which on a repository that has grown a second ruleset of
		// that name is a coin toss nobody approved
		DescribeTable("refuses an action that does not say which ruleset",
			func(operation orgsync.Operation) {
				serve(`[]`, nil)

				err := apply(orgsync.Action{
					Kind: orgsync.KindRulesets, Operation: operation, Subject: "main",
					Payload: payload(orgsync.ResolvedRuleset{
						Ruleset: orgsync.Ruleset{Name: "main"},
					}),
				})

				Expect(err).To(MatchError(errSyncRulesetUnaddressed))
				Expect(requests).To(BeEmpty())
			},
			Entry("replacing one", orgsync.OperationUpdate),
			Entry("removing one", orgsync.OperationDelete),
		)

		It("refuses an action that says nothing at all", func() {
			serve(`[]`, nil)

			err := apply(orgsync.Action{
				Kind: orgsync.KindRulesets, Operation: orgsync.OperationCreate,
				Subject: "main",
			})

			Expect(err).To(MatchError(errSyncPayloadMissing))
			Expect(requests).To(BeEmpty())
		})

		It("refuses an operation it does not know", func() {
			serve(`[]`, nil)

			err := apply(orgsync.Action{
				Kind: orgsync.KindRulesets, Operation: "rename", Subject: "main",
				Payload: payload(orgsync.ResolvedRuleset{ID: 7}),
			})

			Expect(err).To(MatchError(errSyncOperationUnknown))
			Expect(requests).To(BeEmpty())
		})
	})
})
