package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

var _ = Describe("Patch [Unit]", func() {
	Describe("ParsePatch", func() {
		It("preserves explicit zero values", func() {
			patch, err := config.ParsePatch([]byte(`
quiet_success: false
allowed_commands: []
command_aliases: {}
command_prefix: ""
`))
			Expect(err).NotTo(HaveOccurred())
			Expect(patch.QuietSuccess).NotTo(BeNil())
			Expect(*patch.QuietSuccess).To(BeFalse())
			Expect(patch.AllowedCommands).NotTo(BeNil())
			Expect(*patch.AllowedCommands).To(BeEmpty())
			Expect(patch.CommandAliases).NotTo(BeNil())
			Expect(*patch.CommandAliases).To(BeEmpty())
			Expect(patch.CommandPrefix).NotTo(BeNil())
			Expect(*patch.CommandPrefix).To(BeEmpty())
		})

		It("rejects unknown settings", func() {
			_, err := config.ParsePatch([]byte("unexpected_setting: true\n"))
			Expect(err).To(MatchError(ContainSubstring("field unexpected_setting not found")))
		})

		It("validates the runner", func() {
			patch, err := config.ParsePatch([]byte("runner: action\n"))
			Expect(err).NotTo(HaveOccurred())
			Expect(patch.Runner).NotTo(BeNil())
			Expect(*patch.Runner).To(Equal(config.RunnerAction))

			_, err = config.ParsePatch([]byte("runner: workflow\n"))
			Expect(err).To(MatchError(config.ErrUnknownRunner))
		})
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
