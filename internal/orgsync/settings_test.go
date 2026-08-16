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
