package panel

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestConfigurationSettingsShareSaveAndRestoreExclusion(t *testing.T) {
	for _, test := range []struct {
		name      string
		body      string
		selection string
		catalog   int
	}{
		{
			name: "workspace Sync settings", catalog: 1,
			body: `{"sync_configs":[{"kind":"labels","enabled":true,"labels":[],
				"allow_removal":false,"excludes":[],"expected_revision":0}]}`,
			selection: `{"kind":"sync_config","sync_kind":"labels","expected_revision":1}`,
		},
		{
			name: "repository Sync overrides",
			body: `{"sync_overrides":[{"repository_id":"repository-20","kind":"labels",
				"enabled":false,"document":{},"expected_revision":0}]}`,
			selection: `{"kind":"sync_override","repository_id":"repository-20",
				"sync_kind":"labels","expected_revision":1}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			harness := newPanelHarness(t, "owner")
			session := harness.signIn(t)
			guard := &recordingPendingCIController{fakePendingCIController: harness.pendingCI}
			harness.server.pendingCI = guard
			guard.beforeSave = func() error { return context.Canceled }
			blocked := harness.request(t, http.MethodPut, workspaceSettingsBatchPath,
				strings.NewReader(test.body), session)
			requireResponse(t, blocked, "cancelled configuration save", http.StatusInternalServerError)
			requireConfigurationExclusion(t, guard, test.catalog, 1)
			// The same expected revision can still be saved: cancellation did not
			// partially write settings before entering the shared exclusion.
			saved := harness.request(t, http.MethodPut, workspaceSettingsBatchPath,
				strings.NewReader(test.body), session)
			requireResponse(t, saved, "configuration save after cancelled exclusion", http.StatusOK)
			var answer workspaceSettingsBatchResponse
			if err := json.Unmarshal(saved.Body.Bytes(), &answer); err != nil || answer.CheckpointID == nil {
				t.Fatalf("saved configuration = %#v, %v", answer, err)
			}
			requireConfigurationExclusion(t, guard, test.catalog*2, 2)
			path := workspaceSettingsCheckpointPath + *answer.CheckpointID + "/restore"
			restore := `{"state":"before","selections":[` + test.selection + `]}`
			guard.beforeSave = func() error { return context.Canceled }
			blocked = harness.request(t, http.MethodPost, path, strings.NewReader(restore), session)
			requireResponse(t, blocked, "cancelled configuration restore", http.StatusInternalServerError)
			requireConfigurationExclusion(t, guard, test.catalog*3, 3)
			restored := harness.request(t, http.MethodPost, path, strings.NewReader(restore), session)
			requireResponse(t, restored, "configuration restore after cancelled exclusion", http.StatusOK)
			requireConfigurationExclusion(t, guard, test.catalog*4, 4)
		})
	}
}

func requireConfigurationExclusion(t *testing.T, guard *recordingPendingCIController, catalog, exclusive int) {
	t.Helper()
	if guard.catalogCalls != catalog || guard.exclusiveCalls != exclusive ||
		len(guard.repositoryIDs) != 1 || guard.repositoryIDs[0] != "repository-20" {
		t.Fatalf("configuration exclusion = catalog %d, exclusive %d, repositories %v",
			guard.catalogCalls, guard.exclusiveCalls, guard.repositoryIDs)
	}
}
