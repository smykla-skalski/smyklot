package pendingci

import (
	"fmt"
	"strings"
	"time"
)

// Decide applies pending-CI timing and safety rules to a live observation.
func Decide(request Request, observation Observation, timing Timing) (Decision, error) {
	if err := validatePolicyInput(request, observation, timing); err != nil {
		return Decision{}, err
	}
	if observation.CancelReason != "" {
		return finishDecision(observation.CancelReason), nil
	}
	if !observation.PullRequestOpen {
		if observation.PullRequestMerged {
			return Decision{
				Kind: DecisionFinish, Lifecycle: LifecycleMerged,
				Reason: "pull request merged outside pending CI reconciliation",
			}, nil
		}
		return finishDecision("pull request is no longer open"), nil
	}
	if observation.HeadSHA != request.HeadSHA {
		return finishDecision("pull request head changed after command authorization"), nil
	}
	if observation.BaseBranch != request.BaseBranch {
		return finishDecision("pull request base changed after command authorization"), nil
	}
	if !observation.PendingLabelFound {
		return finishDecision("pending CI label was removed"), nil
	}

	progressAt, progressed := observedProgress(request, observation)
	if observation.State == ObservedPassing {
		return passingDecision(request, observation, timing, progressAt, progressed), nil
	}

	nextCheck := observation.ObservedAt.Add(timing.ActiveInterval)
	if observation.State == ObservedNoChecks && (progressed || request.LastObservedState == "") {
		nextCheck = observation.ObservedAt.Add(timing.DiscoveryGrace)
	}
	schedule := ScheduleActive
	if observation.ObservedAt.Sub(progressAt) >= timing.DeferAfter {
		schedule = ScheduleDeferred
		nextCheck = observation.ObservedAt.Add(timing.DeferredInterval)
	}

	return rescheduleDecision(observation, schedule, nextCheck, progressAt), nil
}

func passingDecision(
	request Request,
	observation Observation,
	timing Timing,
	progressAt time.Time,
	progressed bool,
) Decision {
	if !progressed && observation.ObservedAt.Sub(request.LastProgressAt) >= timing.PassingQuiet {
		return Decision{Kind: DecisionMerge, HeadSHA: observation.HeadSHA}
	}

	return rescheduleDecision(
		observation,
		ScheduleActive,
		progressAt.Add(timing.PassingQuiet),
		progressAt,
	)
}

func observedProgress(request Request, observation Observation) (time.Time, bool) {
	progressed := request.LastObservedState != string(observation.State) ||
		request.LastFingerprint != observation.Fingerprint
	if progressed {
		return observation.ObservedAt, true
	}

	return request.LastProgressAt, false
}

func rescheduleDecision(
	observation Observation,
	schedule Schedule,
	nextCheck, progressAt time.Time,
) Decision {
	return Decision{
		Kind: DecisionReschedule, Schedule: schedule, HeadSHA: observation.HeadSHA,
		NextCheckAt: nextCheck, LastProgressAt: progressAt,
		LastObservedState: string(observation.State), LastFingerprint: observation.Fingerprint,
	}
}

func finishDecision(reason string) Decision {
	return Decision{Kind: DecisionFinish, Lifecycle: LifecycleCancelled, Reason: reason}
}

func validatePolicyInput(request Request, observation Observation, timing Timing) error {
	if request.Lifecycle != LifecycleArmed || request.LastProgressAt.IsZero() {
		return invalid("policy requires an armed request with progress time")
	}
	if strings.TrimSpace(observation.HeadSHA) == "" ||
		strings.TrimSpace(observation.BaseBranch) == "" ||
		observation.ObservedAt.IsZero() {
		return invalid("observation head, base branch, and time are required")
	}
	if !observation.State.valid() {
		return invalid("unsupported observed state %q", observation.State)
	}
	if timing.ActiveInterval <= 0 || timing.DiscoveryGrace <= 0 || timing.DeferAfter <= 0 ||
		timing.DeferredInterval <= 0 || timing.PassingQuiet <= 0 {
		return fmt.Errorf("%w: policy timing values must be positive", ErrInvalidRequest)
	}

	return nil
}

func (state ObservedState) valid() bool {
	return state == ObservedPassing || state == ObservedPending || state == ObservedFailing ||
		state == ObservedNoChecks || state == ObservedIndeterminate
}
