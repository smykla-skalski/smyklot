package panel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"golang.org/x/oauth2"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

const maxIdentityFieldLength = 255

type signInProvider interface {
	AuthorizeURL(string) string
	ExchangeIdentity(context.Context, string) (storage.Account, error)
}

type githubSignIn struct {
	oauth    oauth2.Config
	apiURL   string
	provider string
}

func newGitHubSignIn(cfg Config) (*githubSignIn, error) {
	api, err := url.Parse(cfg.APIURL)
	if err != nil {
		return nil, fmt.Errorf("parse GitHub API URL: %w", err)
	}
	provider := "github:" + api.Scheme + "://" + api.Host + strings.TrimRight(api.Path, "/")

	// Scopes stays empty, and the credential must belong to a classic OAuth
	// App rather than to the GitHub App the bot acts as. An OAuth App honours
	// the scope parameter, so asking for nothing gets a consent screen that
	// offers public profile read alone, which is all the panel reads: one
	// GET /user, after which the token is discarded. A GitHub App ignores the
	// parameter and shows whatever its registration asks for instead, so
	// signing in there would ask a reader to grant write access to pull
	// requests and issues
	return &githubSignIn{
		oauth: oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.callbackURL(),
			Endpoint: oauth2.Endpoint{
				AuthURL:  cfg.AuthorizeURL,
				TokenURL: cfg.TokenURL,
			},
		},
		apiURL:   strings.TrimRight(cfg.APIURL, "/"),
		provider: provider,
	}, nil
}

func (g *githubSignIn) AuthorizeURL(state string) string {
	return g.oauth.AuthCodeURL(state)
}

func (g *githubSignIn) ExchangeIdentity(
	ctx context.Context,
	code string,
) (storage.Account, error) {
	token, err := g.oauth.Exchange(ctx, code)
	if err != nil {
		return storage.Account{}, fmt.Errorf("exchange GitHub authorization code: %w", err)
	}

	// apiURL is an absolute HTTP(S) endpoint validated when the panel starts;
	// allowing it to vary is required for GitHub Enterprise.
	request, err := http.NewRequestWithContext( //nolint:gosec
		ctx,
		http.MethodGet,
		g.apiURL+"/user",
		nil,
	)
	if err != nil {
		return storage.Account{}, fmt.Errorf("build GitHub profile request: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+token.AccessToken)
	request.Header.Set("User-Agent", "smyklot-panel")

	response, err := http.DefaultClient.Do(request) //nolint:gosec // Request URL is startup-validated.
	if err != nil {
		return storage.Account{}, fmt.Errorf("read GitHub profile: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return storage.Account{}, fmt.Errorf("read GitHub profile: endpoint returned %s", response.Status)
	}

	var profile struct {
		ID        int64   `json:"id"`
		Login     string  `json:"login"`
		Name      *string `json:"name"`
		AvatarURL *string `json:"avatar_url"`
	}
	decoder := json.NewDecoder(response.Body)
	if err := decoder.Decode(&profile); err != nil {
		return storage.Account{}, fmt.Errorf("decode GitHub profile: %w", err)
	}

	return g.account(profile.ID, profile.Login, profile.Name, profile.AvatarURL)
}

func (g *githubSignIn) account(
	subjectID int64,
	login string,
	name, avatarURL *string,
) (storage.Account, error) {
	return NewGitHubAccount(g.apiURL, subjectID, login, name, avatarURL, time.Time{})
}

// NewGitHubAccount validates one provider profile and builds its stable
// storage identity. OAuth sign-in and administrator lookup share this path.
func NewGitHubAccount(
	apiURL string,
	subjectID int64,
	login string,
	name, avatarURL *string,
	updatedAt time.Time,
) (storage.Account, error) {
	if subjectID <= 0 {
		return storage.Account{}, errors.New("GitHub profile has no stable subject id")
	}
	login, err := checkedIdentityField("login", login)
	if err != nil {
		return storage.Account{}, err
	}
	displayName := login
	if name != nil && strings.TrimSpace(*name) != "" {
		displayName, err = checkedIdentityField("name", *name)
		if err != nil {
			return storage.Account{}, err
		}
	}
	avatar, err := checkedAvatarURL(avatarURL)
	if err != nil {
		return storage.Account{}, err
	}
	subject := fmt.Sprint(subjectID)
	api, err := url.Parse(apiURL)
	if err != nil || (api.Scheme != httpScheme && api.Scheme != httpsScheme) || api.Host == "" {
		return storage.Account{}, errors.New("GitHub API URL must use HTTP or HTTPS")
	}
	provider := "github:" + api.Scheme + "://" + api.Host + strings.TrimRight(api.Path, "/")

	return storage.Account{
		ID:          provider + ":user:" + subject,
		Provider:    provider,
		SubjectID:   subject,
		Login:       login,
		DisplayName: displayName,
		AvatarURL:   avatar,
		UpdatedAt:   updatedAt,
	}, nil
}

func checkedIdentityField(label, raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("GitHub profile %s is blank", label)
	}
	if len(value) > maxIdentityFieldLength || strings.ContainsFunc(value, unicode.IsControl) {
		return "", fmt.Errorf("GitHub profile %s is not safe to store", label)
	}

	return value, nil
}

func checkedAvatarURL(raw *string) (*string, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, nil
	}
	value := strings.TrimSpace(*raw)
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, errors.New("GitHub profile avatar URL must use HTTP or HTTPS")
	}

	return &value, nil
}
