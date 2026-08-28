package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

const panelSyncTarget = "github:installation:10"

type fixedSyncScopeDigest string

func (digest fixedSyncScopeDigest) CurrentSyncScopeDigest(context.Context, string) (string, error) {
	return string(digest), nil
}

// TestSyncConfigShowsTheEditorLogin drives the production API boundary that
// feeds the overview cards. Storage deliberately keeps the stable account key;
// the panel must turn it back into the current GitHub login rather than expose
// an implementation identifier such as github:https://api.github.com:user:1.
func TestSyncConfigShowsTheEditorLogin(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	path := "/panel/api/v1/targets/github:installation:10/sync/config/labels"

	saved := harness.request(t, http.MethodPut, installationSettingsBatchPath, strings.NewReader(
		`{"sync_configs":[{"kind":"labels","enabled":true,"expected_revision":0,`+
			`"labels":[],"allow_removal":false,"excludes":[]}]}`), session)
	if saved.Code != http.StatusOK {
		t.Fatalf("saving labels = %d %s", saved.Code, saved.Body.String())
	}

	read := harness.request(t, http.MethodGet, path, nil, session)
	if read.Code != http.StatusOK {
		t.Fatalf("reading labels = %d %s", read.Code, read.Body.String())
	}

	var answer syncConfigDTO
	if err := json.Unmarshal(read.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if answer.UpdatedBy != "owner" {
		t.Errorf("updated_by = %q, wanted the editor's GitHub login", answer.UpdatedBy)
	}

	audit := harness.request(t, http.MethodGet,
		"/panel/api/v1/targets/github:installation:10/audit?change=sync", nil, session)
	var history pageResponse[auditResponse]
	if err := json.Unmarshal(audit.Body.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	if history.Total != 1 || history.Items[0].SettingsCheckpointID == nil {
		t.Fatalf("settings save audit = %#v", history)
	}
}

// TestSyncPlanShowsRepositoryNames keeps stable catalog identifiers inside the
// service. A plan is an approval surface, so its groups must name repositories
// the reader recognizes instead of exposing github:repository-style keys.
func TestSyncPlanShowsRepositoryNames(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	_, err := harness.store.CreateSyncPlan(t.Context(), orgsync.PlanCreate{
		ID: "sync-plan-names", TargetID: "github:installation:10",
		Trigger: orgsync.TriggerManual, ActorID: "github:test:user:1", Digest: "sha256:plan",
		Actions: []orgsync.Action{{
			RepositoryID: "repository-20", Kind: orgsync.KindLabels,
			Operation: orgsync.OperationCreate, Subject: "ci/test", After: "#ffffff",
			Payload: []byte(`{"name":"ci/test","color":"ffffff"}`),
		}},
		Now: harness.now, ExpiresAt: harness.now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	target, err := harness.store.GetTarget(t.Context(), "github:installation:10")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := harness.store.GetAccount(t.Context(), "github:test:user:1")
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.store.ReconcileInstallation(t.Context(), storage.InstallationSnapshot{
		TargetID: target.ID, InstallationID: target.InstallationID, Kind: target.Kind,
		Account: target.Account, SyncedAt: harness.now.Add(time.Minute),
		Ownership: storage.OwnershipSnapshot{
			Source: storage.OwnershipSourceOrganizationAdmin,
			Status: storage.OwnershipStatusFresh, Owners: []storage.Account{owner},
			SyncedAt: harness.now.Add(time.Minute),
		},
	}); err != nil {
		t.Fatal(err)
	}

	read := harness.request(t, http.MethodGet,
		"/panel/api/v1/targets/github:installation:10/sync/plan", nil, session)
	requireResponse(t, read, "sync plan repository name", http.StatusOK,
		`"repository":"smyklot"`)
	if strings.Contains(read.Body.String(), `"repository":"repository-20"`) {
		t.Fatalf("sync plan exposed stable repository id: %s", read.Body.String())
	}
}

func TestSyncPlanApprovalRecomputesScopeBeforeApproval(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	createPanelSyncPlan(t, harness, "scope-check", harness.now.Add(time.Hour))
	harness.server.syncPlans = fixedSyncScopeDigest("sha256:current")
	document, err := json.Marshal(struct {
		Digest string `json:"digest"`
	}{Digest: "sha256:scope-check"})
	if err != nil {
		t.Fatal(err)
	}
	response := harness.request(
		t,
		http.MethodPost,
		"/panel/api/v1/targets/"+panelSyncTarget+"/sync/plans/scope-check/approval",
		bytes.NewReader(document),
		session,
	)
	requireResponse(t, response, "stale plan approval", http.StatusConflict, `"code":"stale_plan"`)
	_, _, err = harness.store.GetLiveSyncPlan(t.Context(), panelSyncTarget)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("stale plan remained live: %v", err)
	}
}

func TestSyncRunNowSafetyMatrix(t *testing.T) {
	t.Run("queues a drift scan when no plan is live", func(t *testing.T) {
		harness := newPanelHarness(t, "owner")
		response := postPanelSyncRunNow(t, harness, harness.signIn(t), 0)
		requireResponse(t, response, "no-plan run now", http.StatusAccepted,
			`"status":"scan_queued"`, `"kind":"sync_scan"`, `"immediate":true`)
	})

	t.Run("opens a computed plan for approval", func(t *testing.T) {
		harness := newPanelHarness(t, "owner")
		session := harness.signIn(t)
		createPanelSyncPlan(t, harness, "computed", harness.now.Add(time.Hour))
		response := postPanelSyncRunNow(t, harness, session, 0)
		requireResponse(t, response, "computed-plan run now", http.StatusOK,
			`"status":"approval_required"`, `"state":"computed"`)
		item, err := harness.store.GetQueueItem(t.Context(), "sync-plan:computed")
		if err != nil {
			t.Fatal(err)
		}
		if item.State != workqueue.StateAwaitingApproval {
			t.Fatalf("computed plan queue state = %q", item.State)
		}
	})

	t.Run("dispatches an approved plan immediately", func(t *testing.T) {
		harness := newPanelHarness(t, "owner")
		session := harness.signIn(t)
		createPanelSyncPlan(t, harness, "approved", harness.now.Add(time.Hour))
		_, err := harness.store.ApproveSyncPlan(t.Context(), orgsync.PlanApproval{
			TargetID: panelSyncTarget, PlanID: "approved", Digest: "sha256:approved",
			ActorID: "github:test:user:1", Now: harness.now,
		})
		if err != nil {
			t.Fatal(err)
		}
		item, err := harness.store.GetQueueItem(t.Context(), "sync-plan:approved")
		if err != nil {
			t.Fatal(err)
		}
		response := postPanelSyncRunNow(t, harness, session, item.Revision)
		requireResponse(t, response, "approved-plan run now", http.StatusAccepted,
			`"status":"plan_dispatched"`, `"state":"ready"`, `"immediate":true`)
	})

	t.Run("reports a plan that is already applying", func(t *testing.T) {
		harness := newPanelHarness(t, "owner")
		session := harness.signIn(t)
		createPanelSyncPlan(t, harness, "applying", harness.now.Add(time.Hour))
		_, err := harness.store.ApproveSyncPlan(t.Context(), orgsync.PlanApproval{
			TargetID: panelSyncTarget, PlanID: "applying", Digest: "sha256:applying",
			ActorID: "github:test:user:1", Now: harness.now,
		})
		if err != nil {
			t.Fatal(err)
		}
		lease, err := harness.store.LeaseSyncPlan(
			t.Context(), harness.now, harness.now.Add(time.Minute),
		)
		if err != nil || !lease.Found {
			t.Fatalf("lease approved sync plan: found=%t err=%v", lease.Found, err)
		}
		response := postPanelSyncRunNow(t, harness, session, 0)
		requireResponse(t, response, "applying-plan run now", http.StatusOK,
			`"status":"already_running"`, `"state":"applying"`)
	})

	t.Run("queues a fresh scan after a plan expires", func(t *testing.T) {
		harness := newPanelHarness(t, "owner")
		session := harness.signIn(t)
		createPanelSyncPlan(t, harness, "expired", harness.now.Add(time.Minute))
		later := harness.now.Add(2 * time.Minute)
		if err := harness.store.ExpireSyncPlans(t.Context(), later); err != nil {
			t.Fatal(err)
		}
		*harness.clock = later
		response := postPanelSyncRunNow(t, harness, session, 0)
		requireResponse(t, response, "expired-plan run now", http.StatusAccepted,
			`"status":"scan_queued"`, `"kind":"sync_scan"`)
	})
}

func createPanelSyncPlan(
	t *testing.T,
	harness *panelHarness,
	id string,
	expiresAt time.Time,
) {
	t.Helper()
	_, err := harness.store.CreateSyncPlan(t.Context(), orgsync.PlanCreate{
		ID: id, TargetID: panelSyncTarget, Trigger: orgsync.TriggerManual,
		ActorID: "github:test:user:1", Digest: "sha256:" + id,
		Now: harness.now, ExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func postPanelSyncRunNow(
	t *testing.T,
	harness *panelHarness,
	session *http.Cookie,
	expectedRevision int64,
) *httptest.ResponseRecorder {
	t.Helper()
	body := fmt.Sprintf(`{"reason":"operator request","expected_revision":%d}`, expectedRevision)

	return harness.request(t, http.MethodPost,
		"/panel/api/v1/targets/"+panelSyncTarget+"/sync/run-now",
		strings.NewReader(body), session)
}

// TestSyncConfigReportsADocumentItCannotRead is the guard on the difference
// between "nothing is configured" and "nothing could be read".
//
// They render the same - an empty list - and the panel is where somebody then
// presses save. A save built from an invented empty form sends that emptiness
// back and wipes a label set nobody was ever shown, so the two have to be told
// apart on the wire rather than in a comment.
func TestSyncConfigReportsADocumentItCannotRead(t *testing.T) {
	stored := orgsync.Config{
		Kind:      orgsync.KindLabels,
		Enabled:   true,
		Document:  []byte(`{"labels": [ this is not json`),
		Digest:    "digest-1",
		Revision:  3,
		UpdatedAt: time.Now().UTC(),
	}

	dto := syncConfigToDTO(stored, "")

	if !dto.Unreadable {
		t.Error("a document that does not decode was reported as readable")
	}
	if len(dto.Labels) != 0 {
		t.Errorf("labels = %d, wanted none: nothing could be read out of the document",
			len(dto.Labels))
	}

	// The rest still describes the row, because the row is still there and the
	// revision is what a later save would have to match.
	if dto.Revision != stored.Revision {
		t.Errorf("revision = %d, wanted %d", dto.Revision, stored.Revision)
	}
	if !dto.Enabled {
		t.Error("enabled was dropped, though it is a column rather than part of the document")
	}
}

// TestSyncConfigStillAnswersOnAnUnreadableDocument is the guard on the answer
// itself reaching the browser.
//
// A json.RawMessage is copied out verbatim and validated on the way, so a
// document that is not JSON fails the whole response - and what a person then
// gets is a truncated body and a parse error, rather than the careful message
// about a row this version cannot read. The guard above it is worth nothing if
// the response carrying it cannot be written.
func TestSyncConfigStillAnswersOnAnUnreadableDocument(t *testing.T) {
	dto := syncConfigToDTO(orgsync.Config{
		Kind:     orgsync.KindLabels,
		Document: []byte(`{"labels": [ this is not json`),
	}, "")

	answer, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("the answer could not be written at all: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(answer, &decoded); err != nil {
		t.Fatalf("the answer is not readable by a browser: %v", err)
	}
	if decoded["unreadable"] != true {
		t.Error("the answer does not say the document could not be read")
	}
}

// TestSyncConfigReadsADocumentItCan is the other half: a readable document must
// not be reported as unreadable, or the panel would refuse to edit a
// configuration that is perfectly fine.
func TestSyncConfigReadsADocumentItCan(t *testing.T) {
	dto := syncConfigToDTO(orgsync.Config{
		Kind:     orgsync.KindLabels,
		Document: []byte(`{"labels":[{"name":"bug","color":"d73a4a"}],"excludes":["ci/*"]}`),
	}, "")

	if dto.Unreadable {
		t.Fatal("a document that decodes was reported as unreadable")
	}
	if len(dto.Labels) != 1 || dto.Labels[0].Name != "bug" {
		t.Errorf("labels = %v, wanted the one that was stored", dto.Labels)
	}
	if len(dto.Excludes) != 1 || dto.Excludes[0] != "ci/*" {
		t.Errorf("excludes = %v, wanted the one that was stored", dto.Excludes)
	}
}

// TestSyncDocumentRefusesSettingsGitHubWould is the panel half of the settings
// rules: a value GitHub answers with a 422 is refused beside the field somebody
// typed rather than in a plan that fails later against somebody's repositories.
func TestSyncDocumentRefusesSettingsGitHubWould(t *testing.T) {
	for _, invalid := range []struct {
		name     string
		document string
	}{
		{"a wording nobody defined", `{"merge_commit_title":"NONSENSE"}`},
		{"no way of merging at all", `{"allow_merge_commit":false,` +
			`"allow_squash_merge":false,"allow_rebase_merge":false}`},
		{"a wording its own strategy forbids", `{"allow_squash_merge":false,` +
			`"squash_merge_commit_title":"PR_TITLE"}`},
		{"a key this version does not know", `{"allow_forking":true}`},
	} {
		t.Run(invalid.name, func(t *testing.T) {
			_, err := syncDocumentFor(orgsync.KindSettings, syncConfigRequest{
				Document: []byte(invalid.document),
			})
			if err == nil {
				t.Fatalf("%s was accepted", invalid.name)
			}
		})
	}
}

// TestSyncDocumentStoresTheSettingsType keeps what is stored the type the
// planner decodes.
//
// A second shape here is what made every configured exclusion a silent no-op in
// the kind before this one: the panel wrote one document and the planner decoded
// another, which had no field for them.
func TestSyncDocumentStoresTheSettingsType(t *testing.T) {
	document, err := syncDocumentFor(orgsync.KindSettings, syncConfigRequest{
		Document: []byte(`{"has_wiki":false,"allow_squash_merge":true}`),
	})
	if err != nil {
		t.Fatalf("a settings document GitHub accepts was refused: %v", err)
	}

	var stored orgsync.SettingsConfig
	if err := json.Unmarshal(document, &stored); err != nil {
		t.Fatalf("what was stored is not a settings configuration: %v", err)
	}
	if stored.HasWiki == nil || *stored.HasWiki {
		t.Error("has_wiki did not survive the round trip")
	}
	if stored.AllowSquashMerge == nil || !*stored.AllowSquashMerge {
		t.Error("allow_squash_merge did not survive the round trip")
	}
}

// TestSyncDocumentRefusesRulesetsGitHubWould is the panel half of the ruleset
// rules.
//
// The last two entries are the ones worth having. GitHub accepts a branch
// ruleset pointed at tag refs and a status-check rule naming no check, and then
// the first protects nothing while reading exactly like protection and the
// second disappears - which is how the tool this replaces removed a repository's
// required checks with no log and no error.
func TestSyncDocumentRefusesRulesetsGitHubWould(t *testing.T) {
	for _, invalid := range []struct {
		name     string
		document string
	}{
		{"a target nobody defined", `{"rulesets":[
			{"name":"main","target":"brnach","enforcement":"active"}]}`},
		{"an enforcement nobody defined", `{"rulesets":[
			{"name":"main","target":"branch","enforcement":"on"}]}`},
		{"a bypass mode nobody defined", `{"rulesets":[
			{"name":"main","target":"branch","enforcement":"active",
			 "bypass_actors":[{"actor_id":5,"actor_type":"Team",
			                   "bypass_mode":"sometimes"}]}]}`},
		{"a key this version does not know", `{"rulesets":[
			{"name":"main","target":"branch","enforcement":"active",
			 "do_not_enforce":true}]}`},
		{"a branch ruleset aimed at tags", `{"rulesets":[
			{"name":"main","target":"branch","enforcement":"active",
			 "conditions":{"include":["refs/tags/v*"]}}]}`},
		{"required checks that name none", `{"rulesets":[
			{"name":"main","target":"branch","enforcement":"active",
			 "rules":{"required_status_checks":{
			   "strict_required_status_checks_policy":true}}}]}`},
	} {
		t.Run(invalid.name, func(t *testing.T) {
			_, err := syncDocumentFor(orgsync.KindRulesets, syncConfigRequest{
				Document: []byte(invalid.document),
			})
			if err == nil {
				t.Fatalf("%s was accepted", invalid.name)
			}
		})
	}
}

// TestSyncDocumentStoresTheRulesetsType keeps what is stored the type the
// planner decodes, which is the same guard the settings and labels kinds carry
// and for the same reason: two shapes is how chunk 3's exclusions came to be
// saved and never read.
func TestSyncDocumentStoresTheRulesetsType(t *testing.T) {
	document, err := syncDocumentFor(orgsync.KindRulesets, syncConfigRequest{
		Document: []byte(`{"rulesets":[{"name":"main","target":"branch",
			"enforcement":"active","conditions":{"include":["refs/heads/main"]},
			"rules":{"deletion":true}}],
			"allow_removal":true,"excludes":["hand-made"]}`),
	})
	if err != nil {
		t.Fatalf("a ruleset document GitHub accepts was refused: %v", err)
	}

	var stored orgsync.RulesetConfig
	if err := json.Unmarshal(document, &stored); err != nil {
		t.Fatalf("what was stored is not a ruleset configuration: %v", err)
	}

	if len(stored.Rulesets) != 1 || stored.Rulesets[0].Name != "main" {
		t.Errorf("rulesets = %v, wanted the one that was sent", stored.Rulesets)
	}
	if !stored.Rulesets[0].Rules.Deletion {
		t.Error("the deletion rule did not survive the round trip")
	}
	if !stored.AllowRemoval {
		t.Error("allow_removal did not survive the round trip")
	}
	if len(stored.Excludes) != 1 || stored.Excludes[0] != "hand-made" {
		t.Errorf("excludes = %v, wanted the one that was sent", stored.Excludes)
	}
}

// TestSyncConfigNeverAnswersNullLists guards the shape rather than the values.
// A JSON null where the browser expects a list is a crash in the view, and an
// installation that has configured nothing is the ordinary case.
func TestSyncConfigNeverAnswersNullLists(t *testing.T) {
	dto := syncConfigToDTO(orgsync.Config{Kind: orgsync.KindLabels, Document: []byte(`{}`)}, "")

	if dto.Labels == nil {
		t.Error("labels came back null rather than empty")
	}
	if dto.Excludes == nil {
		t.Error("excludes came back null rather than empty")
	}
}

// TestSyncConfigReadsAKindNobodyConfigured is the difference between a row that
// holds nothing and a row this version cannot read.
//
// A kind nobody has touched has no document at all, and "unreadable" is what
// puts the form beyond editing and tells somebody their labels are stored in a
// shape this version does not understand. Saying that about a configuration
// that was never written is a page nobody can use to write one.
func TestSyncConfigReadsAKindNobodyConfigured(t *testing.T) {
	dto := syncConfigToDTO(orgsync.Config{Kind: orgsync.KindLabels}, "")

	if dto.Unreadable {
		t.Error("a kind nobody configured was reported as one this version cannot read")
	}
	if string(dto.Document) != string(emptyDocument) {
		t.Errorf("document = %s, wanted an empty object", dto.Document)
	}
}

// TestSyncConfigSaysWhatThePermissionIsMissing is the guard on the answer an
// operator gets when they switch a kind on and nothing happens.
//
// Settings sync is the first kind needing a permission no existing installation
// has granted. Without the permission the sweep leaves the kind out and says so
// in a server log nobody reading the panel can see, so the page shows an empty
// plan list - which is also exactly what a sweep that has not come round yet
// looks like. The one thing the operator has to do is the one thing the page
// never said.
func TestSyncConfigSaysWhatThePermissionIsMissing(t *testing.T) {
	ungranted := storage.Target{Permissions: map[string]string{"issues": "write"}}

	dto := syncConfigAnswer(
		orgsync.Config{Kind: orgsync.KindSettings, Enabled: true}, ungranted, "")

	if dto.Unavailable == "" {
		t.Fatal("a kind the installation cannot act on was answered as though it could")
	}
	if !strings.Contains(dto.Unavailable, "administration") {
		t.Errorf("unavailable = %q, wanted the permission somebody has to grant named",
			dto.Unavailable)
	}
}

// And the other half: a granted permission must not be reported as missing, or
// every installation would be told to grant what it already has.
func TestSyncConfigSaysNothingOfAPermissionItHas(t *testing.T) {
	granted := storage.Target{Permissions: map[string]string{"administration": "write"}}

	dto := syncConfigAnswer(
		orgsync.Config{Kind: orgsync.KindSettings, Enabled: true}, granted, "")

	if dto.Unavailable != "" {
		t.Errorf("unavailable = %q, wanted nothing: the permission is granted", dto.Unavailable)
	}
}

// A kind can need more than its own permission, and the files kind is the one
// that does: GitHub keeps workflow files behind a permission of their own, so a
// configuration that names one needs Contents and Workflows both. Answered from
// the configuration rather than the kind, because a files configuration naming
// no workflow needs nothing extra.
func TestSyncConfigSaysWhenAWorkflowNeedsMore(t *testing.T) {
	contents := storage.Target{Permissions: map[string]string{"contents": "write"}}
	workflow := orgsync.Config{
		Kind: orgsync.KindFiles, Enabled: true,
		Document: []byte(`{"files":[{"path":".github/workflows/ci.yaml","content":"x"}]}`),
	}

	dto := syncConfigAnswer(workflow, contents, "")
	if !strings.Contains(dto.Unavailable, "workflows") {
		t.Errorf("unavailable = %q, wanted the workflows permission named", dto.Unavailable)
	}

	ordinary := orgsync.Config{
		Kind: orgsync.KindFiles, Enabled: true,
		Document: []byte(`{"files":[{"path":"CONTRIBUTING.md","content":"x"}]}`),
	}
	if answer := syncConfigAnswer(ordinary, contents, ""); answer.Unavailable != "" {
		t.Errorf("unavailable = %q, wanted nothing: no workflow is configured",
			answer.Unavailable)
	}

	granted := storage.Target{
		Permissions: map[string]string{"contents": "write", "workflows": "write"},
	}
	if answer := syncConfigAnswer(workflow, granted, ""); answer.Unavailable != "" {
		t.Errorf("unavailable = %q, wanted nothing: the permission is granted",
			answer.Unavailable)
	}
}
