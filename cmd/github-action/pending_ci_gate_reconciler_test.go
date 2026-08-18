package main

import (
	"testing"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

func TestPendingCIBranchIncludedUsesRawRulesetRefs(t *testing.T) {
	t.Parallel()
	patterns := storage.PendingCIBranchPatterns{
		Include: []string{"~DEFAULT_BRANCH", "refs/heads/release/*"},
		Exclude: []string{"refs/heads/release/private-*"},
	}
	tests := []struct {
		branch string
		want   bool
	}{
		{branch: "main", want: true},
		{branch: "release/1.2", want: true},
		{branch: "release/private-1.2", want: false},
		{branch: "feature/checks", want: false},
	}
	for _, test := range tests {
		if got := pendingCIBranchIncluded(test.branch, "main", patterns); got != test.want {
			t.Errorf("branch %q included = %t, want %t", test.branch, got, test.want)
		}
	}
}

func TestPendingCICheckRenewalKeepsOneGenerationSuffix(t *testing.T) {
	t.Parallel()
	slot := pendingci.CheckSlot{
		ExternalID: "smyklot:merge-after-ci:github:repository:20:abc:g2",
		Generation: 2,
	}
	if got := pendingCIRenewedExternalID(slot); got !=
		"smyklot:merge-after-ci:github:repository:20:abc:g3" {
		t.Fatalf("renewed external ID = %q", got)
	}
}

func TestPendingCIRulesetBindsTheStableContextToTheApp(t *testing.T) {
	t.Parallel()
	patterns := storage.DefaultPendingCIBranchPatterns()
	ruleset := pendingCIRuleset(patterns, 17)
	if ruleset.Name != storage.PendingCIRulesetName || ruleset.Enforcement != "active" {
		t.Fatalf("ruleset identity = %#v", ruleset)
	}
	statusChecks := ruleset.Rules.RequiredStatusChecks
	if statusChecks == nil || len(statusChecks.Checks) != 1 {
		t.Fatalf("required checks = %#v", statusChecks)
	}
	check := statusChecks.Checks[0]
	if check.Context != storage.PendingCICheckName || check.IntegrationID != 17 {
		t.Fatalf("required check = %#v", check)
	}
	if !statusChecks.DoNotEnforceOnCreate {
		t.Fatal("ruleset must not block branch creation before a baseline can be written")
	}
}
