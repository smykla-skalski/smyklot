package apply

import (
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

func TestCheckRetainsReasonsForNoComparison(t *testing.T) {
	for _, test := range []struct {
		name   string
		config orgsync.Config
		live   bool
		want   string
	}{
		{name: "disabled", want: "disabled"},
		{name: "missing permissions", config: orgsync.Config{Kind: orgsync.KindSettings, Enabled: true}, want: "unpermitted"},
		{name: "existing plan", config: orgsync.Config{Kind: orgsync.KindLabels, Enabled: true}, live: true, want: "deferred"},
		{name: "no eligible repositories", config: orgsync.Config{Kind: orgsync.KindLabels, Enabled: true, Document: []byte(`{"labels":[]}`)}, want: "checked"},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &freshCheckStore{saved: test.config, live: test.live}
			summary, err := New(store, nil, "").PlanInstallationForCheck(t.Context(), nil, "target", orgsync.TriggerManual, orgsync.CheckReference{QueueID: "check", Attempt: 1})
			if err != nil {
				t.Fatal(err)
			}
			outcome := store.checkResult.Result.Outcome
			if outcome.Disposition != test.want || outcome.Summary != summary || outcome.CompletedAt.IsZero() || len(outcome.Counts) != 0 {
				t.Fatalf("misleading result: %#v", outcome)
			}
			if test.want == "unpermitted" && len(outcome.MissingPermissions) != 1 {
				t.Fatalf("missing permission lost: %#v", outcome)
			}
		})
	}
}

func TestCheckSnapshotRetainsCachedEvidenceAge(t *testing.T) {
	at := time.Now().Add(-time.Hour).UTC()
	state := orgsync.RepositoryState{RepositoryID: "repo", Kind: orgsync.KindFiles, Observation: orgsync.ObservationDeclined, AppliedAt: at, ObservedDigest: "old-input", ProposalURL: "https://github.com/owner/repo/pull/42"}
	snapshot := checkObservation(storage.Repository{ID: "repo", FullName: "owner/old-name"}, state, true)
	result := (syncScanResult{cached: 1, evidence: []orgsync.CheckObservation{snapshot}}).checkResult("checked", "Recent check reused", nil)
	if !snapshot.ObservedAt.Equal(at) || !snapshot.Cached || snapshot.InputDigest != "old-input" || snapshot.Repository != "owner/old-name" || snapshot.ProposalURL != state.ProposalURL {
		t.Fatalf("snapshot: %#v", snapshot)
	}
	if result.Outcome.Cached != 1 || len(result.Outcome.Counts) != 0 {
		t.Fatalf("cache presented as a fresh observation: %#v", result)
	}
}
