package main

import (
	"context"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

// AppInstallationURL supplies the panel's deployment-specific installation link.
// An absent app credential is unavailable, while a failed lookup is retryable.
func (s *server) AppInstallationURL(ctx context.Context) (string, error) {
	if s.tokens == nil {
		return "", nil
	}
	token, err := s.tokens.AppToken()
	if err != nil {
		return "", err
	}
	client, err := github.NewAppClient(token, s.cfg.apiBaseURL)
	if err != nil {
		return "", err
	}
	return client.AppInstallationURL(ctx)
}
