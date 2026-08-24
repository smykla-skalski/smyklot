// Package apply plans one installation's org-wide sync and applies it.
package apply

import (
	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/pkg/githubapp"
)

type Engine struct {
	store       Store
	tokens      *githubapp.TokenStore
	apiBaseURL  string
	coordinator bot.Exclusive
}

func New(store Store, tokens *githubapp.TokenStore, apiBaseURL string) *Engine {
	return &Engine{store: store, tokens: tokens, apiBaseURL: apiBaseURL}
}

// SetCoordinator shares the repository mutation fence with webhook and
// pending-CI execution. Planning remains concurrent because it only observes;
// applying a plan holds the repository key around every GitHub mutation.
func (s *Engine) SetCoordinator(coordinator bot.Exclusive) { s.coordinator = coordinator }
