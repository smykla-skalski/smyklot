// Package panel serves Smyklot's authenticated administration panel.
package panel

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

const (
	DefaultBasePath   = "/panel"
	DefaultSessionTTL = 12 * time.Hour
	DefaultStateTTL   = 10 * time.Minute
	DefaultPageSize   = 20
	MaxPageSize       = 100
	MaxSessions       = 5
	httpScheme        = "http"
	httpsScheme       = "https"
)

var errInvalidConfig = errors.New("invalid panel configuration")

// Config contains validated runtime settings for the panel HTTP surface.
type Config struct {
	BasePath      string
	PublicOrigin  string
	OwnerLogin    string
	ClientID      string
	ClientSecret  string
	AuthorizeURL  string
	TokenURL      string
	APIURL        string
	Version       string
	ServiceHost   string
	SessionTTL    time.Duration
	StateTTL      time.Duration
	ProcessConfig *config.Config
	Assets        fs.FS
}

func (c Config) validated() (Config, error) {
	basePath, err := normalizeBasePath(c.BasePath)
	if err != nil {
		return Config{}, err
	}
	c.BasePath = basePath

	c.PublicOrigin, err = normalizeOrigin(c.PublicOrigin)
	if err != nil {
		return Config{}, err
	}

	for label, value := range map[string]string{
		"owner login":   c.OwnerLogin,
		"client id":     c.ClientID,
		"client secret": c.ClientSecret,
	} {
		if strings.TrimSpace(value) == "" {
			return Config{}, fmt.Errorf("%w: %s must not be blank", errInvalidConfig, label)
		}
	}

	if c.SessionTTL == 0 {
		c.SessionTTL = DefaultSessionTTL
	}
	if c.StateTTL == 0 {
		c.StateTTL = DefaultStateTTL
	}
	if c.SessionTTL < time.Minute || c.StateTTL < time.Minute {
		return Config{}, fmt.Errorf("%w: authentication TTLs must be at least one minute", errInvalidConfig)
	}
	if c.ProcessConfig == nil {
		c.ProcessConfig = config.Default()
	}
	if c.Assets == nil {
		return Config{}, fmt.Errorf("%w: panel assets are required", errInvalidConfig)
	}

	for label, value := range map[string]string{
		"authorization endpoint": c.AuthorizeURL,
		"token endpoint":         c.TokenURL,
		"API endpoint":           c.APIURL,
	} {
		parsed, parseErr := url.Parse(value)
		if parseErr != nil || parsed.Scheme == "" || parsed.Host == "" {
			return Config{}, fmt.Errorf("%w: %s must be an absolute URL", errInvalidConfig, label)
		}
	}

	return c, nil
}

func normalizeBasePath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "/" {
		return "", nil
	}
	if !strings.HasPrefix(trimmed, "/") || strings.Contains(trimmed, "//") {
		return "", fmt.Errorf("%w: panel base path must be an absolute path", errInvalidConfig)
	}
	if strings.ContainsAny(trimmed, "?#{}*\\\"'<>;&`^") ||
		strings.ContainsFunc(trimmed, func(r rune) bool { return r <= ' ' || r > '~' }) {
		return "", fmt.Errorf("%w: panel base path contains unsafe characters", errInvalidConfig)
	}
	for _, segment := range strings.Split(trimmed, "/") {
		if segment == "." || segment == ".." {
			return "", fmt.Errorf("%w: panel base path contains traversal", errInvalidConfig)
		}
	}

	return strings.TrimRight(trimmed, "/"), nil
}

func normalizeOrigin(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("%w: public origin must be an absolute URL", errInvalidConfig)
	}
	if parsed.Scheme != httpScheme && parsed.Scheme != httpsScheme {
		return "", fmt.Errorf("%w: public origin must use HTTP or HTTPS", errInvalidConfig)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Path != "" && parsed.Path != "/") {
		return "", fmt.Errorf("%w: public origin must not contain credentials, a path, query, or fragment", errInvalidConfig)
	}

	return strings.TrimRight(parsed.String(), "/"), nil
}

func (c Config) callbackURL() string {
	return c.PublicOrigin + c.BasePath + "/auth/github/callback"
}

func (c Config) landingPath() string {
	if c.BasePath == "" {
		return "/"
	}

	return c.BasePath + "/"
}

func (c Config) cookiePath() string {
	if c.BasePath == "" {
		return "/"
	}

	return c.BasePath
}
