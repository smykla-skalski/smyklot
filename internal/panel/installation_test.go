package panel

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

type fakeInstallationLocator func(context.Context) (string, error)

func (f fakeInstallationLocator) AppInstallationURL(ctx context.Context) (string, error) {
	return f(ctx)
}

func TestInstallationRequiresSession(t *testing.T) {
	harness := newPanelHarness(t, "root")
	harness.server.installation = fakeInstallationLocator(func(context.Context) (string, error) {
		t.Fatal("anonymous caller reached provider")
		return "", nil
	})
	response := harness.request(t, http.MethodGet, "/panel/api/v1/installation", nil, nil)
	requireResponse(t, response, "anonymous installation", http.StatusUnauthorized)
}

func TestInstallationLookupStates(t *testing.T) {
	harness := newPanelHarness(t, "root")
	session := harness.signIn(t)
	response := harness.request(t, http.MethodGet, "/panel/api/v1/installation", nil, session)
	requireResponse(t, response, "unavailable installation", http.StatusOK)
	if !strings.Contains(response.Body.String(), `"installation_url":null`) {
		t.Fatal(response.Body.String())
	}
	harness.server.installation = fakeInstallationLocator(func(ctx context.Context) (string, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 5*time.Second {
			t.Error("lookup has no bounded deadline")
		}
		return "https://git.example/github-apps/deployed-bot/installations/new", nil
	})
	response = harness.request(t, http.MethodGet, "/panel/api/v1/installation", nil, session)
	requireResponse(t, response, "installation link", http.StatusOK)
	if !strings.Contains(response.Body.String(), "https://git.example/github-apps/deployed-bot/installations/new") {
		t.Fatal(response.Body.String())
	}
	harness.server.installation = fakeInstallationLocator(func(context.Context) (string, error) {
		return "", errors.New("private provider detail")
	})
	response = harness.request(t, http.MethodGet, "/panel/api/v1/installation", nil, session)
	requireResponse(t, response, "failed installation lookup", http.StatusServiceUnavailable)
	if !strings.Contains(response.Body.String(), "installation_lookup_failed") || strings.Contains(response.Body.String(), "private provider detail") {
		t.Fatal(response.Body.String())
	}
}
