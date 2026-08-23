package storage

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
)

const MaxInstallationSettingsRestoreSelections = 4096

var (
	ErrSettingsCheckpointCorrupt = errors.New("settings checkpoint is corrupt")
	ErrSettingsRestoreBlocked    = errors.New("settings checkpoint resource cannot be restored")
	ErrSettingsRestoreNoop       = errors.New("settings checkpoint restore has no changes")
)

// SettingsCheckpointItemIdentity names one settings aggregate without relying
// on a display name that can change after the checkpoint was recorded.
type SettingsCheckpointItemIdentity struct {
	Kind         SettingsCheckpointItemKind
	RepositoryID string
	SyncKind     orgsync.Kind
}

// SettingsCheckpointIncompatibility is a bounded, safe explanation for why an
// immutable item can be inspected but cannot be selected for restoration.
type SettingsCheckpointIncompatibility struct {
	Code   string
	Reason string
}

// SettingsCheckpointInspectionSide describes one captured side independently.
// Available distinguishes a captured absence from a side that never existed,
// such as Before on a baseline.
type SettingsCheckpointInspectionSide struct {
	Available       bool
	State           *SettingsCheckpointState
	Differs         bool
	Restorable      bool
	Incompatibility *SettingsCheckpointIncompatibility
}

// SettingsCheckpointInspectionItem combines immutable history with the
// current canonical state. Current is nil when the aggregate is absent.
type SettingsCheckpointInspectionItem struct {
	Identity           SettingsCheckpointItemIdentity
	RepositoryFullName string
	DocumentVersion    int
	Before             SettingsCheckpointInspectionSide
	After              SettingsCheckpointInspectionSide
	Current            *SettingsCheckpointState
	Changed            bool
}

// SettingsCheckpointInspection is one immutable checkpoint interpreted
// against the current state of its scope.
type SettingsCheckpointInspection struct {
	Checkpoint SettingsCheckpoint
	Items      []SettingsCheckpointInspectionItem
}

// SettingsCheckpointRestoreSelection carries the optimistic revision for one
// explicitly selected aggregate. An absent Sync aggregate has revision zero.
type SettingsCheckpointRestoreSelection struct {
	Identity         SettingsCheckpointItemIdentity
	ExpectedRevision int64
}

// RestoreInstallationSettingsRequest restores selected installation settings
// from one complete side of an immutable checkpoint.
type RestoreInstallationSettingsRequest struct {
	TargetID                       string
	CheckpointID                   int64
	ActorAccountID                 string
	ElevationID                    *string
	SessionTokenHash               string
	ChangedAt                      time.Time
	DeploymentPendingCIQuietPeriod time.Duration
	Side                           SettingsCheckpointRestoreSide
	Selections                     []SettingsCheckpointRestoreSelection
}

// Validate checks request shape before the store takes any locks.
func (request RestoreInstallationSettingsRequest) Validate() error {
	if strings.TrimSpace(request.TargetID) == "" || request.CheckpointID <= 0 ||
		strings.TrimSpace(request.ActorAccountID) == "" || request.ChangedAt.IsZero() {
		return errors.New("settings restore target, checkpoint, actor, and time are required")
	}
	if !request.Side.Valid() {
		return errors.New("settings restore needs a valid checkpoint side")
	}
	if len(request.Selections) == 0 {
		return errors.New("settings restore needs at least one selected resource")
	}
	if len(request.Selections) > MaxInstallationSettingsRestoreSelections {
		return fmt.Errorf(
			"settings restore selects more than %d resources",
			MaxInstallationSettingsRestoreSelections,
		)
	}
	seen := make(map[string]bool, len(request.Selections))
	for index, selection := range request.Selections {
		if selection.ExpectedRevision < 0 {
			return fmt.Errorf("settings restore selection %d has a negative revision", index)
		}
		if err := selection.Identity.validateInstallation(); err != nil {
			return fmt.Errorf("settings restore selection %d: %w", index, err)
		}
		key := selection.Identity.key()
		if seen[key] {
			return fmt.Errorf("duplicate settings restore selection %q", key)
		}
		seen[key] = true
	}

	return nil
}

func (identity SettingsCheckpointItemIdentity) validateInstallation() error {
	hasRepository := strings.TrimSpace(identity.RepositoryID) != ""
	hasSyncKind := identity.SyncKind.Valid()
	switch identity.Kind {
	case SettingsCheckpointItemTarget:
		if hasRepository || identity.SyncKind != "" {
			return errors.New("target identity cannot name a repository or Sync kind")
		}
	case SettingsCheckpointItemRepository:
		if !hasRepository || identity.SyncKind != "" {
			return errors.New("repository identity needs only a repository")
		}
	case SettingsCheckpointItemSyncConfig:
		if hasRepository || !hasSyncKind {
			return errors.New("sync config identity needs only a valid Sync kind")
		}
	case SettingsCheckpointItemSyncOverride:
		if !hasRepository || !hasSyncKind {
			return errors.New("sync override identity needs a repository and valid Sync kind")
		}
	default:
		return fmt.Errorf("unsupported installation settings kind %q", identity.Kind)
	}

	return nil
}

func (identity SettingsCheckpointItemIdentity) key() string {
	return strings.Join([]string{
		string(identity.Kind), identity.RepositoryID, string(identity.SyncKind),
	}, "\x00")
}
