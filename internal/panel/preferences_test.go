package panel

import (
	"encoding/json"
	"testing"
)

// The canonical strings and checksums below are shared golden vectors — keep
// them in sync with internal/panel/frontend/tests/preferences-sync.test.ts.
var prefsChecksumVectors = []struct {
	name      string
	values    map[string]json.RawMessage
	canonical string
	checksum  string
}{
	{
		name:      "empty document",
		values:    map[string]json.RawMessage{},
		canonical: `{}`,
		checksum:  "44136fa355b3678a",
	},
	{
		name: "single string",
		values: map[string]json.RawMessage{
			prefKeyTheme: json.RawMessage(`"dark"`),
		},
		canonical: `{"theme":"dark"}`,
		checksum:  "0f4f87db4567232a",
	},
	{
		name: "sorted keys with every value shape",
		values: map[string]json.RawMessage{
			prefKeyTheme:         json.RawMessage(`"system"`),
			prefKeyUsersRoles:    json.RawMessage(`["viewer","admin"]`),
			prefKeyLastWorkspace: json.RawMessage(`"smykla-skalski"`),
		},
		canonical: `{"last_workspace":"smykla-skalski","table.users.roles":["viewer","admin"],"theme":"system"}`,
		checksum:  "1e72d4a9604723ef",
	},
	{
		name: "JSON.stringify escaping",
		values: map[string]json.RawMessage{
			prefKeyHistorySearch: json.RawMessage("\"he said \\\"hi\\\" \\\\ <&> \\t tab \\u0001 low\""),
			prefKeyUsersSearch:   json.RawMessage(`"π🙂 emoji"`),
		},
		canonical: `{"table.history.search":"he said \"hi\" \\ <&> \t tab \u0001 low","table.users.search":"π🙂 emoji"}`,
		checksum:  "44161ee8b69d82a4",
	},
}

func TestPrefsChecksumGoldenVectors(t *testing.T) {
	for _, vector := range prefsChecksumVectors {
		t.Run(vector.name, func(t *testing.T) {
			canonical, err := canonicalPrefs(vector.values)
			if err != nil {
				t.Fatalf("canonicalPrefs: %v", err)
			}
			if string(canonical) != vector.canonical {
				t.Fatalf("canonical form %q, want %q", canonical, vector.canonical)
			}
			if sum := prefsChecksum(vector.values); sum != vector.checksum {
				t.Fatalf("checksum %q, want %q", sum, vector.checksum)
			}
		})
	}
}

func TestPrefsChecksumRejectsUnsupportedShapes(t *testing.T) {
	values := map[string]json.RawMessage{
		prefKeyTheme: json.RawMessage(`42`),
	}
	if _, err := canonicalPrefs(values); err == nil {
		t.Fatal("expected an error for a numeric value")
	}
	if sum := prefsChecksum(values); sum != "" {
		t.Fatalf("checksum %q, want empty for an unserializable document", sum)
	}
}

func TestValidatePrefChanges(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    string
		accepted string
	}{
		{name: "theme enum", key: "theme", value: `"system"`, accepted: `"system"`},
		{name: "theme unknown value", key: "theme", value: `"sepia"`},
		{name: "theme wrong type", key: "theme", value: `["dark"]`},
		{name: "sidebar enum", key: "sidebar", value: `"collapsed"`, accepted: `"collapsed"`},
		{name: "time display enum", key: "history.time_display", value: `"absolute"`, accepted: `"absolute"`},
		{name: "last workspace", key: "last_workspace", value: `"smykla-skalski"`, accepted: `"smykla-skalski"`},
		{name: "repository sort", key: "table.repositories.sort", value: `"file_desc"`, accepted: `"file_desc"`},
		{name: "repository sort unknown", key: "table.repositories.sort", value: `"size_asc"`},
		{name: "history sort", key: "table.history.sort", value: `"repository_desc"`, accepted: `"repository_desc"`},
		{name: "history scope", key: "table.history.scope", value: `"repositories"`, accepted: `"repositories"`},
		{name: "history change", key: "table.history.change", value: `"repository"`, accepted: `"repository"`},
		{name: "Sync history change", key: "table.history.change", value: `"sync"`, accepted: `"sync"`},
		{name: "failure kind", key: "table.history.failure_kind", value: `"retryable"`, accepted: `"retryable"`},
		{name: "history type", key: "table.history.type", value: `"failures"`, accepted: `"failures"`},
		{name: "users sort", key: "table.users.sort", value: `"login_newest"`, accepted: `"login_newest"`},
		{name: "invitations sort", key: "table.invitations.sort", value: `"expiry_soonest"`, accepted: `"expiry_soonest"`},
		{
			name:     "file filter set is sorted and deduplicated",
			key:      "table.repositories.files",
			value:    `["valid","missing","valid"]`,
			accepted: `["missing","valid"]`,
		},
		{name: "file filter unknown member", key: "table.repositories.files", value: `["valid","huge"]`},
		{
			name:     "role set canonicalized",
			key:      "table.users.roles",
			value:    `["viewer","admin"]`,
			accepted: `["admin","viewer"]`,
		},
		{name: "statuses set", key: "table.users.statuses", value: `["suspended"]`, accepted: `["suspended"]`},
		{
			name:  "invitation roles exclude none",
			key:   "table.invitations.roles",
			value: `["none"]`,
		},
		{
			name:     "invitation statuses",
			key:      "table.invitations.statuses",
			value:    `["expired","pending"]`,
			accepted: `["expired","pending"]`,
		},
		{name: "state filter", key: "table.repositories.state", value: `"enabled"`, accepted: `"enabled"`},
		{
			name:     "setting filter plain mode",
			key:      "table.repositories.settings",
			value:    `["custom"]`,
			accepted: `["custom"]`,
		},
		{
			name:     "setting filter keys mode canonicalized",
			key:      "table.repositories.settings",
			value:    `["keys","quiet_success","allow_draft_merges","allowed_commands"]`,
			accepted: `["keys","allow_draft_merges","allowed_commands","quiet_success"]`,
		},
		{name: "setting filter keys mode needs keys", key: "table.repositories.settings", value: `["keys"]`},
		{name: "setting filter unknown key", key: "table.repositories.settings", value: `["keys","favourite"]`},
		{
			name:  "setting filter rejects keys the panel cannot decode",
			key:   "table.repositories.settings",
			value: `["keys","runner"]`,
		},
		{name: "setting filter plain mode rejects keys", key: "table.repositories.settings", value: `["all","runner"]`},
		{name: "setting filter empty", key: "table.repositories.settings", value: `[]`},
		{name: "search text", key: "table.users.search", value: `"bots"`, accepted: `"bots"`},
		{name: "search rejects control characters", key: "table.users.search", value: "\"a\\u0000b\""},
		{name: "search rejects line separator", key: "table.users.search", value: "\"a\\u2028b\""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			accepted, rejected := validatePrefChanges(map[string]json.RawMessage{
				test.key: json.RawMessage(test.value),
			})
			if test.accepted == "" {
				if len(rejected) != 1 || rejected[0] != test.key {
					t.Fatalf("rejected %v, want [%s]", rejected, test.key)
				}
				if len(accepted) != 0 {
					t.Fatalf("accepted %v, want none", accepted)
				}

				return
			}

			if len(rejected) != 0 {
				t.Fatalf("rejected %v, want none", rejected)
			}
			if got := string(accepted[test.key]); got != test.accepted {
				t.Fatalf("accepted %q, want %q", got, test.accepted)
			}
		})
	}
}

func TestValidatePrefChangesEdges(t *testing.T) {
	t.Run("unknown key is rejected", func(t *testing.T) {
		accepted, rejected := validatePrefChanges(map[string]json.RawMessage{
			"page_size": json.RawMessage(`"20"`),
		})
		if len(accepted) != 0 || len(rejected) != 1 {
			t.Fatalf("accepted %v rejected %v", accepted, rejected)
		}
	})

	t.Run("null deletes without value validation", func(t *testing.T) {
		accepted, rejected := validatePrefChanges(map[string]json.RawMessage{
			prefKeyTheme: json.RawMessage(`null`),
		})
		if len(rejected) != 0 {
			t.Fatalf("rejected %v, want none", rejected)
		}
		value, present := accepted["theme"]
		if !present || value != nil {
			t.Fatalf("accepted %v, want a nil deletion marker", accepted)
		}
	})

	t.Run("oversized value is rejected", func(t *testing.T) {
		huge := make([]byte, maxPrefValueBytes+2)
		for index := range huge {
			huge[index] = 'a'
		}
		huge[0], huge[len(huge)-1] = '"', '"'
		_, rejected := validatePrefChanges(map[string]json.RawMessage{
			prefKeyUsersSearch: json.RawMessage(huge),
		})
		if len(rejected) != 1 {
			t.Fatalf("rejected %v, want the oversized key", rejected)
		}
	})

	t.Run("mixed patch splits accepted and rejected", func(t *testing.T) {
		accepted, rejected := validatePrefChanges(map[string]json.RawMessage{
			prefKeyTheme:   json.RawMessage(`"dark"`),
			"bogus":        json.RawMessage(`"x"`),
			prefKeySidebar: json.RawMessage(`"nope"`),
		})
		if len(accepted) != 1 || string(accepted["theme"]) != `"dark"` {
			t.Fatalf("accepted %v, want only theme", accepted)
		}
		if len(rejected) != 2 || rejected[0] != "bogus" || rejected[1] != "sidebar" {
			t.Fatalf("rejected %v, want sorted [bogus sidebar]", rejected)
		}
	})
}
