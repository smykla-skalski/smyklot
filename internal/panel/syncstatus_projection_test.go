package panel

import (
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

func TestSyncCellsRequireCurrentObservedEvidence(t *testing.T) {
	now := time.Date(2026, time.September, 13, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name    string
		outcome orgsync.Observation
		input   string
		age     time.Duration
		want    string
	}{
		{"no check", "", "", 0, "unknown"},
		{"legacy check", orgsync.ObservationMatched, "", time.Minute, "unknown"},
		{"matched", orgsync.ObservationMatched, "current", time.Minute, "in_step"},
		{"applied", orgsync.ObservationApplied, "current", time.Minute, "applied"},
		{"proposed", orgsync.ObservationProposed, "current", time.Minute, "proposed"},
		{"declined", orgsync.ObservationDeclined, "current", time.Minute, "declined"},
		{"different", orgsync.ObservationDifferent, "current", time.Minute, "needs_sync"},
		{"failed", orgsync.ObservationFailed, "current", time.Minute, "check_failed"},
		{"blocked", orgsync.ObservationBlocked, "current", time.Minute, "refused"},
		{"changed settings", orgsync.ObservationMatched, "earlier", time.Minute, "outdated"},
		{"superseded failure", orgsync.ObservationFailed, "earlier", time.Minute, "outdated"},
		{"old check", orgsync.ObservationMatched, "current", orgsync.RecheckInterval, "outdated"},
		{"future timestamp", orgsync.ObservationMatched, "current", -time.Minute, "outdated"},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := orgsync.RepositoryState{Observation: test.outcome, ObservedDigest: test.input, AppliedAt: now.Add(-test.age)}
			facts := syncStatusFacts{now: now, observations: map[string]map[orgsync.Kind]orgsync.RepositoryState{"repo": {orgsync.KindFiles: state}}}
			cell := repositorySyncCell("repo", orgsync.KindFiles, true, "current", facts)
			if cell.State != test.want {
				t.Fatalf("state = %q, want %q", cell.State, test.want)
			}
			if cell.ObservedAt != nil && !cell.ObservedAt.Equal(state.AppliedAt) {
				t.Fatal("changed the repository observation time")
			}
			if test.outcome == "" && cell.ObservedAt != nil {
				t.Fatal("invented an observation")
			}
		})
	}
}

func TestSyncPendingChangesRetainTheirInputScope(t *testing.T) {
	facts := syncStatusFacts{
		pending:      map[string]map[orgsync.Kind]int{"repo": {orgsync.KindLabels: 2}},
		actionInputs: map[string]map[orgsync.Kind]string{"repo": {orgsync.KindLabels: "planned"}},
	}
	if got := repositorySyncCell("repo", orgsync.KindLabels, true, "planned", facts); got.State != "pending" || got.Changes != 2 {
		t.Fatalf("current pending changes = %#v", got)
	}
	if got := repositorySyncCell("repo", orgsync.KindLabels, true, "newer", facts); got.State != "outdated" || got.Changes != 0 {
		t.Fatalf("old pending changes were presented as new settings: %#v", got)
	}
	if got := repositorySyncCell("repo", orgsync.KindLabels, false, "planned", facts); got.State != "off" {
		t.Fatalf("disabled policy was overridden by pending work: %#v", got)
	}
}

func TestSyncStatusOverrideCannotEnableDisabledScope(t *testing.T) {
	enabled := true
	for _, test := range []struct {
		name               string
		global, repository bool
		want               string
	}{
		{"globally disabled", false, true, "off"},
		{"repository disabled", true, false, "off"},
		{"covered repository", true, true, "unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			facts := syncStatusFacts{
				target:    storage.Target{Available: true, RepositoryDefaultEnabled: test.repository},
				enabled:   map[orgsync.Kind]bool{orgsync.KindLabels: test.global},
				overrides: map[string]map[orgsync.Kind]*orgsync.RepositoryOverride{"repo": {orgsync.KindLabels: {Enabled: &enabled}}},
			}
			row := syncStatusRow(storage.Repository{ID: "repo", Available: true}, facts)
			if got := row.Cells["labels"].State; got != test.want {
				t.Fatalf("state = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSyncProposalDestinationSurvivesChangedInputs(t *testing.T) {
	now := time.Now().UTC()
	for _, observation := range []orgsync.Observation{orgsync.ObservationProposed, orgsync.ObservationDeclined} {
		state := orgsync.RepositoryState{Observation: observation, ObservedDigest: "saved", AppliedAt: now, ProposalURL: "https://github.com/owner/repo/pull/42"}
		facts := syncStatusFacts{now: now, observations: map[string]map[orgsync.Kind]orgsync.RepositoryState{"repo": {orgsync.KindFiles: state}}}
		for _, digest := range []string{"saved", "changed"} {
			cell := repositorySyncCell("repo", orgsync.KindFiles, true, digest, facts)
			if cell.ProposalURL != state.ProposalURL {
				t.Fatalf("lost proposal reference: %#v", cell)
			}
		}
	}
}
