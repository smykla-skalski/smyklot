package panel

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

const syncConfigBatchPath = "/panel/api/v1/targets/github:installation:10/sync/config"

func TestSyncConfigBatchSavesOneCheckpointAndNoOpWritesNothing(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)

	first := harness.request(t, http.MethodPut, syncConfigBatchPath, strings.NewReader(`{
		"changes":[
			{"kind":"labels","enabled":true,"expected_revision":0,"labels":[]},
			{"kind":"settings","enabled":false,"expected_revision":0,"document":{}}
		]}`), session)
	requireResponse(t, first, "batch save", http.StatusOK)
	var saved syncConfigBatchResponse
	if err := json.Unmarshal(first.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.CheckpointID == nil || len(saved.Configs) != len(orgsync.Kinds()) {
		t.Fatalf("batch save = %#v", saved)
	}

	audit := harness.request(t, http.MethodGet,
		"/panel/api/v1/targets/github:installation:10/audit?change=sync", nil, session)
	var history pageResponse[auditResponse]
	if err := json.Unmarshal(audit.Body.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	if history.Total != 1 || history.Items[0].SyncConfigCheckpointID == nil ||
		*history.Items[0].SyncConfigCheckpointID != *saved.CheckpointID {
		t.Fatalf("sync audit = %#v", history)
	}

	inspection := harness.request(t, http.MethodGet,
		syncConfigBatchPath+"/checkpoints/"+*saved.CheckpointID, nil, session)
	requireResponse(t, inspection, "checkpoint inspection", http.StatusOK)
	var checkpoint syncConfigCheckpointResponse
	if err := json.Unmarshal(inspection.Body.Bytes(), &checkpoint); err != nil {
		t.Fatal(err)
	}
	if checkpoint.Action != "sync.config.saved" || checkpoint.Actor.Login != "owner" ||
		len(checkpoint.AffectedKinds) != 2 || len(checkpoint.Kinds) != len(orgsync.Kinds()) {
		t.Fatalf("checkpoint inspection = %#v", checkpoint)
	}

	second := harness.request(t, http.MethodPut, syncConfigBatchPath, strings.NewReader(`{
		"changes":[
			{"kind":"labels","enabled":true,"expected_revision":1,"labels":[]},
			{"kind":"settings","enabled":false,"expected_revision":1,"document":{}}
		]}`), session)
	requireResponse(t, second, "no-op batch save", http.StatusOK)
	var unchanged syncConfigBatchResponse
	if err := json.Unmarshal(second.Body.Bytes(), &unchanged); err != nil {
		t.Fatal(err)
	}
	if unchanged.CheckpointID != nil {
		t.Fatalf("no-op checkpoint = %q", *unchanged.CheckpointID)
	}
	audit = harness.request(t, http.MethodGet,
		"/panel/api/v1/targets/github:installation:10/audit?change=sync", nil, session)
	if err := json.Unmarshal(audit.Body.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	if history.Total != 1 {
		t.Fatalf("no-op added audit history: %#v", history)
	}
}

func TestSingleKindSyncConfigUsesAuditedNoOpTransaction(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	path := "/panel/api/v1/targets/github:installation:10/sync/config/labels"

	first := harness.request(t, http.MethodPut, path, strings.NewReader(`{
		"enabled":true,"expected_revision":0,"labels":[],
		"allow_removal":false,"excludes":[]
	}`), session)
	requireResponse(t, first, "single-kind save", http.StatusOK)
	var saved syncConfigDTO
	if err := json.Unmarshal(first.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}

	second := harness.request(t, http.MethodPut, path, strings.NewReader(`{
		"enabled":true,"expected_revision":`+strconv.FormatInt(saved.Revision, 10)+`,"labels":[],
		"allow_removal":false,"excludes":[]
	}`), session)
	requireResponse(t, second, "single-kind no-op", http.StatusOK)

	audit := harness.request(t, http.MethodGet,
		"/panel/api/v1/targets/github:installation:10/audit?change=sync", nil, session)
	var history pageResponse[auditResponse]
	if err := json.Unmarshal(audit.Body.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	if len(history.Items) != 1 || history.Items[0].SyncConfigCheckpointID == nil {
		t.Fatalf("single-kind audit history = %#v", history.Items)
	}
}

func TestSyncConfigBatchRejectsMalformedDuplicateAndStaleChangesAtomically(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)

	for name, body := range map[string]string{
		"empty":     `{"changes":[]}`,
		"duplicate": `{"changes":[{"kind":"labels","enabled":true,"expected_revision":0,"labels":[]},{"kind":"labels","enabled":false,"expected_revision":0,"labels":[]}]}`,
		"unknown":   `{"changes":[{"kind":"widgets","enabled":true,"expected_revision":0,"document":{}}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			response := harness.request(t, http.MethodPut, syncConfigBatchPath,
				strings.NewReader(body), session)
			requireResponse(t, response, name, http.StatusBadRequest)
		})
	}

	seed := harness.request(t, http.MethodPut, syncConfigBatchPath, strings.NewReader(`{
		"changes":[{"kind":"labels","enabled":true,"expected_revision":0,"labels":[]}]}`),
		session)
	requireResponse(t, seed, "seed labels", http.StatusOK)

	stale := harness.request(t, http.MethodPut, syncConfigBatchPath, strings.NewReader(`{
		"changes":[
			{"kind":"labels","enabled":true,"expected_revision":1,"labels":[{"name":"bug","color":"d73a4a"}]},
			{"kind":"settings","enabled":true,"expected_revision":1,"document":{}}
		]}`), session)
	requireResponse(t, stale, "stale batch", http.StatusConflict)

	labels := harness.request(t, http.MethodGet, syncConfigBatchPath+"/labels", nil, session)
	var current syncConfigDTO
	if err := json.Unmarshal(labels.Body.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	if current.Revision != 1 || len(current.Labels) != 0 {
		t.Fatalf("partial batch write survived rollback: %#v", current)
	}
	settings := harness.request(t, http.MethodGet, syncConfigBatchPath+"/settings", nil, session)
	if err := json.Unmarshal(settings.Body.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	if current.Revision != 0 {
		t.Fatalf("stale absent kind was inserted: %#v", current)
	}
}

func TestSyncConfigBatchRequiresSameOrigin(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	request := httptest.NewRequest(http.MethodPut, syncConfigBatchPath,
		strings.NewReader(`{"changes":[]}`))
	request.AddCookie(session)
	request.Header.Set("Origin", "https://attacker.example")
	response := httptest.NewRecorder()
	harness.handler.ServeHTTP(response, request)
	requireResponse(t, response, "cross-origin batch save", http.StatusForbidden)

	saved := saveSyncBatch(t, harness, session, `{
		"changes":[{"kind":"labels","enabled":true,"expected_revision":0,"labels":[]}]}`)
	restore := httptest.NewRequest(http.MethodPost,
		syncConfigBatchPath+"/checkpoints/"+*saved.CheckpointID+"/restore",
		strings.NewReader(`{"kinds":[{"kind":"labels","expected_revision":1}]}`))
	restore.AddCookie(session)
	restore.Header.Set("Origin", "https://attacker.example")
	response = httptest.NewRecorder()
	harness.handler.ServeHTTP(response, restore)
	requireResponse(t, response, "cross-origin checkpoint restore", http.StatusForbidden)
}

func TestSyncConfigRestoreSelectsKindsAndCreatesHistory(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	original := saveSyncBatch(t, harness, session, `{
		"changes":[
			{"kind":"labels","enabled":true,"expected_revision":0,"labels":[]},
			{"kind":"settings","enabled":true,"expected_revision":0,"document":{}}
		]}`)
	saveSyncBatch(t, harness, session, `{
		"changes":[
			{"kind":"labels","enabled":true,"expected_revision":1,"labels":[{"name":"bug","color":"d73a4a"}]},
			{"kind":"files","enabled":true,"expected_revision":0,"document":{"files":[]}}
		]}`)

	inspection := harness.request(t, http.MethodGet,
		syncConfigBatchPath+"/checkpoints/"+*original.CheckpointID, nil, session)
	var checkpoint syncConfigCheckpointResponse
	if err := json.Unmarshal(inspection.Body.Bytes(), &checkpoint); err != nil {
		t.Fatal(err)
	}
	differing := map[string]bool{}
	for _, kind := range checkpoint.Kinds {
		differing[kind.Kind] = kind.DiffersFromCurrent
	}
	if !differing["labels"] || !differing["files"] || differing["settings"] {
		t.Fatalf("checkpoint differences = %#v", differing)
	}

	restore := harness.request(t, http.MethodPost,
		syncConfigBatchPath+"/checkpoints/"+*original.CheckpointID+"/restore",
		strings.NewReader(`{"kinds":[
			{"kind":"labels","expected_revision":2},
			{"kind":"files","expected_revision":1}
		]}`), session)
	requireResponse(t, restore, "selected restore", http.StatusOK)
	var restored syncConfigBatchResponse
	if err := json.Unmarshal(restore.Body.Bytes(), &restored); err != nil {
		t.Fatal(err)
	}
	if restored.CheckpointID == nil {
		t.Fatal("restore created no checkpoint")
	}

	labels := harness.request(t, http.MethodGet, syncConfigBatchPath+"/labels", nil, session)
	var labelConfig syncConfigDTO
	if err := json.Unmarshal(labels.Body.Bytes(), &labelConfig); err != nil {
		t.Fatal(err)
	}
	if labelConfig.Revision != 3 || len(labelConfig.Labels) != 0 {
		t.Fatalf("labels after restore = %#v", labelConfig)
	}
	files := harness.request(t, http.MethodGet, syncConfigBatchPath+"/files", nil, session)
	var fileConfig syncConfigDTO
	if err := json.Unmarshal(files.Body.Bytes(), &fileConfig); err != nil {
		t.Fatal(err)
	}
	if fileConfig.Revision != 0 {
		t.Fatalf("files after absent-kind restore = %#v", fileConfig)
	}

	audit := harness.request(t, http.MethodGet,
		"/panel/api/v1/targets/github:installation:10/audit?change=sync", nil, session)
	var history pageResponse[auditResponse]
	if err := json.Unmarshal(audit.Body.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	if history.Total != 3 || history.Items[0].Action != "sync.config.restored" ||
		history.Items[0].SyncConfigCheckpointID == nil ||
		*history.Items[0].SyncConfigCheckpointID != *restored.CheckpointID {
		t.Fatalf("restore audit = %#v", history)
	}
}

func TestSyncConfigRestoreRejectsStaleAndIncompatibleSelections(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	original := saveSyncBatch(t, harness, session, `{
		"changes":[{"kind":"labels","enabled":true,"expected_revision":0,"labels":[]}]}`)
	saveSyncBatch(t, harness, session, `{
		"changes":[{"kind":"labels","enabled":true,"expected_revision":1,"labels":[{"name":"bug","color":"d73a4a"}]}]}`)

	stale := harness.request(t, http.MethodPost,
		syncConfigBatchPath+"/checkpoints/"+*original.CheckpointID+"/restore",
		strings.NewReader(`{"kinds":[{"kind":"labels","expected_revision":1}]}`), session)
	requireResponse(t, stale, "stale restore", http.StatusConflict)

	invalid, err := harness.store.SetSyncConfig(t.Context(), orgsync.ConfigChange{
		TargetID: "github:installation:10", Kind: orgsync.KindLabels, Enabled: true,
		Document: []byte(`{"labels":[{"name":""}]}`), ActorID: "github:test:user:1",
		Now: harness.now, Revision: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	history, err := harness.store.ListAudit(
		t.Context(),
		"github:installation:10",
		storage.AuditPageRequest{
			HistoryPageRequest: storage.HistoryPageRequest{Limit: 1},
			Change:             storage.AuditChangeSync,
		},
	)
	if err != nil || history.Items[0].SyncConfigCheckpointID == nil {
		t.Fatalf("invalid checkpoint audit = %#v, %v", history, err)
	}
	invalidCheckpoint := history.Items[0].SyncConfigCheckpointID
	valid := harness.request(t, http.MethodPut, syncConfigBatchPath+"/labels", strings.NewReader(
		`{"enabled":true,"expected_revision":3,"labels":[]}`), session)
	requireResponse(t, valid, "replace invalid stored state", http.StatusOK)

	refused := harness.request(t, http.MethodPost,
		syncConfigBatchPath+"/checkpoints/"+strconv.FormatInt(*invalidCheckpoint, 10)+"/restore",
		strings.NewReader(`{"kinds":[{"kind":"labels","expected_revision":4}]}`), session)
	requireResponse(t, refused, "incompatible restore", http.StatusBadRequest,
		"invalid_sync_config", "labels")

	current, err := harness.store.GetSyncConfig(t.Context(), "github:installation:10", orgsync.KindLabels)
	if err != nil || current.Revision != invalid.Revision+1 ||
		string(current.Document) != `{"labels":[],"allow_removal":false}` {
		t.Fatalf("incompatible restore changed state: %#v, %v", current, err)
	}
}

func TestSyncCheckpointInspectionIsReadableButRestoreNeedsWriteAccess(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	ownerSession := harness.signIn(t)
	saved := saveSyncBatch(t, harness, ownerSession, `{
		"changes":[{"kind":"labels","enabled":true,"expected_revision":0,"labels":[]}]}`)
	viewerSession := createOrdinarySession(t, harness)
	viewerRole := storage.InstallationRoleViewer
	if _, err := harness.store.SetTargetAccess(t.Context(), storage.TargetAccessChange{
		TargetID: "github:installation:10", SubjectAccountID: "github:test:user:ordinary",
		ActorAccountID: "github:test:user:1", Role: &viewerRole,
		ExpectedRevision: 0, ChangedAt: harness.now,
	}); err != nil {
		t.Fatal(err)
	}
	path := syncConfigBatchPath + "/checkpoints/" + *saved.CheckpointID
	inspection := harness.request(t, http.MethodGet, path, nil, viewerSession)
	requireResponse(t, inspection, "viewer checkpoint inspection", http.StatusOK)
	restore := harness.request(t, http.MethodPost, path+"/restore",
		strings.NewReader(`{"kinds":[{"kind":"labels","expected_revision":1}]}`), viewerSession)
	requireResponse(t, restore, "viewer restore", http.StatusNotFound)
}

func TestRootSyncCheckpointInspectionAndElevatedRestore(t *testing.T) {
	harness := newPanelHarness(t, "root")
	rootSession := harness.signIn(t)
	owner, target := seedNonOwnedInstallation(t, harness)
	original, err := harness.store.SetSyncConfigs(t.Context(), orgsync.ConfigBatchChange{
		TargetID: target.TargetID, ActorID: owner.ID, Now: harness.now,
		Changes: []orgsync.ConfigPatch{{
			Kind: orgsync.KindLabels, Enabled: true, Document: []byte(`{"labels":[]}`),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = harness.store.SetSyncConfigs(t.Context(), orgsync.ConfigBatchChange{
		TargetID: target.TargetID, ActorID: owner.ID, Now: harness.now.Add(time.Minute),
		Changes: []orgsync.ConfigPatch{{
			Kind: orgsync.KindLabels, Enabled: true,
			Document: []byte(`{"labels":[{"name":"changed","color":"d73a4a"}]}`),
			Revision: 1,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	rootBase := "/panel/api/v1/root/installations/" + target.TargetID +
		"/sync/config/checkpoints/" + strconv.FormatInt(*original.CheckpointID, 10)
	inspection := harness.request(t, http.MethodGet, rootBase, nil, rootSession)
	requireResponse(t, inspection, "Root checkpoint inspection", http.StatusOK,
		`"action":"sync.config.saved"`, `"differs_from_current":true`)

	globalAudit := harness.request(t, http.MethodGet,
		"/panel/api/v1/root/history/audit?category=configuration&limit=10", nil, rootSession)
	requireResponse(t, globalAudit, "Root checkpoint audit", http.StatusOK,
		`"target_id":"`+target.TargetID+`"`,
		`"sync_config_checkpoint_id":"`+strconv.FormatInt(*original.CheckpointID, 10)+`"`)

	body := strings.NewReader(`{"kinds":[{"kind":"labels","expected_revision":2}]}`)
	blocked := harness.request(t, http.MethodPost, rootBase+"/restore", body, rootSession)
	requireResponse(t, blocked, "Root restore without elevation", http.StatusForbidden,
		`"code":"elevation_required"`)

	elevated := harness.request(t, http.MethodPost,
		"/panel/api/v1/root/installations/"+target.TargetID+"/elevation",
		strings.NewReader(`{"acknowledged":true,"reason":"restore Sync history"}`), rootSession)
	requireResponse(t, elevated, "start Root elevation", http.StatusCreated)
	restored := harness.request(t, http.MethodPost, rootBase+"/restore",
		strings.NewReader(`{"kinds":[{"kind":"labels","expected_revision":2}]}`), rootSession)
	requireResponse(t, restored, "elevated Root restore", http.StatusOK,
		`"checkpoint_id":`, `"revision":3`)

	notifications, err := harness.store.ListSecurityNotifications(
		t.Context(), owner.ID, storage.NotificationPageRequest{Limit: 10},
	)
	if err != nil || notifications.Unread != 1 ||
		notifications.Items[0].Action != "sync.config.restored" {
		t.Fatalf("Root restore notifications = %#v, %v", notifications, err)
	}
}

func saveSyncBatch(
	t *testing.T,
	harness *panelHarness,
	session *http.Cookie,
	body string,
) syncConfigBatchResponse {
	t.Helper()
	response := harness.request(t, http.MethodPut, syncConfigBatchPath,
		strings.NewReader(body), session)
	requireResponse(t, response, "save sync batch", http.StatusOK)
	var answer syncConfigBatchResponse
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if answer.CheckpointID == nil {
		t.Fatal("save sync batch created no checkpoint")
	}
	return answer
}
