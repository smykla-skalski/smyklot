package main

import (
	"errors"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

const (
	flagListen       = "listen"
	flagWebhookPath  = "webhook-path"
	flagPollInterval = "poll-interval"

	descListen       = "Address to listen on"
	descWebhookPath  = "Path GitHub delivers webhooks to"
	descPollInterval = "How often to sweep reactions and pending-CI PRs (0 disables)"

	envListenAddress = "SMYKLOT_LISTEN_ADDRESS"
	envWebhookPath   = "SMYKLOT_WEBHOOK_PATH"
	envWebhookSecret = "SMYKLOT_WEBHOOK_SECRET" //nolint:gosec // Environment variable name, not a credential
	envPollInterval  = "SMYKLOT_POLL_INTERVAL"

	defaultListenAddress = ":8080"
	defaultWebhookPath   = "/webhook"
	defaultPollInterval  = 5 * time.Minute

	// healthPath answers liveness probes. Readiness, metrics, and structured
	// logging are a separate concern and land with them
	healthPath = "/healthz"
)

// Sentinel errors for service configuration.
var (
	// ErrNoWebhookSecret is returned when no webhook secret is configured.
	// Without one, any caller that can reach the port could drive the bot
	ErrNoWebhookSecret = errors.New("no webhook secret configured")

	// ErrInvalidWebhookPath is returned when the webhook path is not absolute
	ErrInvalidWebhookPath = errors.New("webhook path must start with /")

	// ErrInvalidPollInterval is returned when the poll interval is unparseable
	ErrInvalidPollInterval = errors.New("invalid poll interval")
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run as a webhook-driven service",
	Long: `Run Smyklot as a long-running service.

The service accepts GitHub webhook deliveries and executes the same commands
the Action executes, for every repository the App is installed on. Each
delivery names its own installation, so no repository needs a workflow file or
any per-repository configuration.

GitHub sends no webhook when someone adds or removes a reaction, so reaction
commands are still found by sweeping open pull requests on an interval. The
same sweep merges pull requests that were waiting for CI.

Requires GitHub App credentials and a webhook secret in the environment:
GITHUB_APP_PRIVATE_KEY, GITHUB_APP_CLIENT_ID (or GITHUB_APP_ID), and
SMYKLOT_WEBHOOK_SECRET.`,
	RunE: runServe,
}

func init() {
	serveCmd.Flags().String(flagListen, defaultListenAddress, descListen)
	serveCmd.Flags().String(flagWebhookPath, defaultWebhookPath, descWebhookPath)
	serveCmd.Flags().Duration(flagPollInterval, defaultPollInterval, descPollInterval)

	rootCmd.AddCommand(serveCmd)
}

// serveConfig is everything the service needs to start.
type serveConfig struct {
	listenAddress string
	webhookPath   string
	webhookSecret []byte
	pollInterval  time.Duration

	// apiBaseURL points at a GitHub Enterprise instance; empty uses public
	// GitHub
	apiBaseURL string

	// botUsername identifies the bot's own reviews and comments
	botUsername string

	// appClientID is the App's client ID, or its numeric app ID
	appClientID string

	// appPrivateKey signs the JWTs installation tokens are minted with
	appPrivateKey []byte

	// botConfig is the process-wide default, which each repository's
	// .github/smyklot.yaml is layered over
	botConfig *config.Config
}

func runServe(cmd *cobra.Command, _ []string) error {
	cfg, err := loadServeConfig(cmd)
	if err != nil {
		return err
	}

	server, err := newServer(cfg)
	if err != nil {
		return err
	}

	// A rolling update sends SIGTERM, and dropping a delivery that is already
	// executing would leave the pull request half-handled
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return server.Run(ctx)
}

// loadServeConfig reads the service's configuration from flags and environment.
//
// Everything it can reject, it rejects here: a service that starts without a
// webhook secret would accept unsigned deliveries for as long as nobody
// noticed.
func loadServeConfig(cmd *cobra.Command) (*serveConfig, error) {
	v := viper.New()
	config.SetupViper(v)

	botConfig, err := loadPollBotConfig(v)
	if err != nil {
		return nil, err
	}

	cfg := &serveConfig{
		webhookSecret: []byte(os.Getenv(envWebhookSecret)),
		apiBaseURL:    os.Getenv(envAPIBaseURL),
		botUsername:   os.Getenv(envBotUsername),
		appClientID:   os.Getenv(envGitHubAppClientID),
		appPrivateKey: []byte(os.Getenv(envGitHubAppPrivateKey)),
		botConfig:     botConfig,
	}

	if cfg.appClientID == "" {
		cfg.appClientID = os.Getenv(envGitHubAppID)
	}

	if cfg.botUsername == "" {
		cfg.botUsername = defaultBotUsername
	}

	if len(cfg.webhookSecret) == 0 {
		return nil, ErrNoWebhookSecret
	}

	if err := applyServeFlags(cmd, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// applyServeFlags layers flags and their environment fallbacks onto cfg.
func applyServeFlags(cmd *cobra.Command, cfg *serveConfig) error {
	listen, err := cmd.Flags().GetString(flagListen)
	if err != nil {
		return err
	}

	cfg.listenAddress = flagOrEnv(cmd, flagListen, listen, envListenAddress)

	webhookPath, err := cmd.Flags().GetString(flagWebhookPath)
	if err != nil {
		return err
	}

	cfg.webhookPath = flagOrEnv(cmd, flagWebhookPath, webhookPath, envWebhookPath)
	if !strings.HasPrefix(cfg.webhookPath, "/") {
		return ErrInvalidWebhookPath
	}

	interval, err := cmd.Flags().GetDuration(flagPollInterval)
	if err != nil {
		return err
	}

	cfg.pollInterval, err = flagOrEnvDuration(cmd, flagPollInterval, interval, envPollInterval)

	return err
}

// flagOrEnv resolves one setting, letting an explicit flag win over the
// environment and the environment win over the flag's default.
func flagOrEnv(cmd *cobra.Command, flagName, flagValue, envVar string) string {
	if cmd.Flags().Changed(flagName) {
		return flagValue
	}

	if fromEnv := os.Getenv(envVar); fromEnv != "" {
		return fromEnv
	}

	return flagValue
}

// flagOrEnvDuration resolves a duration setting under flagOrEnv's precedence.
func flagOrEnvDuration(
	cmd *cobra.Command,
	flagName string,
	flagValue time.Duration,
	envVar string,
) (time.Duration, error) {
	if cmd.Flags().Changed(flagName) {
		return flagValue, nil
	}

	raw := os.Getenv(envVar)
	if raw == "" {
		return flagValue, nil
	}

	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return 0, NewInputError(ErrInvalidPollInterval, raw, err.Error())
	}

	return parsed, nil
}
