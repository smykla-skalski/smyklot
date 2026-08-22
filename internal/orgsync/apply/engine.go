// Package apply plans one installation's org-wide sync and applies it.
//
// The planner reads what a repository holds against what the account's
// configuration says it should, and writes a plan; the applier leases one plan
// at a time and does what it says to GitHub. Both live here because the plan is
// the only thing they share, and neither is anything without the other.
//
// It holds three things: a store, a token source and the API base URL. That is
// not restraint, it is the measurement - the whole subsystem read exactly those
// three off the service it used to be a part of, which is why it could leave.
package apply

import (
	"github.com/smykla-skalski/smyklot/pkg/githubapp"
)

// Engine is org sync, for every installation the App is on.
type Engine struct {
	store      Store
	tokens     *githubapp.TokenStore
	apiBaseURL string
}

// New builds the engine.
//
// apiBaseURL is empty for github.com and set for an Enterprise host; it is
// carried rather than read from a config so that this package needs no
// configuration type of its own.
func New(store Store, tokens *githubapp.TokenStore, apiBaseURL string) *Engine {
	return &Engine{store: store, tokens: tokens, apiBaseURL: apiBaseURL}
}
