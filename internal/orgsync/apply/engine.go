// Package apply plans one installation's org-wide sync and applies it.
package apply

import (
	"github.com/smykla-skalski/smyklot/pkg/githubapp"
)

type Engine struct {
	store      Store
	tokens     *githubapp.TokenStore
	apiBaseURL string
}

func New(store Store, tokens *githubapp.TokenStore, apiBaseURL string) *Engine {
	return &Engine{store: store, tokens: tokens, apiBaseURL: apiBaseURL}
}
