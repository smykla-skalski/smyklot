package panel

import (
	"bytes"
	"context"
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

const workspaceSettingsCheckpointPath = "/panel/api/v1/targets/github:installation:10/settings/checkpoints/"

func TestSettingsCheckpointInspectionAndAtomicRestore(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	saved := harness.request(
		t, http.MethodPut, workspaceSettingsBatchPath,
		bytes.NewReader(mixedWorkspaceSettingsBatchBody(t, workspaceBatchRevisions{
			target: 1, repository: 1,
		})), session,
	)
	requireResponse(t, saved, "seed settings checkpoint", http.StatusOK, `"checkpoint_id":`)
	var savedAnswer workspaceSettingsBatchResponse
	if err := json.Unmarshal(saved.Body.Bytes(), &savedAnswer); err != nil ||
		savedAnswer.CheckpointID == nil {
		t.Fatalf("saved settings = %#v, %v", savedAnswer, err)
	}

	target, err := harness.store.GetTarget(t.Context(), "github:installation:10")
	if err != nil {
		t.Fatal(err)
	}
	changedTarget := harness.request(
		t, http.MethodPut, workspaceSettingsBatchPath,
		bytes.NewReader(targetWorkspaceSettingsBatchBody(t, target, false)), session,
	)
	requireResponse(t, changedTarget, "change current target", http.StatusOK, `"revision":3`)
	changedLabels := harness.request(t, http.MethodPut, workspaceSettingsBatchPath,
		strings.NewReader(`{"sync_configs":[{"kind":"labels","enabled":true,
			"labels":[{"name":"changed","color":"00ff00"}],
			"allow_removal":false,"excludes":[],"expected_revision":1}]}`), session)
	requireResponse(t, changedLabels, "change current labels", http.StatusOK, `"revision":2`)

	path := workspaceSettingsCheckpointPath + *savedAnswer.CheckpointID
	inspectionResponse := harness.request(t, http.MethodGet, path, nil, session)
	requireResponse(t, inspectionResponse, "inspect settings checkpoint", http.StatusOK,
		`"action":"installation.settings.saved"`, `"login":"owner"`,
		`"kind":"target"`, `"kind":"sync_config"`, `"differs":true`,
		`"restorable":true`, `"document":{"repository_default_enabled":true`)
	var inspection settingsCheckpointResponse
	if err := json.Unmarshal(inspectionResponse.Body.Bytes(), &inspection); err != nil {
		t.Fatal(err)
	}
	assertCheckpointItem(t, inspection, storage.SettingsCheckpointItemTarget, "", 3)
	assertCheckpointItem(t, inspection, storage.SettingsCheckpointItemSyncConfig, orgsync.KindLabels, 2)

	subscriber, unsubscribe := harness.server.events.subscribe("", "settings-restore")
	t.Cleanup(unsubscribe)
	restored := harness.request(t, http.MethodPost, path+"/restore", strings.NewReader(`{
		"state":"after",
		"selections":[
			{"kind":"target","expected_revision":3},
			{"kind":"sync_config","sync_kind":"labels","expected_revision":2}
		]}`), session)
	requireResponse(t, restored, "restore selected settings", http.StatusOK,
		`"checkpoint_id":`, `"target_id":"github:installation:10"`, `"revision":4`,
		`"kind":"labels"`)
	requirePanelEvent(t, subscriber.events, "target.changed")

	restoredTarget, err := harness.store.GetTarget(t.Context(), target.ID)
	if err != nil || !restoredTarget.RepositoryDefaultEnabled || restoredTarget.Revision != 4 {
		t.Fatalf("restored target = %#v, %v", restoredTarget, err)
	}
	restoredLabels, err := harness.store.GetSyncConfig(t.Context(), target.ID, orgsync.KindLabels)
	if err != nil || restoredLabels.Revision != 3 ||
		!strings.Contains(string(restoredLabels.Document), `"color":"ff0000"`) {
		t.Fatalf("restored labels = %#v, %v", restoredLabels, err)
	}
	assertSettingsRestoreAudit(t, harness, restoredTarget.ID, restored.Body.Bytes())
	var restoreAnswer workspaceSettingsBatchResponse
	if err := json.Unmarshal(restored.Body.Bytes(), &restoreAnswer); err != nil ||
		restoreAnswer.CheckpointID == nil {
		t.Fatalf("restore answer = %#v, %v", restoreAnswer, err)
	}
	audit := harness.request(t, http.MethodGet,
		"/panel/api/v1/targets/"+restoredTarget.ID+"/audit?limit=100", nil, session)
	requireResponse(t, audit, "settings checkpoint audit DTO", http.StatusOK,
		`"settings_checkpoint_id":"`+*restoreAnswer.CheckpointID+`"`)
}

func assertCheckpointItem(
	t *testing.T,
	inspection settingsCheckpointResponse,
	kind storage.SettingsCheckpointItemKind,
	syncKind orgsync.Kind,
	currentRevision int64,
) {
	t.Helper()
	for _, item := range inspection.Items {
		if item.Kind == kind && item.SyncKind == syncKind {
			if !item.Changed || !item.After.Available || !item.After.Differs ||
				!item.After.Restorable || item.After.State == nil || item.Current == nil ||
				item.Current.Revision != currentRevision || item.After.Incompatibility != nil {
				t.Fatalf("checkpoint item %s/%s = %#v", kind, syncKind, item)
			}
			return
		}
	}
	t.Fatalf("checkpoint has no %s/%s item: %#v", kind, syncKind, inspection.Items)
}

func assertSettingsRestoreAudit(
	t *testing.T,
	harness *panelHarness,
	targetID string,
	responseBody []byte,
) {
	t.Helper()
	var answer workspaceSettingsBatchResponse
	if err := json.Unmarshal(responseBody, &answer); err != nil || answer.CheckpointID == nil {
		t.Fatalf("restore answer = %#v, %v", answer, err)
	}
	page, err := harness.store.ListAudit(t.Context(), targetID, storage.AuditPageRequest{
		HistoryPageRequest: storage.HistoryPageRequest{Limit: 100},
		Scope:              storage.AuditAll, Change: storage.AuditChangeAll,
	})
	if err != nil {
		t.Fatal(err)
	}
	wanted, err := strconv.ParseInt(*answer.CheckpointID, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	matches := 0
	for _, entry := range page.Items {
		if entry.SettingsCheckpointID != nil && *entry.SettingsCheckpointID == wanted {
			matches++
			if entry.Action != "installation.settings.restored" {
				t.Fatalf("restore audit action = %q", entry.Action)
			}
		}
	}
	if matches != 1 {
		t.Fatalf("restore checkpoint audit matches = %d", matches)
	}
}

func TestWorkspaceSettingsCheckpointRestoreConflictAndNoop(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	target, err := harness.store.GetTarget(t.Context(), "github:installation:10")
	if err != nil {
		t.Fatal(err)
	}
	saved := saveTargetSettingsCheckpoint(t, harness, session, target, true)
	path := workspaceSettingsCheckpointPath + *saved.CheckpointID + "/restore"

	noop := harness.request(t, http.MethodPost, path,
		strings.NewReader(`{"state":"after","selections":[{"kind":"target","expected_revision":2}]}`), session)
	requireResponse(t, noop, "no-op settings restore", http.StatusConflict,
		`"code":"settings_restore_noop"`)

	target, err = harness.store.GetTarget(t.Context(), target.ID)
	if err != nil {
		t.Fatal(err)
	}
	changed := harness.request(t, http.MethodPut, workspaceSettingsBatchPath,
		bytes.NewReader(targetWorkspaceSettingsBatchBody(t, target, false)), session)
	requireResponse(t, changed, "change settings after checkpoint", http.StatusOK, `"revision":3`)
	stale := harness.request(t, http.MethodPost, path,
		strings.NewReader(`{"state":"after","selections":[{"kind":"target","expected_revision":2}]}`), session)
	requireResponse(t, stale, "stale settings restore", http.StatusConflict,
		`"code":"conflict"`, "inspect the checkpoint again")
	target, err = harness.store.GetTarget(t.Context(), target.ID)
	if err != nil || target.RepositoryDefaultEnabled || target.Revision != 3 {
		t.Fatalf("target after stale restore = %#v, %v", target, err)
	}
}

func saveTargetSettingsCheckpoint(
	t *testing.T,
	harness *panelHarness,
	session *http.Cookie,
	target storage.Target,
	enabled bool,
) workspaceSettingsBatchResponse {
	t.Helper()
	response := harness.request(t, http.MethodPut, workspaceSettingsBatchPath,
		bytes.NewReader(targetWorkspaceSettingsBatchBody(t, target, enabled)), session)
	requireResponse(t, response, "save target settings checkpoint", http.StatusOK,
		`"checkpoint_id":`)
	var answer workspaceSettingsBatchResponse
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil || answer.CheckpointID == nil {
		t.Fatalf("target settings save = %#v, %v", answer, err)
	}

	return answer
}

func TestWorkspaceSettingsCheckpointAuthorizationAndSameOrigin(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	ownerSession := harness.signIn(t)
	target, err := harness.store.GetTarget(t.Context(), "github:installation:10")
	if err != nil {
		t.Fatal(err)
	}
	saved := saveTargetSettingsCheckpoint(t, harness, ownerSession, target, true)
	path := workspaceSettingsCheckpointPath + *saved.CheckpointID
	unauthenticated := harness.request(t, http.MethodGet, path, nil, nil)
	requireResponse(t, unauthenticated, "unauthenticated inspection", http.StatusUnauthorized)

	viewerSession := createOrdinarySession(t, harness)
	viewerRole := storage.InstallationRoleViewer
	if _, err := harness.store.SetTargetAccess(t.Context(), storage.TargetAccessChange{
		TargetID: target.ID, SubjectAccountID: "github:test:user:ordinary",
		ActorAccountID: "github:test:user:1", Role: &viewerRole,
		ExpectedRevision: 0, ChangedAt: harness.now,
	}); err != nil {
		t.Fatal(err)
	}
	inspection := harness.request(t, http.MethodGet, path, nil, viewerSession)
	requireResponse(t, inspection, "viewer settings inspection", http.StatusOK)
	blocked := harness.request(t, http.MethodPost, path+"/restore",
		strings.NewReader(`{"state":"after","selections":[{"kind":"target","expected_revision":2}]}`), viewerSession)
	requireResponse(t, blocked, "viewer settings restore", http.StatusNotFound)

	request := httptest.NewRequest(http.MethodPost, path+"/restore",
		strings.NewReader(`{"state":"after","selections":[{"kind":"target","expected_revision":2}]}`))
	request.AddCookie(ownerSession)
	request.Header.Set("Origin", "https://untrusted.example")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	harness.handler.ServeHTTP(response, request)
	requireResponse(t, response, "cross-origin settings restore", http.StatusForbidden)
}

type settingsHistoryFailureStore struct {
	storage.Store
	inspectErr error
	restoreErr error
	seen       *storage.RestoreInstallationSettingsRequest
}

func (store settingsHistoryFailureStore) InspectInstallationSettingsCheckpoint(
	ctx context.Context,
	ref storage.SettingsCheckpointRef,
) (storage.SettingsCheckpointInspection, error) {
	if store.inspectErr != nil {
		return storage.SettingsCheckpointInspection{}, store.inspectErr
	}

	return store.Store.InspectInstallationSettingsCheckpoint(ctx, ref)
}

func (store settingsHistoryFailureStore) RestoreInstallationSettings(
	_ context.Context,
	request storage.RestoreInstallationSettingsRequest,
) (storage.SaveInstallationSettingsResult, error) {
	if store.seen != nil {
		*store.seen = request
	}

	return storage.SaveInstallationSettingsResult{}, store.restoreErr
}

func TestWorkspaceSettingsCheckpointMapsSafeErrors(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	realStore := harness.store
	path := workspaceSettingsCheckpointPath + "1"
	harness.server.store = settingsHistoryFailureStore{
		Store: realStore, inspectErr: storage.ErrSettingsCheckpointCorrupt,
	}
	corruptInspection := harness.request(t, http.MethodGet, path, nil, session)
	requireResponse(t, corruptInspection, "corrupt settings inspection",
		http.StatusInternalServerError, `"code":"settings_checkpoint_corrupt"`)

	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"not found", storage.ErrNotFound, http.StatusNotFound, "not_found"},
		{"conflict", storage.ErrConflict, http.StatusConflict, "conflict"},
		{"blocked", storage.ErrSettingsRestoreBlocked, http.StatusConflict, "settings_restore_blocked"},
		{"no-op", storage.ErrSettingsRestoreNoop, http.StatusConflict, "settings_restore_noop"},
		{"corrupt", storage.ErrSettingsCheckpointCorrupt, http.StatusInternalServerError, "settings_checkpoint_corrupt"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var seen storage.RestoreInstallationSettingsRequest
			harness.server.store = settingsHistoryFailureStore{
				Store: realStore, restoreErr: test.err, seen: &seen,
			}
			response := harness.request(t, http.MethodPost, path+"/restore",
				strings.NewReader(`{"state":"after","selections":[{"kind":"target","expected_revision":1}]}`), session)
			requireResponse(t, response, test.name, test.status, `"code":"`+test.code+`"`)
			if seen.DeploymentPendingCIQuietPeriod != 30*time.Second {
				t.Fatalf("deployment quiet period = %s", seen.DeploymentPendingCIQuietPeriod)
			}
		})
	}
}

func TestRootWorkspaceSettingsRestoreDelegatesRootErrors(t *testing.T) {
	harness := newPanelHarness(t, "root")
	session := harness.signIn(t)
	_, snapshot := seedNonOwnedWorkspace(t, harness)
	elevated := harness.request(t, http.MethodPost,
		"/panel/api/v1/root/workspaces/"+snapshot.TargetID+"/elevation",
		strings.NewReader(`{"acknowledged":true,"reason":"test restore failure mapping"}`),
		session)
	requireResponse(t, elevated, "start Root settings elevation", http.StatusCreated)

	realStore := harness.store
	path := "/panel/api/v1/root/workspaces/" + snapshot.TargetID +
		"/settings/checkpoints/1/restore"
	tests := []struct {
		name    string
		err     error
		status  int
		code    string
		message string
	}{
		{
			"conflict", storage.ErrConflict, http.StatusConflict,
			"owner_snapshot_unavailable", "fresh Owners are required before an operator visit",
		},
		{
			"not found", storage.ErrNotFound, http.StatusNotFound,
			"not_found", "the requested panel record was not found",
		},
		{
			"expired elevation", storage.ErrExpired, http.StatusGone,
			"elevation_expired", "the operator visit has ended",
		},
		{
			"revoked elevation", storage.ErrRevoked, http.StatusGone,
			"elevation_expired", "the operator visit has ended",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness.server.store = settingsHistoryFailureStore{
				Store: realStore, restoreErr: test.err,
			}
			response := harness.request(t, http.MethodPost, path,
				strings.NewReader(`{"state":"after","selections":[{"kind":"target","expected_revision":1}]}`),
				session)
			requireResponse(t, response, test.name, test.status,
				`"code":"`+test.code+`"`, test.message)
		})
	}
}
