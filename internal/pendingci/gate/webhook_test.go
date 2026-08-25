package gate

import (
	"context"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

type wakingStore struct {
	Store

	woke    bool
	drafted *pendingci.DraftTransitionRequest
}

func (s *wakingStore) Wake(context.Context, pendingci.WakeRequest) (bool, error) {
	s.woke = true

	return true, nil
}

func (s *wakingStore) RecordDraftTransition(
	_ context.Context,
	request pendingci.DraftTransitionRequest,
) (pendingci.DraftTransitionResult, error) {
	s.drafted = &request

	return pendingci.DraftTransitionResult{Changed: true}, nil
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
		{
			name: "draft transition terminalizes an armed request",
			notification: &pendingci.Notification{
				Event: webhook.EventPullRequest, Action: "converted_to_draft",
				Source: webhook.Source{Repository: webhook.Repository{ID: 1}},
				Signals: []pendingci.Signal{{
					Kind: pendingci.SignalPullRequestDraft, PullRequest: 7,
					EventKey:   "pull_request:1:7:converted_to_draft",
					OccurredAt: time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC),
				}},
			},
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
			if test.notification.Action == "converted_to_draft" {
				if store.drafted == nil || store.drafted.PullRequest != 7 ||
					store.drafted.EventKey == "" || store.drafted.DraftedAt.IsZero() ||
					store.drafted.RecordedAt.IsZero() {
					t.Fatalf("draft transition request = %#v", store.drafted)
				}
			}
		})
	}
}
