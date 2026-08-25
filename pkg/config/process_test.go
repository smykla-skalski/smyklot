package config_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/pflag"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

// setEnv sets a variable for one spec and puts back whatever was there.
//
// The specs read the real environment rather than an injected lookup, because
// that is the thing being tested: LoadProcess is what a process calls, and a
// resolver proved against a map would not have caught a variable spelled one
// way in the code and another on the machine.
func setEnv(name, value string) {
	GinkgoHelper()

	previous, had := os.LookupEnv(name)
	Expect(os.Setenv(name, value)).To(Succeed())

	DeferCleanup(func() {
		if had {
			Expect(os.Setenv(name, previous)).To(Succeed())

			return
		}

		Expect(os.Unsetenv(name)).To(Succeed())
	})
}

// clearEnv removes every variable LoadProcess reads, so a spec starts from
// nothing whatever the developer's shell happens to export.
func clearEnv() {
	GinkgoHelper()

	for _, key := range config.Keys() {
		setEnvUnset(config.EnvVar(key))
	}

	setEnvUnset(config.EnvConfig)
	setEnvUnset(config.EnvConfigFile)
}

func setEnvUnset(name string) {
	GinkgoHelper()

	previous, had := os.LookupEnv(name)
	if !had {
		return
	}

	Expect(os.Unsetenv(name)).To(Succeed())
	DeferCleanup(func() {
		Expect(os.Setenv(name, previous)).To(Succeed())
	})
}

// newFlags builds the flag set an entry point registers, parsed from args.
func newFlags(args ...string) *pflag.FlagSet {
	GinkgoHelper()

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	config.RegisterFlags(flags)
	Expect(flags.Parse(args)).To(Succeed())

	return flags
}

// writeConfigFile writes a configuration file and returns its path.
func writeConfigFile(name, content string) string {
	GinkgoHelper()

	path := filepath.Join(GinkgoT().TempDir(), name)
	Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())

	return path
}

// precedencePair is two adjacent layers and what wins between them.
//
// resolve sets the setting at both layers to the layers' own names, so the
// expected answer is the name of the higher one and an entry cannot assert
// something other than what it describes.
type precedencePair struct {
	lower   string
	higher  string
	resolve func() string
}

// precedencePairs has one entry per adjacent pair of layers, which a spec
// below holds to PrecedenceLayers. Adding a layer without saying what it beats
// fails the suite rather than going untested.
var precedencePairs = []precedencePair{
	{
		lower:  "defaults",
		higher: "process file",
		resolve: func() string {
			setEnv(config.EnvConfigFile, writeConfigFile("smyklot.toml",
				`command_prefix = "process file"`))

			return processPrefix(nil)
		},
	},
	{
		lower:  "process file",
		higher: "process document",
		resolve: func() string {
			setEnv(config.EnvConfigFile, writeConfigFile("smyklot.toml",
				`command_prefix = "process file"`))
			setEnv(config.EnvConfig, `{"command_prefix": "process document"}`)

			return processPrefix(nil)
		},
	},
	{
		lower:  "process document",
		higher: "process environment",
		resolve: func() string {
			setEnv(config.EnvConfig, `{"command_prefix": "process document"}`)
			setEnv(config.EnvVar(config.KeyCommandPrefix), "process environment")

			return processPrefix(nil)
		},
	},
	{
		lower:  "process environment",
		higher: "process flags",
		resolve: func() string {
			setEnv(config.EnvVar(config.KeyCommandPrefix), "process environment")

			return processPrefix(newFlags("--" + config.KeyCommandPrefix + "=process flags"))
		},
	},
	{
		lower:  "process flags",
		higher: "account settings",
		resolve: func() string {
			return layeredPrefix(
				newFlags("--"+config.KeyCommandPrefix+"=process flags"),
				layerSetting(config.SourceTarget, "account settings"),
			)
		},
	},
	{
		lower:  "account settings",
		higher: "repository file",
		resolve: func() string {
			return layeredPrefix(nil,
				layerSetting(config.SourceTarget, "account settings"),
				layerSetting(config.SourceRepositoryFile, "repository file"),
			)
		},
	},
	{
		lower:  "repository file",
		higher: "repository settings",
		resolve: func() string {
			return layeredPrefix(nil,
				layerSetting(config.SourceRepositoryFile, "repository file"),
				layerSetting(config.SourceRepositoryPanel, "repository settings"),
			)
		},
	},
}

func processPrefix(flags *pflag.FlagSet) string {
	GinkgoHelper()

	cfg, err := config.LoadProcess(flags)
	Expect(err).NotTo(HaveOccurred())

	return cfg.CommandPrefix
}

func layeredPrefix(flags *pflag.FlagSet, layers ...config.Layer) string {
	GinkgoHelper()

	cfg, err := config.LoadProcess(flags)
	Expect(err).NotTo(HaveOccurred())

	return config.Resolve(cfg, layers...).Values.CommandPrefix
}

func layerSetting(source config.Source, prefix string) config.Layer {
	return config.Layer{Source: source, Patch: config.Patch{CommandPrefix: &prefix}}
}

var _ = Describe("Precedence [Unit]", func() {
	BeforeEach(clearEnv)

	DescribeTable("loads allow_draft_merges through every process spelling",
		func(setup func() *pflag.FlagSet) {
			cfg, err := config.LoadProcess(setup())
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.AllowDraftMerges).To(BeTrue())
		},
		Entry("config document", func() *pflag.FlagSet {
			setEnv(config.EnvConfig, `{"allow_draft_merges":true}`)
			return nil
		}),
		Entry("environment", func() *pflag.FlagSet {
			setEnv(config.EnvVar(config.KeyAllowDraftMerges), "true")
			return nil
		}),
		Entry("flag", func() *pflag.FlagSet {
			return newFlags("--" + config.KeyAllowDraftMerges + "=true")
		}),
	)

	for _, pair := range precedencePairs {
		It(pair.higher+" beats "+pair.lower, func() {
			Expect(pair.resolve()).To(Equal(pair.higher))
		})
	}

	It("has an entry for every adjacent pair of layers", func() {
		layers := config.PrecedenceLayers()

		// One fewer pair than layers, because a pair is a gap between two of
		// them rather than a layer itself
		Expect(precedencePairs).To(HaveLen(len(layers) - 1))

		for index, pair := range precedencePairs {
			Expect(pair.lower).To(Equal(layers[index].Name))
			Expect(pair.higher).To(Equal(layers[index+1].Name))
		}
	})

	It("is documented in the same order everywhere it is documented", func() {
		for _, name := range []string{"README.md", "CLAUDE.md"} {
			content, err := os.ReadFile(filepath.Join("..", "..", name))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring(config.PrecedenceDoc()),
				"%s no longer states the order PrecedenceDoc does", name)
		}
	})

	// The address is written into other people's repositories, so a README
	// naming a different one teaches an editor to fetch something nothing
	// serves - and nothing would fail, it would just stop understanding the
	// file
	It("publishes the schema at the address the README gives out", func() {
		content, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content)).To(ContainSubstring("#:schema " + config.SchemaURL))
	})

	// A flag carries its default whether or not anyone passed it, so a
	// resolver reading the value rather than asking whether it changed would
	// let the command line silently outrank every layer below it
	It("leaves a flag nobody passed out of the ladder", func() {
		setEnv(config.EnvVar(config.KeyCommandPrefix), "!")

		Expect(processPrefix(newFlags())).To(Equal("!"))
	})
})

var _ = Describe("LoadProcess [Unit]", func() {
	BeforeEach(clearEnv)

	It("returns the defaults when nothing is configured", func() {
		cfg, err := config.LoadProcess(nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg).To(Equal(config.Default()))
	})

	Describe("the process file", func() {
		It("reads a TOML file", func() {
			setEnv(config.EnvConfigFile, writeConfigFile("smyklot.toml", `
quiet_success = true
allowed_commands = ["approve", "merge"]
`))

			cfg, err := config.LoadProcess(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.QuietSuccess).To(BeTrue())
			Expect(cfg.AllowedCommands).To(ConsistOf("approve", "merge"))
		})

		It("prefers the flag to the variable", func() {
			setEnv(config.EnvConfigFile, writeConfigFile("variable.toml",
				`command_prefix = "variable"`))

			flags := newFlags("--" + config.FlagConfigFile + "=" +
				writeConfigFile("flag.toml", `command_prefix = "flag"`))

			Expect(processPrefix(flags)).To(Equal("flag"))
		})

		// Somebody meant to configure the process from it, and starting on
		// defaults instead would look like it had worked
		It("refuses to start when the named file is not there", func() {
			setEnv(config.EnvConfigFile, filepath.Join(GinkgoT().TempDir(), "absent.toml"))

			_, err := config.LoadProcess(nil)
			Expect(err).To(MatchError(os.ErrNotExist))
		})

		It("refuses a file whose extension names no format", func() {
			setEnv(config.EnvConfigFile, writeConfigFile("smyklot.ini", ""))

			_, err := config.LoadProcess(nil)
			Expect(err).To(MatchError(config.ErrUnknownFormat))
		})

		It("refuses a setting it does not know", func() {
			setEnv(config.EnvConfigFile, writeConfigFile("smyklot.toml", `not_a_setting = true`))

			_, err := config.LoadProcess(nil)
			Expect(err).To(MatchError(config.ErrUnknownSetting))
			Expect(err).To(MatchError(ContainSubstring("not_a_setting")))
		})
	})

	Describe("the process document", func() {
		It("reads a TOML document", func() {
			setEnv(config.EnvConfig, `
quiet_success = true
command_aliases = { app = "approve" }
`)

			cfg, err := config.LoadProcess(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.QuietSuccess).To(BeTrue())
			Expect(cfg.CommandAliases).To(HaveKeyWithValue("app", "approve"))
		})

		It("refuses a setting a TOML document does not name correctly", func() {
			setEnv(config.EnvConfig, `not_a_setting = true`)

			_, err := config.LoadProcess(nil)
			Expect(err).To(MatchError(config.ErrUnknownSetting))
		})

		// The variable is already deployed in repositories Smyklot has no
		// permission to edit, so the older spelling has to keep working
		It("reads a JSON document", func() {
			setEnv(config.EnvConfig, `{"quiet_success": true, "command_aliases": {"app": "approve"}}`)

			cfg, err := config.LoadProcess(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.QuietSuccess).To(BeTrue())
			Expect(cfg.CommandAliases).To(HaveKeyWithValue("app", "approve"))
		})

		It("reads an empty document as setting nothing", func() {
			setEnv(config.EnvConfig, "{}")

			cfg, err := config.LoadProcess(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).To(Equal(config.Default()))
		})

		It("reads an empty variable as setting nothing", func() {
			setEnv(config.EnvConfig, "  ")

			cfg, err := config.LoadProcess(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).To(Equal(config.Default()))
		})

		// viper dropped these silently, so a misspelled allowed_commands left
		// every command allowed and nothing said so
		It("refuses a setting it does not know", func() {
			setEnv(config.EnvConfig, `{"not_a_setting": true}`)

			_, err := config.LoadProcess(nil)
			Expect(err).To(MatchError(ContainSubstring("not_a_setting")))
		})

		It("refuses a document that is not JSON", func() {
			setEnv(config.EnvConfig, "not a document")

			_, err := config.LoadProcess(nil)
			Expect(err).To(HaveOccurred())
		})

		It("refuses a second document after the first", func() {
			setEnv(config.EnvConfig, `{"quiet_success": true} {"quiet_reactions": true}`)

			_, err := config.LoadProcess(nil)
			Expect(err).To(MatchError(config.ErrTrailingContent))
		})

		DescribeTable("tells the two spellings apart",
			func(raw string, legacy bool) {
				setEnv(config.EnvConfig, raw)

				Expect(config.DocumentIsLegacyJSON()).To(Equal(legacy))
			},
			Entry("an object", `{"quiet_success": true}`, true),
			Entry("an object behind whitespace", "\n  {\"quiet_success\": true}", true),
			Entry("a TOML assignment", `quiet_success = true`, false),
			// No TOML document can open with a brace, and no JSON object can
			// open with anything else, which is what makes the sniff safe
			Entry("a TOML comment", "# nothing set yet", false),
			Entry("nothing", "", false),
		)
	})

	Describe("the process environment", func() {
		It("reads a boolean", func() {
			setEnv(config.EnvVar(config.KeyQuietSuccess), "true")

			cfg, err := config.LoadProcess(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.QuietSuccess).To(BeTrue())
		})

		// viper's cast read anything it did not recognise as false, so this
		// turned the setting off
		It("refuses a boolean it cannot read rather than calling it false", func() {
			setEnv(config.EnvVar(config.KeyQuietSuccess), "yes")

			_, err := config.LoadProcess(nil)
			Expect(err).To(MatchError(config.ErrInvalidValue))
			Expect(err).To(MatchError(ContainSubstring(config.KeyQuietSuccess)))
		})

		DescribeTable("reads a list however it is separated",
			func(raw string) {
				setEnv(config.EnvVar(config.KeyAllowedCommands), raw)

				cfg, err := config.LoadProcess(nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.AllowedCommands).To(ConsistOf("approve", "merge"))
			},
			// viper split on whitespace and the matching flag splits on
			// commas, so both spellings are already in use
			Entry("commas", "approve,merge"),
			Entry("whitespace", "approve merge"),
			Entry("both, with spaces around them", "approve, merge"),
		)

		// A workflow forwarding an input nobody filled in writes an empty
		// string. Read as an instruction it would narrow allowed_commands to
		// nothing, and the ladder is fail-closed, so every command would be
		// refused
		It("reads a variable set to nothing as unset", func() {
			setEnv(config.EnvConfig, `{"command_prefix": "!"}`)
			setEnv(config.EnvVar(config.KeyCommandPrefix), "")

			cfg, err := config.LoadProcess(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.CommandPrefix).To(Equal("!"))
		})

		// viper could not decode a mapping from a variable at all, so this
		// setting silently had no environment spelling
		It("reads a mapping", func() {
			setEnv(config.EnvVar(config.KeyCommandAliases), "app=approve,ship=merge")

			cfg, err := config.LoadProcess(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.CommandAliases).To(HaveKeyWithValue("app", "approve"))
			Expect(cfg.CommandAliases).To(HaveKeyWithValue("ship", "merge"))
		})

		It("refuses a mapping entry that names nothing", func() {
			setEnv(config.EnvVar(config.KeyCommandAliases), "app")

			_, err := config.LoadProcess(nil)
			Expect(err).To(MatchError(config.ErrInvalidValue))
		})

		// The same rule the repository's own file is held to, because it is
		// the same rule rather than a copy of it
		It("refuses a runner it does not know", func() {
			setEnv(config.EnvVar(config.KeyRunner), "workflow")

			_, err := config.LoadProcess(nil)
			Expect(err).To(MatchError(config.ErrUnknownRunner))
		})

		It("reads a runner it does know", func() {
			setEnv(config.EnvVar(config.KeyRunner), "action")

			cfg, err := config.LoadProcess(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Runner).To(Equal(config.RunnerAction))
		})
	})

	Describe("the process flags", func() {
		It("reads every kind of setting", func() {
			flags := newFlags(
				"--"+config.KeyQuietSuccess,
				"--"+config.KeyAllowedCommands+"=approve,merge",
				"--"+config.KeyCommandAliases+"=app=approve",
				"--"+config.KeyCommandPrefix+"=!",
			)

			cfg, err := config.LoadProcess(flags)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.QuietSuccess).To(BeTrue())
			Expect(cfg.AllowedCommands).To(ConsistOf("approve", "merge"))
			Expect(cfg.CommandAliases).To(HaveKeyWithValue("app", "approve"))
			Expect(cfg.CommandPrefix).To(Equal("!"))
		})

		// An entry point that registers fewer flags reads fewer layers rather
		// than a different ladder
		It("accepts a flag set that registered none of the settings", func() {
			cfg, err := config.LoadProcess(pflag.NewFlagSet("bare", pflag.ContinueOnError))
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).To(Equal(config.Default()))
		})
	})
})
