package storage

import (
	"time"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

// RuntimeSettings contains the persisted overrides layered over deployment
// defaults. Nil fields keep the corresponding deployment value.
type RuntimeSettings struct {
	BotConfig            *config.Config
	LogLevel             *string
	PollInterval         *time.Duration
	PendingCIQuietPeriod *time.Duration
	SessionTTL           *time.Duration

	// PathIndexInterval is how often a repository's file list is checked for
	// changes, for every installation that does not say otherwise. Zero is
	// every sweep: what a check costs is the commit the branch points at, and
	// the list is read only where that moved.
	PathIndexInterval *time.Duration

	Revision  int64
	UpdatedAt            *time.Time
	UpdatedBy            *Account
}

// RuntimeSettingsChange atomically replaces every persisted runtime override
// and appends its application-wide audit event.
type RuntimeSettingsChange struct {
	BotConfig                     *config.Config
	LogLevel                      *string
	PollInterval                  *time.Duration
	PendingCIQuietPeriod          *time.Duration
	SessionTTL                    *time.Duration
	PathIndexInterval             *time.Duration
	EffectivePollInterval         time.Duration
	EffectivePendingCIQuietPeriod time.Duration
	EffectiveSessionTTL           time.Duration
	ExpectedRevision              int64
	ActorAccountID                string
	ChangedAt                     time.Time
}
