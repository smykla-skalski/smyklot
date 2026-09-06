package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
)

// ConfigFileState is the reconciler's durable comparison and proposal state.
// RepositoryID is empty for the workspace's organization configuration file.
// Its revision is independent of settings, so polling cannot invalidate drafts.
type ConfigFileState struct {
	TargetID     string
	RepositoryID string
	Revision     int64
	Document     []byte
	UpdatedAt    time.Time
	// Every activation needs a fresh reconciliation before file restrictions
	// can be replaced by panel settings. The old merge baseline stays intact.
	InitializationRequired bool
}

// ConfigFileStateChange compares both the settings owner and prior observation.
// A disabled connection or a concurrent settings save invalidates background work.
type ConfigFileStateChange struct {
	TargetID         string
	RepositoryID     string
	ExpectedRevision int64
	OwnerRevision    int64
	SyncRevisions    map[orgsync.Kind]int64
	Document         []byte
	ChangedAt        time.Time
	Initialized      bool
}

const MaxConfigFileStateBytes = 32 << 20

func (change ConfigFileStateChange) Validate() error {
	if strings.TrimSpace(change.TargetID) == "" || change.ExpectedRevision < 0 ||
		change.OwnerRevision < 0 || change.ChangedAt.IsZero() {
		return errors.New("configuration file state needs a target, revisions and change time")
	}
	trimmed := bytes.TrimSpace(change.Document)
	if len(trimmed) == 0 || len(trimmed) > MaxConfigFileStateBytes ||
		trimmed[0] != '{' || !json.Valid(trimmed) {
		return errors.New("configuration file state must be a bounded JSON object")
	}
	if len(change.SyncRevisions) != len(orgsync.Kinds()) {
		return errors.New("configuration file state needs every sync resource revision")
	}
	for _, kind := range orgsync.Kinds() {
		if revision, exists := change.SyncRevisions[kind]; !exists || revision < 0 {
			return errors.New("configuration file state has an invalid sync resource revision")
		}
	}
	return nil
}

// ConfigFileImport is internal background-work provenance. It is never decoded
// from a panel request. Its state is committed atomically with imported settings,
// and the resulting checkpoint is attributed to Smyklot rather than a user.
type ConfigFileImport struct {
	State   ConfigFileStateChange
	Path    string
	HeadSHA string
}
