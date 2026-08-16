package config_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

func TestConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Config Suite")
}

var _ = Describe("Config [Unit]", func() {
	Describe("Default", func() {
		It("should return default config values", func() {
			cfg := config.Default()

			Expect(cfg.QuietSuccess).To(BeFalse())
			Expect(cfg.AllowedCommands).To(BeEmpty())
			Expect(cfg.CommandAliases).To(BeEmpty())
			Expect(cfg.CommandPrefix).To(Equal("/"))
			Expect(cfg.DisableMentions).To(BeFalse())
			Expect(cfg.DisableBareCommands).To(BeFalse())
		})
	})

	Describe("IsCommandAllowed", func() {
		It("should allow all commands when AllowedCommands is empty", func() {
			cfg := config.Default()

			Expect(cfg.IsCommandAllowed("approve")).To(BeTrue())
			Expect(cfg.IsCommandAllowed("merge")).To(BeTrue())
			Expect(cfg.IsCommandAllowed("anything")).To(BeTrue())
		})

		It("should only allow specified commands", func() {
			cfg := &config.Config{
				AllowedCommands: []string{"approve", "merge"},
			}

			Expect(cfg.IsCommandAllowed("approve")).To(BeTrue())
			Expect(cfg.IsCommandAllowed("merge")).To(BeTrue())
			Expect(cfg.IsCommandAllowed("close")).To(BeFalse())
		})
	})

	Describe("ResolveAlias", func() {
		It("should resolve alias to command name", func() {
			cfg := &config.Config{
				CommandAliases: map[string]string{
					"app": "approve",
					"a":   "approve",
				},
			}

			Expect(cfg.ResolveAlias("app")).To(Equal("approve"))
			Expect(cfg.ResolveAlias("a")).To(Equal("approve"))
		})

		It("should return the original command if no alias exists", func() {
			cfg := &config.Config{
				CommandAliases: map[string]string{
					"app": "approve",
				},
			}

			Expect(cfg.ResolveAlias("approve")).To(Equal("approve"))
			Expect(cfg.ResolveAlias("merge")).To(Equal("merge"))
		})
	})
})
