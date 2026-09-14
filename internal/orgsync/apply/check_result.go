package apply

import (
	"context"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

func checkObservation(repository storage.Repository, state orgsync.RepositoryState, cached bool) orgsync.CheckObservation {
	return orgsync.CheckObservation{
		RepositoryID: repository.ID, Repository: repository.FullName, Kind: state.Kind,
		Outcome: state.Observation, ObservedAt: state.AppliedAt, InputDigest: state.ObservedDigest,
		Reason: state.Problem, ProposalURL: state.ProposalURL, Cached: cached,
	}
}

func (result syncScanResult) checkResult(disposition, summary string, missing []orgsync.Kind) orgsync.CheckResult {
	counts := make(map[orgsync.Observation]int)
	for _, observed := range result.observations {
		counts[observed.Observation]++
	}
	return orgsync.CheckResult{
		Outcome: orgsync.CheckOutcome{
			CompletedAt: time.Now().UTC(), Disposition: disposition,
			Summary: summary, Counts: counts, Cached: result.cached, MissingPermissions: missing,
		},
		Observations: result.evidence,
	}
}

func (s *Engine) retainCheck(ctx context.Context, targetID string, check *orgsync.CheckReference, result orgsync.CheckResult) (string, error) {
	if check != nil {
		if err := s.store.RecordSyncCheckResult(ctx, orgsync.CheckResultCreate{Check: *check, TargetID: targetID, Result: result, Now: time.Now().UTC()}); err != nil {
			return "", fmt.Errorf("retain sync check outcome: %w", err)
		}
	}
	return result.Outcome.Summary, nil
}

func missingCheckPermissions(enabled, active []orgsync.Config) []orgsync.Kind {
	missing := []orgsync.Kind{}
	permitted := make(map[orgsync.Kind]bool)
	for _, config := range active {
		permitted[config.Kind] = true
	}
	for _, config := range enabled {
		if !permitted[config.Kind] {
			missing = append(missing, config.Kind)
		}
	}
	return missing
}

func inactiveCheckDisposition(enabled []orgsync.Config) string {
	if len(enabled) == 0 {
		return "disabled"
	}
	return "unpermitted"
}

func (result syncScanResult) deferredCheckResult(planID, summary string, missing []orgsync.Kind) orgsync.CheckResult {
	check := result.checkResult("deferred", summary, missing)
	check.Outcome.BlockingPlanID = planID
	return check
}
