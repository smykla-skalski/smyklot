package orgsync

import (
	"fmt"
	"strings"
)

// Validate checks consistency before a snapshot becomes immutable.
func (result CheckResult) Validate() error {
	outcome := result.Outcome
	if outcome.CompletedAt.IsZero() || strings.TrimSpace(outcome.Summary) == "" || outcome.Cached < 0 {
		return fmt.Errorf("check outcome requires a completion time, summary and nonnegative counts")
	}
	switch outcome.Disposition {
	case "checked", "disabled", "unpermitted", "deferred":
	default:
		return fmt.Errorf("unknown check disposition %q", outcome.Disposition)
	}
	if outcome.BlockingPlanID != "" && (outcome.Disposition != "deferred" || strings.TrimSpace(outcome.BlockingPlanID) != outcome.BlockingPlanID) {
		return fmt.Errorf("only deferred checks can name a blocking plan with a nonblank identity")
	}
	counts := map[Observation]int{}
	cached := 0
	seen := map[string]bool{}
	for _, observation := range result.Observations {
		key := observation.RepositoryID + ":" + string(observation.Kind)
		if observation.RepositoryID == "" || seen[key] {
			return fmt.Errorf("check evidence requires distinct repository and kind identities")
		}
		seen[key] = true
		if observation.Cached {
			cached++
		} else {
			counts[observation.Outcome]++
		}
	}
	if cached != outcome.Cached {
		return fmt.Errorf("cached check count differs from its evidence")
	}
	for state, count := range outcome.Counts {
		if count < 0 || counts[state] != count {
			return fmt.Errorf("check outcome count differs from its evidence")
		}
		delete(counts, state)
	}
	if len(counts) > 0 {
		return fmt.Errorf("check outcome omits observed evidence")
	}
	return nil
}
