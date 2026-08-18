package storage

import (
	"fmt"
	"strings"
	"time"
)

const (
	PendingCICheckName   = "Smyklot / merge after CI"
	PendingCIRulesetName = "Smyklot: merge after CI"
)

// PendingCIMode selects the artifact used to represent a merge-after-CI
// authorization. It is panel-owned: repository configuration files cannot
// change the repository protection contract.
type PendingCIMode string

const (
	PendingCIModeLabels PendingCIMode = "labels"
	PendingCIModeChecks PendingCIMode = "checks"
)

func (mode PendingCIMode) Valid() bool {
	return mode == PendingCIModeLabels || mode == PendingCIModeChecks
}

// PendingCIBranchPatterns is the GitHub ruleset ref condition. Values use
// GitHub's raw ruleset spelling, including ~DEFAULT_BRANCH and refs/heads/*.
type PendingCIBranchPatterns struct {
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}

func DefaultPendingCIBranchPatterns() PendingCIBranchPatterns {
	return PendingCIBranchPatterns{Include: []string{"~DEFAULT_BRANCH"}, Exclude: []string{}}
}

func (patterns PendingCIBranchPatterns) Validate() error {
	if len(patterns.Include) == 0 {
		return fmt.Errorf("at least one pending CI branch include is required")
	}
	for _, pattern := range append(append([]string{}, patterns.Include...), patterns.Exclude...) {
		if !validPendingCIRefPattern(pattern) {
			return fmt.Errorf("invalid pending CI branch pattern %q", pattern)
		}
	}

	return nil
}

func validPendingCIRefPattern(pattern string) bool {
	if pattern == "~DEFAULT_BRANCH" || pattern == "~ALL" {
		return true
	}

	return strings.HasPrefix(pattern, "refs/heads/") &&
		len(pattern) > len("refs/heads/") &&
		!strings.ContainsAny(pattern, "\r\n\t ")
}

func validatePendingCIQuietPeriod(value *time.Duration) error {
	if value == nil {
		return nil
	}
	if *value < 0 || *value > 24*time.Hour || *value%time.Second != 0 {
		return fmt.Errorf("pending CI quiet period must be whole seconds from 0 to 24 hours")
	}

	return nil
}

func ValidateTargetPendingCISettings(
	mode PendingCIMode,
	patterns PendingCIBranchPatterns,
	quietPeriod *time.Duration,
) error {
	if !mode.Valid() {
		return fmt.Errorf("invalid pending CI mode %q", mode)
	}
	if err := patterns.Validate(); err != nil {
		return err
	}

	return validatePendingCIQuietPeriod(quietPeriod)
}

func ValidateRepositoryPendingCISettings(
	mode *PendingCIMode,
	patterns *PendingCIBranchPatterns,
	quietPeriod *time.Duration,
) error {
	if mode != nil && !mode.Valid() {
		return fmt.Errorf("invalid pending CI mode %q", *mode)
	}
	if patterns != nil {
		if err := patterns.Validate(); err != nil {
			return err
		}
	}

	return validatePendingCIQuietPeriod(quietPeriod)
}

func EffectivePendingCISettings(
	target Target,
	repository Repository,
	globalQuiet time.Duration,
) (PendingCIMode, PendingCIBranchPatterns, time.Duration) {
	mode := target.PendingCIModeDefault
	if repository.PendingCIModeOverride != nil {
		mode = *repository.PendingCIModeOverride
	}
	patterns := target.PendingCIBranchPatternsDefault
	if repository.PendingCIBranchPatternsOverride != nil {
		patterns = *repository.PendingCIBranchPatternsOverride
	}
	quiet := globalQuiet
	if target.PendingCIQuietPeriodOverride != nil {
		quiet = *target.PendingCIQuietPeriodOverride
	}
	if repository.PendingCIQuietPeriodOverride != nil {
		quiet = *repository.PendingCIQuietPeriodOverride
	}

	return mode, patterns, quiet
}

type PendingCIEffectiveMode string

const (
	PendingCIEffectiveNone   PendingCIEffectiveMode = "none"
	PendingCIEffectiveLabels PendingCIEffectiveMode = "labels"
	PendingCIEffectiveChecks PendingCIEffectiveMode = "checks"
)

type PendingCIReadiness string

const (
	PendingCIReady        PendingCIReadiness = "ready"
	PendingCIProvisioning PendingCIReadiness = "provisioning"
	PendingCIDraining     PendingCIReadiness = "draining"
	PendingCIBlocked      PendingCIReadiness = "blocked"
)

type PendingCIRepositoryGate struct {
	RepositoryID       string
	TargetID           string
	DesiredMode        PendingCIMode
	EffectiveMode      PendingCIEffectiveMode
	Readiness          PendingCIReadiness
	Reason             string
	AppID              *int64
	RulesetID          *int64
	RulesetFingerprint string
	Generation         int64
	ObservedAt         *time.Time
	UpdatedAt          time.Time
	Revision           int64
}

type PendingCIGateChange struct {
	RepositoryID       string
	ExpectedRevision   int64
	EffectiveMode      PendingCIEffectiveMode
	Readiness          PendingCIReadiness
	Reason             string
	AppID              *int64
	RulesetID          *int64
	RulesetFingerprint string
	ObservedAt         time.Time
}
