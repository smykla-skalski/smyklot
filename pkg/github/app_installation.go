package github

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// AppInstallationURL locates this authenticated app's installation flow.
// Preserve the API-provided web host and app path, including Enterprise's
// /github-apps path, instead of assuming github.com or a deployment's name.
func (c *Client) AppInstallationURL(ctx context.Context) (string, error) {
	app, err := doJSON[struct {
		Slug    string `json:"slug"`
		HTMLURL string `json:"html_url"`
	}](ctx, c, http.MethodGet, "/app", nil)
	if err != nil {
		return "", err
	}
	location, err := url.Parse(app.HTMLURL)
	if err != nil {
		return "", errors.New("GitHub returned an invalid app page")
	}
	if app.Slug == "" || strings.ContainsAny(app.Slug, "/\\?#%") || app.Slug == "." || app.Slug == ".." ||
		(location.Scheme != "https" && location.Scheme != "http") || location.Host == "" || location.User != nil ||
		location.RawQuery != "" || location.Fragment != "" ||
		(location.Path != "/apps/"+app.Slug && location.Path != "/github-apps/"+app.Slug) {
		return "", errors.New("GitHub returned an invalid app page")
	}
	return location.JoinPath("installations", "new").String(), nil
}
