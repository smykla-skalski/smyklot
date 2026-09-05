package panel

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

// TestSyncDocumentRefusesFilesGitHubOrGitWould keeps the answer beside the
// field somebody typed rather than in a sweep log an hour later.
func TestSyncDocumentRefusesFilesGitHubOrGitWould(t *testing.T) {
	for _, invalid := range []struct {
		name     string
		document string
	}{
		{"a file with no path", `{"files":[{"path":"","content":"x"}]}`},
		{"a file with nothing in it", `{"files":[{"path":"README.md","content":""}]}`},
		{"a path that climbs out", `{"files":[{"path":"../x","content":"x"}]}`},
		{"a path anchored at the root", `{"files":[{"path":"/etc/x","content":"x"}]}`},
		{"two files a checkout could not tell apart", `{"files":[
			{"path":"Readme.md","content":"a"},{"path":"README.md","content":"b"}]}`},
		{"a placeholder nothing fills in", `{"files":[
			{"path":"README.md","content":"See {{REPO}}."}]}`},
		{"a path written and retired at once", `{
			"files":[{"path":"a.md","content":"x"}],"retired":["a.md"]}`},
		{"a key this version does not know", `{
			"files":[{"path":"a.md","content":"x"}],"delete_everything":true}`},
	} {
		t.Run(invalid.name, func(t *testing.T) {
			_, err := syncDocumentFor(orgsync.KindFiles, syncConfigRequest{
				Document: []byte(invalid.document),
			})
			if err == nil {
				t.Fatalf("%s was accepted", invalid.name)
			}
		})
	}
}

// TestSyncDocumentStoresTheFilesType keeps what is stored the type the planner
// decodes. Two shapes is how chunk 3's exclusions came to be saved and never
// read.
func TestSyncDocumentStoresTheFilesType(t *testing.T) {
	document, err := syncDocumentFor(orgsync.KindFiles, syncConfigRequest{
		Document: []byte(`{"files":[{"path":"CONTRIBUTING.md","content":"# Contributing\n"}],
			"retired":[".github/workflows/sync-trigger.yml"],"excludes":["LICENSE"]}`),
	})
	if err != nil {
		t.Fatalf("a file document git accepts was refused: %v", err)
	}

	var stored orgsync.FileConfig
	if err := json.Unmarshal(document, &stored); err != nil {
		t.Fatalf("what was stored is not a file configuration: %v", err)
	}

	if len(stored.Files) != 1 || stored.Files[0].Path != "CONTRIBUTING.md" {
		t.Errorf("files = %v, wanted the one that was sent", stored.Files)
	}
	if stored.Files[0].Content != "# Contributing\n" {
		t.Errorf("content = %q, wanted what was sent", stored.Files[0].Content)
	}
	if len(stored.Retired) != 1 {
		t.Errorf("retired = %v, wanted the one that was sent", stored.Retired)
	}
	if len(stored.Excludes) != 1 {
		t.Errorf("excludes = %v, wanted the one that was sent", stored.Excludes)
	}
}

// overridePath addresses what one repository says about one kind, through the
// route rather than through the function behind it.
const overridePath = "/panel/api/v1/targets/github:installation:10" +
	"/repositories/repository-20/sync/"

// TestSyncFilesContextCountsRepositoryOptIn covers the inverse of the global
// baseline: a repository can turn file sync on while the workspace leaves
// it off, and the page's coverage count must use that effective answer.
func TestSyncFilesContextCountsRepositoryOptIn(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)

	enabled := true
	if _, err := harness.store.SaveInstallationSettings(
		t.Context(), storage.SaveInstallationSettingsRequest{
			TargetID: "github:installation:10", ActorAccountID: "github:test:user:1",
			ChangedAt: harness.now,
			SyncConfigs: []storage.InstallationSyncConfigChange{{
				Kind: orgsync.KindFiles, Enabled: false,
				Document: []byte(`{"files":[{"path":"README.md","content":"hello"}]}`),
			}},
			SyncOverrides: []storage.InstallationSyncOverrideChange{{
				RepositoryID: "repository-20", Kind: orgsync.KindFiles, Enabled: &enabled,
				Document: []byte(`{}`),
			}},
		},
	); err != nil {
		t.Fatal(err)
	}

	response := harness.request(t, http.MethodGet,
		"/panel/api/v1/targets/github:installation:10/sync/files/context",
		nil, session)
	if response.Code != http.StatusOK {
		t.Fatalf("reading files context = %d %s", response.Code, response.Body.String())
	}
	var answer struct {
		Repositories int `json:"repositories"`
		Covered      int `json:"covered"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if answer.Repositories != 1 || answer.Covered != 1 {
		t.Errorf("files context = %+v, wanted the opted-in repository covered", answer)
	}
}

func TestSyncFilesContextCarriesRepositoryIndexAndPathAdjustments(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)

	saved := harness.request(t, http.MethodPut, workspaceSettingsBatchPath, strings.NewReader(
		`{"target":{"repository_default_enabled":true,"pending_ci_mode_default":"checks",
			"pending_ci_branch_patterns_default":{"include":["~DEFAULT_BRANCH"],"exclude":[]},
			"pending_ci_quiet_period_seconds_override":null,
			"path_index_interval_seconds_override":null,
			"config_patch":{"formatting":{"json":{"arrays":"expanded"}}},"expected_revision":1},
		"repositories":[{"repository_id":"repository-20","enabled_override":null,
			"pending_ci_mode_override":null,"pending_ci_branch_patterns_override":null,
			"pending_ci_quiet_period_seconds_override":null,
			"path_index_interval_seconds_override":null,
			"config_patch":{"formatting":{"json":{"arrays":"compact"}}},
			"ignore_repository_file":false,"expected_revision":1}],
		"sync_configs":[{"kind":"files","enabled":true,"expected_revision":0,
			"document":{"files":[{"path":"renovate.json","content":"{}",
			"formatting":{"common":{"final_newline":"insert"}}}]}}],
		"sync_overrides":[{"repository_id":"repository-20","kind":"files",
			"enabled":null,"expected_revision":0,"document":{
			"merges":[{"path":"renovate.json","overrides":{"timezone":"Europe/Warsaw"}}],
			"formats":[{"path":"renovate.json","formatting":{"json":{"arrays":"preserve"}}}]}}]}`), session)
	if saved.Code != http.StatusOK {
		t.Fatalf("saving formatting settings = %d %s", saved.Code, saved.Body.String())
	}

	response := harness.request(t, http.MethodGet,
		"/panel/api/v1/targets/github:installation:10/sync/files/context", nil, session)
	if response.Code != http.StatusOK {
		t.Fatalf("reading files context = %d %s", response.Code, response.Body.String())
	}
	var answer struct {
		RepositoryPolicies []syncFileRepositoryDTO `json:"repository_policies"`
		Merges             []syncFileMergeEntryDTO `json:"merges"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if len(answer.RepositoryPolicies) != 1 ||
		answer.RepositoryPolicies[0].RepositoryID != "repository-20" {
		t.Fatalf("repository policies = %#v", answer.RepositoryPolicies)
	}
	if strings.Contains(response.Body.String(), "base_formatting") ||
		strings.Contains(response.Body.String(), "base_policy") {
		t.Fatalf("files context still exposes browser-composed policy bases: %s", response.Body.String())
	}
	if len(answer.Merges) != 1 || answer.Merges[0].Formatting == nil ||
		answer.Merges[0].Formatting.JSON == nil ||
		answer.Merges[0].Formatting.JSON.Arrays == nil ||
		*answer.Merges[0].Formatting.JSON.Arrays != "preserve" ||
		!strings.Contains(string(answer.Merges[0].Merge), "Europe/Warsaw") {
		t.Fatalf("normalized path adjustment = %#v", answer.Merges)
	}
}

// TestSyncOverrideRoundTripsThroughTheEndpoint drives the addresses rather than
// the helpers behind them.
//
// The helpers were the whole of what these specs used to reach, so the
// validation the endpoint does - and the fitting of an adjustment against what
// the workspace synchronizes - was covered by nothing.
func TestSyncOverrideRoundTripsThroughTheEndpoint(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)

	saved := harness.request(t, http.MethodPut, workspaceSettingsBatchPath, strings.NewReader(
		`{"sync_configs":[{"kind":"files","enabled":true,"expected_revision":0,
			"document":{"files":[{"path":"renovate.json","content":"{}"}]}}],
		"sync_overrides":[{"repository_id":"repository-20","kind":"files",
			"enabled":null,"expected_revision":0,"document":{"merges":[
				{"path":"renovate.json","overrides":{"timezone":"Europe/Warsaw"}}]}}]}`), session)
	if saved.Code != http.StatusOK {
		t.Fatalf("saving the adjustment = %d %s", saved.Code, saved.Body.String())
	}

	read := harness.request(t, http.MethodGet, overridePath+"files", nil, session)
	if read.Code != http.StatusOK {
		t.Fatalf("reading it back = %d %s", read.Code, read.Body.String())
	}

	var answer syncOverrideDTO
	if err := json.Unmarshal(read.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}

	if answer.Enabled != nil {
		t.Errorf("enabled = %v, wanted nothing: this repository inherits", *answer.Enabled)
	}
	if !strings.Contains(string(answer.Document), "Europe/Warsaw") {
		t.Errorf("document = %s, wanted the adjustment that was sent", answer.Document)
	}
	if answer.Revision != 1 {
		t.Errorf("revision = %d, wanted 1", answer.Revision)
	}
	if answer.UpdatedBy != "owner" {
		t.Errorf("updated_by = %q, wanted the editor's GitHub login", answer.UpdatedBy)
	}
}

// TestSyncOverrideAnswersAKindNobodyHasAdjusted keeps the shape one thing for a
// browser: a repository that has never answered reads the same way as one that
// has, rather than as a 404 it would have to guard against.
func TestSyncOverrideAnswersAKindNobodyHasAdjusted(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)

	read := harness.request(t, http.MethodGet, overridePath+"files", nil, session)
	if read.Code != http.StatusOK {
		t.Fatalf("reading an answer nobody gave = %d %s", read.Code, read.Body.String())
	}

	var answer syncOverrideDTO
	if err := json.Unmarshal(read.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}

	if answer.Kind != "files" || answer.Enabled != nil || string(answer.Document) != "{}" {
		t.Errorf("answer = %+v, wanted an empty one for the files kind", answer)
	}
}

// TestSyncOverrideRefusesAKindNothingSynchronizes is the same refusal the
// workspace's own configuration makes, at the same address shape.
func TestSyncOverrideRefusesAKindNothingSynchronizes(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)

	read := harness.request(t, http.MethodGet, overridePath+"widgets", nil, session)
	if read.Code != http.StatusNotFound {
		t.Fatalf("reading a kind nothing syncs = %d %s", read.Code, read.Body.String())
	}
}

// TestSyncOverrideReadsARepositoryThatHasNeverAnswered is the same guard the
// workspace's own configuration carries: a repository that has said nothing
// and one that said no are different, and the browser gets one shape either
// way.
func TestSyncOverrideReadsARepositoryThatHasNeverAnswered(t *testing.T) {
	dto := syncOverrideToDTO(orgsync.KindFiles, nil, "")

	if dto.Enabled != nil {
		t.Errorf("enabled = %v, wanted nothing: this repository has never answered", *dto.Enabled)
	}
	if string(dto.Document) != "{}" {
		t.Errorf("document = %s, wanted an empty object rather than null", dto.Document)
	}
	if dto.Revision != 0 {
		t.Errorf("revision = %d, wanted none", dto.Revision)
	}
}

// TestSyncOverrideReportsADocumentItCannotRead is what keeps a row this version
// cannot decode from rendering as a repository that adjusts nothing - which is
// what somebody would then save over.
func TestSyncOverrideReportsADocumentItCannotRead(t *testing.T) {
	dto := syncOverrideToDTO(orgsync.KindFiles, &orgsync.RepositoryOverride{
		Kind:      orgsync.KindFiles,
		Document:  []byte(`{"merges": [ this is not json`),
		Revision:  4,
		UpdatedAt: time.Now().UTC(),
	}, "")

	if !dto.Unreadable {
		t.Error("a document that does not decode was reported as readable")
	}
	if string(dto.Document) != "{}" {
		t.Errorf("document = %s, wanted an empty object", dto.Document)
	}
	if dto.Revision != 4 {
		t.Errorf("revision = %d, wanted 4: the row is still there", dto.Revision)
	}
}

func TestSyncOverrideKeepsWhatARepositoryAdjusts(t *testing.T) {
	document := []byte(`{"merges":[{"path":"renovate.json"}]}`)

	dto := syncOverrideToDTO(orgsync.KindFiles, &orgsync.RepositoryOverride{
		Kind: orgsync.KindFiles, Document: document, Revision: 2,
	}, "")

	if dto.Unreadable {
		t.Error("a document that decodes was reported as unreadable")
	}
	if string(dto.Document) != string(document) {
		t.Errorf("document = %s, wanted %s", dto.Document, document)
	}
}

// TestSyncOverrideSaysWhyARepositoryIsNotBeingSynced is the whole point of the
// recorded refusal: before it, a repository receiving none of the
// organization's files looked on this page exactly like one receiving all of
// them, and the only account of why was a line in the service log.
func TestSyncOverrideSaysWhyARepositoryIsNotBeingSynced(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)

	refused := time.Date(2026, time.August, 9, 9, 30, 0, 0, time.UTC)
	if err := harness.store.RecordSyncRepositoryState(
		t.Context(), []orgsync.RepositoryState{{
			RepositoryID: "repository-20", Kind: orgsync.KindFiles,
			AppliedAt: refused,
			Problem:   "these files cannot be composed: docs is not a directory here",
		}},
	); err != nil {
		t.Fatal(err)
	}

	read := harness.request(t, http.MethodGet, overridePath+"files", nil, session)
	if read.Code != http.StatusOK {
		t.Fatalf("reading the adjustment = %d %s", read.Code, read.Body.String())
	}

	var answer syncOverrideDTO
	if err := json.Unmarshal(read.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(answer.Problem, "docs is not a directory here") {
		t.Errorf("problem = %q, wanted the reason the planner recorded", answer.Problem)
	}

	// With when it was found, so a fix saved a minute ago can be told from one
	// this notice already knows about.
	if answer.ProblemAt == nil || !answer.ProblemAt.Equal(refused) {
		t.Errorf("problem_at = %v, wanted %v", answer.ProblemAt, refused)
	}
}

// TestSyncOverrideKeepsARefusalThroughASave keeps a save from reading as a fix.
//
// Saving an adjustment does not plan anything, so whatever stopped this
// repository still stands until the next sweep looks. Dropping the notice from
// the answer would tell somebody their change worked before anything had tried
// it.
func TestSyncOverrideKeepsARefusalThroughASave(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)

	configured := harness.request(t, http.MethodPut, workspaceSettingsBatchPath, strings.NewReader(
		`{"sync_configs":[{"kind":"files","enabled":true,"expected_revision":0,
			"document":{"files":[{"path":"renovate.json","content":"{}"}]}}]}`), session)
	if configured.Code != http.StatusOK {
		t.Fatalf("configuring the files = %d %s", configured.Code, configured.Body.String())
	}

	if err := harness.store.RecordSyncRepositoryState(
		t.Context(), []orgsync.RepositoryState{{
			RepositoryID: "repository-20", Kind: orgsync.KindFiles,
			AppliedAt: harness.now,
			Problem:   "the adjustments saved for this repository cannot be used",
		}},
	); err != nil {
		t.Fatal(err)
	}

	saved := harness.request(t, http.MethodPut, workspaceSettingsBatchPath, strings.NewReader(
		`{"sync_overrides":[{"repository_id":"repository-20","kind":"files",
			"enabled":null,"expected_revision":0,"document":{"merges":[
				{"path":"renovate.json","overrides":{"timezone":"Europe/Warsaw"}}]}}]}`), session)
	if saved.Code != http.StatusOK {
		t.Fatalf("saving the adjustment = %d %s", saved.Code, saved.Body.String())
	}

	read := harness.request(t, http.MethodGet, overridePath+"files", nil, session)
	var answer syncOverrideDTO
	if err := json.Unmarshal(read.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}

	if answer.Problem == "" {
		t.Error("a save reported the repository as fine, and nothing had looked at it")
	}
}

// TestSyncConfigTakesTheTemplatesItValidates keeps the limit that is checked
// reachable through the only writer there is.
//
// The files kind allows a megabyte of templates in total, and the request body
// was capped at 64 KiB - so a dozen medium workflow files pasted into the form
// were truncated and refused as invalid JSON, sending whoever pasted them
// looking for a syntax error in a YAML file that has none.
func TestSyncConfigTakesTheTemplatesItValidates(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)

	// Comfortably past the old cap and inside what FileConfig allows.
	document := fmt.Sprintf(
		`{"sync_configs":[{"kind":"files","enabled":true,"expected_revision":0,
			"document":{"files":[{"path":"CONTRIBUTING.md","content":%q}]}}]}`,
		strings.Repeat("a line of a shared template\n", 4000))

	saved := harness.request(t, http.MethodPut, workspaceSettingsBatchPath,
		strings.NewReader(document), session)
	if saved.Code != http.StatusOK {
		t.Fatalf("saving %d bytes of templates = %d %s",
			len(document), saved.Code, saved.Body.String())
	}
}

// TestSyncConfigKeepsTheOrdinaryBoundOnEveryOtherKind is the other half of the
// same decision.
//
// Files earned the larger bound because FileConfig validates a total. No other
// kind has one - a label document bounds each name and colour and not how many
// - so raising it for them raises nothing but the size of a mistake, and a
// label document becomes an action per label per repository once it is planned.
func TestSyncConfigKeepsTheOrdinaryBoundOnEveryOtherKind(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)

	labels := make([]string, 0, 4000)
	for index := range cap(labels) {
		labels = append(labels,
			fmt.Sprintf(`{"name":"label-%05d","color":"ff0000"}`, index))
	}

	document := fmt.Sprintf(
		`{"sync_configs":[{"kind":"labels","enabled":true,"expected_revision":0,
			"labels":[%s],"allow_removal":false,"excludes":[]}]}`,
		strings.Join(labels, ","))

	refused := harness.request(t, http.MethodPut, workspaceSettingsBatchPath,
		strings.NewReader(document), session)
	if refused.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("saving %d bytes of labels = %d %s",
			len(document), refused.Code, refused.Body.String())
	}
}

// And the other half: past the bound it is a refusal that says so, rather than
// one that blames the JSON.
func TestSyncConfigSaysWhenTheRequestIsTooLarge(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)

	document := fmt.Sprintf(
		`{"sync_configs":[{"kind":"files","enabled":true,"expected_revision":0,
			"document":{"files":[{"path":"CONTRIBUTING.md","content":%q}]}}]}`,
		strings.Repeat("x", 5<<20))

	refused := harness.request(t, http.MethodPut, workspaceSettingsBatchPath,
		strings.NewReader(document), session)
	if refused.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("saving an oversized document = %d %s",
			refused.Code, refused.Body.String())
	}
	if !strings.Contains(refused.Body.String(), "too large") {
		t.Errorf("refusal = %s, wanted it to say the request is too large",
			refused.Body.String())
	}
}

// TestSyncOverrideDropsARefusalOnceTheKindIsOff keeps the pane from
// contradicting its own switch.
//
// A state row is only rewritten while the planner is looking at the repository,
// and it stops looking the moment the kind is switched off here - usually the
// moment somebody switched it off *because* of the refusal. Left in, the reason
// is rendered as a live notice directly under a control reading "Disabled".
func TestSyncOverrideDropsARefusalOnceTheKindIsOff(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)

	if err := harness.store.RecordSyncRepositoryState(
		t.Context(), []orgsync.RepositoryState{{
			RepositoryID: "repository-20", Kind: orgsync.KindFiles,
			AppliedAt: harness.now, Problem: "these files cannot be composed",
		}},
	); err != nil {
		t.Fatal(err)
	}

	// Shown while the kind still runs here.
	read := harness.request(t, http.MethodGet, overridePath+"files", nil, session)

	var before syncOverrideDTO
	if err := json.Unmarshal(read.Body.Bytes(), &before); err != nil {
		t.Fatal(err)
	}
	if before.Problem == "" {
		t.Fatal("a repository the planner refused was answered as though it were fine")
	}

	off := harness.request(t, http.MethodPut, workspaceSettingsBatchPath, strings.NewReader(
		`{"sync_overrides":[{"repository_id":"repository-20","kind":"files",
			"enabled":false,"expected_revision":0,"document":{}}]}`), session)
	if off.Code != http.StatusOK {
		t.Fatalf("switching the sync off = %d %s", off.Code, off.Body.String())
	}

	afterRead := harness.request(t, http.MethodGet, overridePath+"files", nil, session)
	var after syncOverrideDTO
	if err := json.Unmarshal(afterRead.Body.Bytes(), &after); err != nil {
		t.Fatal(err)
	}
	if after.Problem != "" {
		t.Errorf("problem = %q, wanted none: this repository has the kind switched off",
			after.Problem)
	}
}

// TestSyncOverrideReportsNoProblemWhereNothingHasLooked keeps a fresh
// workspace quiet. A repository nothing has planned yet is not a repository
// with something wrong with it.
func TestSyncOverrideReportsNoProblemWhereNothingHasLooked(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)

	read := harness.request(t, http.MethodGet, overridePath+"files", nil, session)
	if read.Code != http.StatusOK {
		t.Fatalf("reading the adjustment = %d %s", read.Code, read.Body.String())
	}

	var answer syncOverrideDTO
	if err := json.Unmarshal(read.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}

	if answer.Problem != "" || answer.ProblemAt != nil {
		t.Errorf("problem = %q at %v, wanted none", answer.Problem, answer.ProblemAt)
	}
}
