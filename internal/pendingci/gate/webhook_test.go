package gate

import (
	"context"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

type wakingStore struct {
	Store

	woke bool
}

func (s *wakingStore) Wake(context.Context, pendingci.WakeRequest) (bool, error) {
	s.woke = true

	return true, nil
}

func TestHandleWebhookSurvivesAGateWithNothingToWake(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		notification *pendingci.Notification
		wantWoke     bool
	}{
		{
			// Given the first pull request on a repository, which reaches the
			// catalog callback and no signal
			name: "opened pull request with no signal",
			notification: &pendingci.Notification{
				Event: webhook.EventPullRequest, Action: "opened",
				Source: webhook.Source{Repository: webhook.Repository{ID: 1}},
			},
		},
		{
			// Given a signal that actually wakes a row, which reaches the
			// scheduler below it
			name: "signal that wakes an armed request",
			notification: &pendingci.Notification{
				Event: webhook.EventPullRequest, Action: "closed",
				Source:  webhook.Source{Repository: webhook.Repository{ID: 1}},
				Signals: []pendingci.Signal{{Kind: pendingci.SignalPullRequestDone, PullRequest: 7}},
			},
			wantWoke: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Given a gate assembled with neither a WakeGates callback nor a scheduler
			store := &wakingStore{}
			gate := &Gate{coordinator: bot.NewCoordinator(), store: store}

			// When the delivery arrives
			err := gate.HandleWebhook(t.Context(), test.notification, "d1")
			// Then neither missing collaborator is called
			if err != nil {
				t.Fatalf("HandleWebhook: %v", err)
			}
			if store.woke != test.wantWoke {
				t.Fatalf("store woken = %t, want %t", store.woke, test.wantWoke)
			}
		})
	}
}
