package panel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

const (
	// maxPrefValueBytes bounds one encoded preference value on the wire.
	maxPrefValueBytes = 1024
	// maxPrefStringLength bounds free-text values such as table searches.
	maxPrefStringLength = 256
	// prefsChecksumLength is the hex length of the handshake digest. 64 bits
	// of SHA-256 is plenty to detect divergence, and keeps dial URLs short.
	prefsChecksumLength = 16
)

// prefValidator checks one decoded preference value and returns its canonical
// form: a string, or a sorted, deduplicated []string.
type prefValidator func(value any) (any, bool)

// Wire names of the syncable preference keys. The frontend duplicates them
// literally in PREF_DEFAULTS; renaming one orphans every stored value.
const (
	prefKeyTheme                = "theme"
	prefKeySidebar              = "sidebar"
	prefKeyTimeDisplay          = "history.time_display"
	prefKeyLastInstallation     = "last_installation"
	prefKeyRepositoriesSort     = "table.repositories.sort"
	prefKeyRepositoriesState    = "table.repositories.state"
	prefKeyRepositoriesFiles    = "table.repositories.files"
	prefKeyRepositoriesSettings = "table.repositories.settings"
	prefKeyRepositoriesSearch   = "table.repositories.search"
	prefKeyHistoryType          = "table.history.type"
	prefKeyHistorySort          = "table.history.sort"
	prefKeyHistoryScope         = "table.history.scope"
	prefKeyHistoryChange        = "table.history.change"
	prefKeyHistoryFailureKind   = "table.history.failure_kind"
	prefKeyHistorySearch        = "table.history.search"
	prefKeyUsersSort            = "table.users.sort"
	prefKeyUsersRoles           = "table.users.roles"
	prefKeyUsersStatuses        = "table.users.statuses"
	prefKeyUsersSearch          = "table.users.search"
	prefKeyInvitationsSort      = "table.invitations.sort"
	prefKeyInvitationsRoles     = "table.invitations.roles"
	prefKeyInvitationsStatuses  = "table.invitations.statuses"
	prefKeyInvitationsSearch    = "table.invitations.search"
)

// prefRegistry is the closed set of syncable preference keys. Unknown keys
// are rejected so the stored documents only ever hold values every shipped
// panel build can interpret.
var prefRegistry = map[string]prefValidator{
	prefKeyTheme:            oneOf("system", "light", "dark"),
	prefKeySidebar:          oneOf("expanded", "collapsed"),
	prefKeyTimeDisplay:      oneOf("relative", "absolute"),
	prefKeyLastInstallation: freeText(),

	prefKeyRepositoriesSort: oneOf(
		string(storage.RepositoryNameAscending),
		string(storage.RepositoryNameDescending),
		string(storage.RepositoryFileAscending),
		string(storage.RepositoryFileDescending),
		string(storage.RepositoryOverridesAscending),
		string(storage.RepositoryOverridesDescending),
		string(storage.RepositoryNewest),
		string(storage.RepositoryOldest),
	),
	prefKeyRepositoriesState: oneOf("all", "enabled", "disabled"),
	prefKeyRepositoriesFiles: setOf(
		string(storage.RepositoryFileMissing),
		string(storage.RepositoryFileValid),
		string(storage.RepositoryFileInvalid),
		string(storage.RepositoryFileBypassed),
	),
	prefKeyRepositoriesSettings: settingFilter(),
	prefKeyRepositoriesSearch:   freeText(),

	prefKeyHistoryType: oneOf("audit", "failures"),
	prefKeyHistorySort: oneOf(
		string(storage.HistoryNewest),
		string(storage.HistoryOldest),
		string(storage.HistoryActorAscending),
		string(storage.HistoryActorDescending),
		string(storage.HistoryTargetAscending),
		string(storage.HistoryTargetDescending),
		string(storage.HistoryChangeAscending),
		string(storage.HistoryChangeDescending),
		string(storage.HistoryStatusAscending),
		string(storage.HistoryStatusDescending),
		string(storage.HistoryRepositoryAscending),
		string(storage.HistoryRepositoryDescending),
	),
	prefKeyHistoryScope: oneOf(
		string(storage.AuditAll),
		string(storage.AuditAccount),
		string(storage.AuditRepositories),
	),
	prefKeyHistoryChange: oneOf(
		string(storage.AuditChangeAll),
		string(storage.AuditChangeEnablement),
		string(storage.AuditChangeRepository),
		string(storage.AuditChangeAccount),
		string(storage.AuditChangeSync),
	),
	prefKeyHistoryFailureKind: oneOf("all", "retryable", "permanent"),
	prefKeyHistorySearch:      freeText(),

	prefKeyUsersSort: oneOf(
		string(storage.PanelUserNameAscending),
		string(storage.PanelUserNameDescending),
		string(storage.PanelUserRoleAscending),
		string(storage.PanelUserRoleDescending),
		string(storage.PanelUserUpdatedNewest),
		string(storage.PanelUserUpdatedOldest),
		string(storage.PanelUserLoginNewest),
		string(storage.PanelUserLoginOldest),
	),
	prefKeyUsersRoles: setOf(
		string(storage.InstallationRoleNone),
		string(storage.InstallationRoleViewer),
		string(storage.InstallationRoleEditor),
		string(storage.InstallationRoleAdmin),
	),
	prefKeyUsersStatuses: setOf(
		string(storage.PanelUserListActive),
		string(storage.PanelUserListBanned),
		string(storage.PanelUserListSuspended),
	),
	prefKeyUsersSearch: freeText(),

	prefKeyInvitationsSort: oneOf(
		string(storage.InvitationCreatedNewest),
		string(storage.InvitationCreatedOldest),
		string(storage.InvitationExpirySoonest),
		string(storage.InvitationExpiryLatest),
		string(storage.InvitationNameAscending),
		string(storage.InvitationNameDescending),
		string(storage.InvitationRoleAscending),
		string(storage.InvitationRoleDescending),
	),
	prefKeyInvitationsRoles: setOf(
		string(storage.InstallationRoleViewer),
		string(storage.InstallationRoleEditor),
		string(storage.InstallationRoleAdmin),
	),
	prefKeyInvitationsStatuses: setOf(
		string(storage.InvitationPending),
		string(storage.InvitationAccepted),
		string(storage.InvitationDeclined),
		string(storage.InvitationRevoked),
		string(storage.InvitationExpired),
	),
	prefKeyInvitationsSearch: freeText(),
}

// validatePrefChanges splits one patch into canonically re-encoded accepted
// values and rejected keys. Deletions (JSON null) pass without a value check
// so any registered key can always be reset.
func validatePrefChanges(
	changes map[string]json.RawMessage,
) (map[string]json.RawMessage, []string) {
	accepted := make(map[string]json.RawMessage, len(changes))
	var rejected []string

	for key, raw := range changes {
		validate, known := prefRegistry[key]
		if !known {
			rejected = append(rejected, key)
			continue
		}

		if raw == nil || string(raw) == "null" {
			accepted[key] = nil
			continue
		}

		if len(raw) > maxPrefValueBytes || !utf8.Valid(raw) {
			rejected = append(rejected, key)
			continue
		}

		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			rejected = append(rejected, key)
			continue
		}

		canonical, ok := validate(value)
		if !ok {
			rejected = append(rejected, key)
			continue
		}

		encoded, err := canonicalValue(canonical)
		if err != nil {
			rejected = append(rejected, key)
			continue
		}

		accepted[key] = encoded
	}

	sort.Strings(rejected)

	return accepted, rejected
}

// prefsReadyInfo builds the handshake payload for the ready frame: revision
// and checksum always, the canonical document only when the client's dial
// parameters did not match the stored state.
func prefsReadyInfo(query url.Values, prefs storage.Preferences) *panelPrefsInfo {
	sum := prefsChecksum(prefs.Values)
	info := &panelPrefsInfo{Rev: prefs.Revision, Sum: sum}

	clientRev, err := strconv.ParseInt(query.Get("prefs_rev"), 10, 64)
	if err == nil && clientRev == prefs.Revision && sum != "" && query.Get("prefs_sum") == sum {
		return info
	}

	snapshot, err := canonicalPrefs(prefs.Values)
	if err != nil {
		// The stored document cannot be serialized; reset the client to
		// empty and let its pending changes rebuild it.
		snapshot = []byte("{}")
	}
	info.Values = json.RawMessage(snapshot)

	return info
}

// applyPrefsPatch commits validated changes and fans them out to every
// connection of the account, the originator included — its echo doubles as
// the acknowledgement. The mutex makes fan-out order match commit order;
// without it two concurrent patches could announce revisions out of order
// and the clients' monotonic-revision rule would drop one.
func (s *Server) applyPrefsPatch(
	ctx context.Context,
	accountID string,
	accepted map[string]json.RawMessage,
) error {
	s.prefsMu.Lock()
	defer s.prefsMu.Unlock()

	updated, err := s.store.ApplyPreferences(ctx, storage.PreferenceChange{
		AccountID: accountID,
		Changes:   accepted,
		ChangedAt: s.now(),
	})
	if err != nil {
		return err
	}

	s.events.announceAccount(accountID, panelEvent{
		Type:    panelEventPrefsChanged,
		Rev:     updated.Revision,
		Changes: accepted,
	})

	return nil
}

// prefsChecksum digests a preference document for the connect handshake. A
// document that cannot be serialized digests to "", which never matches and
// degrades to a snapshot.
func prefsChecksum(values map[string]json.RawMessage) string {
	canonical, err := canonicalPrefs(values)
	if err != nil {
		return ""
	}

	digest := sha256.Sum256(canonical)

	return hex.EncodeToString(digest[:])[:prefsChecksumLength]
}

// canonicalPrefs serializes a preference document to the canonical bytes both
// sides digest: keys sorted bytewise, compact separators, and string escaping
// that matches JSON.stringify exactly.
func canonicalPrefs(values map[string]json.RawMessage) ([]byte, error) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	builder.WriteByte('{')
	for index, key := range keys {
		if index > 0 {
			builder.WriteByte(',')
		}

		writeCanonicalString(&builder, key)
		builder.WriteByte(':')

		var value any
		if err := json.Unmarshal(values[key], &value); err != nil {
			return nil, fmt.Errorf("decode preference %q: %w", key, err)
		}

		if err := writeCanonicalValue(&builder, value); err != nil {
			return nil, fmt.Errorf("serialize preference %q: %w", key, err)
		}
	}
	builder.WriteByte('}')

	return []byte(builder.String()), nil
}

func canonicalValue(value any) (json.RawMessage, error) {
	var builder strings.Builder
	if err := writeCanonicalValue(&builder, value); err != nil {
		return nil, err
	}

	return json.RawMessage(builder.String()), nil
}

func writeCanonicalValue(builder *strings.Builder, value any) error {
	switch typed := value.(type) {
	case string:
		writeCanonicalString(builder, typed)
		return nil
	case []string:
		elements := make([]any, len(typed))
		for index, element := range typed {
			elements[index] = element
		}

		return writeCanonicalArray(builder, elements)
	case []any:
		return writeCanonicalArray(builder, typed)
	case map[string]any:
		return writeCanonicalObject(builder, typed)
	default:
		return fmt.Errorf("preference values must be strings, string arrays, or string objects, got %T", value)
	}
}

func writeCanonicalArray(builder *strings.Builder, elements []any) error {
	builder.WriteByte('[')
	for index, element := range elements {
		if index > 0 {
			builder.WriteByte(',')
		}
		if err := writeCanonicalValue(builder, element); err != nil {
			return err
		}
	}
	builder.WriteByte(']')

	return nil
}

func writeCanonicalObject(builder *strings.Builder, values map[string]any) error {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	builder.WriteByte('{')
	for index, key := range keys {
		if index > 0 {
			builder.WriteByte(',')
		}
		writeCanonicalString(builder, key)
		builder.WriteByte(':')
		if err := writeCanonicalValue(builder, values[key]); err != nil {
			return err
		}
	}
	builder.WriteByte('}')

	return nil
}

// writeCanonicalString escapes exactly like JSON.stringify: the two mandatory
// escapes, the five short control escapes, \u00xx for the remaining control
// characters, and everything else raw — no HTML or U+2028/U+2029 escaping.
func writeCanonicalString(builder *strings.Builder, value string) {
	const hexDigits = "0123456789abcdef"

	builder.WriteByte('"')
	for _, character := range value {
		switch character {
		case '"':
			builder.WriteString(`\"`)
		case '\\':
			builder.WriteString(`\\`)
		case '\b':
			builder.WriteString(`\b`)
		case '\t':
			builder.WriteString(`\t`)
		case '\n':
			builder.WriteString(`\n`)
		case '\f':
			builder.WriteString(`\f`)
		case '\r':
			builder.WriteString(`\r`)
		default:
			if character < 0x20 {
				builder.WriteString(`\u00`)
				builder.WriteByte(hexDigits[character>>4])
				builder.WriteByte(hexDigits[character&0xf])
				continue
			}

			builder.WriteRune(character)
		}
	}
	builder.WriteByte('"')
}

// oneOf accepts a single string drawn from a closed set.
func oneOf(allowed ...string) prefValidator {
	return func(value any) (any, bool) {
		text, ok := value.(string)
		if !ok || !slices.Contains(allowed, text) {
			return nil, false
		}

		return text, true
	}
}

// setOf accepts an array of strings drawn from a closed set, canonicalized by
// sorting and deduplication so equal selections digest identically.
func setOf(allowed ...string) prefValidator {
	return func(value any) (any, bool) {
		elements, ok := stringSlice(value)
		if !ok || len(elements) > len(allowed) {
			return nil, false
		}

		for _, element := range elements {
			if !slices.Contains(allowed, element) {
				return nil, false
			}
		}

		return sortedSet(elements), true
	}
}

// freeText accepts one line of user text: length-capped, well-formed, and
// free of control characters and the U+2028/U+2029 separators, mirroring the
// client-side sanitizer so client-accepted values are server-accepted.
func freeText() prefValidator {
	return func(value any) (any, bool) {
		text, ok := value.(string)
		if !ok || !validPrefText(text) {
			return nil, false
		}

		return text, true
	}
}

// settingFilter accepts the flat encoding of the repository setting filter:
// a mode in the first element, config keys after it when the mode is "keys".
// The key list must stay identical to the frontend's CONFIG_KEYS: a key only
// one side knows would be stored here but silently collapse the filter to
// "all" in decodeRepositorySettingFilter. `runner` stays out until the panel
// models it.
func settingFilter() prefValidator {
	configKeys := []string{
		config.KeyQuietSuccess,
		config.KeyQuietReactions,
		config.KeyQuietPending,
		config.KeyAllowedCommands,
		config.KeyCommandAliases,
		config.KeyCommandPrefix,
		config.KeyDisableMentions,
		config.KeyDisableBareCommands,
		config.KeyDisableUnapprove,
		config.KeyDisableReactions,
		config.KeyDisableDeletedComments,
		config.KeyAllowSelfApproval,
	}

	return func(value any) (any, bool) {
		elements, ok := stringSlice(value)
		if !ok || len(elements) == 0 {
			return nil, false
		}

		const modeAll, modeCustom, modeNone, modeKeys = "all", "custom", "none", "keys"

		mode, keys := elements[0], elements[1:]
		switch mode {
		case modeAll, modeCustom, modeNone:
			if len(keys) != 0 {
				return nil, false
			}

			return []string{mode}, true
		case modeKeys:
			if len(keys) == 0 || len(keys) > len(configKeys) {
				return nil, false
			}
			for _, key := range keys {
				if !slices.Contains(configKeys, key) {
					return nil, false
				}
			}

			return append([]string{mode}, sortedSet(keys)...), true
		default:
			return nil, false
		}
	}
}

func stringSlice(value any) ([]string, bool) {
	raw, ok := value.([]any)
	if !ok {
		return nil, false
	}

	elements := make([]string, 0, len(raw))
	for _, element := range raw {
		text, ok := element.(string)
		if !ok || !validPrefText(text) {
			return nil, false
		}

		elements = append(elements, text)
	}

	return elements, true
}

func sortedSet(elements []string) []string {
	sorted := slices.Clone(elements)
	sort.Strings(sorted)

	return slices.Compact(sorted)
}

func validPrefText(text string) bool {
	if utf8.RuneCountInString(text) > maxPrefStringLength || !utf8.ValidString(text) {
		return false
	}

	for _, character := range text {
		if character < 0x20 || character == 0x7f ||
			character == '\u2028' || character == '\u2029' {
			return false
		}
	}

	return true
}
