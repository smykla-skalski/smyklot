package main

import (
	"testing"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

// TestUnavailableForTargetIgnoresWorkAlreadyDone keeps a plan finishable after
// a permission is withdrawn part-way through it.
//
// A lease can expire with some of a plan applied, and the moment a workflow
// proposal lands is exactly when somebody might take Workflows access back.
// Holding the re-leased plan on an action that already applied leaves the rest
// of it - other repositories, other kinds - unrun for ever: the plan stays
// `applying`, which fills the installation's one live slot, and nothing expires
// a plan in that state.
func TestUnavailableForTargetIgnoresWorkAlreadyDone(t *testing.T) {
	target := storage.Target{Permissions: map[string]string{
		"issues": "write", "contents": "write",
	}}

	done := []orgsync.Action{
		{
			Kind: orgsync.KindFiles, Subject: ".github/workflows/ci.yaml",
			State: orgsync.ActionApplied,
		},
		{Kind: orgsync.KindLabels, Subject: "bug", State: orgsync.ActionPending},
	}

	if unavailable, missing := unavailableForTarget(target, done); missing {
		t.Errorf("held the plan on %s, which had already applied", unavailable.Permission)
	}

	// And the same action still pending is still refused, because that is work
	// this attempt would go on to do.
	pending := []orgsync.Action{{
		Kind: orgsync.KindFiles, Subject: ".github/workflows/ci.yaml",
		State: orgsync.ActionPending,
	}}

	unavailable, missing := unavailableForTarget(target, pending)
	if !missing {
		t.Fatal("a workflow nobody permitted was allowed through")
	}
	if unavailable.Permission != "workflows" {
		t.Errorf("permission = %q, wanted workflows", unavailable.Permission)
	}
}
