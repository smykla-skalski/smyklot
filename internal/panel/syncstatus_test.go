package panel

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
)

func TestSyncStatusCountsOnlyUnfinishedActions(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	harness.signIn(t)
	ctx := t.Context()
	actions := make([]orgsync.Action, 3)
	for index := range actions {
		actions[index] = orgsync.Action{
			RepositoryID: "repository-20", Kind: orgsync.KindLabels,
			Operation: orgsync.OperationCreate, Subject: fmt.Sprintf("label-%d", index),
			Payload: []byte(`{"name":"label","color":"ffffff"}`),
		}
	}
	plan, err := harness.store.CreateSyncPlan(ctx, orgsync.PlanCreate{
		ID: "status-actions", TargetID: panelSyncTarget, ActorID: "github:test:user:1",
		Trigger: orgsync.TriggerReconcile, Digest: "saved", Actions: actions, Automatic: true,
		Now: harness.now, ExpiresAt: harness.now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	lease, err := harness.store.LeaseSyncPlan(ctx, harness.now, harness.now.Add(time.Minute))
	if err != nil || !lease.Found {
		t.Fatalf("lease = %#v, %v", lease, err)
	}
	for index, state := range []orgsync.ActionState{orgsync.ActionApplied, orgsync.ActionFailed} {
		if err := harness.store.RecordSyncActionOutcome(ctx, orgsync.ActionOutcome{
			ActionID: lease.Actions[index].ID, State: state, Error: "",
		}); err != nil {
			t.Fatal(err)
		}
	}
	target, err := harness.store.GetTarget(ctx, panelSyncTarget)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := harness.server.syncStatusFacts(httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx), target)
	if err != nil {
		t.Fatal(err)
	}
	if got := facts.pending["repository-20"][orgsync.KindLabels]; got != 1 {
		t.Errorf("pending = %d, want one unfinished action", got)
	}
	if facts.problems["repository-20"][orgsync.KindLabels] == "" {
		t.Fatal("a failure without provider text must still have a recovery reason")
	}
	if err := harness.store.FinishSyncPlan(ctx, orgsync.PlanOutcome{PlanID: plan.ID, State: orgsync.PlanFailed, Now: harness.now}); err != nil {
		t.Fatal(err)
	}
	facts, err = harness.server.syncStatusFacts(httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx), target)
	if err != nil {
		t.Fatal(err)
	}
	if facts.problems["repository-20"][orgsync.KindLabels] == "" {
		t.Fatal("finishing a failed run hid its unresolved problem")
	}
}

func TestSyncStatusSurfacesInstallationPermission(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	saved := harness.request(t, http.MethodPut, workspaceSettingsBatchPath, strings.NewReader(
		`{"sync_configs":[{"kind":"settings","enabled":true,"expected_revision":0,"document":{"has_wiki":false}}]}`), session)
	if saved.Code != http.StatusOK {
		t.Fatalf("save settings: %d %s", saved.Code, saved.Body.String())
	}
	read := harness.request(t, http.MethodGet, "/panel/api/v1/targets/"+panelSyncTarget+"/sync/status", nil, session)
	if read.Code != http.StatusOK {
		t.Fatalf("status: %d %s", read.Code, read.Body.String())
	}
	var answer struct {
		Unavailable  map[string]string         `json:"unavailable"`
		Repositories []syncRepositoryStatusDTO `json:"repositories"`
	}
	if err := json.Unmarshal(read.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if answer.Unavailable["settings"] == "" {
		t.Fatal("missing administration permission was invisible")
	}
	if len(answer.Repositories) == 0 {
		t.Fatal("missing repository inventory")
	}
	for _, row := range answer.Repositories {
		if row.Cells["settings"].State != "refused" || row.Cells["settings"].Reason == "" {
			t.Errorf("permission blocker did not reach %s: %#v", row.Repository, row.Cells["settings"])
		}
	}
}
