// Package apply plans one installation's org-wide sync and applies it.
package apply

import (
	"sync"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/githubapp"
)

type Engine struct {
	store        Store
	tokens       *githubapp.TokenStore
	apiBaseURL   string
	coordinator  bot.Exclusive
	beginWork    func() (func(), bool)
	queueChanged func(string)
	formattingMu sync.RWMutex
	formatting   config.FormattingPolicy
}

func New(store Store, tokens *githubapp.TokenStore, apiBaseURL string) *Engine {
	return &Engine{
		store: store, tokens: tokens, apiBaseURL: apiBaseURL,
		formatting: config.DefaultFormattingPolicy(),
	}
}

// SetFormattingPolicy replaces the process-level formatting defaults used by
// future plans and by scope validation immediately before execution.
func (s *Engine) SetFormattingPolicy(policy config.FormattingPolicy) {
	s.formattingMu.Lock()
	s.formatting = policy
	s.formattingMu.Unlock()
}

func (s *Engine) formattingPolicy() config.FormattingPolicy {
	s.formattingMu.RLock()
	defer s.formattingMu.RUnlock()

	return s.formatting
}

// SetCoordinator shares the repository mutation fence with webhook and
// pending-CI execution. Planning remains concurrent because it only observes;
// applying a plan holds the repository key around every GitHub mutation.
func (s *Engine) SetCoordinator(coordinator bot.Exclusive) { s.coordinator = coordinator }

// SetBeginWork makes sync-plan lease acquisition participate in the service's
// background-work pause fence. The returned release closes only the lease
// acquisition window; a plan already durably leased remains allowed to finish.
func (s *Engine) SetBeginWork(begin func() (func(), bool)) { s.beginWork = begin }

// SetQueueObserver publishes scoped queue revisions after sync execution
// starts and after it reaches its next durable state.
func (s *Engine) SetQueueObserver(observer func(string)) { s.queueChanged = observer }

func (s *Engine) announceQueue(targetID string) {
	if s.queueChanged != nil {
		s.queueChanged(targetID)
	}
}
