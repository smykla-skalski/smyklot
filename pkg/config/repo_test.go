package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

var _ = Describe("LoadRepoConfig [Unit]", func() {
	Context("when the repository has no configuration file", func() {
		DescribeTable("should hand back the base unchanged",
			func(content []byte) {
				base := config.Default()
				base.QuietSuccess = true

				cfg, err := config.LoadRepoConfig(base, config.FormatYAML, content)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg).To(BeIdenticalTo(base))
			},
			Entry("nil content", nil),
			Entry("empty content", []byte{}),
			Entry("whitespace only", []byte("\n  \n")),
		)
	})

	Context("when the repository configures a setting", func() {
		It("should override that setting", func() {
			cfg, err := config.LoadRepoConfig(config.Default(), config.FormatYAML, []byte("quiet_success: true\n"))
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.QuietSuccess).To(BeTrue())
		})

		// The point of layering rather than replacing: a repository that sets
		// one key must not silently reset every other one to its zero value
		It("should keep the base value for every setting it omits", func() {
			base := config.Default()
			base.CommandPrefix = "!"
			base.DisableUnapprove = true
			base.AllowedCommands = []string{"approve", "merge"}
			base.CommandAliases = map[string]string{"a": "approve"}

			cfg, err := config.LoadRepoConfig(base, config.FormatYAML, []byte("quiet_success: true\n"))
			Expect(err).NotTo(HaveOccurred())

			Expect(cfg.QuietSuccess).To(BeTrue())
			Expect(cfg.CommandPrefix).To(Equal("!"))
			Expect(cfg.DisableUnapprove).To(BeTrue())
			Expect(cfg.AllowedCommands).To(Equal([]string{"approve", "merge"}))
			Expect(cfg.CommandAliases).To(Equal(map[string]string{"a": "approve"}))
		})

		It("should override a list wholesale rather than appending to it", func() {
			base := config.Default()
			base.AllowedCommands = []string{"approve", "merge", "cleanup"}

			cfg, err := config.LoadRepoConfig(base, config.FormatYAML, []byte("allowed_commands:\n  - approve\n"))
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.AllowedCommands).To(Equal([]string{"approve"}))
		})

		It("should override a map", func() {
			base := config.Default()
			base.CommandAliases = map[string]string{"a": "approve"}

			cfg, err := config.LoadRepoConfig(base, config.FormatYAML, []byte("command_aliases:\n  ship: merge\n"))
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.CommandAliases).To(Equal(map[string]string{"ship": "merge"}))
		})

		// A repository turning a service-wide restriction back off is the
		// direction that matters - false in the file must beat true in the base
		It("should let the file switch a setting back off", func() {
			base := config.Default()
			base.DisableReactions = true

			cfg, err := config.LoadRepoConfig(base, config.FormatYAML, []byte("disable_reactions: false\n"))
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.DisableReactions).To(BeFalse())
		})

		It("should carry every documented setting", func() {
			content := []byte(`
quiet_success: true
quiet_reactions: true
quiet_pending: true
allowed_commands:
  - approve
command_aliases:
  ship: merge
command_prefix: "!"
disable_mentions: true
disable_bare_commands: true
disable_unapprove: true
disable_reactions: true
disable_deleted_comments: true
allow_self_approval: true
runner: action
`)

			cfg, err := config.LoadRepoConfig(config.Default(), config.FormatYAML, content)
			Expect(err).NotTo(HaveOccurred())

			Expect(cfg).To(Equal(&config.Config{
				QuietSuccess:           true,
				QuietReactions:         true,
				QuietPending:           true,
				AllowedCommands:        []string{"approve"},
				CommandAliases:         map[string]string{"ship": "merge"},
				CommandPrefix:          "!",
				DisableMentions:        true,
				DisableBareCommands:    true,
				DisableUnapprove:       true,
				DisableReactions:       true,
				DisableDeletedComments: true,
				AllowSelfApproval:      true,
				Runner:                 config.RunnerAction,
			}))
		})
	})

	Context("the runner key", func() {
		// A repository that says nothing is served by the service, because the
		// App is already installed on it and no file has to be added to say so
		It("should default to the service", func() {
			cfg, err := config.LoadRepoConfig(config.Default(), config.FormatYAML, []byte("quiet_success: true\n"))
			Expect(err).NotTo(HaveOccurred())

			Expect(cfg.RunBy(config.RunnerService)).To(BeTrue())
			Expect(cfg.RunBy(config.RunnerAction)).To(BeFalse())
		})

		It("should let a repository fall back to the Action", func() {
			cfg, err := config.LoadRepoConfig(config.Default(), config.FormatYAML, []byte("runner: action\n"))
			Expect(err).NotTo(HaveOccurred())

			Expect(cfg.RunBy(config.RunnerAction)).To(BeTrue())
			Expect(cfg.RunBy(config.RunnerService)).To(BeFalse())
		})

		// Left to compare equal to neither name, a typo would stand both entry
		// points down and the repository would go silent with nothing to say why
		It("should reject a runner it does not know", func() {
			_, err := config.LoadRepoConfig(config.Default(), config.FormatYAML, []byte("runner: workflow\n"))
			Expect(err).To(MatchError(config.ErrUnknownRunner))
			Expect(err).To(MatchError(ContainSubstring("workflow")))
		})

		// A Config built in code, as every caller that does not read a file
		// builds one, must behave like a file that omits the key
		It("should read an unset runner as the default", func() {
			Expect((&config.Config{}).RunBy(config.RunnerService)).To(BeTrue())
			Expect((&config.Config{}).RunBy(config.RunnerAction)).To(BeFalse())
		})
	})

	Context("when the file is unusable", func() {
		It("should return an error rather than silently ignoring it", func() {
			_, err := config.LoadRepoConfig(config.Default(), config.FormatYAML, []byte("quiet_success: [unclosed\n"))
			Expect(err).To(HaveOccurred())
		})
	})

	// A file that sets nothing is a file somebody added before deciding what to
	// put in it, or one whose settings are commented out for the moment. It used
	// to be rejected, and because an unusable file is fail-closed, that took the
	// repository offline and posted "Invalid Configuration File" on the pull
	// request. Nothing in the file said anything, so nothing should change.
	DescribeTable("a file that sets nothing leaves the base alone",
		func(format config.Format, content string) {
			base := config.Default()
			base.QuietSuccess = true

			cfg, err := config.LoadRepoConfig(base, format, []byte(content))
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.QuietSuccess).To(BeTrue())
			Expect(cfg.CommandPrefix).To(Equal(config.DefaultCommandPrefix))
		},
		Entry("yaml comments only", config.FormatYAML, "# smyklot config\n# nothing set yet\n"),
		Entry("toml comments only", config.FormatTOML, "# smyklot config\n# nothing set yet\n"),
		Entry("yaml blank lines", config.FormatYAML, "\n\n\n"),
		Entry("toml blank lines", config.FormatTOML, "\n\n\n"),
	)

	// The same settings written in either format have to mean the same thing,
	// or migrating a repository would change its behaviour.
	It("reads a TOML file the same way as its YAML equivalent", func() {
		yamlConfig, err := config.LoadRepoConfig(config.Default(), config.FormatYAML, []byte(
			"quiet_success: true\nallowed_commands:\n  - approve\ncommand_aliases:\n  ship: merge\ncommand_prefix: \"!\"\nrunner: action\n",
		))
		Expect(err).NotTo(HaveOccurred())

		tomlConfig, err := config.LoadRepoConfig(config.Default(), config.FormatTOML, []byte(
			"quiet_success = true\nallowed_commands = [\"approve\"]\ncommand_prefix = \"!\"\nrunner = \"action\"\n\n[command_aliases]\nship = \"merge\"\n",
		))
		Expect(err).NotTo(HaveOccurred())

		Expect(tomlConfig).To(Equal(yamlConfig))
	})

	It("should start from defaults when no base is given", func() {
		cfg, err := config.LoadRepoConfig(nil, config.FormatYAML, []byte("quiet_success: true\n"))
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.QuietSuccess).To(BeTrue())
		Expect(cfg.CommandPrefix).To(Equal(config.DefaultCommandPrefix))
	})
})

var _ = Describe("EffectiveRunner [Unit]", func() {
	// Both entry points decide whether to act by comparing the runner to their
	// own name, so a third value matches neither: the service stands down, the
	// Action stands down, and the repository goes quiet with nothing anywhere
	// to say why. Every decoder refuses one it does not know, so this should be
	// unreachable - which is a claim about today's callers, not about the
	// failure, and the failure is a repository nobody can see has stopped.
	DescribeTable("reads anything it does not recognise as the default",
		func(runner config.Runner, want config.Runner) {
			cfg := &config.Config{Runner: runner}

			Expect(cfg.EffectiveRunner()).To(Equal(want))
			Expect(cfg.RunBy(want)).To(BeTrue())
		},
		Entry("the service", config.RunnerService, config.RunnerService),
		Entry("the action", config.RunnerAction, config.RunnerAction),
		Entry("unset", config.Runner(""), config.DefaultRunner),
		Entry("a value nothing writes", config.Runner("workflow"), config.DefaultRunner),
	)

	It("is always run by exactly one entry point", func() {
		for _, runner := range []config.Runner{"", "service", "action", "workflow", "  "} {
			cfg := &config.Config{Runner: runner}

			Expect(cfg.RunBy(config.RunnerService) != cfg.RunBy(config.RunnerAction)).
				To(BeTrue(), "runner %q is run by neither entry point or by both", runner)
		}
	})
})
