package orgsync_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
)

// protection is the ruleset the organization actually runs, which is what makes
// it the right starting point for a spec: every refusal below is a change to
// something somebody has written down.
func protection() orgsync.Ruleset {
	return orgsync.Ruleset{
		Name:        "main-branch-protection",
		Target:      orgsync.RulesetTargetBranch,
		Enforcement: orgsync.RulesetEnforcementActive,
		Conditions: orgsync.RulesetConditions{
			IncludeRefs: []string{"refs/heads/main"},
		},
		BypassActors: []orgsync.RulesetBypassActor{{
			ActorID: 5, ActorType: "OrganizationAdmin", Mode: "always",
		}},
		Rules: orgsync.RulesetRules{
			Deletion:              true,
			NonFastForward:        true,
			RequiredLinearHistory: true,
			RequiredSignatures:    true,
			PullRequest: &orgsync.RulesetPullRequestRule{
				RequiredApprovingReviewCount: 1,
				DismissStaleReviewsOnPush:    true,
				RequireCodeOwnerReview:       true,
				AllowedMergeMethods:          []string{"squash"},
			},
			RequiredStatusChecks: &orgsync.RulesetStatusChecksRule{
				Strict: true,
				Checks: []orgsync.RulesetStatusCheck{{Context: "test"}},
			},
		},
	}
}

// with applies one change to that ruleset, so each entry below reads as the one
// thing it is about.
func with(change func(*orgsync.Ruleset)) orgsync.Ruleset {
	ruleset := protection()
	change(&ruleset)

	return ruleset
}

var _ = Describe("Ruleset configuration [Unit]", func() {
	It("accepts what GitHub accepts", func() {
		Expect(orgsync.RulesetConfig{Rulesets: []orgsync.Ruleset{protection()}}.
			Validate()).To(Succeed())
	})

	// The organization's repositories do not agree on what their default branch
	// is called, so a configuration naming refs/heads/main protects nothing on
	// the ones still calling it master. This is the pattern that covers both
	It("accepts the default branch named as itself", func() {
		Expect(orgsync.RulesetConfig{Rulesets: []orgsync.Ruleset{
			with(func(r *orgsync.Ruleset) {
				r.Conditions.IncludeRefs = []string{"~DEFAULT_BRANCH"}
			}),
		}}.Validate()).To(Succeed())
	})

	// The one special value that means something on either target: every
	// branch of a branch ruleset, every tag of a tag one
	It("accepts a ruleset that applies to every ref", func() {
		Expect(orgsync.RulesetConfig{Rulesets: []orgsync.Ruleset{
			with(func(r *orgsync.Ruleset) {
				r.Conditions = orgsync.RulesetConditions{
					IncludeRefs: []string{"~ALL"},
					ExcludeRefs: []string{"refs/heads/tmp/*"},
				}
			}),
		}}.Validate()).To(Succeed())
	})

	It("accepts every ref of a tag ruleset too", func() {
		Expect(orgsync.RulesetConfig{Rulesets: []orgsync.Ruleset{
			with(func(r *orgsync.Ruleset) {
				r.Target = orgsync.RulesetTargetTag
				r.Conditions = orgsync.RulesetConditions{IncludeRefs: []string{"~ALL"}}
			}),
		}}.Validate()).To(Succeed())
	})

	It("accepts a tag ruleset over tag refs", func() {
		Expect(orgsync.RulesetConfig{Rulesets: []orgsync.Ruleset{
			with(func(r *orgsync.Ruleset) {
				r.Target = orgsync.RulesetTargetTag
				r.Conditions.IncludeRefs = []string{"refs/tags/v*"}
			}),
		}}.Validate()).To(Succeed())
	})

	// Every entry below is either a refusal GitHub would have made at apply
	// time - where a 422 abandons the whole ruleset and everything the tool was
	// going to do after it - or something GitHub accepts and then does nothing
	// with, which is worse because nobody finds out.
	DescribeTable("refuses configuration GitHub would refuse or ignore",
		func(ruleset orgsync.Ruleset, because string) {
			err := orgsync.RulesetConfig{Rulesets: []orgsync.Ruleset{ruleset}}.Validate()

			Expect(err).To(MatchError(orgsync.ErrInvalidConfig))
			Expect(err.Error()).To(ContainSubstring(because))
		},

		Entry("a name nobody wrote",
			with(func(r *orgsync.Ruleset) { r.Name = "" }), "has no name"),
		Entry("a name GitHub would trim",
			with(func(r *orgsync.Ruleset) { r.Name = " main " }), "whitespace"),

		// Passed through untouched by the tool this replaces, so a typo became
		// a 422 against somebody's repository
		Entry("a target nobody defined",
			with(func(r *orgsync.Ruleset) { r.Target = "brnach" }), "targets \"brnach\""),
		Entry("a push ruleset, which shares no rule with these",
			with(func(r *orgsync.Ruleset) { r.Target = "push" }),
			"restricts file paths and sizes rather than refs"),
		Entry("an enforcement nobody defined",
			with(func(r *orgsync.Ruleset) { r.Enforcement = "on" }), "enforced \"on\""),

		// The failure this whole chunk replaces. GitHub accepts it, matches no
		// ref that can ever exist, and the ruleset reads as protection in the
		// panel, in the plan and on GitHub's own page
		Entry("a branch ruleset aimed at tags",
			with(func(r *orgsync.Ruleset) {
				r.Conditions.IncludeRefs = []string{"refs/tags/v*"}
			}),
			"no branch can ever match"),
		Entry("a tag ruleset aimed at branches",
			with(func(r *orgsync.Ruleset) {
				r.Target = orgsync.RulesetTargetTag
				r.Conditions.IncludeRefs = []string{"refs/heads/main"}
			}),
			"no tag can ever match"),
		Entry("an exclusion aimed at the wrong kind of ref",
			with(func(r *orgsync.Ruleset) {
				r.Conditions.ExcludeRefs = []string{"refs/tags/v*"}
			}),
			"no branch can ever match"),
		Entry("a branch named without its ref",
			with(func(r *orgsync.Ruleset) { r.Conditions.IncludeRefs = []string{"main"} }),
			"which is not a ref"),
		Entry("an empty ref pattern",
			with(func(r *orgsync.Ruleset) { r.Conditions.IncludeRefs = []string{"  "} }),
			"an empty ref pattern"),

		// The same silence as the wrong-prefix case, one step earlier: GitHub
		// takes it, the rules page shows it, and no ref matches
		Entry("a ruleset that covers no refs at all",
			with(func(r *orgsync.Ruleset) { r.Conditions.IncludeRefs = nil }),
			"covers no refs"),

		// It names a branch, and no tag is ever the default branch. The one
		// special value that is not target-generic
		Entry("the default branch on a tag ruleset",
			with(func(r *orgsync.Ruleset) {
				r.Target = orgsync.RulesetTargetTag
				r.Conditions.IncludeRefs = []string{"~DEFAULT_BRANCH"}
			}),
			"no tag can ever be"),

		Entry("a bypass actor with no id",
			with(func(r *orgsync.Ruleset) {
				r.BypassActors = []orgsync.RulesetBypassActor{{
					ActorType: "Integration", Mode: "always",
				}}
			}),
			"bypass actor with no id"),
		Entry("a bypass actor of a type nobody defined",
			with(func(r *orgsync.Ruleset) {
				r.BypassActors[0].ActorType = "Robot"
			}),
			"type \"Robot\""),
		Entry("a bypass mode nobody defined",
			with(func(r *orgsync.Ruleset) { r.BypassActors[0].Mode = "sometimes" }),
			"bypass \"sometimes\""),
		Entry("a deploy key bypassing on pull requests",
			with(func(r *orgsync.Ruleset) {
				r.BypassActors[0].ActorType = "DeployKey"
				r.BypassActors[0].Mode = "pull_request"
			}),
			"a deploy key does not open one"),

		Entry("a pull request rule allowing no way to merge",
			with(func(r *orgsync.Ruleset) {
				r.Rules.PullRequest.AllowedMergeMethods = nil
			}),
			"allows no way of merging"),
		Entry("a merge method nobody defined",
			with(func(r *orgsync.Ruleset) {
				r.Rules.PullRequest.AllowedMergeMethods = []string{"fast-forward"}
			}),
			"merging by \"fast-forward\""),
		Entry("a merge method listed twice",
			with(func(r *orgsync.Ruleset) {
				r.Rules.PullRequest.AllowedMergeMethods = []string{"squash", "squash"}
			}),
			"merging by \"squash\" twice"),
		Entry("a negative review count",
			with(func(r *orgsync.Ruleset) {
				r.Rules.PullRequest.RequiredApprovingReviewCount = -1
			}),
			"requires -1 approving reviews"),

		// The tool this replaces dropped the whole rule when the list came out
		// empty - no log, no statistic, no error - and on an update removed a
		// rule that was already there
		Entry("required status checks that name none",
			with(func(r *orgsync.Ruleset) { r.Rules.RequiredStatusChecks.Checks = nil }),
			"names none"),
		Entry("a status check with no name",
			with(func(r *orgsync.Ruleset) {
				r.Rules.RequiredStatusChecks.Checks = []orgsync.RulesetStatusCheck{{
					Context: " ",
				}}
			}),
			"status check with no name"),
		Entry("a status check required twice",
			with(func(r *orgsync.Ruleset) {
				r.Rules.RequiredStatusChecks.Checks = []orgsync.RulesetStatusCheck{
					{Context: "test"}, {Context: "test"},
				}
			}),
			"the status check \"test\" twice"),

		// A check is satisfied by a report arriving under exactly this string,
		// so one with a space on the end is a check nothing will ever report -
		// and it sits beside its unpadded twin as a second requirement neither
		// of them is
		Entry("a status check with a space on the end",
			with(func(r *orgsync.Ruleset) {
				r.Rules.RequiredStatusChecks.Checks = []orgsync.RulesetStatusCheck{
					{Context: "test"}, {Context: "test "},
				}
			}),
			"leading or trailing whitespace"),

		Entry("code scanning with no tool",
			with(func(r *orgsync.Ruleset) {
				r.Rules.CodeScanning = &orgsync.RulesetCodeScanningRule{}
			}),
			"names no tool"),
		Entry("an alert threshold nobody defined",
			with(func(r *orgsync.Ruleset) {
				r.Rules.CodeScanning = &orgsync.RulesetCodeScanningRule{
					Tools: []orgsync.RulesetCodeScanningTool{{
						Tool:                    "CodeQL",
						AlertsThreshold:         "loud",
						SecurityAlertsThreshold: "all",
					}},
				}
			}),
			"alert threshold to \"loud\""),
		Entry("a security alert threshold nobody defined",
			with(func(r *orgsync.Ruleset) {
				r.Rules.CodeScanning = &orgsync.RulesetCodeScanningRule{
					Tools: []orgsync.RulesetCodeScanningTool{{
						Tool:                    "CodeQL",
						AlertsThreshold:         "all",
						SecurityAlertsThreshold: "scary",
					}},
				}
			}),
			"security alert threshold to \"scary\""),
	)

	// GitHub accepts exempt, whatever the organization's own file being written
	// against an older reading of the docs might suggest. Refusing a value the
	// API takes is a worse failure than the 422 the validation exists to save,
	// because nothing on screen would explain it
	It("accepts a bypass that files no audit entry", func() {
		Expect(orgsync.RulesetConfig{Rulesets: []orgsync.Ruleset{
			with(func(r *orgsync.Ruleset) { r.BypassActors[0].Mode = "exempt" }),
		}}.Validate()).To(Succeed())
	})

	Describe("naming one ruleset twice", func() {
		// GitHub permits two rulesets with the same name, which is exactly why
		// this has to be refused here: the name is the only handle sync has,
		// and nothing downstream could say which entry meant which ruleset
		It("refuses two entries with the same name", func() {
			err := orgsync.RulesetConfig{Rulesets: []orgsync.Ruleset{
				protection(), protection(),
			}}.Validate()

			Expect(err).To(MatchError(orgsync.ErrInvalidConfig))
			Expect(err.Error()).To(ContainSubstring("listed twice"))
		})

		It("refuses two entries differing only in case", func() {
			err := orgsync.RulesetConfig{Rulesets: []orgsync.Ruleset{
				protection(),
				with(func(r *orgsync.Ruleset) { r.Name = "MAIN-BRANCH-PROTECTION" }),
			}}.Validate()

			Expect(err).To(MatchError(orgsync.ErrInvalidConfig))
			Expect(err.Error()).To(ContainSubstring("differ only in case"))
		})
	})

	It("refuses an exclusion that cannot mean anything", func() {
		err := orgsync.RulesetConfig{Excludes: []string{""}}.Validate()

		Expect(err).To(MatchError(orgsync.ErrInvalidConfig))
	})

	Describe("decoding what an action carries", func() {
		// Twenty-odd fields across five nested types, and the payload is the
		// contract between what somebody approved and what runs. A tag that
		// stops round-tripping is a rule silently dropped from the request
		It("reads back every field of a ruleset written out", func() {
			resolved := orgsync.ResolvedRuleset{Ruleset: protection(), ID: 7}

			payload, err := json.Marshal(resolved)
			Expect(err).NotTo(HaveOccurred())

			Expect(orgsync.DecodeRulesetAction(payload)).To(Equal(resolved))
		})

		It("reports a payload it cannot read", func() {
			_, err := orgsync.DecodeRulesetAction([]byte(`{"name":`))

			Expect(err).To(MatchError(orgsync.ErrInvalidPlan))
		})
	})
})
