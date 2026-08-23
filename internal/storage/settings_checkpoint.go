package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

// SettingsCheckpointScope says which settings boundary one checkpoint belongs
// to. Installation checkpoints always name a target; Root checkpoints never do.
type SettingsCheckpointScope string

const (
	SettingsCheckpointScopeRoot         SettingsCheckpointScope = "root"
	SettingsCheckpointScopeInstallation SettingsCheckpointScope = "installation"
)

// SettingsCheckpointAction records why a complete settings snapshot exists.
type SettingsCheckpointAction string

const (
	SettingsCheckpointActionBaseline SettingsCheckpointAction = "baseline"
	SettingsCheckpointActionSave     SettingsCheckpointAction = "save"
	SettingsCheckpointActionRestore  SettingsCheckpointAction = "restore"
)

// SettingsCheckpointRestoreSide names the complete point-in-time state a
// restore selected from its immutable source checkpoint.
type SettingsCheckpointRestoreSide string

const (
	SettingsCheckpointRestoreBefore SettingsCheckpointRestoreSide = "before"
	SettingsCheckpointRestoreAfter  SettingsCheckpointRestoreSide = "after"
)

// Valid reports whether the side can be selected for restoration.
func (side SettingsCheckpointRestoreSide) Valid() bool {
	return side == SettingsCheckpointRestoreBefore || side == SettingsCheckpointRestoreAfter
}

// SettingsCheckpointItemKind is the bounded set of resources the settings
// coordinator can save together. Each kind has a fixed discriminator shape;
// there is no arbitrary resource-name registry hidden in persistence.
type SettingsCheckpointItemKind string

const (
	SettingsCheckpointItemTarget       SettingsCheckpointItemKind = "target"
	SettingsCheckpointItemRepository   SettingsCheckpointItemKind = "repository"
	SettingsCheckpointItemSyncConfig   SettingsCheckpointItemKind = "sync_config"
	SettingsCheckpointItemSyncOverride SettingsCheckpointItemKind = "sync_override"
	SettingsCheckpointItemRuntime      SettingsCheckpointItemKind = "runtime"
)

// SettingsCheckpointDocumentVersion is the current full-resource document
// shape. The store keeps this beside each item so restore can reject a document
// it no longer knows how to decode rather than guessing at immutable history.
const SettingsCheckpointDocumentVersion = 1

// SettingsCheckpointState is one complete resource document at one revision.
// A nil state on an item means that resource did not exist on that side of the
// change. Digest fingerprints the exact stored document bytes.
type SettingsCheckpointState struct {
	Document []byte
	Revision int64
	Digest   string
}

// NewSettingsCheckpointState builds a state with its matching digest.
func NewSettingsCheckpointState(document []byte, revision int64) SettingsCheckpointState {
	return SettingsCheckpointState{
		Document: append([]byte(nil), document...),
		Revision: revision,
		Digest:   DigestSettingsCheckpointDocument(document),
	}
}

// DigestSettingsCheckpointDocument fingerprints one full resource document.
func DigestSettingsCheckpointDocument(document []byte) string {
	sum := sha256.Sum256(document)

	return hex.EncodeToString(sum[:])
}

// SettingsCheckpointItem is one resource in an immutable checkpoint.
// Repository and Sync fields are typed discriminators, not values callers must
// rediscover by decoding the document.
type SettingsCheckpointItem struct {
	Kind               SettingsCheckpointItemKind
	RepositoryID       string
	RepositoryFullName string
	SyncKind           orgsync.Kind
	DocumentVersion    int
	Before             *SettingsCheckpointState
	After              *SettingsCheckpointState
}

// RuntimeSettingsDocument is the complete set of persisted Root runtime
// overrides. Deployment defaults are deliberately absent: a restore must put
// the stored overrides back, then let the current deployment resolve them.
type RuntimeSettingsDocument struct {
	BotConfig            *config.Config `json:"bot_config"`
	LogLevel             *string        `json:"log_level"`
	PollInterval         *time.Duration `json:"poll_interval"`
	PendingCIQuietPeriod *time.Duration `json:"pending_ci_quiet_period"`
	SessionTTL           *time.Duration `json:"session_ttl"`
	PathIndexInterval    *time.Duration `json:"path_index_interval"`
}

// SettingsCheckpointCreate is one complete point-in-time snapshot. Every
// present side is a complete resource document; an omitted optional resource
// is captured as absence by the complete snapshot boundary.
type SettingsCheckpointCreate struct {
	Scope          SettingsCheckpointScope
	TargetID       string
	ActorAccountID string
	Action         SettingsCheckpointAction
	RestoredFromID *int64
	RestoredSide   SettingsCheckpointRestoreSide
	CreatedAt      time.Time
	Items          []SettingsCheckpointItem
}

// SettingsCheckpoint is one immutable settings snapshot read from persistence.
type SettingsCheckpoint struct {
	ID             int64
	Scope          SettingsCheckpointScope
	TargetID       string
	ActorAccountID string
	Action         SettingsCheckpointAction
	RestoredFromID *int64
	RestoredSide   SettingsCheckpointRestoreSide
	CreatedAt      time.Time
	Items          []SettingsCheckpointItem
}

// SettingsCheckpointRef scopes a checkpoint read. Requiring the scope again
// prevents a numeric id from crossing an installation or Root boundary.
type SettingsCheckpointRef struct {
	ID       int64
	Scope    SettingsCheckpointScope
	TargetID string
}

// Validate checks the invariants shared by every storage engine.
func (create SettingsCheckpointCreate) Validate() error {
	if err := validateSettingsCheckpointScope(create.Scope, create.TargetID); err != nil {
		return err
	}
	if strings.TrimSpace(create.ActorAccountID) == "" || create.CreatedAt.IsZero() {
		return errors.New("settings checkpoint actor and creation time are required")
	}
	if err := validateSettingsCheckpointAction(
		create.Action,
		create.RestoredFromID,
		create.RestoredSide,
	); err != nil {
		return err
	}

	return validateSettingsCheckpointItems(create.Scope, create.Action, create.Items)
}

func validateSettingsCheckpointAction(
	action SettingsCheckpointAction,
	restoredFromID *int64,
	restoredSide SettingsCheckpointRestoreSide,
) error {
	switch action {
	case SettingsCheckpointActionBaseline:
		if restoredFromID != nil || restoredSide != "" {
			return errors.New("baseline settings checkpoint cannot name a restore source or side")
		}
	case SettingsCheckpointActionSave:
		if restoredFromID != nil || restoredSide != "" {
			return errors.New("saved settings checkpoint cannot name a restore source or side")
		}
	case SettingsCheckpointActionRestore:
		if restoredFromID == nil || *restoredFromID <= 0 {
			return errors.New("restored settings checkpoint needs a restore source")
		}
		if !restoredSide.Valid() {
			return errors.New("restored settings checkpoint needs a valid restore side")
		}
	default:
		return fmt.Errorf("unsupported settings checkpoint action %q", action)
	}

	return nil
}

func validateSettingsCheckpointItems(
	scope SettingsCheckpointScope,
	action SettingsCheckpointAction,
	items []SettingsCheckpointItem,
) error {
	if len(items) == 0 {
		return errors.New("settings checkpoint needs at least one changed item")
	}

	seen := make(map[string]bool, len(items))
	for index, item := range items {
		if err := item.validate(scope); err != nil {
			return fmt.Errorf("settings checkpoint item %d: %w", index, err)
		}
		if action == SettingsCheckpointActionBaseline &&
			(item.Before != nil || item.After == nil) {
			return fmt.Errorf(
				"settings checkpoint item %d: baseline item needs only an after state",
				index,
			)
		}
		identity := item.identity()
		if seen[identity] {
			return fmt.Errorf("duplicate settings checkpoint item %q", identity)
		}
		seen[identity] = true
	}

	return nil
}

// Validate checks a scoped read reference.
func (ref SettingsCheckpointRef) Validate() error {
	if ref.ID <= 0 {
		return errors.New("settings checkpoint id must be positive")
	}

	return validateSettingsCheckpointScope(ref.Scope, ref.TargetID)
}

func validateSettingsCheckpointScope(scope SettingsCheckpointScope, targetID string) error {
	switch scope {
	case SettingsCheckpointScopeRoot:
		if targetID != "" {
			return errors.New("root settings checkpoint cannot name an installation")
		}
	case SettingsCheckpointScopeInstallation:
		if strings.TrimSpace(targetID) == "" {
			return errors.New("installation settings checkpoint needs a target")
		}
	default:
		return fmt.Errorf("unsupported settings checkpoint scope %q", scope)
	}

	return nil
}

func (item SettingsCheckpointItem) validate(scope SettingsCheckpointScope) error {
	if item.DocumentVersion <= 0 {
		return errors.New("document version must be positive")
	}
	if err := item.validateDiscriminators(scope); err != nil {
		return err
	}
	if item.Before == nil && item.After == nil {
		return errors.New("before or after state is required")
	}
	if err := validateSettingsCheckpointState("before", item.Before); err != nil {
		return err
	}
	return validateSettingsCheckpointState("after", item.After)
}

func (item SettingsCheckpointItem) validateDiscriminators(scope SettingsCheckpointScope) error {
	hasRepository := strings.TrimSpace(item.RepositoryID) != "" &&
		strings.TrimSpace(item.RepositoryFullName) != ""
	hasNoRepository := item.RepositoryID == "" && item.RepositoryFullName == ""
	hasSyncKind := item.SyncKind.Valid()

	switch item.Kind {
	case SettingsCheckpointItemTarget:
		if scope != SettingsCheckpointScopeInstallation || !hasNoRepository || item.SyncKind != "" {
			return errors.New("target item needs installation scope and no resource discriminator")
		}
	case SettingsCheckpointItemRepository:
		if scope != SettingsCheckpointScopeInstallation || !hasRepository || item.SyncKind != "" {
			return errors.New("repository item needs installation scope and a repository discriminator")
		}
	case SettingsCheckpointItemSyncConfig:
		if scope != SettingsCheckpointScopeInstallation || !hasNoRepository || !hasSyncKind {
			return errors.New("sync config item needs installation scope and a sync kind")
		}
	case SettingsCheckpointItemSyncOverride:
		if scope != SettingsCheckpointScopeInstallation || !hasRepository || !hasSyncKind {
			return errors.New("sync override item needs installation scope, repository, and sync kind")
		}
	case SettingsCheckpointItemRuntime:
		if scope != SettingsCheckpointScopeRoot || !hasNoRepository || item.SyncKind != "" {
			return errors.New("runtime item needs Root scope and no resource discriminator")
		}
	default:
		return fmt.Errorf("unsupported settings checkpoint item kind %q", item.Kind)
	}

	return nil
}

func validateSettingsCheckpointState(name string, state *SettingsCheckpointState) error {
	if state == nil {
		return nil
	}
	if state.Revision < 0 {
		return fmt.Errorf("%s revision cannot be negative", name)
	}
	if !json.Valid(state.Document) {
		return fmt.Errorf("%s document must be valid JSON", name)
	}
	if state.Digest != DigestSettingsCheckpointDocument(state.Document) {
		return fmt.Errorf("%s document digest does not match", name)
	}

	return nil
}

func (item SettingsCheckpointItem) identity() string {
	return strings.Join([]string{
		string(item.Kind), item.RepositoryID, string(item.SyncKind),
	}, "\x00")
}
