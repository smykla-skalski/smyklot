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
	envPendingCIQuietPeriod,
	envLogFormat,
	envLogLevel,
	envDatabase,
	envState,
	envPanelOrigin,
	envPanelBase,
	envPanelState,
	envPanelSuperRootID,
	envPanelTTL,
	envPanelClientID,
	envPanelClientSecret,
	envGitHubAuthURL,
	envGitHubTokenURL,
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
	cmd.Flags().Duration(
		flagPendingCIQuietPeriod,
		defaultPendingCIQuietPeriod,
		descPendingCIQuietPeriod,
	)
	cmd.Flags().String(flagLogFormat, defaultLogFormat, descLogFormat)
	cmd.Flags().String(flagLogLevel, defaultLogLevel, descLogLevel)
	cmd.Flags().String(flagDatabase, defaultState, descDatabase)
	cmd.Flags().String(flagState, "", descState)
	cmd.Flags().String(flagPanelOrigin, "", descPanelOrigin)
	cmd.Flags().String(flagPanelBase, defaultPanelBase, descPanelBase)
	cmd.Flags().String(flagPanelState, "", descPanelState)
	cmd.Flags().Int64(flagPanelSuperRootID, 0, descPanelSuperRootID)
	cmd.Flags().Duration(flagPanelTTL, defaultPanelTTL, descPanelTTL)

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
		Expect(cfg.pendingCIQuietPeriod).To(Equal(defaultPendingCIQuietPeriod))
		Expect(cfg.botUsername).To(Equal(defaultBotUsername))
		Expect(cfg.database).To(Equal(defaultState))
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
				envListenAddress:        "127.0.0.1:9000",
				envAdminAddress:         "127.0.0.1:9001",
				envWebhookPath:          "/hooks/smyklot",
				envPollInterval:         "90s",
				envPendingCIQuietPeriod: "45s",
				envLogFormat:            "text",
				envLogLevel:             "debug",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(cfg.listenAddress).To(Equal("127.0.0.1:9000"))
			Expect(cfg.adminAddress).To(Equal("127.0.0.1:9001"))
			Expect(cfg.webhookPath).To(Equal("/hooks/smyklot"))
			Expect(cfg.pollInterval).To(Equal(90 * time.Second))
			Expect(cfg.pendingCIQuietPeriod).To(Equal(45 * time.Second))
			Expect(cfg.logFormat).To(Equal(logging.FormatText))
			Expect(cfg.logLevel).To(Equal(slog.LevelDebug))
		})

		It("should let an explicit flag beat the environment", func() {
			cfg, err := loadServe(
				map[string]string{
					envListenAddress:        "127.0.0.1:9000",
					envPollInterval:         "90s",
					envPendingCIQuietPeriod: "45s",
				},
				"--listen", "127.0.0.1:9999",
				"--poll-interval", "30s",
				"--pending-ci-quiet-period", "20s",
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(cfg.listenAddress).To(Equal("127.0.0.1:9999"))
			Expect(cfg.pollInterval).To(Equal(30 * time.Second))
			Expect(cfg.pendingCIQuietPeriod).To(Equal(20 * time.Second))
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

		DescribeTable("should reject an unsupported pending-CI quiet period", func(value string) {
			_, err := loadServe(map[string]string{envPendingCIQuietPeriod: value})
			Expect(err).To(MatchError(ErrInvalidPendingCIQuietPeriod))
		},
			Entry("zero", "0"),
			Entry("sub-second", "500ms"),
			Entry("over one day", "25h"),
		)

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

	Describe("Panel configuration", func() {
		var enabledPanel = map[string]string{
			envPanelOrigin:       "https://smyklot.example",
			envPanelSuperRootID:  "42",
			envPanelClientID:     "Ov23li.panel",
			envPanelClientSecret: "oauth-secret",
		}

		It("should remain disabled without a public origin", func() {
			cfg, err := loadServe(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.panel).To(BeNil())
		})

		It("should apply safe defaults when enabled", func() {
			cfg, err := loadServe(enabledPanel)
			Expect(err).NotTo(HaveOccurred())

			Expect(cfg.panel).To(Equal(&panelServeConfig{
				publicOrigin: "https://smyklot.example",
				basePath:     defaultPanelBase,
				superRootID:  42,
				clientID:     "Ov23li.panel",
				clientSecret: "oauth-secret",
				authorizeURL: defaultGitHubAuthURL,
				tokenURL:     defaultGitHubTokenURL,
				sessionTTL:   defaultPanelTTL,
			}))
		})

		// Authorizing a GitHub App shows the permissions its registration
		// asks for, so reusing the App's OAuth credentials here would ask
		// someone reading a dashboard to grant the write access the bot
		// approves and merges with. The panel takes a classic OAuth App or
		// nothing
		It("should refuse to sign in with the App's own credentials", func() {
			_, err := loadServe(map[string]string{
				envPanelOrigin:             "https://smyklot.example",
				envPanelSuperRootID:        "42",
				envGitHubAppClientID:       "Iv1.app",
				"GITHUB_APP_CLIENT_SECRET": "app-secret",
			})
			Expect(err).To(MatchError(ErrPanelConfig))
		})

		It("should allow the panel to own the public root", func() {
			env := map[string]string{
				envPanelOrigin:       "https://smyklot.com",
				envPanelBase:         "/",
				envPanelSuperRootID:  "42",
				envPanelClientID:     "Ov23li.panel",
				envPanelClientSecret: "oauth-secret",
			}

			cfg, err := loadServe(env)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.panel.basePath).To(BeEmpty())
		})

		It("should let explicit flags override panel environment values", func() {
			env := map[string]string{
				envPanelOrigin:       "https://old.example",
				envPanelBase:         "/old",
				envState:             "/tmp/old.sqlite3",
				envPanelSuperRootID:  "42",
				envPanelTTL:          "24h",
				envPanelClientID:     "Ov23li.panel",
				envPanelClientSecret: "oauth-secret",
			}

			cfg, err := loadServe(env,
				"--panel-public-origin", "https://new.example",
				"--panel-base-path", "/admin",
				"--state-path", "/tmp/new.sqlite3",
				"--panel-super-root-id", "84",
				"--panel-session-ttl", "2h",
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(cfg.panel.publicOrigin).To(Equal("https://new.example"))
			Expect(cfg.panel.basePath).To(Equal("/admin"))
			Expect(cfg.database).To(Equal("/tmp/new.sqlite3"))
			Expect(cfg.panel.superRootID).To(Equal(int64(84)))
			Expect(cfg.panel.sessionTTL).To(Equal(2 * time.Hour))
		})

		It("should accept the old panel state path for one compatibility release", func() {
			cfg, err := loadServe(map[string]string{envPanelState: "/tmp/legacy.sqlite3"})
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.database).To(Equal("/tmp/legacy.sqlite3"))
		})

		It("should accept a database URL and prefer it over every older spelling", func() {
			cfg, err := loadServe(map[string]string{
				envDatabase:   "postgres://smyklot@db.internal:5432/smyklot",
				envState:      "/tmp/ignored.sqlite3",
				envPanelState: "/tmp/also-ignored.sqlite3",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.database).To(Equal("postgres://smyklot@db.internal:5432/smyklot"))
		})

		It("should reject a database it has no engine for", func() {
			_, err := loadServe(map[string]string{envDatabase: "mysql://smyklot@db.internal/smyklot"})
			Expect(err).To(MatchError(ErrStateConfig))
		})

		DescribeTable("should reject incomplete enabled panel configuration",
			func(name string) {
				env := map[string]string{
					envPanelOrigin:       "https://smyklot.example",
					envPanelSuperRootID:  "42",
					envPanelClientID:     "Ov23li.panel",
					envPanelClientSecret: "oauth-secret",
				}
				env[name] = ""

				_, err := loadServe(env)
				Expect(err).To(MatchError(ErrPanelConfig))
			},
			Entry("without a Super Root ID", envPanelSuperRootID),
			Entry("without an OAuth client ID", envPanelClientID),
			Entry("without an OAuth secret", envPanelClientSecret),
		)

		DescribeTable("should reject unsafe panel settings",
			func(env map[string]string) {
				for name, value := range enabledPanel {
					if _, present := env[name]; !present {
						env[name] = value
					}
				}

				_, err := loadServe(env)
				Expect(err).To(MatchError(ContainSubstring(ErrPanelConfig.Error())))
			},
			Entry("with a non-positive session TTL", map[string]string{envPanelTTL: "0"}),
			Entry("with a non-positive Super Root ID", map[string]string{envPanelSuperRootID: "-1"}),
			Entry("with a non-numeric Super Root ID", map[string]string{envPanelSuperRootID: "root"}),
			Entry("at the webhook route", map[string]string{envPanelBase: defaultWebhookPath}),
			Entry("at the health route", map[string]string{envPanelBase: healthPath}),
			Entry("at the schema route", map[string]string{envPanelBase: schemaRoot}),
			Entry("at a schema document route", map[string]string{
				envPanelBase: schemaRoot + "/team",
			}),
		)
	})
})
