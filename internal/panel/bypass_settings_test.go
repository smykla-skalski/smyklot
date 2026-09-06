package panel

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

func TestWorkspaceBypassSettingsPreserveOmittedPolicyAndClearExplicitNull(t *testing.T) {
	t.Parallel()
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	gates := &countingPendingCIGates{}
	harness.server.gates = gates
	for index, input := range []struct {
		present bool
		policy  any
	}{
		{true, map[string]any{"allow": true, "actors": []any{map[string]any{"actor_id": 1197525, "actor_type": "Integration", "bypass_mode": "always"}}}},
		{false, nil},
		{true, nil},
	} {
		target, err := harness.store.GetTarget(t.Context(), "github:installation:10")
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]map[string]any
		if err := json.Unmarshal(targetWorkspaceSettingsBatchBody(t, target, true), &body); err != nil {
			t.Fatal(err)
		}
		if input.present {
			body["target"]["pending_ci_bypass_policy_default"] = input.policy
		}
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		response := harness.request(t, http.MethodPut, workspaceSettingsBatchPath, bytes.NewReader(encoded), session)
		requireResponse(t, response, "bypass settings", http.StatusOK)
		current, err := harness.store.GetTarget(t.Context(), target.ID)
		if err != nil {
			t.Fatal(err)
		}
		if index < 2 {
			if current.PendingCIBypassPolicyDefault == nil || !current.PendingCIBypassPolicyDefault.Allow || len(current.PendingCIBypassPolicyDefault.Actors) != 1 {
				t.Fatalf("policy was lost on save %d: %+v", index, current.PendingCIBypassPolicyDefault)
			}
		} else if current.PendingCIBypassPolicyDefault != nil {
			t.Fatal("explicit null did not restore GitHub-owned exceptions")
		}
	}
	if gates.wakes != 2 {
		t.Fatalf("gate wakes = %d, want 2 actual policy changes", gates.wakes)
	}
}

func TestWorkspaceBypassSettingsRejectMalformedPolicy(t *testing.T) {
	t.Parallel()
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	target, err := harness.store.GetTarget(t.Context(), "github:installation:10")
	if err != nil {
		t.Fatal(err)
	}
	for _, policy := range []string{
		`{"actors":[]}`, `{"allow":true}`, `{"allow":null,"actors":[]}`,
		`{"allow":true,"actors":null}`, `{"allow":true,"actors":[],"typo":true}`,
		`{"allow":true,"actors":[{"actor_id":0,"actor_type":"Integration","bypass_mode":"always"}]}`,
	} {
		var body map[string]map[string]any
		if err := json.Unmarshal(targetWorkspaceSettingsBatchBody(t, target, true), &body); err != nil {
			t.Fatal(err)
		}
		body["target"]["pending_ci_bypass_policy_default"] = json.RawMessage(policy)
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		response := harness.request(t, http.MethodPut, workspaceSettingsBatchPath, bytes.NewReader(encoded), session)
		requireResponse(t, response, "malformed policy", http.StatusBadRequest)
	}
	current, err := harness.store.GetTarget(t.Context(), target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != target.Revision {
		t.Fatal("invalid policy modified settings")
	}
}
