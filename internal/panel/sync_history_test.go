package panel

import (
	"net/http"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
)

func TestRetainedSyncPlanCannotCrossWorkspaceScope(t *testing.T) {
	h := newPanelHarness(t, "owner")
	session := h.signIn(t)
	_, workspace := seedNonOwnedWorkspace(t, h)
	_, err := h.store.CreateSyncPlan(t.Context(), orgsync.PlanCreate{
		ID: "foreign-history", TargetID: workspace.TargetID, ActorID: "github:test:user:owner-2",
		Trigger: orgsync.TriggerManual, Digest: "foreign",
		Actions: []orgsync.Action{{
			RepositoryID: "repository-30", Kind: orgsync.KindLabels,
			Operation: orgsync.OperationCreate, Subject: "private-label",
			Payload: []byte(`{"name":"private-label","color":"ffffff"}`),
		}},
		Now: h.now, ExpiresAt: h.now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{panelSyncTarget, workspace.TargetID} {
		response := h.request(t, http.MethodGet,
			"/panel/api/v1/targets/"+target+"/sync/plans/foreign-history", nil, session)
		requireResponse(t, response, "foreign retained plan", http.StatusNotFound)
	}
}
