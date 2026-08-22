package gate

import (
	"testing"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

func TestHandleWebhookSurvivesAGateWithNoGatesToWake(t *testing.T) {
	t.Parallel()

	// Given a gate assembled without a WakeGates callback
	gate := &Gate{coordinator: bot.NewCoordinator()}

	// When the first pull request on a repository arrives
	err := gate.HandleWebhook(t.Context(), &pendingci.Notification{
		Event:  webhook.EventPullRequest,
		Action: "opened",
		Source: webhook.Source{Repository: webhook.Repository{ID: 1}},
	}, "d1")
	// Then the nil callback is not called
	if err != nil {
		t.Fatalf("HandleWebhook: %v", err)
	}
}
