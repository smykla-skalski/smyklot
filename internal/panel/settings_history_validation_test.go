package panel

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

func TestInstallationSettingsRestoreRejectsMalformedSelections(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	path := installationSettingsCheckpointPath + "1/restore"
	tests := []struct {
		name string
		body string
	}{
		{"empty", `{}`},
		{"missing state", `{"selections":[{"kind":"target","expected_revision":1}]}`},
		{"unknown state", `{"state":"middle","selections":[{"kind":"target","expected_revision":1}]}`},
		{"empty selections", `{"state":"after","selections":[]}`},
		{"unknown field", `{"state":"after","selections":[],"surprise":true}`},
		{"missing revision", `{"state":"after","selections":[{"kind":"target"}]}`},
		{"null revision", `{"state":"after","selections":[{"kind":"target","expected_revision":null}]}`},
		{"negative revision", `{"state":"after","selections":[{"kind":"target","expected_revision":-1}]}`},
		{"unknown kind", `{"state":"after","selections":[{"kind":"future","expected_revision":1}]}`},
		{"Root kind", `{"state":"after","selections":[{"kind":"runtime","expected_revision":1}]}`},
		{"target discriminator", `{"state":"after","selections":[{"kind":"target","repository_id":"repository-20","expected_revision":1}]}`},
		{"missing repository", `{"state":"after","selections":[{"kind":"repository","expected_revision":1}]}`},
		{"missing Sync kind", `{"state":"after","selections":[{"kind":"sync_config","expected_revision":0}]}`},
		{"unknown Sync kind", `{"state":"after","selections":[{"kind":"sync_config","sync_kind":"future","expected_revision":0}]}`},
		{"override missing repository", `{"state":"after","selections":[{"kind":"sync_override","sync_kind":"files","expected_revision":0}]}`},
		{"duplicate", `{"state":"after","selections":[{"kind":"target","expected_revision":1},{"kind":"target","expected_revision":1}]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := harness.request(t, http.MethodPost, path,
				strings.NewReader(test.body), session)
			requireResponse(t, response, test.name, http.StatusBadRequest,
				`"code":"invalid_request"`)
		})
	}

	var selections strings.Builder
	selections.WriteString(`{"state":"after","selections":[`)
	for index := 0; index <= storage.MaxInstallationSettingsRestoreSelections; index++ {
		if index > 0 {
			selections.WriteByte(',')
		}
		selections.WriteString(`{"kind":"repository","repository_id":"repository-`)
		selections.WriteString(strconv.Itoa(index))
		selections.WriteString(`","expected_revision":1}`)
	}
	selections.WriteString(`]}`)
	response := harness.request(t, http.MethodPost, path,
		strings.NewReader(selections.String()), session)
	requireResponse(t, response, "too many selections", http.StatusBadRequest,
		`"code":"invalid_request"`)
}

func TestInstallationSettingsCheckpointCannotCrossScopeOrTarget(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	rootCheckpointID := saveRuntimeSettingsCheckpoint(t, harness, "debug", 0)
	wrongScope := harness.request(t, http.MethodGet,
		installationSettingsCheckpointPath+strconv.FormatInt(rootCheckpointID, 10), nil, session)
	requireResponse(t, wrongScope, "Root checkpoint through installation scope",
		http.StatusNotFound, `"code":"not_found"`)

	target, err := harness.store.GetTarget(t.Context(), "github:installation:10")
	if err != nil {
		t.Fatal(err)
	}
	saved := saveTargetSettingsCheckpoint(t, harness, session, target, true)
	_, other := seedNonOwnedInstallation(t, harness)
	wrongTarget := harness.request(t, http.MethodGet,
		"/panel/api/v1/root/installations/"+other.TargetID+"/settings/checkpoints/"+
			*saved.CheckpointID, nil, session)
	requireResponse(t, wrongTarget, "checkpoint through another target",
		http.StatusNotFound, `"code":"not_found"`)
}

func TestInstallationSettingsInspectionExposesIncompatibility(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	target, err := harness.store.GetTarget(t.Context(), "github:installation:10")
	if err != nil {
		t.Fatal(err)
	}
	saved := saveTargetSettingsCheckpoint(
		t, harness, session, target, !target.RepositoryDefaultEnabled,
	)
	checkpointID, err := strconv.ParseInt(*saved.CheckpointID, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	rewriteSettingsCheckpointDocumentVersion(t, harness, checkpointID,
		storage.SettingsCheckpointItemIdentity{Kind: storage.SettingsCheckpointItemTarget})
	path := installationSettingsCheckpointPath + *saved.CheckpointID
	response := harness.request(t, http.MethodGet, path, nil, session)
	requireResponse(t, response, "incompatible settings inspection", http.StatusOK,
		`"restorable":false`, `"differs":false`,
		`"code":"unsupported_document_version"`,
		`"reason":"This checkpoint uses a settings format this version cannot restore"`)
	blocked := harness.request(t, http.MethodPost, path+"/restore",
		strings.NewReader(`{"state":"after","selections":[{"kind":"target","expected_revision":2}]}`), session)
	requireResponse(t, blocked, "incompatible settings restore", http.StatusConflict,
		`"code":"settings_restore_blocked"`)
}

func TestRootInstallationSettingsRestoreRequiresElevation(t *testing.T) {
	harness := newPanelHarness(t, "root")
	rootSession := harness.signIn(t)
	owner, snapshot := seedNonOwnedInstallation(t, harness)
	target, err := harness.store.GetTarget(t.Context(), snapshot.TargetID)
	if err != nil {
		t.Fatal(err)
	}
	first := saveTargetSettingsDirect(
		t, harness, target, !target.RepositoryDefaultEnabled, owner.ID, harness.now.Add(time.Minute),
	)
	second := saveTargetSettingsDirect(
		t, harness, *first.Target, target.RepositoryDefaultEnabled, owner.ID,
		harness.now.Add(2*time.Minute),
	)
	path := "/panel/api/v1/root/installations/" + target.ID + "/settings/checkpoints/" +
		strconv.FormatInt(*first.CheckpointID, 10)
	inspection := harness.request(t, http.MethodGet, path, nil, rootSession)
	requireResponse(t, inspection, "Root settings inspection", http.StatusOK,
		`"differs":true`, `"login":"installation-owner"`)
	body := `{"state":"after","selections":[{"kind":"target","expected_revision":3}]}`
	blocked := harness.request(t, http.MethodPost, path+"/restore",
		strings.NewReader(body), rootSession)
	requireResponse(t, blocked, "Root settings restore without elevation",
		http.StatusForbidden, `"code":"elevation_required"`)

	elevated := harness.request(t, http.MethodPost,
		"/panel/api/v1/root/installations/"+target.ID+"/elevation",
		strings.NewReader(`{"acknowledged":true,"reason":"restore settings history"}`),
		rootSession)
	requireResponse(t, elevated, "start settings elevation", http.StatusCreated)
	var elevation elevationResponse
	if err := json.Unmarshal(elevated.Body.Bytes(), &elevation); err != nil {
		t.Fatal(err)
	}
	restored := harness.request(t, http.MethodPost, path+"/restore",
		strings.NewReader(body), rootSession)
	requireResponse(t, restored, "elevated Root settings restore", http.StatusOK,
		`"checkpoint_id":`, `"revision":4`)
	current, err := harness.store.GetTarget(t.Context(), target.ID)
	if err != nil || current.RepositoryDefaultEnabled != first.Target.RepositoryDefaultEnabled ||
		current.Revision != 4 || second.Target.Revision != 3 {
		t.Fatalf("Root restored target = %#v, second = %#v, %v", current, second.Target, err)
	}
	rootAudit := harness.request(t, http.MethodGet,
		"/panel/api/v1/root/history/audit?category=configuration&target="+target.ID+
			"&limit=100", nil, rootSession)
	requireResponse(t, rootAudit, "Root settings restore audit", http.StatusOK,
		`"elevation_id":"`+elevation.ID+`"`, `"settings_checkpoint_id":`)
	notifications, err := harness.store.ListSecurityNotifications(
		t.Context(), owner.ID, storage.NotificationPageRequest{Limit: 10},
	)
	if err != nil || notifications.Unread != 1 || len(notifications.Items) != 1 ||
		notifications.Items[0].Action != "installation.settings.restored" {
		t.Fatalf("Root restore notifications = %#v, %v", notifications, err)
	}
}

func saveTargetSettingsDirect(
	t *testing.T,
	harness *panelHarness,
	target storage.Target,
	enabled bool,
	actorID string,
	changedAt time.Time,
) storage.SaveInstallationSettingsResult {
	t.Helper()
	result, err := harness.store.SaveInstallationSettings(t.Context(),
		storage.SaveInstallationSettingsRequest{
			TargetID: target.ID, ActorAccountID: actorID, ChangedAt: changedAt,
			Target: &storage.InstallationTargetSettingsChange{
				RepositoryDefaultEnabled:       enabled,
				PendingCIModeDefault:           target.PendingCIModeDefault,
				PendingCIBranchPatternsDefault: target.PendingCIBranchPatternsDefault,
				PendingCIQuietPeriodOverride:   target.PendingCIQuietPeriodOverride,
				PathIndexIntervalOverride:      target.PathIndexIntervalOverride,
				ConfigPatch:                    target.ConfigPatch,
				ExpectedRevision:               target.Revision,
				RetunePendingCIQuietPeriod:     true,
				DeploymentPendingCIQuietPeriod: 30 * time.Second,
			},
		},
	)
	if err != nil || result.Target == nil || result.CheckpointID == nil {
		t.Fatalf("direct target settings save = %#v, %v", result, err)
	}

	return result
}
