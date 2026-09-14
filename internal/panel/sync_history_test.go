package panel

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
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

func TestSyncHistoryPagesSummariesAndRejectsInvalidCursors(t *testing.T) {
	h := newPanelHarness(t, "owner")
	session := h.signIn(t)
	for _, id := range []string{"history-a", "history-b", "history-c"} {
		createPanelSyncPlan(t, h, id, h.now.Add(time.Hour))
		if err := h.store.InvalidateSyncPlans(t.Context(), panelSyncTarget, h.now); err != nil {
			t.Fatal(err)
		}
	}
	path := "/panel/api/v1/targets/" + panelSyncTarget + "/sync/plans"
	response := h.request(t, http.MethodGet, path+"?limit=2", nil, session)
	requireResponse(t, response, "history page", http.StatusOK)
	var first pageResponse[syncPlanSummaryDTO]
	if err := json.Unmarshal(response.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || first.Items[0].ID != "history-c" || first.Total != 3 || first.NextCursor == nil {
		t.Fatalf("first page: %#v", first)
	}
	if strings.Contains(response.Body.String(), `"actions"`) || strings.Contains(response.Body.String(), `"digest"`) {
		t.Fatalf("summary exposed execution payload: %s", response.Body.String())
	}
	response = h.request(t, http.MethodGet, path+"?limit=2&cursor="+url.QueryEscape(*first.NextCursor), nil, session)
	var second pageResponse[syncPlanSummaryDTO]
	if err := json.Unmarshal(response.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || len(second.Items) != 1 || second.Items[0].ID != "history-a" || second.NextCursor != nil {
		t.Fatalf("last page: %d %#v", response.Code, second)
	}
	for _, query := range []string{"limit=0", "limit=101", "cursor=broken", "cursor=e30", "cursor=bnVsbA", "limit=2&limit=3", "sort=oldest", "q=ignored"} {
		response := h.request(t, http.MethodGet, path+"?"+query, nil, session)
		requireResponse(t, response, query, http.StatusBadRequest)
	}
}
