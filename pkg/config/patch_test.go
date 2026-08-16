package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

var _ = Describe("Patch [Unit]", func() {
	// TOML is what a repository writes now and YAML is what it may already
	// have. Both are decoded into the same type by the same rules, so the
	// assertions are written once and run against each - a difference between
	// them is a repository whose settings change meaning when it migrates.
	Describe("ParsePatch", func() {
		DescribeTable("preserves explicit zero values",
			func(format config.Format, document string) {
				patch, err := config.ParsePatch(format, []byte(document))
				Expect(err).NotTo(HaveOccurred())
				Expect(patch.QuietSuccess).NotTo(BeNil())
				Expect(*patch.QuietSuccess).To(BeFalse())
				Expect(patch.AllowedCommands).NotTo(BeNil())
				Expect(*patch.AllowedCommands).To(BeEmpty())
				Expect(patch.CommandAliases).NotTo(BeNil())
				Expect(*patch.CommandAliases).To(BeEmpty())
				Expect(patch.CommandPrefix).NotTo(BeNil())
				Expect(*patch.CommandPrefix).To(BeEmpty())
			},
			Entry("yaml", config.FormatYAML, `
quiet_success: false
allowed_commands: []
command_aliases: {}
command_prefix: ""
`),
			Entry("toml", config.FormatTOML, `
quiet_success = false
allowed_commands = []
command_aliases = {}
command_prefix = ""
`),
		)

		// The message reaches a repository owner as the reason nothing ran, so
		// it has to name the key they mistyped. go-toml's own wording is
		// "fields in the document are missing in the target struct", which
		// names neither the key nor anything they could act on.
		DescribeTable("rejects unknown settings, naming them",
			func(format config.Format, document string) {
				_, err := config.ParsePatch(format, []byte(document))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unexpected_setting"))
			},
			Entry("yaml", config.FormatYAML, "unexpected_setting: true\n"),
			Entry("toml", config.FormatTOML, "unexpected_setting = true\n"),
		)

		It("names every unknown setting, not only the first", func() {
			_, err := config.ParsePatch(config.FormatTOML, []byte(
				"unexpected_setting = true\nalso_wrong = 1\n",
			))
			Expect(err).To(MatchError(config.ErrUnknownSetting))
			Expect(err.Error()).To(ContainSubstring("unexpected_setting"))
			Expect(err.Error()).To(ContainSubstring("also_wrong"))
		})

		DescribeTable("validates the runner",
			func(format config.Format, valid, invalid string) {
				patch, err := config.ParsePatch(format, []byte(valid))
				Expect(err).NotTo(HaveOccurred())
				Expect(patch.Runner).NotTo(BeNil())
				Expect(*patch.Runner).To(Equal(config.RunnerAction))

				_, err = config.ParsePatch(format, []byte(invalid))
				Expect(err).To(MatchError(config.ErrUnknownRunner))
			},
			Entry("yaml", config.FormatYAML, "runner: action\n", "runner: workflow\n"),
			Entry("toml", config.FormatTOML, `runner = "action"`, `runner = "workflow"`),
		)

		// An empty document is a file somebody created and has not filled in.
		// It has to read as "nothing set" rather than as an error, or adding
		// the file before adding a setting would take the repository offline.
		DescribeTable("reads an empty document as setting nothing",
			func(format config.Format, document string) {
				patch, err := config.ParsePatch(format, []byte(document))
				Expect(err).NotTo(HaveOccurred())
				Expect(patch.SetKeys()).To(BeEmpty())
			},
			Entry("yaml comment only", config.FormatYAML, "# nothing here yet\n"),
			Entry("toml comment only", config.FormatTOML, "# nothing here yet\n"),
			Entry("toml blank", config.FormatTOML, "\n\n"),
		)

		DescribeTable("reports a syntax error naming the format",
			func(format config.Format, document string) {
				_, err := config.ParsePatch(format, []byte(document))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(string(format)))
			},
			Entry("yaml", config.FormatYAML, "quiet_success: [unclosed\n"),
			Entry("toml", config.FormatTOML, "quiet_success = [unclosed\n"),
		)

		It("refuses a format it does not read", func() {
			_, err := config.ParsePatch(config.Format("ini"), []byte("quiet_success=1"))
			Expect(err).To(MatchError(config.ErrUnknownFormat))
		})
	})

	Describe("FormatOf", func() {
		DescribeTable("reads the format from the file name",
			func(path string, want config.Format) {
				format, err := config.FormatOf(path)
				Expect(err).NotTo(HaveOccurred())
				Expect(format).To(Equal(want))
			},
			Entry("dotfile", ".smyklot.toml", config.FormatTOML),
			Entry("directory", ".smyklot/config.toml", config.FormatTOML),
			Entry("under .github", ".github/.smyklot.toml", config.FormatTOML),
			Entry("legacy", ".github/smyklot.yaml", config.FormatYAML),
			Entry("legacy short", ".github/smyklot.yml", config.FormatYAML),
			Entry("upper case", ".github/SMYKLOT.YAML", config.FormatYAML),
		)

		DescribeTable("refuses a name it cannot place",
			func(path string) {
				_, err := config.FormatOf(path)
				Expect(err).To(MatchError(config.ErrUnknownFormat))
			},
			Entry("no extension", ".smyklot"),
			Entry("wrong extension", ".smyklot.json"),
			Entry("empty", ""),
		)
	})

	Describe("ApplyPatch", func() {
		It("replaces collections and leaves the base untouched", func() {
			base := config.Default()
			base.AllowedCommands = []string{"approve"}
			base.CommandAliases = map[string]string{"a": "approve"}

			emptyCommands := []string{}
			emptyAliases := map[string]string{}
			result := config.ApplyPatch(base, config.Patch{
				AllowedCommands: &emptyCommands,
				CommandAliases:  &emptyAliases,
			})

			Expect(result.AllowedCommands).To(BeEmpty())
			Expect(result.CommandAliases).To(BeEmpty())
			Expect(base.AllowedCommands).To(Equal([]string{"approve"}))
			Expect(base.CommandAliases).To(Equal(map[string]string{"a": "approve"}))
		})
	})

	Describe("Resolve", func() {
		It("applies layers in order and reports the winning source", func() {
			trueValue := true
			falseValue := false
			process := config.Default()
			process.CommandPrefix = "/"

			resolved := config.Resolve(
				process,
				config.Layer{
					Source: config.SourceTarget,
					Patch:  config.Patch{QuietSuccess: &trueValue},
				},
				config.Layer{
					Source: config.SourceRepositoryFile,
					Patch: config.Patch{
						QuietSuccess:  &falseValue,
						CommandPrefix: stringPointer("!"),
					},
				},
				config.Layer{
					Source: config.SourceRepositoryPanel,
					Patch:  config.Patch{QuietSuccess: &trueValue},
				},
			)

			Expect(resolved.Values.QuietSuccess).To(BeTrue())
			Expect(resolved.Values.CommandPrefix).To(Equal("!"))
			Expect(resolved.Sources).To(HaveKeyWithValue(
				config.KeyQuietSuccess,
				config.SourceRepositoryPanel,
			))
			Expect(resolved.Sources).To(HaveKeyWithValue(
				config.KeyCommandPrefix,
				config.SourceRepositoryFile,
			))
			Expect(resolved.Sources).To(HaveKeyWithValue(
				config.KeyDisableMentions,
				config.SourceProcess,
			))
		})
	})
})

func stringPointer(value string) *string {
	return &value
}
