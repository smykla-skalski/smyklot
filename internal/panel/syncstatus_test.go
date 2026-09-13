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
	"github.com/smykla-skalski/smyklot/internal/storage"
)

func TestSyncStatusCountsOnlyUnfinishedActions(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	ctx := t.Context()
	actions := make([]orgsync.Action, 3)
	for index := range actions {
		actions[index] = orgsync.Action{
			RepositoryID: "repository-20", Kind: orgsync.KindLabels,
			Operation: orgsync.OperationCreate, Subject: fmt.Sprintf("label-%d", index),
			Payload: []byte(`{"name":"label","color":"ffffff"}`),
		}
	}
	actions[0].Kind = orgsync.KindFiles
	actions[0].Subject = "config.json"
	actions[0].Payload = []byte(`{"path":"config.json","proposal":"proposal"}`)
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
			ProposalURL: map[int]string{0: "https://github.com/smykla-skalski/smyklot/pull/42"}[index],
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
	response := harness.request(t, http.MethodGet, "/panel/api/v1/targets/"+panelSyncTarget+"/sync/plan", nil, session)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"proposal_url":"https://github.com/smykla-skalski/smyklot/pull/42"`) {
		t.Fatalf("proposal absent from live action: %d %s", response.Code, response.Body.String())
	}
	if err := harness.store.FinishSyncPlan(ctx, orgsync.PlanOutcome{PlanID: plan.ID, State: orgsync.PlanFailed, Now: harness.now}); err != nil {
		t.Fatal(err)
	}
	facts, err = harness.server.syncStatusFacts(httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx), target)
	if err != nil {
		t.Fatal(err)
	}
	if facts.observations["repository-20"][orgsync.KindLabels].Problem == "" {
		t.Fatal("finishing a failed run hid its unresolved problem")
	}
}

func TestSyncStatusSurfacesInstallationPermission(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	enableStatusRepositories(t, harness, session)
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

func enableStatusRepositories(t *testing.T, harness *panelHarness, session *http.Cookie) {
	t.Helper()
	saved := harness.request(t, http.MethodPut, workspaceSettingsBatchPath, strings.NewReader(
		`{"target":{"repository_default_enabled":true,"pending_ci_mode_default":"checks",
   "pending_ci_branch_patterns_default":{"include":["~DEFAULT_BRANCH"],"exclude":[]},
   "pending_ci_quiet_period_seconds_override":null,"path_index_interval_seconds_override":null,
   "config_patch":{},"expected_revision":1}}`), session)
	if saved.Code != http.StatusOK {
		t.Fatalf("enable repositories: %d %s", saved.Code, saved.Body.String())
	}
}

func TestSyncStatusNeverInventsAgreementAfterSave(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	target, err := harness.store.GetTarget(t.Context(), panelSyncTarget)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := harness.store.GetAccount(t.Context(), "github:test:user:1")
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.store.ReconcileInstallation(t.Context(), storage.InstallationSnapshot{
		TargetID: target.ID, InstallationID: target.InstallationID, Kind: target.Kind,
		Account: target.Account, SyncedAt: harness.now, Permissions: map[string]string{"issues": "write"},
		Repositories: []storage.RepositorySnapshot{{ID: "repository-20", Name: "smyklot", FullName: "smykla-skalski/smyklot"}},
		Ownership:    storage.OwnershipSnapshot{Source: target.Ownership.Source, Status: storage.OwnershipStatusFresh, Owners: []storage.Account{owner}, SyncedAt: harness.now},
	}); err != nil {
		t.Fatal(err)
	}
	enableStatusRepositories(t, harness, session)
	save := func(revision int, color string) {
		t.Helper()
		body := fmt.Sprintf(`{"sync_configs":[{"kind":"labels","enabled":true,"expected_revision":%d,"labels":[{"name":"bug","color":"%s"}],"allow_removal":false,"excludes":[]}]}`, revision, color)
		response := harness.request(t, http.MethodPut, workspaceSettingsBatchPath, strings.NewReader(body), session)
		if response.Code != http.StatusOK {
			t.Fatalf("save: %d %s", response.Code, response.Body.String())
		}
	}
	read := func() (syncCellDTO, *time.Time) {
		t.Helper()
		response := harness.request(t, http.MethodGet, "/panel/api/v1/targets/"+panelSyncTarget+"/sync/status", nil, session)
		if response.Code != http.StatusOK {
			t.Fatalf("read: %d %s", response.Code, response.Body.String())
		}
		var answer struct {
			Latest       *time.Time                `json:"latest_observed_at"`
			Repositories []syncRepositoryStatusDTO `json:"repositories"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
			t.Fatal(err)
		}
		if len(answer.Repositories) != 1 {
			t.Fatalf("repositories = %#v", answer.Repositories)
		}
		return answer.Repositories[0].Cells["labels"], answer.Latest
	}
	save(0, "ffffff")
	cell, latest := read()
	if cell.State != "unknown" || cell.ObservedAt != nil || latest != nil {
		t.Fatalf("new settings fabricated observation: %#v, %v", cell, latest)
	}
	config, err := harness.store.GetSyncConfig(t.Context(), panelSyncTarget, orgsync.KindLabels)
	if err != nil {
		t.Fatal(err)
	}
	observed := harness.now.Add(-time.Minute)
	digest := orgsync.DigestRepositoryKind(config.Digest, nil)
	if err := harness.store.RecordSyncRepositoryState(t.Context(), []orgsync.RepositoryState{{
		RepositoryID: "repository-20", Kind: orgsync.KindLabels, AppliedDigest: digest,
		ObservedDigest: digest, Observation: orgsync.ObservationMatched, AppliedAt: observed,
	}}); err != nil {
		t.Fatal(err)
	}
	cell, latest = read()
	if cell.State != "in_step" || cell.ObservedAt == nil || !cell.ObservedAt.Equal(observed) || latest == nil || !latest.Equal(observed) {
		t.Fatalf("matching evidence missing: %#v, %v", cell, latest)
	}
	save(1, "000000")
	cell, latest = read()
	if cell.State != "outdated" || cell.ObservedAt == nil || !cell.ObservedAt.Equal(observed) || latest == nil || !latest.Equal(observed) {
		t.Fatalf("save refreshed old evidence: %#v, %v", cell, latest)
	}
}
