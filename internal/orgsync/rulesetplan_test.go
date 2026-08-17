package orgsync_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
)

var _ = Describe("Planning rulesets [Unit]", func() {
	// onGitHub is the same ruleset as protection(), as GitHub would answer
	// about it: whole, at the repository's own level, with an id.
	onGitHub := func(id int64, change func(*orgsync.Ruleset)) orgsync.CurrentRuleset {
		defined := protection()
		if change != nil {
			change(&defined)
		}

		return orgsync.CurrentRuleset{ID: id, Name: defined.Name, Defined: &defined}
	}

	// plan is the answer a caller acts on. The ambiguous names have their own
	// specs below; everywhere else, a plan that reported any would be a plan
	// this helper is the wrong shape for.
	plan := func(
		config orgsync.RulesetConfig, current ...orgsync.CurrentRuleset,
	) []orgsync.Action {
		GinkgoHelper()

		actions, ambiguous := orgsync.PlanRulesets(
			"repo-1", config, current, config.Exclusions())
		Expect(ambiguous).To(BeEmpty())

		return actions
	}

	ambiguity := func(
		config orgsync.RulesetConfig, current ...orgsync.CurrentRuleset,
	) []string {
		_, ambiguous := orgsync.PlanRulesets("repo-1", config, current, config.Exclusions())

		return ambiguous
	}

	wanted := orgsync.RulesetConfig{Rulesets: []orgsync.Ruleset{protection()}}

	Describe("a ruleset the repository does not have", func() {
		// Pinned whole rather than by fragments, because what is being checked
		// is that a person can read it. A rule carrying a list of its own -
		// the reviews, the checks - has to be told apart from its neighbours,
		// and a fragment match cannot see that they have run together
		It("plans creating it, with everything it will enforce", func() {
			actions := plan(wanted)

			Expect(actions).To(HaveLen(1))
			Expect(actions[0].Kind).To(Equal(orgsync.KindRulesets))
			Expect(actions[0].Operation).To(Equal(orgsync.OperationCreate))
			Expect(actions[0].Subject).To(Equal("main-branch-protection"))
			Expect(actions[0].Before).To(BeEmpty())
			Expect(actions[0].After).To(Equal(
				"branch, active; on refs/heads/main; " +
					"no deletion, no force pushes, linear history, signed commits, " +
					"pull requests (1 approving review, from code owners, " +
					"dismissed on push, merged by squash), " +
					"checks (test, branch up to date); " +
					"bypassed by OrganizationAdmin 5 always",
			))
		})

		// The executor writes what the plan carries, and a create has no
		// ruleset to write to yet
		It("carries the whole ruleset and no id", func() {
			resolved, err := orgsync.DecodeRulesetAction(plan(wanted)[0].Payload)
			Expect(err).NotTo(HaveOccurred())

			Expect(resolved.ID).To(BeZero())
			Expect(resolved.Ruleset).To(Equal(protection()))
		})
	})

	Describe("a ruleset the repository already matches", func() {
		It("plans nothing", func() {
			Expect(plan(wanted, onGitHub(7, nil))).To(BeEmpty())
		})

		// GitHub answers with the order it stored, and configuration is
		// whatever somebody typed. Comparing those literally rewrites a
		// matching repository on every tick for ever, which is the cost the
		// recorded digest exists to remove
		It("plans nothing when only the order differs", func() {
			shuffled := orgsync.RulesetConfig{Rulesets: []orgsync.Ruleset{
				with(func(r *orgsync.Ruleset) {
					r.Rules.PullRequest.AllowedMergeMethods = []string{"squash", "merge"}
					r.Conditions.IncludeRefs = []string{"refs/heads/main", "refs/heads/next"}
					r.Rules.RequiredStatusChecks.Checks = []orgsync.RulesetStatusCheck{
						{Context: "lint"}, {Context: "test"},
					}
				}),
			}}

			current := onGitHub(7, func(r *orgsync.Ruleset) {
				r.Rules.PullRequest.AllowedMergeMethods = []string{"merge", "squash"}
				r.Conditions.IncludeRefs = []string{"refs/heads/next", "refs/heads/main"}
				r.Rules.RequiredStatusChecks.Checks = []orgsync.RulesetStatusCheck{
					{Context: "test"}, {Context: "lint"},
				}
			})

			Expect(plan(shuffled, current)).To(BeEmpty())
		})

		// GitHub answers with an empty list where configuration left the field
		// out, and the two say the same thing
		It("plans nothing when one side spells absent as empty", func() {
			current := onGitHub(7, func(r *orgsync.Ruleset) {
				r.Conditions.ExcludeRefs = []string{}
			})

			Expect(plan(wanted, current)).To(BeEmpty())
		})
	})

	Describe("a ruleset that has drifted", func() {
		It("plans replacing it, and says what is there now", func() {
			current := onGitHub(7, func(r *orgsync.Ruleset) {
				r.Rules.PullRequest.RequiredApprovingReviewCount = 0
				r.Rules.RequiredSignatures = false
			})

			actions := plan(wanted, current)

			Expect(actions).To(HaveLen(1))
			Expect(actions[0].Operation).To(Equal(orgsync.OperationUpdate))
			Expect(actions[0].Before).To(ContainSubstring("no approving review"))
			Expect(actions[0].Before).NotTo(ContainSubstring("signed commits"))
			Expect(actions[0].After).To(ContainSubstring("1 approving review"))
			Expect(actions[0].After).To(ContainSubstring("signed commits"))
		})

		// A ruleset is written by replacement, at an id GitHub minted. Looking
		// the id up again when the work runs would find whatever holds the name
		// by then, which is not what anybody approved
		It("carries the id of the ruleset it would replace", func() {
			current := onGitHub(7, func(r *orgsync.Ruleset) { r.Enforcement = "disabled" })

			resolved, err := orgsync.DecodeRulesetAction(plan(wanted, current)[0].Payload)
			Expect(err).NotTo(HaveOccurred())

			Expect(resolved.ID).To(BeEquivalentTo(7))
			Expect(resolved.Enforcement).To(Equal(orgsync.RulesetEnforcementActive))
		})

		// A rule this version has no field for is enforced now and gone after a
		// replacement. Approving a plan that described the change out of the
		// half of the ruleset it could read would be approving a description
		// with the destruction left out of it
		It("says what a replacement drops that it cannot express", func() {
			current := onGitHub(7, nil)
			current.Unmanaged = []string{"merge_queue", "commit_message_pattern"}

			actions := plan(wanted, current)

			Expect(actions).To(HaveLen(1))
			Expect(actions[0].Operation).To(Equal(orgsync.OperationUpdate))
			Expect(actions[0].After).To(ContainSubstring(
				"this drops merge_queue, commit_message_pattern, " +
					"which this version cannot express"))
		})

		// The one that stops the removal happening quietly later. Everything
		// else about this ruleset already matches, so without this it settles -
		// and the rule goes the next time anything at all changes, in a plan
		// that never mentioned it
		It("never calls a ruleset settled while it enforces something unreadable", func() {
			current := onGitHub(7, nil)
			current.Unmanaged = []string{"merge_queue"}

			Expect(plan(wanted, current)).To(HaveLen(1))
		})

		It("says nothing of a drop where there is nothing to drop", func() {
			current := onGitHub(7, func(r *orgsync.Ruleset) { r.Enforcement = "disabled" })

			Expect(plan(wanted, current)[0].After).NotTo(ContainSubstring("drops"))
		})

		// The whole of the blind full replace this chunk exists to remove: a
		// request computed from an answer nobody received, dropping every rule,
		// condition and actor the repository has
		It("plans nothing against a ruleset nothing read", func() {
			unread := orgsync.CurrentRuleset{ID: 7, Name: "main-branch-protection"}

			Expect(plan(wanted, unread)).To(BeEmpty())
		})
	})

	Describe("a ruleset the organization already applies", func() {
		// The two do not replace each other, they both enforce, and GitHub
		// takes the union. Refusing to write would decide on somebody's behalf
		// that the organization's is the one they meant; saying so puts it in
		// front of whoever approves the plan, at the moment the stack is made
		It("plans the repository's own and says the organization's applies too", func() {
			inherited := orgsync.CurrentRuleset{
				ID: 99, Name: "main-branch-protection", Inherited: true,
			}

			actions := plan(wanted, inherited)

			Expect(actions).To(HaveLen(1))
			Expect(actions[0].Operation).To(Equal(orgsync.OperationCreate))
			Expect(actions[0].After).To(ContainSubstring(
				"an organization ruleset of this name also applies here"))
		})

		// It is not this repository's to change and not its to delete. A
		// request that tried would report a repository failing at something
		// nobody asked for
		It("never proposes removing one", func() {
			inherited := orgsync.CurrentRuleset{ID: 99, Name: "org-wide", Inherited: true}

			removing := wanted
			removing.AllowRemoval = true

			Expect(plan(removing, inherited)).To(HaveLen(1))
			Expect(plan(removing, inherited)[0].Operation).To(Equal(orgsync.OperationCreate))
		})

		// An inherited ruleset is not the one to update, so a repository with
		// only the inherited copy needs its own created
		It("does not mistake it for the repository's own", func() {
			inherited := orgsync.CurrentRuleset{
				ID: 99, Name: "main-branch-protection", Inherited: true,
				Defined: func() *orgsync.Ruleset { r := protection(); return &r }(),
			}

			Expect(plan(wanted, inherited)[0].Operation).To(Equal(orgsync.OperationCreate))
		})
	})

	// GitHub permits two rulesets with one name, and the tool this replaces
	// made them: it read one page of thirty, created what it could not see, and
	// from then on updated whichever came back first
	Describe("two rulesets of the same name on one repository", func() {
		drifted := func(id int64) orgsync.CurrentRuleset {
			return onGitHub(id, func(r *orgsync.Ruleset) { r.Enforcement = "disabled" })
		}

		It("plans nothing rather than writing to an arbitrary one", func() {
			actions, ambiguous := orgsync.PlanRulesets(
				"repo-1", wanted, []orgsync.CurrentRuleset{drifted(7), drifted(8)},
				wanted.Exclusions())

			Expect(actions).To(BeEmpty())
			Expect(ambiguous).To(Equal([]string{"main-branch-protection"}))
		})

		// The one that catches an implementation reaching for the first of
		// them: writing to the drifted copy leaves the matching one enforcing
		// beside it, and the plan would have said the repository was fixed
		It("plans nothing even where one of the two already matches", func() {
			Expect(ambiguity(wanted, drifted(7), onGitHub(8, nil))).
				To(Equal([]string{"main-branch-protection"}))
			Expect(ambiguity(wanted, onGitHub(8, nil), drifted(7))).
				To(Equal([]string{"main-branch-protection"}))
		})

		// The whole reason it is answered separately. An empty plan is what a
		// repository that already matches produces, so a caller reading only
		// the actions records this one as settled and stops looking at it -
		// and a ruleset nothing manages ends up indistinguishable from one
		// that is up to date
		It("says which name it could not answer for", func() {
			Expect(ambiguity(wanted, drifted(7), drifted(8))).NotTo(BeEmpty())
			Expect(ambiguity(wanted, onGitHub(7, nil))).To(BeEmpty())
		})

		// An inherited ruleset of the same name is not a second copy: it is not
		// this repository's, and the repository's own one is unambiguous
		It("is not confused by an inherited ruleset of the name", func() {
			inherited := orgsync.CurrentRuleset{
				ID: 99, Name: "main-branch-protection", Inherited: true,
			}

			Expect(ambiguity(wanted, drifted(7), inherited)).To(BeEmpty())
		})
	})

	Describe("a ruleset configuration no longer names", func() {
		surplus := orgsync.CurrentRuleset{ID: 9, Name: "old-protection"}

		It("leaves it alone while removal is off", func() {
			Expect(plan(wanted, onGitHub(7, nil), surplus)).To(BeEmpty())
		})

		It("proposes removing it once removal is on", func() {
			removing := wanted
			removing.AllowRemoval = true

			actions := plan(removing, onGitHub(7, nil), surplus)

			Expect(actions).To(HaveLen(1))
			Expect(actions[0].Operation).To(Equal(orgsync.OperationDelete))
			Expect(actions[0].Subject).To(Equal("old-protection"))
			Expect(actions[0].After).To(BeEmpty())
		})

		// Nothing read it whole, so its name is the only certain thing about
		// it. Printing an empty ruleset would read as one that enforced nothing
		It("says only what it knows about a ruleset nothing read", func() {
			removing := wanted
			removing.AllowRemoval = true

			Expect(plan(removing, surplus)[1].Before).To(
				Equal("old-protection, whatever it enforces"))
		})

		It("carries the id to remove", func() {
			removing := wanted
			removing.AllowRemoval = true

			resolved, err := orgsync.DecodeRulesetAction(plan(removing, surplus)[1].Payload)
			Expect(err).NotTo(HaveOccurred())

			Expect(resolved.ID).To(BeEquivalentTo(9))
			Expect(resolved.Name).To(Equal("old-protection"))
		})

		// Two plans of the same state have to be the same plan, or the digest
		// comparison means nothing and two runs cannot be told apart
		It("removes in a fixed order whatever order GitHub listed them in", func() {
			removing := wanted
			removing.AllowRemoval = true

			one := orgsync.CurrentRuleset{ID: 3, Name: "beta"}
			two := orgsync.CurrentRuleset{ID: 2, Name: "alpha"}
			three := orgsync.CurrentRuleset{ID: 1, Name: "beta"}

			subjects := func(actions []orgsync.Action) []string {
				var named []string
				for _, action := range actions {
					named = append(named, action.Subject)
				}

				return named
			}

			forwards := plan(removing, one, two, three)
			backwards := plan(removing, three, two, one)

			Expect(subjects(forwards)).To(Equal(subjects(backwards)))
			Expect(subjects(forwards)).To(Equal([]string{
				"main-branch-protection", "alpha", "beta", "beta",
			}))
			Expect(forwards).To(Equal(backwards))
		})
	})

	Describe("a ruleset somebody asked to be left alone", func() {
		excluded := orgsync.RulesetConfig{
			Rulesets:     []orgsync.Ruleset{protection()},
			AllowRemoval: true,
			Excludes:     []string{"main-*", "hand-made"},
		}

		It("neither writes it nor removes it", func() {
			handMade := orgsync.CurrentRuleset{ID: 4, Name: "hand-made"}

			Expect(plan(excluded, handMade)).To(BeEmpty())
		})

		It("leaves a configured one alone once it is excluded", func() {
			drifted := onGitHub(7, func(r *orgsync.Ruleset) { r.Enforcement = "disabled" })

			Expect(plan(excluded, drifted)).To(BeEmpty())
		})
	})
})
