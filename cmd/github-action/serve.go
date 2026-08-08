package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/logging"
)

const (
	flagListen       = "listen"
	flagAdminListen  = "admin-listen"
	flagWebhookPath  = "webhook-path"
	flagPollInterval = "poll-interval"
	flagLogFormat    = "log-format"
	flagLogLevel     = "log-level"
	flagPanelOrigin  = "panel-public-origin"
	flagPanelBase    = "panel-base-path"
	flagPanelState   = "panel-state-path"
	flagPanelOwner   = "panel-owner"
	flagPanelTTL     = "panel-session-ttl"

	descListen       = "Address to listen on"
	descAdminListen  = "Address to serve probes, metrics and recent failures on"
	descWebhookPath  = "Path GitHub delivers webhooks to"
	descPollInterval = "How often to sweep reactions and pending-CI PRs (0 disables)"
	descLogFormat    = "Log format: json or text"
	descLogLevel     = "Log level: debug, info, warn or error"
	descPanelOrigin  = "Public origin for the panel (empty disables it)"
	descPanelBase    = "Path subtree that serves the panel"
	descPanelState   = "Path to the panel SQLite database"
	descPanelOwner   = "GitHub login allowed to claim panel ownership"
	descPanelTTL     = "How long a signed-in panel session remains valid"

	envListenAddress  = "SMYKLOT_LISTEN_ADDRESS"
	envAdminAddress   = "SMYKLOT_ADMIN_ADDRESS"
	envWebhookPath    = "SMYKLOT_WEBHOOK_PATH"
	envWebhookSecret  = "SMYKLOT_WEBHOOK_SECRET" //nolint:gosec // Environment variable name, not a credential
	envPollInterval   = "SMYKLOT_POLL_INTERVAL"
	envLogFormat      = "SMYKLOT_LOG_FORMAT"
	envLogLevel       = "SMYKLOT_LOG_LEVEL"
	envPanelOrigin    = "SMYKLOT_PANEL_PUBLIC_ORIGIN"
	envPanelBase      = "SMYKLOT_PANEL_BASE_PATH"
	envPanelState     = "SMYKLOT_PANEL_STATE_PATH"
	envPanelOwner     = "SMYKLOT_PANEL_OWNER"
	envPanelTTL       = "SMYKLOT_PANEL_SESSION_TTL"
	envAppSecret      = "GITHUB_APP_CLIENT_SECRET" //nolint:gosec // Environment variable name, not a credential
	envGitHubAuthURL  = "SMYKLOT_GITHUB_AUTHORIZE_URL"
	envGitHubTokenURL = "SMYKLOT_GITHUB_TOKEN_URL" //nolint:gosec // Environment variable name, not a credential

	defaultListenAddress = ":8080"
	defaultAdminAddress  = ":9090"
	defaultWebhookPath   = "/webhook"
	defaultPollInterval  = 5 * time.Minute

	// defaultLogFormat is JSON because the service's lines are read by a query,
	// not by a person. The Action keeps writing for a person to read
	defaultLogFormat = string(logging.FormatJSON)

	defaultLogLevel       = "info"
	defaultPanelBase      = "/panel"
	defaultPanelState     = "/var/lib/smyklot/panel.sqlite3"
	defaultPanelTTL       = 12 * time.Hour
	defaultGitHubAPIURL   = "https://api.github.com"
	defaultGitHubAuthURL  = "https://github.com/login/oauth/authorize"
	defaultGitHubTokenURL = "https://github.com/login/oauth/access_token" //nolint:gosec // Public OAuth endpoint, not a credential

	// healthPath answers a liveness probe on the public listener, for an
	// ingress or a tunnel that needs one path it can reach. Everything an
	// operator reads is on the admin listener instead
	healthPath = "/healthz"
)

// Sentinel errors for service configuration.
var (
	version = "dev"

	// ErrNoWebhookSecret is returned when no webhook secret is configured.
	// Without one, any caller that can reach the port could drive the bot
	ErrNoWebhookSecret = errors.New("no webhook secret configured")

	// ErrInvalidWebhookPath is returned when the webhook path is not absolute
	ErrInvalidWebhookPath = errors.New("webhook path must start with /")

	// ErrInvalidPollInterval is returned when the poll interval is unparseable
	ErrInvalidPollInterval = errors.New("invalid poll interval")

	// ErrAddressConflict is returned when the admin listener would bind the
	// same address as the webhook listener, which would publish everything the
	// admin listener exists to keep private
	ErrAddressConflict = errors.New("admin address must differ from the listen address")

	// ErrPanelConfig is returned when the enabled panel lacks a required
	// public URL, owner, state path, or OAuth credential.
	ErrPanelConfig = errors.New("invalid panel configuration")
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

Probes, metrics and recent failures are served on a second port, which is not
meant to be public: /livez, /readyz, /metrics and /failures.

Requires GitHub App credentials and a webhook secret in the environment:
GITHUB_APP_PRIVATE_KEY, GITHUB_APP_CLIENT_ID (or GITHUB_APP_ID for service-only
JWT authentication), and SMYKLOT_WEBHOOK_SECRET.`,
	RunE: runServe,
}

func init() {
	serveCmd.Flags().String(flagListen, defaultListenAddress, descListen)
	serveCmd.Flags().String(flagAdminListen, defaultAdminAddress, descAdminListen)
	serveCmd.Flags().String(flagWebhookPath, defaultWebhookPath, descWebhookPath)
	serveCmd.Flags().Duration(flagPollInterval, defaultPollInterval, descPollInterval)
	serveCmd.Flags().String(flagLogFormat, defaultLogFormat, descLogFormat)
	serveCmd.Flags().String(flagLogLevel, defaultLogLevel, descLogLevel)
	serveCmd.Flags().String(flagPanelOrigin, "", descPanelOrigin)
	serveCmd.Flags().String(flagPanelBase, defaultPanelBase, descPanelBase)
	serveCmd.Flags().String(flagPanelState, defaultPanelState, descPanelState)
	serveCmd.Flags().String(flagPanelOwner, "", descPanelOwner)
	serveCmd.Flags().Duration(flagPanelTTL, defaultPanelTTL, descPanelTTL)

	rootCmd.AddCommand(serveCmd)
}

// serveConfig is everything the service needs to start.
type serveConfig struct {
	listenAddress string
	webhookPath   string
	webhookSecret []byte
	pollInterval  time.Duration

	// adminAddress carries the probes, metrics and recent failures, on its own
	// port because the webhook port is reachable from the internet
	adminAddress string

	logFormat logging.Format
	logLevel  slog.Level

	// logWriter is where log lines go; nil means standard output
	logWriter io.Writer

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

	panel *panelServeConfig
}

type panelServeConfig struct {
	publicOrigin string
	basePath     string
	statePath    string
	ownerLogin   string
	clientID     string
	clientSecret string
	authorizeURL string
	tokenURL     string
	sessionTTL   time.Duration
}

func runServe(cmd *cobra.Command, _ []string) (runErr error) {
	cfg, err := loadServeConfig(cmd)
	if err != nil {
		return err
	}

	server, err := newServer(cfg)
	if err != nil {
		return err
	}
	defer func() {
		runErr = errors.Join(runErr, server.Close())
	}()

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
	if err := applyPanelFlags(cmd, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func applyPanelFlags(cmd *cobra.Command, cfg *serveConfig) error {
	origin, err := cmd.Flags().GetString(flagPanelOrigin)
	if err != nil {
		return err
	}
	origin = flagOrEnv(cmd, flagPanelOrigin, origin, envPanelOrigin)
	if strings.TrimSpace(origin) == "" {
		return nil
	}
	basePath, err := cmd.Flags().GetString(flagPanelBase)
	if err != nil {
		return err
	}
	statePath, err := cmd.Flags().GetString(flagPanelState)
	if err != nil {
		return err
	}
	owner, err := cmd.Flags().GetString(flagPanelOwner)
	if err != nil {
		return err
	}
	ttl, err := cmd.Flags().GetDuration(flagPanelTTL)
	if err != nil {
		return err
	}
	ttl, err = flagOrEnvDuration(cmd, flagPanelTTL, ttl, envPanelTTL)
	if err != nil {
		return fmt.Errorf("%w: invalid session TTL", ErrPanelConfig)
	}

	cfg.panel = &panelServeConfig{
		publicOrigin: origin,
		basePath:     normalizePanelBasePath(flagOrEnv(cmd, flagPanelBase, basePath, envPanelBase)),
		statePath:    flagOrEnv(cmd, flagPanelState, statePath, envPanelState),
		ownerLogin:   flagOrEnv(cmd, flagPanelOwner, owner, envPanelOwner),
		clientID:     strings.TrimSpace(os.Getenv(envGitHubAppClientID)),
		clientSecret: os.Getenv(envAppSecret),
		authorizeURL: envOrDefault(envGitHubAuthURL, defaultGitHubAuthURL),
		tokenURL:     envOrDefault(envGitHubTokenURL, defaultGitHubTokenURL),
		sessionTTL:   ttl,
	}
	if strings.TrimSpace(cfg.panel.statePath) == "" ||
		strings.TrimSpace(cfg.panel.ownerLogin) == "" ||
		cfg.panel.clientID == "" ||
		strings.TrimSpace(cfg.panel.clientSecret) == "" || ttl <= 0 {
		return ErrPanelConfig
	}
	if cfg.panel.basePath == cfg.webhookPath || cfg.panel.basePath == healthPath {
		return fmt.Errorf("%w: panel base path conflicts with a public service route", ErrPanelConfig)
	}

	return nil
}

func normalizePanelBasePath(basePath string) string {
	basePath = strings.TrimSpace(basePath)
	if basePath == "/" {
		return ""
	}

	return basePath
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}

	return fallback
}

// applyServeFlags layers flags and their environment fallbacks onto cfg.
func applyServeFlags(cmd *cobra.Command, cfg *serveConfig) error {
	listen, err := cmd.Flags().GetString(flagListen)
	if err != nil {
		return err
	}

	cfg.listenAddress = flagOrEnv(cmd, flagListen, listen, envListenAddress)

	adminListen, err := cmd.Flags().GetString(flagAdminListen)
	if err != nil {
		return err
	}

	cfg.adminAddress = flagOrEnv(cmd, flagAdminListen, adminListen, envAdminAddress)
	if addressesConflict(cfg.adminAddress, cfg.listenAddress) {
		return ErrAddressConflict
	}

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
	if err != nil {
		return err
	}

	return applyLogFlags(cmd, cfg)
}

// applyLogFlags resolves how the service writes its log lines.
func applyLogFlags(cmd *cobra.Command, cfg *serveConfig) error {
	format, err := cmd.Flags().GetString(flagLogFormat)
	if err != nil {
		return err
	}

	rawFormat := flagOrEnv(cmd, flagLogFormat, format, envLogFormat)

	cfg.logFormat, err = logging.ParseFormat(rawFormat)
	if err != nil {
		return NewInputError(logging.ErrUnknownLogFormat, rawFormat, err.Error())
	}

	level, err := cmd.Flags().GetString(flagLogLevel)
	if err != nil {
		return err
	}

	rawLevel := flagOrEnv(cmd, flagLogLevel, level, envLogLevel)

	cfg.logLevel, err = logging.ParseLevel(rawLevel)
	if err != nil {
		return NewInputError(logging.ErrUnknownLogLevel, rawLevel, err.Error())
	}

	return nil
}

// addressesConflict reports whether two listen addresses want the same socket.
//
// Comparing the two strings would miss the several ways one address is written:
// ":8080", "0.0.0.0:8080" and "[::]:8080" all take every interface. Getting it
// wrong costs a bind failure at startup rather than a leak, but a service that
// explains the misconfiguration beats one that reports "address already in use"
// and leaves the operator to work out which of its two listeners lost.
//
// Nothing is resolved here. This runs while flags are being read, and a name
// lookup would make starting up depend on the network.
func addressesConflict(a, b string) bool {
	hostA, portA, errA := net.SplitHostPort(a)
	hostB, portB, errB := net.SplitHostPort(b)

	// Not addresses this can reason about, so the text is all there is to go on
	if errA != nil || errB != nil {
		return a == b
	}

	// Port zero asks the kernel for whatever is free, and it hands out a
	// different port to each caller
	if portA == "0" || portB == "0" {
		return false
	}

	if portA != portB {
		return false
	}

	// A wildcard host takes every interface, so it collides with anything else
	// sharing its port
	return hostA == hostB || wildcardHost(hostA) || wildcardHost(hostB)
}

// wildcardHost reports whether a host binds every interface.
func wildcardHost(host string) bool {
	switch host {
	case "", "0.0.0.0", "::":
		return true

	default:
		return false
	}
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
