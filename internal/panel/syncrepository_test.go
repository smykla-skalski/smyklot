package panel

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
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

// configPath is the installation's own configuration for a kind, which is what
// a repository's adjustments have to fit.
const configPath = "/panel/api/v1/targets/github:installation:10/sync/config/"

// TestSyncOverrideRoundTripsThroughTheEndpoint drives the addresses rather than
// the helpers behind them.
//
// The helpers were the whole of what these specs used to reach, so the
// validation the endpoint does - and the fitting of an adjustment against what
// the installation synchronizes - was covered by nothing.
func TestSyncOverrideRoundTripsThroughTheEndpoint(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)

	configured := harness.request(t, http.MethodPut, configPath+"files", strings.NewReader(
		`{"enabled":true,"expected_revision":0,"document":{"files":[
			{"path":"renovate.json","content":"{}"}]}}`), session)
	if configured.Code != http.StatusOK {
		t.Fatalf("configuring the files = %d %s", configured.Code, configured.Body.String())
	}

	saved := harness.request(t, http.MethodPut, overridePath+"files", strings.NewReader(
		`{"enabled":null,"expected_revision":0,"document":{"merges":[
			{"path":"renovate.json","overrides":{"timezone":"Europe/Warsaw"}}]}}`), session)
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
}

// TestSyncOverrideRefusesWhatCouldNeverApply covers the endpoint's own
// validation, which nothing reached before.
func TestSyncOverrideRefusesWhatCouldNeverApply(t *testing.T) {
	for name, request := range map[string]struct {
		kind string
		body string
	}{
		// An adjustment naming a file nobody synchronizes reads as configured
		// and quietly leaves the repository with the plain template.
		"a file the installation does not synchronize": {
			kind: "files",
			body: `{"enabled":null,"expected_revision":0,"document":{"merges":[
				{"path":"package.json"}]}}`,
		},
		"a merge the file could not take": {
			kind: "files",
			body: `{"enabled":null,"expected_revision":0,"document":{"merges":[
				{"path":"renovate.json","strategy":"markdown"}]}}`,
		},
		"a key this version does not know": {
			kind: "files",
			body: `{"enabled":null,"expected_revision":0,"document":{"merges":[],
				"delete_everything":true}}`,
		},
		// Every kind but files is the same everywhere the installation
		// switches it on, so storing a document for one nothing reads is worse
		// than refusing it.
		"a document for a kind with nothing to adjust": {
			kind: "labels",
			body: `{"enabled":null,"expected_revision":0,"document":{"merges":[]}}`,
		},
		"a repository that will not say whether the kind runs": {
			kind: "files",
			body: `{"expected_revision":0,"document":{}}`,
		},
		"a repository that will not say what it replaces": {
			kind: "files",
			body: `{"enabled":null,"document":{}}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			harness := newPanelHarness(t, "owner")
			session := harness.signIn(t)

			configured := harness.request(t, http.MethodPut, configPath+"files",
				strings.NewReader(`{"enabled":true,"expected_revision":0,"document":{"files":[
					{"path":"renovate.json","content":"{}"}]}}`), session)
			if configured.Code != http.StatusOK {
				t.Fatalf("configuring the files = %d %s",
					configured.Code, configured.Body.String())
			}

			refused := harness.request(t, http.MethodPut,
				overridePath+request.kind, strings.NewReader(request.body), session)
			if refused.Code != http.StatusBadRequest {
				t.Fatalf("%s = %d %s", name, refused.Code, refused.Body.String())
			}
		})
	}
}

// TestSyncOverrideSaysWhyAnAdjustmentCannotBeChecked keeps the message about
// the real cause.
//
// An adjustment is checked against the files the installation synchronizes, so
// one cannot be saved while those cannot be read - and being told the file is
// "not one of the files synchronized" would send somebody looking in the wrong
// place. What a repository wants left alone still saves: it names paths rather
// than fitting them.
func TestSyncOverrideSaysWhyAnAdjustmentCannotBeChecked(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)

	// Written past the panel, which is the only way a document this version
	// cannot read gets in.
	if _, err := harness.store.SetSyncConfig(t.Context(), orgsync.ConfigChange{
		TargetID: "github:installation:10", Kind: orgsync.KindFiles, Enabled: true,
		Document: []byte(`{"files": [ this is not json`),
		ActorID:  "github:test:user:1", Now: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	refused := harness.request(t, http.MethodPut, overridePath+"files", strings.NewReader(
		`{"enabled":null,"expected_revision":0,"document":{"merges":[
			{"path":"renovate.json"}]}}`), session)
	if refused.Code != http.StatusBadRequest {
		t.Fatalf("adjusting against an unreadable configuration = %d %s",
			refused.Code, refused.Body.String())
	}
	if !strings.Contains(refused.Body.String(), "cannot read") {
		t.Errorf("refusal = %s, wanted it to name the real cause", refused.Body.String())
	}

	kept := harness.request(t, http.MethodPut, overridePath+"files", strings.NewReader(
		`{"enabled":null,"expected_revision":0,"document":{"excludes":["renovate.json"]}}`),
		session)
	if kept.Code != http.StatusOK {
		t.Fatalf("keeping a file out = %d %s", kept.Code, kept.Body.String())
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
// installation's own configuration makes, at the same address shape.
func TestSyncOverrideRefusesAKindNothingSynchronizes(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)

	read := harness.request(t, http.MethodGet, overridePath+"widgets", nil, session)
	if read.Code != http.StatusNotFound {
		t.Fatalf("reading a kind nothing syncs = %d %s", read.Code, read.Body.String())
	}
}

// TestSyncOverrideReadsARepositoryThatHasNeverAnswered is the same guard the
// installation's own configuration carries: a repository that has said nothing
// and one that said no are different, and the browser gets one shape either
// way.
func TestSyncOverrideReadsARepositoryThatHasNeverAnswered(t *testing.T) {
	dto := syncOverrideToDTO(orgsync.KindFiles, nil)

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
	})

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
	})

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

	configured := harness.request(t, http.MethodPut, configPath+"files", strings.NewReader(
		`{"enabled":true,"expected_revision":0,"document":{"files":[
			{"path":"renovate.json","content":"{}"}]}}`), session)
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

	saved := harness.request(t, http.MethodPut, overridePath+"files", strings.NewReader(
		`{"enabled":null,"expected_revision":0,"document":{"merges":[
			{"path":"renovate.json","overrides":{"timezone":"Europe/Warsaw"}}]}}`), session)
	if saved.Code != http.StatusOK {
		t.Fatalf("saving the adjustment = %d %s", saved.Code, saved.Body.String())
	}

	var answer syncOverrideDTO
	if err := json.Unmarshal(saved.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}

	if answer.Problem == "" {
		t.Error("a save reported the repository as fine, and nothing had looked at it")
	}
}

// TestSyncOverrideReportsNoProblemWhereNothingHasLooked keeps a fresh
// installation quiet. A repository nothing has planned yet is not a repository
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
