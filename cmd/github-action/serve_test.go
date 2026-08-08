package main

import (
	"log/slog"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/spf13/cobra"

	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/logging"
)

// serveEnv lists every variable loadServeConfig reads, so a spec starts from a
// known state whatever the developer's shell already exports
var serveEnv = []string{
	envWebhookSecret,
	envListenAddress,
	envAdminAddress,
	envWebhookPath,
	envPollInterval,
	envLogFormat,
	envLogLevel,
	envAPIBaseURL,
	envBotUsername,
	envGitHubAppClientID,
	envGitHubAppID,
	envGitHubAppPrivateKey,
	config.EnvConfig,
}

// loadServe builds the serve command's config from env and the given flags
func loadServe(env map[string]string, args ...string) (*serveConfig, error) {
	GinkgoHelper()

	for _, name := range serveEnv {
		GinkgoT().Setenv(name, "")
	}

	settings := map[string]string{envWebhookSecret: "a-secret"}
	for key, value := range env {
		settings[key] = value
	}

	for key, value := range settings {
		GinkgoT().Setenv(key, value)
	}

	cmd := &cobra.Command{}
	cmd.Flags().String(flagListen, defaultListenAddress, descListen)
	cmd.Flags().String(flagAdminListen, defaultAdminAddress, descAdminListen)
	cmd.Flags().String(flagWebhookPath, defaultWebhookPath, descWebhookPath)
	cmd.Flags().Duration(flagPollInterval, defaultPollInterval, descPollInterval)
	cmd.Flags().String(flagLogFormat, defaultLogFormat, descLogFormat)
	cmd.Flags().String(flagLogLevel, defaultLogLevel, descLogLevel)

	if err := cmd.ParseFlags(args); err != nil {
		return nil, err
	}

	return loadServeConfig(cmd)
}

var _ = Describe("Serve configuration [Unit]", func() {
	// A service that starts without a secret would execute commands for anyone
	// who can reach the port, and nothing would say so
	It("should refuse to start without a webhook secret", func() {
		_, err := loadServe(map[string]string{envWebhookSecret: ""})
		Expect(err).To(MatchError(ErrNoWebhookSecret))
	})

	It("should default the listen address, path, and interval", func() {
		cfg, err := loadServe(nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(cfg.listenAddress).To(Equal(defaultListenAddress))
		Expect(cfg.webhookPath).To(Equal(defaultWebhookPath))
		Expect(cfg.pollInterval).To(Equal(defaultPollInterval))
		Expect(cfg.botUsername).To(Equal(defaultBotUsername))
	})

	// Everything an operator reads belongs off the port GitHub talks to
	It("should put the admin listener on its own port by default", func() {
		cfg, err := loadServe(nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(cfg.adminAddress).To(Equal(defaultAdminAddress))
		Expect(cfg.adminAddress).ToNot(Equal(cfg.listenAddress))
	})

	It("should default to JSON at info level", func() {
		cfg, err := loadServe(nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(cfg.logFormat).To(Equal(logging.FormatJSON))
		Expect(cfg.logLevel).To(Equal(slog.LevelInfo))
	})

	Context("precedence", func() {
		It("should take settings from the environment", func() {
			cfg, err := loadServe(map[string]string{
				envListenAddress: "127.0.0.1:9000",
				envAdminAddress:  "127.0.0.1:9001",
				envWebhookPath:   "/hooks/smyklot",
				envPollInterval:  "90s",
				envLogFormat:     "text",
				envLogLevel:      "debug",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(cfg.listenAddress).To(Equal("127.0.0.1:9000"))
			Expect(cfg.adminAddress).To(Equal("127.0.0.1:9001"))
			Expect(cfg.webhookPath).To(Equal("/hooks/smyklot"))
			Expect(cfg.pollInterval).To(Equal(90 * time.Second))
			Expect(cfg.logFormat).To(Equal(logging.FormatText))
			Expect(cfg.logLevel).To(Equal(slog.LevelDebug))
		})

		It("should let an explicit flag beat the environment", func() {
			cfg, err := loadServe(
				map[string]string{
					envListenAddress: "127.0.0.1:9000",
					envPollInterval:  "90s",
				},
				"--listen", "127.0.0.1:9999",
				"--poll-interval", "30s",
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(cfg.listenAddress).To(Equal("127.0.0.1:9999"))
			Expect(cfg.pollInterval).To(Equal(30 * time.Second))
		})
	})

	Context("validation", func() {
		It("should reject a webhook path that is not absolute", func() {
			_, err := loadServe(map[string]string{envWebhookPath: "hooks"})
			Expect(err).To(MatchError(ErrInvalidWebhookPath))
		})

		It("should reject an unparseable poll interval", func() {
			_, err := loadServe(map[string]string{envPollInterval: "every-so-often"})
			Expect(err).To(MatchError(ErrInvalidPollInterval))
		})

		// Turning the sweep off is a legitimate choice for an operator running
		// it out of band
		It("should accept a zero poll interval", func() {
			cfg, err := loadServe(map[string]string{envPollInterval: "0"})
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.pollInterval).To(BeZero())
		})

		// Sharing the port would publish the metrics and failure reasons the
		// admin listener exists to keep off the internet
		It("should reject an admin address equal to the listen address", func() {
			_, err := loadServe(map[string]string{
				envListenAddress: "127.0.0.1:9000",
				envAdminAddress:  "127.0.0.1:9000",
			})
			Expect(err).To(MatchError(ErrAddressConflict))
		})

		// Both listeners would fail to bind anyway. Saying which two settings
		// clash beats leaving the operator to work it out from "address
		// already in use"
		DescribeTable("should recognise the same socket written another way",
			func(listen, admin string, conflicts bool) {
				_, err := loadServe(map[string]string{
					envListenAddress: listen,
					envAdminAddress:  admin,
				})

				if conflicts {
					Expect(err).To(MatchError(ErrAddressConflict))

					return
				}

				Expect(err).NotTo(HaveOccurred())
			},
			Entry("a bare port against the IPv4 wildcard", ":8080", "0.0.0.0:8080", true),
			Entry("a bare port against the IPv6 wildcard", ":8080", "[::]:8080", true),
			Entry("a wildcard against one interface", "0.0.0.0:8080", "127.0.0.1:8080", true),
			Entry("two ports on one interface", "127.0.0.1:8080", "127.0.0.1:9090", false),
			Entry("one port on two interfaces", "127.0.0.1:8080", "192.168.1.5:8080", false),
			Entry("two kernel-assigned ports", "127.0.0.1:0", "127.0.0.1:0", false),
		)

		It("should reject an unknown log format", func() {
			_, err := loadServe(map[string]string{envLogFormat: "logfmt"})
			Expect(err).To(MatchError(logging.ErrUnknownLogFormat))
		})

		It("should reject an unknown log level", func() {
			_, err := loadServe(map[string]string{envLogLevel: "chatty"})
			Expect(err).To(MatchError(logging.ErrUnknownLogLevel))
		})
	})

	Context("App credentials", func() {
		It("should prefer the client ID", func() {
			cfg, err := loadServe(map[string]string{
				envGitHubAppClientID: "Iv1.client",
				envGitHubAppID:       "12345",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.appClientID).To(Equal("Iv1.client"))
		})

		It("should fall back to the numeric app ID", func() {
			cfg, err := loadServe(map[string]string{envGitHubAppID: "12345"})
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.appClientID).To(Equal("12345"))
		})

		// Every delivery names its own installation, so there is nothing to act
		// as without App credentials
		It("should refuse to build a server without them", func() {
			cfg, err := loadServe(nil)
			Expect(err).NotTo(HaveOccurred())

			_, err = newServer(cfg)
			Expect(err).To(MatchError(ContainSubstring("GitHub App authentication failed")))
		})
	})

	It("should read the process-wide bot configuration", func() {
		cfg, err := loadServe(map[string]string{
			config.EnvConfig: `{"quiet_success": true, "command_prefix": "!"}`,
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(cfg.botConfig.QuietSuccess).To(BeTrue())
		Expect(cfg.botConfig.CommandPrefix).To(Equal("!"))
	})
})
