package orgsync_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
)

// enabled and disabled say a setting was configured, which is the thing a bare
// bool cannot say: these specs turn on the difference between "somebody said
// no" and "nobody said".
func enabled() *bool  { value := true; return &value }
func disabled() *bool { value := false; return &value }

var _ = Describe("Settings configuration [Unit]", func() {
	It("accepts what GitHub accepts", func() {
		Expect(orgsync.SettingsConfig{
			AllowSquashMerge:         enabled(),
			AllowMergeCommit:         disabled(),
			SquashMergeCommitTitle:   text("PR_TITLE"),
			SquashMergeCommitMessage: text("BLANK"),
		}.Validate()).To(Succeed())
	})

	// The tool this replaces passed these through unvalidated, so a typo
	// reached GitHub as a 422 that abandoned the whole settings change
	DescribeTable("refuses a value GitHub does not define",
		func(config orgsync.SettingsConfig, because string) {
			err := config.Validate()

			Expect(err).To(MatchError(orgsync.ErrInvalidConfig))
			Expect(err.Error()).To(ContainSubstring(because))
		},
		Entry("a squash title nobody defined",
			orgsync.SettingsConfig{SquashMergeCommitTitle: text("PR_BODY")},
			"squash_merge_commit_title must be one of"),
		Entry("a squash message nobody defined",
			orgsync.SettingsConfig{SquashMergeCommitMessage: text("PR_TITLE")},
			"squash_merge_commit_message must be one of"),
		Entry("a merge title nobody defined",
			orgsync.SettingsConfig{MergeCommitTitle: text("COMMIT_MESSAGES")},
			"merge_commit_title must be one of"),
		Entry("a merge message nobody defined",
			orgsync.SettingsConfig{MergeCommitMessage: text("MERGE_MESSAGE")},
			"merge_commit_message must be one of"),
	)

	// GitHub refuses the last way of merging being turned off, and refuses it
	// as a 422 on the whole request - so this would break every other setting
	// in the same change rather than only itself
	It("refuses a repository that could not be merged at all", func() {
		err := orgsync.SettingsConfig{
			AllowMergeCommit: disabled(), AllowSquashMerge: disabled(), AllowRebaseMerge: disabled(),
		}.Validate()

		Expect(err).To(MatchError(orgsync.ErrInvalidConfig))
		Expect(err.Error()).To(ContainSubstring("at least one way to merge"))
	})

	// Three unset strategies is not a repository that forbids merging. It is a
	// repository nobody said anything about
	It("accepts a configuration that says nothing about merging", func() {
		Expect(orgsync.SettingsConfig{HasWiki: disabled()}.Validate()).To(Succeed())
	})

	It("accepts two off when the third is on", func() {
		Expect(orgsync.SettingsConfig{
			AllowMergeCommit: disabled(), AllowSquashMerge: enabled(), AllowRebaseMerge: disabled(),
		}.Validate()).To(Succeed())
	})

	// GitHub judges a commit wording against the merge strategy beside it, and
	// refuses the pair as a 422 on the whole request. A repository that merely
	// has the strategy off has the wording withheld from it; a configuration
	// that turns the strategy off itself is asking for something no repository
	// could accept, so the answer belongs beside the field somebody typed
	DescribeTable("refuses a wording its own configuration makes impossible",
		func(config orgsync.SettingsConfig, because string) {
			err := config.Validate()

			Expect(err).To(MatchError(orgsync.ErrInvalidConfig))
			Expect(err.Error()).To(ContainSubstring(because))
		},
		Entry("a squash title with squash merges off", orgsync.SettingsConfig{
			AllowSquashMerge: disabled(), SquashMergeCommitTitle: text("PR_TITLE"),
		}, "squash_merge_commit_title needs allow_squash_merge"),
		Entry("a squash message with squash merges off", orgsync.SettingsConfig{
			AllowSquashMerge: disabled(), SquashMergeCommitMessage: text("BLANK"),
		}, "squash_merge_commit_message needs allow_squash_merge"),
		Entry("a merge title with merge commits off", orgsync.SettingsConfig{
			AllowMergeCommit: disabled(), MergeCommitTitle: text("PR_TITLE"),
		}, "merge_commit_title needs allow_merge_commit"),
		Entry("a merge message with merge commits off", orgsync.SettingsConfig{
			AllowMergeCommit: disabled(), MergeCommitMessage: text("BLANK"),
		}, "merge_commit_message needs allow_merge_commit"),

		// The same rule, one endpoint down: GitHub refuses push protection on a
		// repository whose secret scanning is off
		Entry("push protection with secret scanning off", orgsync.SettingsConfig{
			SecretScanning: disabled(), SecretScanningPushProtection: enabled(),
		}, "secret_scanning_push_protection needs secret_scanning"),

		// And the far end of the chain, which is no more reachable for the
		// distance: turning advanced security off takes secret scanning with
		// it, and push protection goes with that. The answer names what the
		// configuration turned off rather than the link in between, because
		// that is the line somebody has to change
		Entry("push protection with advanced security off", orgsync.SettingsConfig{
			AdvancedSecurity: disabled(), SecretScanningPushProtection: enabled(),
		}, "secret_scanning_push_protection needs advanced_security"),
	)

	// Only one direction needs anything underneath it. GitHub refuses a feature
	// being switched on where what it needs is off, and accepts both being
	// switched off together - which is an ordinary thing for an organization to
	// want, and was refused at the keyboard with no way round it
	DescribeTable("accepts a feature and its dependency both being turned off",
		func(config orgsync.SettingsConfig) {
			Expect(config.Validate()).To(Succeed())
		},
		Entry("secret scanning under advanced security", orgsync.SettingsConfig{
			AdvancedSecurity: disabled(), SecretScanning: disabled(),
		}),
		Entry("push protection under secret scanning", orgsync.SettingsConfig{
			SecretScanning: disabled(), SecretScanningPushProtection: disabled(),
		}),
		Entry("all three at once", orgsync.SettingsConfig{
			AdvancedSecurity:             disabled(),
			SecretScanning:               disabled(),
			SecretScanningPushProtection: disabled(),
		}),
	)

	// The same configuration turning the strategy on is what makes the wording
	// beside it legal, and it is the ordinary way to configure both
	It("accepts a wording whose strategy it turns on", func() {
		Expect(orgsync.SettingsConfig{
			AllowSquashMerge:       enabled(),
			SquashMergeCommitTitle: text("PR_TITLE"),
		}.Validate()).To(Succeed())
	})
})

var _ = Describe("Settings planning [Unit]", func() {
	const repo = "github:repository:1"

	body := func(actions []orgsync.Action) map[string]any {
		GinkgoHelper()

		Expect(actions).To(HaveLen(1))
		sent, err := orgsync.DecodeSettings(actions[0].Payload)
		Expect(err).NotTo(HaveOccurred())

		return sent
	}

	It("proposes nothing when the repository already matches", func() {
		Expect(orgsync.PlanSettings(repo,
			orgsync.SettingsConfig{AllowSquashMerge: enabled(), HasWiki: disabled()},
			orgsync.CurrentSettings{AllowSquashMerge: true, HasWiki: false},
		)).To(BeEmpty())
	})

	It("changes a setting that drifted", func() {
		actions := orgsync.PlanSettings(repo,
			orgsync.SettingsConfig{DeleteBranchOnMerge: enabled()},
			orgsync.CurrentSettings{DeleteBranchOnMerge: false},
		)

		Expect(actions).To(HaveLen(1))
		Expect(actions[0].Kind).To(Equal(orgsync.KindSettings))
		Expect(actions[0].Operation).To(Equal(orgsync.OperationUpdate))
		Expect(actions[0].Subject).To(Equal(orgsync.SettingsSubject))
		Expect(actions[0].After).To(Equal("delete_branch_on_merge"))
		Expect(actions[0].Before).To(Equal("delete_branch_on_merge=off"))
	})

	// The bug that made the tool this replaces destructive. It used plain bools
	// for half the fields and omitted the rest, so "nobody said" and "somebody
	// said no" were the same value - and against an endpoint that replaces what
	// it is sent, that turned features off nobody had asked about
	It("says nothing about a setting nobody configured", func() {
		actions := orgsync.PlanSettings(repo,
			orgsync.SettingsConfig{DeleteBranchOnMerge: enabled()},
			orgsync.CurrentSettings{
				DeleteBranchOnMerge: false,
				HasWiki:             true,
				HasIssues:           true,
				AllowMergeCommit:    true,
			},
		)

		sent := body(actions)
		Expect(sent).To(HaveKeyWithValue("delete_branch_on_merge", true))
		Expect(sent).NotTo(HaveKey("has_wiki"))
		Expect(sent).NotTo(HaveKey("has_issues"))
		Expect(sent).NotTo(HaveKey("allow_merge_commit"))
		Expect(sent).To(HaveLen(1))
	})

	// Only the settings that differ, not every one configured. The endpoint
	// replaces what it is given, so writing back a value read a moment ago
	// would lose whatever changed in between
	It("sends only what differs", func() {
		sent := body(orgsync.PlanSettings(repo,
			orgsync.SettingsConfig{HasWiki: disabled(), HasIssues: enabled()},
			orgsync.CurrentSettings{HasWiki: true, HasIssues: true},
		))

		Expect(sent).To(HaveKeyWithValue("has_wiki", false))
		Expect(sent).NotTo(HaveKey("has_issues"))
	})

	// Three fields the tool this replaces parsed, validated against its own
	// schema, and then never sent
	It("sends the fields the old tool dropped", func() {
		sent := body(orgsync.PlanSettings(repo,
			orgsync.SettingsConfig{
				AllowUpdateBranch:        enabled(),
				SquashMergeCommitTitle:   text("PR_TITLE"),
				SquashMergeCommitMessage: text("BLANK"),
			},
			orgsync.CurrentSettings{AllowSquashMerge: true},
		))

		Expect(sent).To(HaveKeyWithValue("allow_update_branch", true))
		Expect(sent).To(HaveKeyWithValue("squash_merge_commit_title", "PR_TITLE"))
		Expect(sent).To(HaveKeyWithValue("squash_merge_commit_message", "BLANK"))
	})

	It("names every changed field, sorted, so two runs read the same", func() {
		actions := orgsync.PlanSettings(repo,
			orgsync.SettingsConfig{HasWiki: enabled(), AllowAutoMerge: enabled(), HasIssues: enabled()},
			orgsync.CurrentSettings{},
		)

		Expect(actions[0].After).To(Equal("allow_auto_merge, has_issues, has_wiki"))
	})

	// A setting turned off is a change like any other, and a payload that
	// dropped false would leave it on
	It("sends a setting being turned off", func() {
		sent := body(orgsync.PlanSettings(repo,
			orgsync.SettingsConfig{HasWiki: disabled()},
			orgsync.CurrentSettings{HasWiki: true},
		))

		Expect(sent).To(HaveKeyWithValue("has_wiki", false))
	})

	// One repository having the strategy off must not cost it every other
	// setting in the same change. GitHub answers the pair with a 422 on the
	// whole request, which is how the tool this replaces let one unavailable
	// feature take a run down with it
	DescribeTable("withholds a wording the repository could not accept",
		func(config orgsync.SettingsConfig, withheld string) {
			actions := orgsync.PlanSettings(repo, config,
				orgsync.CurrentSettings{
					// Squash-only, and nothing in any of these configurations
					// says otherwise
					AllowSquashMerge: false, AllowMergeCommit: false, HasWiki: true,
				})

			sent := body(actions)
			Expect(sent).To(HaveKeyWithValue("has_wiki", false))
			Expect(sent).NotTo(HaveKey(withheld))

			// And it says so where somebody reading the plan will see it, rather
			// than leaving a configured setting quietly unapplied for ever
			Expect(actions[0].After).To(ContainSubstring(withheld))
			Expect(actions[0].After).To(ContainSubstring("leaving"))
		},
		Entry("a squash title", orgsync.SettingsConfig{
			HasWiki: disabled(), SquashMergeCommitTitle: text("PR_TITLE"),
		}, "squash_merge_commit_title"),
		Entry("a squash message", orgsync.SettingsConfig{
			HasWiki: disabled(), SquashMergeCommitMessage: text("BLANK"),
		}, "squash_merge_commit_message"),
		Entry("a merge title", orgsync.SettingsConfig{
			HasWiki: disabled(), MergeCommitTitle: text("PR_TITLE"),
		}, "merge_commit_title"),
		Entry("a merge message", orgsync.SettingsConfig{
			HasWiki: disabled(), MergeCommitMessage: text("BLANK"),
		}, "merge_commit_message"),
	)

	// The resulting repository is what GitHub judges the wording against, so a
	// strategy switched on in the same request carries the wording with it
	DescribeTable("sends a wording whose strategy the same change turns on",
		func(config orgsync.SettingsConfig, field, value string) {
			sent := body(orgsync.PlanSettings(repo, config, orgsync.CurrentSettings{}))

			Expect(sent).To(HaveKeyWithValue(field, value))
		},
		Entry("a squash title", orgsync.SettingsConfig{
			AllowSquashMerge: enabled(), SquashMergeCommitTitle: text("PR_TITLE"),
		}, "squash_merge_commit_title", "PR_TITLE"),
		Entry("a squash message", orgsync.SettingsConfig{
			AllowSquashMerge: enabled(), SquashMergeCommitMessage: text("BLANK"),
		}, "squash_merge_commit_message", "BLANK"),
		Entry("a merge title", orgsync.SettingsConfig{
			AllowMergeCommit: enabled(), MergeCommitTitle: text("PR_TITLE"),
		}, "merge_commit_title", "PR_TITLE"),
		Entry("a merge message", orgsync.SettingsConfig{
			AllowMergeCommit: enabled(), MergeCommitMessage: text("BLANK"),
		}, "merge_commit_message", "BLANK"),
	)

	// A repository that already allows it needs nothing said about the strategy
	It("sends a wording to a repository that already allows the strategy", func() {
		sent := body(orgsync.PlanSettings(repo,
			orgsync.SettingsConfig{MergeCommitTitle: text("PR_TITLE")},
			orgsync.CurrentSettings{AllowMergeCommit: true},
		))

		Expect(sent).To(HaveKeyWithValue("merge_commit_title", "PR_TITLE"))
		Expect(sent).To(HaveLen(1))
	})

	// Two merge methods off is a legal thing to ask for - a repository whose
	// third is on takes it - so the configuration alone cannot be refused. The
	// pair that GitHub answers with a 422 is that configuration meeting a
	// repository that already had the third one off, and only here are both
	// halves in hand
	It("withholds the merge methods that would leave a repository unmergeable", func() {
		actions := orgsync.PlanSettings(repo,
			orgsync.SettingsConfig{
				AllowMergeCommit: disabled(),
				AllowSquashMerge: disabled(),
				HasWiki:          disabled(),
			},
			orgsync.CurrentSettings{
				AllowMergeCommit: true, AllowSquashMerge: true,
				// Nothing configured says otherwise, and this is what makes the
				// pair above impossible here
				AllowRebaseMerge: false,
				HasWiki:          true,
			},
		)

		sent := body(actions)
		Expect(sent).To(HaveKeyWithValue("has_wiki", false))

		// Both, not one of them. Turning one off and keeping the other would
		// leave a repository half-way to a policy it can never reach, and which
		// half would depend on the order of a table
		Expect(sent).NotTo(HaveKey("allow_merge_commit"))
		Expect(sent).NotTo(HaveKey("allow_squash_merge"))
		Expect(actions[0].After).To(ContainSubstring("no way to merge"))
	})

	// The same configuration against a repository that does allow rebasing is
	// exactly what it asks for, and has to go through
	It("switches merge methods off where one is left", func() {
		sent := body(orgsync.PlanSettings(repo,
			orgsync.SettingsConfig{
				AllowMergeCommit: disabled(), AllowSquashMerge: disabled(),
			},
			orgsync.CurrentSettings{
				AllowMergeCommit: true, AllowSquashMerge: true, AllowRebaseMerge: true,
			},
		))

		Expect(sent).To(HaveKeyWithValue("allow_merge_commit", false))
		Expect(sent).To(HaveKeyWithValue("allow_squash_merge", false))
	})

	// And the method the same change turns on counts, so a repository can be
	// moved from merge commits to rebasing in one go
	It("counts a merge method the same change switches on", func() {
		sent := body(orgsync.PlanSettings(repo,
			orgsync.SettingsConfig{
				AllowMergeCommit: disabled(),
				AllowSquashMerge: disabled(),
				AllowRebaseMerge: enabled(),
			},
			orgsync.CurrentSettings{AllowMergeCommit: true, AllowSquashMerge: true},
		))

		Expect(sent).To(HaveKeyWithValue("allow_merge_commit", false))
		Expect(sent).To(HaveKeyWithValue("allow_squash_merge", false))
		Expect(sent).To(HaveKeyWithValue("allow_rebase_merge", true))
	})

	// Nothing this can do about it until somebody configures the strategy or
	// the repository turns it on, and an action whose body would be empty is a
	// PATCH that can only fail
	It("proposes nothing when the only difference is withheld", func() {
		Expect(orgsync.PlanSettings(repo,
			orgsync.SettingsConfig{SquashMergeCommitTitle: text("PR_TITLE")},
			orgsync.CurrentSettings{AllowSquashMerge: false},
		)).To(BeEmpty())
	})

	// GitHub takes the security features nested, with a status string rather
	// than the boolean everything else here uses
	It("sends a security feature where GitHub keeps it", func() {
		sent := body(orgsync.PlanSettings(repo,
			orgsync.SettingsConfig{SecretScanning: enabled(), HasWiki: disabled()},
			orgsync.CurrentSettings{
				SecretScanning: orgsync.FeatureOff, HasWiki: true,
			},
		))

		Expect(sent).To(HaveKeyWithValue("has_wiki", false))
		Expect(sent).To(HaveKeyWithValue("security_and_analysis", map[string]any{
			"secret_scanning": map[string]any{"status": "enabled"},
		}))
	})

	It("sends a security feature being turned off", func() {
		sent := body(orgsync.PlanSettings(repo,
			orgsync.SettingsConfig{SecretScanning: disabled()},
			orgsync.CurrentSettings{SecretScanning: orgsync.FeatureOn},
		))

		Expect(sent).To(HaveKeyWithValue("security_and_analysis", map[string]any{
			"secret_scanning": map[string]any{"status": "disabled"},
		}))
	})

	It("says nothing about a security feature that already matches", func() {
		Expect(orgsync.PlanSettings(repo,
			orgsync.SettingsConfig{SecretScanning: enabled()},
			orgsync.CurrentSettings{SecretScanning: orgsync.FeatureOn},
		)).To(BeEmpty())
	})

	// The bug this port indicts the tool it replaces for, in the exact shape it
	// had: a repository without the feature diffed empty against enabled on
	// every run, 422d, and took the whole settings change down with it
	It("withholds a security feature the repository does not have", func() {
		actions := orgsync.PlanSettings(repo,
			orgsync.SettingsConfig{AdvancedSecurity: enabled(), HasWiki: disabled()},
			orgsync.CurrentSettings{
				AdvancedSecurity: orgsync.FeatureUnavailable, HasWiki: true,
			},
		)

		sent := body(actions)
		Expect(sent).To(HaveKeyWithValue("has_wiki", false))
		Expect(sent).NotTo(HaveKey("security_and_analysis"))

		// And it says which, and that nothing configured here will fix it
		Expect(actions[0].After).To(ContainSubstring("advanced_security"))
		Expect(actions[0].After).To(ContainSubstring("does not offer it"))
	})

	It("proposes nothing for a repository whose only gap is a feature it lacks", func() {
		Expect(orgsync.PlanSettings(repo,
			orgsync.SettingsConfig{AdvancedSecurity: enabled()},
			orgsync.CurrentSettings{AdvancedSecurity: orgsync.FeatureUnavailable},
		)).To(BeEmpty())
	})

	// GitHub refuses secret scanning on a repository whose advanced security is
	// off, and refuses it as a 422 on the whole request - so a repository that
	// has advanced security and has it switched off is one to leave alone
	It("withholds secret scanning from a repository with advanced security off", func() {
		actions := orgsync.PlanSettings(repo,
			orgsync.SettingsConfig{SecretScanning: enabled(), HasWiki: disabled()},
			orgsync.CurrentSettings{
				AdvancedSecurity: orgsync.FeatureOff,
				SecretScanning:   orgsync.FeatureOff,
				HasWiki:          true,
			},
		)

		sent := body(actions)
		Expect(sent).To(HaveKeyWithValue("has_wiki", false))
		Expect(sent).NotTo(HaveKey("security_and_analysis"))
		Expect(actions[0].After).To(ContainSubstring("secret_scanning"))
	})

	// And the case that makes a blanket rule wrong: a public repository has
	// secret scanning without advanced security, and GitHub reports no advanced
	// security there at all. Reading that absence as "off" would withhold a
	// setting nothing was ever going to refuse
	It("sends secret scanning where the repository has no advanced security to have", func() {
		sent := body(orgsync.PlanSettings(repo,
			orgsync.SettingsConfig{SecretScanning: enabled()},
			orgsync.CurrentSettings{
				AdvancedSecurity: orgsync.FeatureUnavailable,
				SecretScanning:   orgsync.FeatureOff,
			},
		))

		Expect(sent).To(HaveKeyWithValue("security_and_analysis", map[string]any{
			"secret_scanning": map[string]any{"status": "enabled"},
		}))
	})

	// GitHub disables what depends on a feature when that feature is disabled,
	// so a plan naming only the setting somebody typed describes less than
	// approving it does - and describing what it does is the whole of what a
	// plan is for
	It("says what GitHub switches off along with the change", func() {
		actions := orgsync.PlanSettings(repo,
			orgsync.SettingsConfig{AdvancedSecurity: disabled()},
			orgsync.CurrentSettings{
				AdvancedSecurity:             orgsync.FeatureOn,
				SecretScanning:               orgsync.FeatureOn,
				SecretScanningPushProtection: orgsync.FeatureOn,
			},
		)

		// Both of them, because what follows has dependants of its own - and
		// each said once, whole, because a dependant reached twice is a spread
		// that never settles
		Expect(actions[0].After).To(Equal(
			"advanced_security; GitHub also switches off " +
				"secret_scanning, secret_scanning_push_protection"))

		// And it is still not sent: the endpoint replaces what it is given, and
		// nobody configured these
		sent := body(actions)
		Expect(sent["security_and_analysis"]).To(Equal(map[string]any{
			"advanced_security": map[string]any{"status": "disabled"},
		}))
	})

	// The one that reads worst: a feature somebody explicitly asked to have on,
	// already on, and switched off by GitHub because this change takes what it
	// depends on away. Nothing sends it and nothing refuses it, so unless the
	// plan says so the only account of it is the repository afterwards
	It("says what goes off even where the configuration asked for it", func() {
		actions := orgsync.PlanSettings(repo,
			orgsync.SettingsConfig{
				AdvancedSecurity:             disabled(),
				SecretScanningPushProtection: enabled(),
			},
			orgsync.CurrentSettings{
				AdvancedSecurity:             orgsync.FeatureOn,
				SecretScanning:               orgsync.FeatureOn,
				SecretScanningPushProtection: orgsync.FeatureOn,
			},
		)

		Expect(actions[0].After).To(ContainSubstring("secret_scanning_push_protection"))
	})

	It("says nothing about a feature that was already off", func() {
		actions := orgsync.PlanSettings(repo,
			orgsync.SettingsConfig{AdvancedSecurity: disabled()},
			orgsync.CurrentSettings{
				AdvancedSecurity: orgsync.FeatureOn,
				SecretScanning:   orgsync.FeatureOff,
			},
		)

		Expect(actions[0].After).NotTo(ContainSubstring("secret_scanning"))
	})

	// The plan half of the same rule. A repository whose advanced security has
	// lapsed still has secret scanning on, and turning that off is a change
	// nothing refuses - withholding it would leave the feature enforced on a
	// repository the configuration says should not have it
	It("switches a security feature off under a dependency that is off", func() {
		sent := body(orgsync.PlanSettings(repo,
			orgsync.SettingsConfig{SecretScanning: disabled()},
			orgsync.CurrentSettings{
				AdvancedSecurity: orgsync.FeatureOff, SecretScanning: orgsync.FeatureOn,
			},
		))

		Expect(sent).To(HaveKeyWithValue("security_and_analysis", map[string]any{
			"secret_scanning": map[string]any{"status": "disabled"},
		}))
	})

	// A chain has to be judged against what will happen rather than what was
	// asked for. Secret scanning withheld because advanced security is off is
	// secret scanning that stays off - and push protection sent on its own into
	// exactly the 422 withholding exists to avoid
	It("withholds what depends on a setting that was itself withheld", func() {
		actions := orgsync.PlanSettings(repo,
			orgsync.SettingsConfig{
				SecretScanning:               enabled(),
				SecretScanningPushProtection: enabled(),
				HasWiki:                      disabled(),
			},
			orgsync.CurrentSettings{
				AdvancedSecurity:             orgsync.FeatureOff,
				SecretScanning:               orgsync.FeatureOff,
				SecretScanningPushProtection: orgsync.FeatureOff,
				HasWiki:                      true,
			},
		)

		sent := body(actions)
		Expect(sent).To(HaveKeyWithValue("has_wiki", false))
		Expect(sent).NotTo(HaveKey("security_and_analysis"))

		// Both of them, each with the reason: the one whose dependency is off,
		// and the one whose dependency is now known not to be changing
		Expect(actions[0].After).
			To(ContainSubstring("leaving secret_scanning alone: the setting it needs is off"))
		Expect(actions[0].After).
			To(ContainSubstring("leaving secret_scanning_push_protection alone"))
	})

	It("sends secret scanning with the advanced security the same change turns on", func() {
		sent := body(orgsync.PlanSettings(repo,
			orgsync.SettingsConfig{AdvancedSecurity: enabled(), SecretScanning: enabled()},
			orgsync.CurrentSettings{
				AdvancedSecurity: orgsync.FeatureOff, SecretScanning: orgsync.FeatureOff,
			},
		))

		Expect(sent).To(HaveKeyWithValue("security_and_analysis", map[string]any{
			"advanced_security": map[string]any{"status": "enabled"},
			"secret_scanning":   map[string]any{"status": "enabled"},
		}))
	})

	// Push protection needs secret scanning, which is the wording rule one
	// endpoint down - and the same field in the table says so
	It("withholds push protection from a repository without secret scanning", func() {
		actions := orgsync.PlanSettings(repo,
			orgsync.SettingsConfig{
				SecretScanningPushProtection: enabled(), HasWiki: disabled(),
			},
			orgsync.CurrentSettings{
				SecretScanning:               orgsync.FeatureOff,
				SecretScanningPushProtection: orgsync.FeatureOff,
				HasWiki:                      true,
			},
		)

		Expect(body(actions)).NotTo(HaveKey("security_and_analysis"))
		Expect(actions[0].After).To(ContainSubstring("secret_scanning_push_protection"))
	})

	It("sends push protection with the scanning the same change turns on", func() {
		sent := body(orgsync.PlanSettings(repo,
			orgsync.SettingsConfig{
				SecretScanning: enabled(), SecretScanningPushProtection: enabled(),
			},
			orgsync.CurrentSettings{
				SecretScanning:               orgsync.FeatureOff,
				SecretScanningPushProtection: orgsync.FeatureOff,
			},
		))

		Expect(sent).To(HaveKeyWithValue("security_and_analysis", map[string]any{
			"secret_scanning":                 map[string]any{"status": "enabled"},
			"secret_scanning_push_protection": map[string]any{"status": "enabled"},
		}))
	})

	It("carries a payload the executor can read back", func() {
		actions := orgsync.PlanSettings(repo,
			orgsync.SettingsConfig{HasWiki: disabled()},
			orgsync.CurrentSettings{HasWiki: true},
		)

		var raw map[string]any
		Expect(json.Unmarshal(actions[0].Payload, &raw)).To(Succeed())
		Expect(raw).To(HaveKeyWithValue("has_wiki", false))
	})
})
