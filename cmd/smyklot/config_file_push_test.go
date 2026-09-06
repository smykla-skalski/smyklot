package main

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

func configFilePushPayload(ref, branch string, deleted bool) []byte {
	payload, _ := json.Marshal(map[string]any{
		"ref": ref, "deleted": deleted, "forced": true,
		"installation": map[string]any{"id": 987},
		"repository": map[string]any{
			"id": 123456, "name": ".github", "full_name": "smykla-skalski/.github",
			"default_branch": branch, "owner": map[string]any{"login": "smykla-skalski"},
		},
		// Intentionally no commits: truncated payloads and force pushes still
		// require a fresh read rather than guessing which paths have changed.
	})
	return payload
}

func TestConfigFilePushScreensBranchesWithoutCommitLists(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		ref, branch string
		deleted     bool
		want        bool
	}{
		{ref: "refs/heads/main", branch: "main", want: true},
		{ref: "refs/heads/main", want: true},
		{ref: "refs/heads/main", branch: "main", deleted: true},
		{ref: "refs/heads/smyklot/config", branch: "main"},
		{ref: "refs/tags/v1.0", branch: "main"},
		{ref: "refs/heads/release/stable", branch: "release/stable", want: true},
	} {
		push, err := parseConfigFilePush(configFilePushPayload(test.ref, test.branch, test.deleted))
		if err != nil || push.relevant() != test.want {
			t.Errorf("ref=%q default=%q deleted=%t: relevant=%t err=%v", test.ref, test.branch, test.deleted, push.relevant(), err)
		}
	}
	for _, payload := range []string{"not json", "{}", `{"ref":false}`, `{"ref":"main"}`, `{"ref":"refs/heads/"}`, `{"ref":"refs/tags/"}`} {
		if _, err := parseConfigFilePush([]byte(payload)); !errors.Is(err, webhook.ErrMalformedPayload) {
			t.Errorf("invalid push %q returned %v", payload, err)
		}
	}
}
