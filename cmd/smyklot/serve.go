package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/panel"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage/open"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/logging"
)

const (
	flagListen               = "listen"
	flagAdminListen          = "admin-listen"
	flagWebhookPath          = "webhook-path"
	flagPollInterval         = "poll-interval"
	flagPendingCIQuietPeriod = "pending-ci-quiet-period"
	flagPathIndexInterval    = "path-index-interval"
	flagLogFormat            = "log-format"
	flagLogLevel             = "log-level"
	flagDatabase             = "database-url"
	flagState                = "state-path"
	flagPanelOrigin          = "panel-public-origin"
	flagPanelBase            = "panel-base-path"
	flagPanelState           = "panel-state-path" // Deprecated compatibility alias.
	flagPanelSuperRootID     = "panel-super-root-id"
	flagPanelTTL             = "panel-session-ttl"

	descListen               = "Address to listen on"
	descAdminListen          = "Address to serve probes, metrics and recent failures on"
	descWebhookPath          = "Path GitHub delivers webhooks to"
	descPollInterval         = "How often to sweep reactions and pending-CI PRs (0 disables)"
	descPendingCIQuietPeriod = "How long CI must remain unchanged and passing before merge"
	descPathIndexInterval    = "How often a repository's file list is checked for changes"
	descLogFormat            = "Log format: json or text"
	descLogLevel             = "Log level: debug, info, warn or error"
	descDatabase             = "Database to store service state in: a postgres:// URL or a file path"
	descState                = "Deprecated alias for --database-url"
	descPanelOrigin          = "Public origin for the panel (empty disables it)"
	descPanelBase            = "Path subtree that serves the panel"
	descPanelState           = "Deprecated alias for --database-url"
	descPanelSuperRootID     = "Numeric GitHub user ID assigned as the panel Super Root"
	descPanelTTL             = "How long a signed-in panel session remains valid"

	envListenAddress        = "SMYKLOT_LISTEN_ADDRESS"
	envAdminAddress         = "SMYKLOT_ADMIN_ADDRESS"
	envWebhookPath          = "SMYKLOT_WEBHOOK_PATH"
	envWebhookSecret        = "SMYKLOT_WEBHOOK_SECRET" //nolint:gosec // Environment variable name, not a credential
	envPollInterval         = "SMYKLOT_POLL_INTERVAL"
	envPendingCIQuietPeriod = "SMYKLOT_PENDING_CI_QUIET_PERIOD"
	envPathIndexInterval    = "SMYKLOT_PATH_INDEX_INTERVAL"
	envLogFormat            = "SMYKLOT_LOG_FORMAT"
	envLogLevel             = "SMYKLOT_LOG_LEVEL"
	envDatabase             = "SMYKLOT_DATABASE_URL"
	envState                = "SMYKLOT_STATE_PATH"
	envPanelOrigin          = "SMYKLOT_PANEL_PUBLIC_ORIGIN"
	envPanelBase            = "SMYKLOT_PANEL_BASE_PATH"
	envPanelState           = "SMYKLOT_PANEL_STATE_PATH" // Deprecated compatibility alias.
	envPanelSuperRootID     = "SMYKLOT_PANEL_SUPER_ROOT_ID"
	envPanelTTL             = "SMYKLOT_PANEL_SESSION_TTL"
	envGitHubAuthURL        = "SMYKLOT_GITHUB_AUTHORIZE_URL"
	envGitHubTokenURL       = "SMYKLOT_GITHUB_TOKEN_URL" //nolint:gosec // Environment variable name, not a credential

	// Panel sign-in deliberately does not reuse the App's OAuth credentials.
	// Authorizing a GitHub App shows the consent screen the App registration
	// asks for, so signing in to read a dashboard listed the permissions the
	// bot needs to approve and merge pull requests. Nothing the client sends
	// trims that list: a GitHub App ignores the scope parameter. A separate
	// classic OAuth App does honour it, and the panel asks for no scope, so
	// the screen offers public profile read and nothing else
	envPanelClientID     = "SMYKLOT_PANEL_CLIENT_ID"
	envPanelClientSecret = "SMYKLOT_PANEL_CLIENT_SECRET" //nolint:gosec // Environment variable name, not a credential

	defaultListenAddress        = ":8080"
	defaultAdminAddress         = ":9090"
	defaultWebhookPath          = "/webhook"
	defaultPollInterval         = 5 * time.Minute
	defaultPendingCIQuietPeriod = pendingci.DefaultPassingQuiet

	// defaultPathIndexInterval is how often a repository's file list is checked
	// for changes, before an installation or a repository says otherwise.
	//
	// An hour. What a check costs is the commit the default branch points at -
	// a few hundred bytes whatever the repository holds - and the list itself
	// is read only where that moved. It was a day when every check meant
	// reading the whole tree.
	defaultPathIndexInterval = time.Hour

	// defaultLogFormat is JSON because the service's lines are read by a query,
	// not by a person. The Action keeps writing for a person to read
	defaultLogFormat = string(logging.FormatJSON)

	defaultLogLevel       = "info"
	defaultPanelBase      = "/panel"
	defaultState          = "/var/lib/smyklot/panel.sqlite3"
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

	// ErrInvalidPendingCIQuietPeriod is returned for an invalid stability window.
	ErrInvalidPendingCIQuietPeriod = errors.New("invalid pending-CI quiet period")

	// ErrInvalidPathIndexInterval is returned for an unusable refresh interval.
	ErrInvalidPathIndexInterval = errors.New("invalid path index interval")

	// ErrStateConfig is returned when mandatory durable state cannot be configured.
	ErrStateConfig = errors.New("invalid service state configuration")

	// ErrAddressConflict is returned when the admin listener would bind the
	// same address as the webhook listener, which would publish everything the
	// admin listener exists to keep private
	ErrAddressConflict = errors.New("admin address must differ from the listen address")

	// ErrPanelConfig is returned when the enabled panel lacks a required public
	// URL, Super Root identity, state path, or OAuth credential.
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
JWT authentication), and SMYKLOT_WEBHOOK_SECRET.

The panel signs users in through its own classic OAuth App, registered apart
from the bot's GitHub App so that reading a dashboard does not ask anyone to
grant the permissions the bot approves and merges with. Enabling the panel
therefore also requires SMYKLOT_PANEL_CLIENT_ID and SMYKLOT_PANEL_CLIENT_SECRET.`,
	RunE: runServe,
}

func init() {
	serveCmd.Flags().String(flagListen, defaultListenAddress, descListen)
	serveCmd.Flags().String(flagAdminListen, defaultAdminAddress, descAdminListen)
	serveCmd.Flags().String(flagWebhookPath, defaultWebhookPath, descWebhookPath)
	serveCmd.Flags().Duration(flagPollInterval, defaultPollInterval, descPollInterval)
	serveCmd.Flags().Duration(
		flagPendingCIQuietPeriod,
		defaultPendingCIQuietPeriod,
		descPendingCIQuietPeriod,
	)
	serveCmd.Flags().Duration(
		flagPathIndexInterval,
		defaultPathIndexInterval,
		descPathIndexInterval,
	)
	serveCmd.Flags().String(flagLogFormat, defaultLogFormat, descLogFormat)
	serveCmd.Flags().String(flagLogLevel, defaultLogLevel, descLogLevel)
	serveCmd.Flags().String(flagDatabase, defaultState, descDatabase)
	serveCmd.Flags().String(flagState, "", descState)
	_ = serveCmd.Flags().MarkDeprecated(flagState, "use --database-url")
	serveCmd.Flags().String(flagPanelOrigin, "", descPanelOrigin)
	serveCmd.Flags().String(flagPanelBase, defaultPanelBase, descPanelBase)
	serveCmd.Flags().String(flagPanelState, "", descPanelState)
	_ = serveCmd.Flags().MarkDeprecated(flagPanelState, "use --database-url")
	serveCmd.Flags().Int64(flagPanelSuperRootID, 0, descPanelSuperRootID)
	serveCmd.Flags().Duration(flagPanelTTL, defaultPanelTTL, descPanelTTL)

	// The service resolves the same settings from the same layers the Action
	// does, so it takes the same flags
	config.RegisterFlags(serveCmd.Flags())

	rootCmd.AddCommand(serveCmd)
}

// serveConfig is everything the service needs to start.
type serveConfig struct {
	listenAddress        string
	webhookPath          string
	webhookSecret        []byte
	pollInterval         time.Duration
	pendingCIQuietPeriod time.Duration
	pathIndexInterval    time.Duration

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

	// database is mandatory even when the panel is disabled. It owns webhook
	// delivery identity and pending-CI state as well as optional panel data.
	// Which engine it names is the storage layer's business, not this one's.
	database string

	panel *panelServeConfig
}

type panelServeConfig struct {
	publicOrigin string
	basePath     string
	superRootID  int64
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
	botConfig, err := loadBotConfig(cmd)
	if err != nil {
		return nil, err
	}

	cfg := &serveConfig{
		webhookSecret: []byte(os.Getenv(envWebhookSecret)),
		apiBaseURL:    os.Getenv(bot.EnvAPIBaseURL),
		botUsername:   os.Getenv(bot.EnvBotUsername),
		appClientID:   os.Getenv(bot.EnvGitHubAppClientID),
		appPrivateKey: []byte(os.Getenv(bot.EnvGitHubAppPrivateKey)),
		botConfig:     botConfig,
	}

	if cfg.appClientID == "" {
		cfg.appClientID = os.Getenv(bot.EnvGitHubAppID)
	}

	if cfg.botUsername == "" {
		cfg.botUsername = bot.DefaultBotUsername
	}

	if len(cfg.webhookSecret) == 0 {
		return nil, ErrNoWebhookSecret
	}

	if err := applyServeFlags(cmd, cfg); err != nil {
		return nil, err
	}
	if err := applyDatabase(cmd, cfg); err != nil {
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
	superRootID, err := cmd.Flags().GetInt64(flagPanelSuperRootID)
	if err != nil {
		return err
	}
	superRootID, err = flagOrEnvInt64(
		cmd, flagPanelSuperRootID, superRootID, envPanelSuperRootID,
	)
	if err != nil {
		return fmt.Errorf("%w: invalid Super Root ID", ErrPanelConfig)
	}
	ttl, err := cmd.Flags().GetDuration(flagPanelTTL)
	if err != nil {
		return err
	}
	ttl, err = flagOrEnvDuration(cmd, flagPanelTTL, ttl, envPanelTTL, ErrPanelConfig)
	if err != nil {
		return fmt.Errorf("%w: invalid session TTL", ErrPanelConfig)
	}

	cfg.panel = &panelServeConfig{
		publicOrigin: origin,
		basePath:     normalizePanelBasePath(flagOrEnv(cmd, flagPanelBase, basePath, envPanelBase)),
		superRootID:  superRootID,
		clientID:     strings.TrimSpace(os.Getenv(envPanelClientID)),
		clientSecret: os.Getenv(envPanelClientSecret),
		authorizeURL: envOrDefault(envGitHubAuthURL, defaultGitHubAuthURL),
		tokenURL:     envOrDefault(envGitHubTokenURL, defaultGitHubTokenURL),
		sessionTTL:   ttl,
	}
	if cfg.panel.clientID == "" || strings.TrimSpace(cfg.panel.clientSecret) == "" {
		return fmt.Errorf(
			"%w: panel sign-in needs %s and %s from a classic OAuth App, not the App's own credentials",
			ErrPanelConfig, envPanelClientID, envPanelClientSecret,
		)
	}
	if cfg.panel.superRootID <= 0 || ttl <= 0 {
		return ErrPanelConfig
	}
	if cfg.panel.basePath == cfg.webhookPath ||
		cfg.panel.basePath == healthPath ||
		cfg.panel.basePath == schemaRoot ||
		path.Dir(cfg.panel.basePath) == schemaRoot {
		return fmt.Errorf("%w: panel base path conflicts with a public service route", ErrPanelConfig)
	}

	return nil
}

// applyDatabase resolves where service state lives.
//
// The newest spelling wins, then its environment variable, then each older
// spelling in turn, so a deployment that was configured before a second engine
// existed keeps working untouched. Every older spelling named a file path,
// which still selects SQLite.
func applyDatabase(cmd *cobra.Command, cfg *serveConfig) error {
	database, err := cmd.Flags().GetString(flagDatabase)
	if err != nil {
		return err
	}
	statePath, err := cmd.Flags().GetString(flagState)
	if err != nil {
		return err
	}
	legacyPath, err := cmd.Flags().GetString(flagPanelState)
	if err != nil {
		return err
	}

	switch {
	case cmd.Flags().Changed(flagDatabase):
		cfg.database = database
	case strings.TrimSpace(os.Getenv(envDatabase)) != "":
		cfg.database = os.Getenv(envDatabase)
	case cmd.Flags().Changed(flagState):
		cfg.database = statePath
	case strings.TrimSpace(os.Getenv(envState)) != "":
		cfg.database = os.Getenv(envState)
	case cmd.Flags().Changed(flagPanelState):
		cfg.database = legacyPath
	case strings.TrimSpace(os.Getenv(envPanelState)) != "":
		cfg.database = os.Getenv(envPanelState)
	default:
		cfg.database = database
	}
	if strings.TrimSpace(cfg.database) == "" {
		return fmt.Errorf("%w: database must not be empty", ErrStateConfig)
	}
	// Failing here rather than at connect time means a typo is reported with
	// the rest of the configuration instead of after the listener is up.
	if _, _, err := open.Resolve(cfg.database); err != nil {
		return fmt.Errorf("%w: %w", ErrStateConfig, err)
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

	cfg.pollInterval, err = flagOrEnvDuration(
		cmd, flagPollInterval, interval, envPollInterval, ErrInvalidPollInterval,
	)
	if err != nil {
		return err
	}
	quietPeriod, err := cmd.Flags().GetDuration(flagPendingCIQuietPeriod)
	if err != nil {
		return err
	}
	cfg.pendingCIQuietPeriod, err = flagOrEnvDuration(
		cmd,
		flagPendingCIQuietPeriod,
		quietPeriod,
		envPendingCIQuietPeriod,
		ErrInvalidPendingCIQuietPeriod,
	)
	if err != nil {
		return err
	}
	if cfg.pendingCIQuietPeriod < pendingci.MinPassingQuiet ||
		cfg.pendingCIQuietPeriod > pendingci.MaxPassingQuiet ||
		(cfg.pendingCIQuietPeriod > 0 && cfg.pendingCIQuietPeriod < time.Second) {
		return ErrInvalidPendingCIQuietPeriod
	}
	pathIndex, err := cmd.Flags().GetDuration(flagPathIndexInterval)
	if err != nil {
		return err
	}
	cfg.pathIndexInterval, err = flagOrEnvDuration(
		cmd,
		flagPathIndexInterval,
		pathIndex,
		envPathIndexInterval,
		ErrInvalidPathIndexInterval,
	)
	if err != nil {
		return err
	}
	// Zero is every sweep, so only the ends are refused.
	if cfg.pathIndexInterval < 0 || cfg.pathIndexInterval > panel.MaxPathIndexInterval {
		return ErrInvalidPathIndexInterval
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
		return bot.NewInputError(logging.ErrUnknownLogFormat, rawFormat, err.Error())
	}

	level, err := cmd.Flags().GetString(flagLogLevel)
	if err != nil {
		return err
	}

	rawLevel := flagOrEnv(cmd, flagLogLevel, level, envLogLevel)

	cfg.logLevel, err = logging.ParseLevel(rawLevel)
	if err != nil {
		return bot.NewInputError(logging.ErrUnknownLogLevel, rawLevel, err.Error())
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
	invalid error,
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
		return 0, bot.NewInputError(invalid, raw, err.Error())
	}

	return parsed, nil
}

// flagOrEnvInt64 resolves an integer setting under flagOrEnv's precedence.
func flagOrEnvInt64(
	cmd *cobra.Command,
	flagName string,
	flagValue int64,
	envVar string,
) (int64, error) {
	if cmd.Flags().Changed(flagName) {
		return flagValue, nil
	}

	raw := strings.TrimSpace(os.Getenv(envVar))
	if raw == "" {
		return flagValue, nil
	}

	return strconv.ParseInt(raw, 10, 64)
}
