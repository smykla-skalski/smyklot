package panel

import (
	"encoding/json"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
)

func encoded(t *testing.T, value any) []byte {
	t.Helper()

	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encoding %T: %v", value, err)
	}

	return payload
}

// A label reaches the panel as a label, not as a sentence about one.
//
// `describeLabel` writes `name #color - description`, and the plan page prints
// its subject and then that sentence - so a label creation read `dependencies -
// dependencies #0e8a16 - Dependency updates`, with the name twice and a second
// dash doing separator duty after the first already had.
func TestSyncActionDetailReadsALabel(t *testing.T) {
	payload := encoded(t, orgsync.ResolvedLabel{
		Name: "dependencies", Color: "0e8a16", Description: "Dependency updates",
	})

	for _, operation := range []orgsync.Operation{
		orgsync.OperationCreate, orgsync.OperationUpdate, orgsync.OperationDelete,
	} {
		detail := syncActionDetail(orgsync.Action{
			Kind: orgsync.KindLabels, Operation: operation, Payload: payload,
		})
		if detail == nil || detail.Label == nil {
			t.Fatalf("%s carried no label", operation)
		}
		if detail.Label.Name != "dependencies" || detail.Label.Color != "0e8a16" {
			t.Fatalf("%s read %#v", operation, detail.Label)
		}
		if detail.Label.Description != "Dependency updates" {
			t.Fatalf("%s lost the description: %q", operation, detail.Label.Description)
		}
	}
}

// A CHANGE HAS TWO SIDES. Shown only the label it would end up with, a reader
// cannot see what moved - a colour drifting from red to orange is `bug` twice
// with no difference between them.
func TestSyncActionDetailReadsBothSidesOfALabelChange(t *testing.T) {
	payload := encoded(t, orgsync.LabelPlan{
		Label:    orgsync.ResolvedLabel{Name: "bug", Color: "d73a4a", Description: "Broken"},
		Previous: &orgsync.ResolvedLabel{Name: "bug", Color: "ff8800", Description: "Broke"},
	})

	detail := syncActionDetail(orgsync.Action{
		Kind: orgsync.KindLabels, Operation: orgsync.OperationUpdate, Payload: payload,
	})
	if detail == nil || detail.Label == nil || detail.PreviousLabel == nil {
		t.Fatalf("a label change carried one side: %#v", detail)
	}
	if detail.Label.Color != "d73a4a" || detail.PreviousLabel.Color != "ff8800" {
		t.Fatalf("colours = %q then %q", detail.PreviousLabel.Color, detail.Label.Color)
	}
}

// A creation replaces nothing, so there is no previous side to draw.
func TestSyncActionDetailLeavesACreationOneSided(t *testing.T) {
	payload := encoded(t, orgsync.LabelPlan{
		Label: orgsync.ResolvedLabel{Name: "chore", Color: "6b7280"},
	})

	detail := syncActionDetail(orgsync.Action{
		Kind: orgsync.KindLabels, Operation: orgsync.OperationCreate, Payload: payload,
	})
	if detail == nil || detail.Label == nil {
		t.Fatal("a creation carried no label")
	}
	if detail.PreviousLabel != nil {
		t.Fatalf("a creation answered with a previous label: %#v", detail.PreviousLabel)
	}
}

// A settings change is ONE action - GitHub replaces a repository's settings in
// one request - and several facts. It used to be one sentence naming every
// field at once.
func TestSyncActionDetailReadsEverySettingSeparately(t *testing.T) {
	payload := encoded(t, orgsync.SettingsPlan{
		Body: map[string]any{"has_wiki": false, "allow_squash_merge": true},
		Changes: []orgsync.SettingsFieldChange{
			{Field: "allow_squash_merge", From: "off", To: "on"},
			{Field: "has_wiki", From: "on", To: "off"},
		},
		Follows:  []string{"allow merge commit"},
		Withheld: []orgsync.Withholding{{Field: "projects", Reason: "not on this plan"}},
	})

	detail := syncActionDetail(orgsync.Action{
		Kind: orgsync.KindSettings, Operation: orgsync.OperationUpdate,
		Subject: orgsync.SettingsSubject, Payload: payload,
	})
	if detail == nil {
		t.Fatal("a settings change carried no detail")
	}
	if len(detail.Setting) != 2 {
		t.Fatalf("read %d settings, wanted 2: %#v", len(detail.Setting), detail.Setting)
	}
	if detail.Setting[0] != (syncSettingDTO{Field: "allow_squash_merge", From: "off", To: "on"}) {
		t.Fatalf("first setting = %#v", detail.Setting[0])
	}
	if len(detail.Follows) != 1 || detail.Follows[0] != "allow merge commit" {
		t.Fatalf("follows = %#v", detail.Follows)
	}
	if len(detail.Withheld) != 1 || detail.Withheld[0].Field != "projects" {
		t.Fatalf("withheld = %#v", detail.Withheld)
	}
}

// Dependabot is a settings action whose payload is a different shape. Decoded
// as a settings plan it would answer with a change of no fields, which reads as
// "this action does nothing" rather than as "ask its sentence".
func TestSyncActionDetailLeavesDependabotToItsSentence(t *testing.T) {
	payload := encoded(t, orgsync.DependabotChange{Enabled: true})

	detail := syncActionDetail(orgsync.Action{
		Kind: orgsync.KindSettings, Operation: orgsync.OperationUpdate,
		Subject: orgsync.DependabotSubject, Payload: payload,
	})
	if detail != nil {
		t.Fatalf("Dependabot answered with a detail: %#v", detail)
	}
}

// A ruleset says what it enforces, one phrase per rule, off the same walk the
// sentence uses - so a rule added to the struct reaches both or neither.
func TestSyncActionDetailReadsARuleset(t *testing.T) {
	payload := encoded(t, orgsync.ResolvedRuleset{
		ID: 7,
		Ruleset: orgsync.Ruleset{
			Name: "main-protection", Target: "branch", Enforcement: "active",
			Rules: orgsync.RulesetRules{Deletion: true, NonFastForward: true},
		},
	})

	detail := syncActionDetail(orgsync.Action{
		Kind: orgsync.KindRulesets, Operation: orgsync.OperationUpdate, Payload: payload,
	})
	if detail == nil || detail.Ruleset == nil {
		t.Fatal("a ruleset carried no detail")
	}
	if detail.Ruleset.Name != "main-protection" || detail.Ruleset.Enforcement != "active" {
		t.Fatalf("ruleset = %#v", detail.Ruleset)
	}
	if len(detail.Ruleset.Rules) != 2 {
		t.Fatalf("rules = %#v", detail.Ruleset.Rules)
	}
}

func TestSyncActionDetailReadsAFile(t *testing.T) {
	payload := encoded(t, orgsync.ResolvedFile{
		Path: "renovate.json", Proposal: "smyklot/sync", Content: []byte("{}\n"),
	})

	detail := syncActionDetail(orgsync.Action{
		Kind: orgsync.KindFiles, Operation: orgsync.OperationCreate, Payload: payload,
	})
	if detail == nil || detail.File == nil {
		t.Fatal("a file carried no detail")
	}
	if detail.File.Path != "renovate.json" || detail.File.Proposal != "smyklot/sync" {
		t.Fatalf("file = %#v", detail.File)
	}
	if detail.File.Bytes != 3 {
		t.Fatalf("bytes = %d, wanted 3", detail.File.Bytes)
	}
}

// What the panel cannot read it declines to draw, and the sentence beside it
// still says what changes - so a row is never left blank.
func TestSyncActionDetailDeclinesWhatItCannotRead(t *testing.T) {
	cases := map[string]orgsync.Action{
		"no payload at all": {Kind: orgsync.KindLabels},
		"empty payload":     {Kind: orgsync.KindLabels, Payload: []byte{}},
		"not JSON":          {Kind: orgsync.KindLabels, Payload: []byte("dependencies #0e8a16")},
		"a kind with no shape here": {
			Kind: orgsync.Kind("something-new"), Payload: []byte(`{"a":1}`),
		},
	}

	for name, action := range cases {
		if detail := syncActionDetail(action); detail != nil {
			t.Fatalf("%s answered with a detail: %#v", name, detail)
		}
	}
}
