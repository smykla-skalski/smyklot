package panel

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

const workspaceSettingsBatchPath = "/panel/api/v1/targets/github:installation:10/settings"

type workspaceBatchRevisions struct {
	target, repository, files, labels, override int64
}

type countingPendingCIGates struct{ wakes int }

func (gates *countingPendingCIGates) WakePendingCIGates() { gates.wakes++ }

type recordingPendingCIController struct {
	*fakePendingCIController
	exclusiveCalls int
	catalogCalls   int
	repositoryIDs  []string
	beforeSave     func() error
}

func (controller *recordingPendingCIController) Exclusive(
	_ context.Context,
	repositoryIDs []string,
	operation func() error,
) error {
	controller.exclusiveCalls++
	controller.repositoryIDs = append([]string(nil), repositoryIDs...)
	if controller.beforeSave != nil {
		before := controller.beforeSave
		controller.beforeSave = nil
		if err := before(); err != nil {
			return err
		}
	}

	return operation()
}

func (controller *recordingPendingCIController) ExclusiveCatalog(
	_ context.Context,
	operation func() error,
) error {
	controller.catalogCalls++

	return operation()
}

func mixedWorkspaceSettingsBatchBody(
	t *testing.T,
	revisions workspaceBatchRevisions,
) []byte {
	t.Helper()
	body := map[string]any{
		"target": map[string]any{
			"repository_default_enabled": true,
			"pending_ci_mode_default":    "checks",
			"pending_ci_branch_patterns_default": map[string]any{
				"include": []string{"~DEFAULT_BRANCH"}, "exclude": []string{},
			},
			"pending_ci_quiet_period_seconds_override": int64(0),
			"path_index_interval_seconds_override":     nil,
			"config_patch":                             map[string]any{"quiet_success": true},
			"expected_revision":                        revisions.target,
		},
		"repositories": []any{map[string]any{
			"repository_id": "repository-20", "enabled_override": false,
			"pending_ci_mode_override": nil, "pending_ci_branch_patterns_override": nil,
			"pending_ci_quiet_period_seconds_override": nil,
			"path_index_interval_seconds_override":     nil, "config_patch": map[string]any{},
			"ignore_repository_file": false, "expected_revision": revisions.repository,
		}},
		"sync_configs": []any{
			map[string]any{
				"kind": "labels", "enabled": true,
				"labels":        []any{map[string]any{"name": "ci/skip", "color": "ff0000"}},
				"allow_removal": false, "excludes": []string{},
				"expected_revision": revisions.labels,
			},
			map[string]any{
				"kind": "files", "enabled": true,
				"document": map[string]any{"files": []any{map[string]any{
					"path": "renovate.json", "content": "{}",
				}}},
				"expected_revision": revisions.files,
			},
		},
		"sync_overrides": []any{map[string]any{
			"repository_id": "repository-20", "kind": "files", "enabled": nil,
			"document": map[string]any{"merges": []any{map[string]any{
				"path":      "renovate.json",
				"overrides": map[string]any{"timezone": "Europe/Warsaw"},
			}}},
			"expected_revision": revisions.override,
		}},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}

	return encoded
}

func requireMixedWorkspaceSettingsBatchAnswer(
	t *testing.T,
	answer workspaceSettingsBatchResponse,
) int64 {
	t.Helper()
	if answer.CheckpointID == nil || answer.Target == nil || answer.Target.Revision != 2 ||
		len(answer.Repositories) != 1 || answer.Repositories[0].Revision != 2 ||
		len(answer.SyncConfigs) != 2 || answer.SyncConfigs[0].Kind != orgsync.KindFiles ||
		answer.SyncConfigs[1].Kind != orgsync.KindLabels ||
		len(answer.SyncOverrides) != 1 || answer.SyncOverrides[0].Revision != 1 {
		t.Fatalf("canonical mixed answer = %#v", answer)
	}
	checkpointID, err := strconv.ParseInt(*answer.CheckpointID, 10, 64)
	if err != nil {
		t.Fatal(err)
	}

	return checkpointID
}

func requireWorkspaceSettingsCheckpoint(
	t *testing.T,
	harness *panelHarness,
	checkpointID int64,
) {
	t.Helper()
	inspection, err := harness.store.InspectInstallationSettingsCheckpoint(
		t.Context(), storage.SettingsCheckpointRef{
			ID: checkpointID, Scope: storage.SettingsCheckpointScopeInstallation,
			TargetID: "github:installation:10",
		})
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Checkpoint.Items) != 5 {
		t.Fatalf("checkpoint items = %d, want 5", len(inspection.Checkpoint.Items))
	}
	audit, err := harness.store.ListAudit(t.Context(), "github:installation:10", storage.AuditPageRequest{
		HistoryPageRequest: storage.HistoryPageRequest{Limit: 100},
		Scope:              storage.AuditAll, Change: storage.AuditChangeAll,
	})
	if err != nil {
		t.Fatal(err)
	}
	checkpointAudits := 0
	for _, entry := range audit.Items {
		if entry.SettingsCheckpointID != nil && *entry.SettingsCheckpointID == checkpointID {
			checkpointAudits++
		}
	}
	if checkpointAudits != 1 {
		t.Fatalf("checkpoint audit entries = %d, want 1", checkpointAudits)
	}
}

func TestWorkspaceSettingsBatchSavesMixedResourcesOnce(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	gates := &countingPendingCIGates{}
	harness.server.gates = gates
	serialization := &recordingPendingCIController{fakePendingCIController: harness.pendingCI}
	harness.server.pendingCI = serialization
	subscriber, unsubscribe := harness.server.events.subscribe("", "settings-batch")
	t.Cleanup(unsubscribe)

	response := harness.request(
		t, http.MethodPut, workspaceSettingsBatchPath,
		strings.NewReader(string(mixedWorkspaceSettingsBatchBody(t, workspaceBatchRevisions{
			target: 1, repository: 1,
		}))), session,
	)
	requireResponse(t, response, "mixed settings batch", http.StatusOK, `"checkpoint_id":`)
	var answer workspaceSettingsBatchResponse
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	checkpointID := requireMixedWorkspaceSettingsBatchAnswer(t, answer)
	requireWorkspaceSettingsCheckpoint(t, harness, checkpointID)
	if harness.pendingCI.wakes != 1 || gates.wakes != 1 {
		t.Fatalf("fan-out = Pending CI %d, gates %d", harness.pendingCI.wakes, gates.wakes)
	}
	if serialization.catalogCalls != 1 || serialization.exclusiveCalls != 1 ||
		len(serialization.repositoryIDs) != 1 || serialization.repositoryIDs[0] != "repository-20" {
		t.Fatalf("serialization = catalog %d, repositories %#v",
			serialization.catalogCalls, serialization.repositoryIDs)
	}
	requirePanelEvent(t, subscriber.events, "target.changed")

	noOp := harness.request(
		t, http.MethodPut, workspaceSettingsBatchPath,
		strings.NewReader(string(mixedWorkspaceSettingsBatchBody(t, workspaceBatchRevisions{
			target: 2, repository: 2, files: 1, labels: 1, override: 1,
		}))), session,
	)
	requireResponse(t, noOp, "no-op settings batch", http.StatusOK)
	var unchanged workspaceSettingsBatchResponse
	if err := json.Unmarshal(noOp.Body.Bytes(), &unchanged); err != nil {
		t.Fatal(err)
	}
	if unchanged.CheckpointID != nil || harness.pendingCI.wakes != 1 || gates.wakes != 1 {
		t.Fatalf("no-op answer/signals = %#v, %d, %d", unchanged, harness.pendingCI.wakes, gates.wakes)
	}
	if serialization.catalogCalls != 2 || serialization.exclusiveCalls != 2 {
		t.Fatalf("no-op serialization = catalog %d, exclusive %d",
			serialization.catalogCalls, serialization.exclusiveCalls)
	}
	select {
	case event := <-subscriber.events:
		t.Fatalf("no-op announced %#v", event)
	default:
	}
}

func TestWorkspaceSettingsBatchSyncOnlySkipsPendingCISerialization(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	gates := &countingPendingCIGates{}
	harness.server.gates = gates
	serialization := &recordingPendingCIController{fakePendingCIController: harness.pendingCI}
	harness.server.pendingCI = serialization
	subscriber, unsubscribe := harness.server.events.subscribe("", "sync-only-settings")
	t.Cleanup(unsubscribe)
	body := `{"sync_configs":[{"kind":"labels","enabled":true,"labels":[],
		"allow_removal":false,"excludes":[],"expected_revision":0}]}`
	response := harness.request(
		t, http.MethodPut, workspaceSettingsBatchPath, strings.NewReader(body), session,
	)
	requireResponse(t, response, "Sync-only settings batch", http.StatusOK, `"checkpoint_id":`)
	if serialization.catalogCalls != 0 || serialization.exclusiveCalls != 0 ||
		harness.pendingCI.wakes != 0 || gates.wakes != 0 {
		t.Fatalf("Sync-only coordination = catalog %d, exclusive %d, wakes %d/%d",
			serialization.catalogCalls, serialization.exclusiveCalls,
			harness.pendingCI.wakes, gates.wakes)
	}
	requirePanelEvent(t, subscriber.events, "target.changed")
}

func TestWorkspaceSettingsBatchMapsTransactionDocumentRace(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	seed := harness.request(t, http.MethodPut,
		workspaceSettingsBatchPath,
		strings.NewReader(`{"sync_configs":[{"kind":"files","enabled":true,
			"document":{"files":[{"path":"renovate.json","content":"{}"}]},
			"expected_revision":0}]}`), session)
	requireResponse(t, seed, "seed files config", http.StatusOK, `"revision":1`)
	serialization := &recordingPendingCIController{fakePendingCIController: harness.pendingCI}
	serialization.beforeSave = func() error {
		_, err := harness.store.SaveInstallationSettings(
			t.Context(), storage.SaveInstallationSettingsRequest{
				TargetID: "github:installation:10", ActorAccountID: "github:test:user:1",
				ChangedAt: harness.now.Add(1), SyncConfigs: []storage.InstallationSyncConfigChange{{
					Kind: orgsync.KindFiles, Enabled: true,
					Document:         []byte(`{"files":[{"path":"other.json","content":"{}"}]}`),
					ExpectedRevision: 1,
				}},
			},
		)
		return err
	}
	harness.server.pendingCI = serialization
	body := `{"repositories":[{"repository_id":"repository-20","enabled_override":null,
		"pending_ci_mode_override":null,"pending_ci_branch_patterns_override":null,
		"pending_ci_quiet_period_seconds_override":null,
		"path_index_interval_seconds_override":null,"config_patch":{},
		"ignore_repository_file":false,"expected_revision":1}],
		"sync_overrides":[{"repository_id":"repository-20","kind":"files",
		"enabled":null,"document":{"merges":[{"path":"renovate.json",
		"overrides":{"timezone":"Europe/Warsaw"}}]},
		"expected_revision":0}]}`
	response := harness.request(
		t, http.MethodPut, workspaceSettingsBatchPath, strings.NewReader(body), session,
	)
	requireResponse(t, response, "transaction document race", http.StatusBadRequest,
		`"code":"invalid_sync_config"`, "not one of the files synchronized")
	if _, err := harness.store.GetSyncRepositoryOverride(
		t.Context(), "github:installation:10", "repository-20", orgsync.KindFiles,
	); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("override after transaction validation = %v", err)
	}
}

func TestWorkspaceSettingsBatchValidatesOverrideAgainstProposedFiles(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)

	accepted := harness.request(
		t, http.MethodPut, workspaceSettingsBatchPath,
		strings.NewReader(string(mixedWorkspaceSettingsBatchBody(t, workspaceBatchRevisions{
			target: 1, repository: 1,
		}))), session,
	)
	requireResponse(t, accepted, "proposed files override", http.StatusOK)

	other := strings.ReplaceAll(
		string(mixedWorkspaceSettingsBatchBody(t, workspaceBatchRevisions{
			target: 2, repository: 2, files: 1, labels: 1, override: 1,
		})), "renovate.json", "other.json",
	)
	// Keep the proposed config on renovate.json, while asking the override to
	// adjust other.json.
	other = strings.Replace(other, "other.json", "renovate.json", 1)
	rejected := harness.request(
		t, http.MethodPut, workspaceSettingsBatchPath, strings.NewReader(other), session,
	)
	requireResponse(t, rejected, "mismatched proposed files", http.StatusBadRequest,
		`"code":"invalid_sync_override"`)
	config, err := harness.store.GetSyncConfig(
		t.Context(), "github:installation:10", orgsync.KindFiles,
	)
	if err != nil || config.Revision != 1 {
		t.Fatalf("files after rejected preflight = %#v, %v", config, err)
	}
}

func TestWorkspaceSettingsBatchReportsAllStaleResources(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	seed := harness.request(
		t, http.MethodPut, workspaceSettingsBatchPath,
		strings.NewReader(string(mixedWorkspaceSettingsBatchBody(t, workspaceBatchRevisions{
			target: 1, repository: 1,
		}))), session,
	)
	requireResponse(t, seed, "seed mixed batch", http.StatusOK)

	targetUpdate := `{"target":{
		"repository_default_enabled":false,"pending_ci_mode_default":"checks",
		"pending_ci_branch_patterns_default":{"include":["~DEFAULT_BRANCH"],"exclude":[]},
		"pending_ci_quiet_period_seconds_override":0,
		"path_index_interval_seconds_override":null,
		"config_patch":{"quiet_success":false},"expected_revision":2}}`
	requireResponse(t, harness.request(t, http.MethodPut,
		workspaceSettingsBatchPath,
		strings.NewReader(targetUpdate), session), "concurrent target", http.StatusOK)
	repositoryUpdate := `{"repositories":[{"repository_id":"repository-20",
		"enabled_override":true,"pending_ci_mode_override":null,
		"pending_ci_branch_patterns_override":null,
		"pending_ci_quiet_period_seconds_override":null,
		"path_index_interval_seconds_override":null,
		"config_patch":{"command_prefix":"!"},"ignore_repository_file":false,
		"expected_revision":2}]}`
	requireResponse(t, harness.request(t, http.MethodPut,
		workspaceSettingsBatchPath,
		strings.NewReader(repositoryUpdate), session), "concurrent repository", http.StatusOK)
	labelsUpdate := `{"sync_configs":[{"kind":"labels","enabled":true,
		"labels":[{"name":"ci/skip","color":"00ff00"}],
		"allow_removal":false,"excludes":[],"expected_revision":1}]}`
	requireResponse(t, harness.request(t, http.MethodPut,
		workspaceSettingsBatchPath,
		strings.NewReader(labelsUpdate), session), "concurrent labels", http.StatusOK)
	subscriber, unsubscribe := harness.server.events.subscribe("", "settings-conflict")
	t.Cleanup(unsubscribe)

	conflicted := harness.request(
		t, http.MethodPut, workspaceSettingsBatchPath,
		strings.NewReader(string(mixedWorkspaceSettingsBatchBody(t, workspaceBatchRevisions{
			target: 2, repository: 2, files: 1, labels: 1, override: 1,
		}))), session,
	)
	requireResponse(t, conflicted, "stale mixed batch", http.StatusConflict,
		`"resource":"target"`, `"resource":"repository"`,
		`"resource":"sync_config"`, `"actual_revision":3`, `"latest":`)
	var answer workspaceSettingsBatchConflictResponse
	if err := json.Unmarshal(conflicted.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if len(answer.Error.Conflicts) != 3 ||
		answer.Error.Conflicts[2].Kind != orgsync.KindLabels ||
		answer.Error.Conflicts[2].ActualRevision != 2 {
		t.Fatalf("conflicts = %#v", answer.Error.Conflicts)
	}
	target, _ := harness.store.GetTarget(t.Context(), "github:installation:10")
	repository, _ := harness.store.GetRepository(t.Context(), target.ID, "repository-20")
	labels, _ := harness.store.GetSyncConfig(t.Context(), target.ID, orgsync.KindLabels)
	if target.Revision != 3 || target.RepositoryDefaultEnabled || repository.Revision != 3 ||
		repository.EnabledOverride == nil || !*repository.EnabledOverride || labels.Revision != 2 {
		t.Fatalf("state after rollback = target %#v, repository %#v, labels %#v", target, repository, labels)
	}
	select {
	case event := <-subscriber.events:
		t.Fatalf("conflict announced %#v", event)
	default:
	}
}
