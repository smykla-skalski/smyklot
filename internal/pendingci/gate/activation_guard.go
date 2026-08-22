package gate

import (
	"context"
	"errors"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

// ActivationGuard revalidates the repository's runner only
// after activation owns its repository coordinator. This fences deliveries
// that read service ownership before a completed handoff to the Action.
type ActivationGuard struct {
	config       RepositoryConfig
	store        Store
	panelled     bool
	client       *github.Client
	targetID     string
	repositoryID string
	owner        string
	repository   string
}

func (guard ActivationGuard) AllowsActivation(
	ctx context.Context,
	expected pendingci.ArtifactKind,
	baseBranch string,
	requiredChecksOnly bool,
) (bool, error) {
	eligible, err := guard.repositoryAllowsActivation(ctx, baseBranch)
	if err != nil || !eligible {
		return false, err
	}
	botConfig, err := guard.config(
		ctx, guard.client, guard.targetID, guard.repositoryID, guard.owner, guard.repository,
	)
	if err != nil {
		return false, fmt.Errorf("read repository configuration: %w", err)
	}

	if bot.ServiceStandsDown(ctx, botConfig) {
		return false, nil
	}
	mode, err := guard.PendingCIMode(ctx, baseBranch)
	if err != nil {
		return false, err
	}
	actual := pendingci.ArtifactLabel
	if mode == storage.PendingCIModeChecks {
		actual = pendingci.ArtifactCheck
	}
	if expected != actual {
		return false, fmt.Errorf("merge-after-CI mode changed while the command was being authorized")
	}
	if !requiredChecksOnly {
		return true, nil
	}

	return guard.requiredCIAllowsActivation(ctx, baseBranch, actual)
}

func (guard ActivationGuard) repositoryAllowsActivation(
	ctx context.Context,
	baseBranch string,
) (bool, error) {
	if !guard.panelled {
		return true, nil
	}
	target, repository, err := readControls(
		ctx, guard.store, guard.targetID, guard.repositoryID,
	)
	if errors.Is(err, storage.ErrNotFound) ||
		(err == nil && (!target.Available || !repository.Available)) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read repository controls: %w", err)
	}
	if !storage.RepositoryEnabled(target, repository) {
		return false, nil
	}
	_, patterns, _ := storage.EffectivePendingCISettings(target, repository, 0)
	if !branchIncluded(baseBranch, repository.DefaultBranch, patterns) {
		return false, nil
	}

	return true, nil
}

func (guard ActivationGuard) requiredCIAllowsActivation(
	ctx context.Context,
	baseBranch string,
	artifact pendingci.ArtifactKind,
) (bool, error) {
	requirements, err := guard.client.GetRequiredCIRequirements(
		ctx, guard.owner, guard.repository, baseBranch,
	)
	if err != nil {
		return false, fmt.Errorf("read required-CI policy: %w", err)
	}
	if requirements.RequiredWorkflow {
		return false, bot.ErrRequiredWorkflowsUnsupported
	}
	required := requirements.StatusChecks
	if artifact == pendingci.ArtifactCheck {
		gate, err := guard.store.GetPendingCIRepositoryGate(ctx, guard.repositoryID)
		if err != nil {
			return false, fmt.Errorf("read pending CI check identity: %w", err)
		}
		if gate.AppID == nil {
			return false, errors.New("merge-after-CI check identity is unavailable")
		}
		required = externalRequiredChecks(
			required,
			storage.PendingCICheckName,
			*gate.AppID,
		)
	}
	if len(required) == 0 {
		return false, errNoRequiredStatusChecks
	}

	return true, nil
}

func (guard ActivationGuard) PendingCIMode(
	ctx context.Context,
	baseBranch string,
) (storage.PendingCIMode, error) {
	// Merge-after-CI mode is installation policy owned by the panel. Keep
	// panel-less service deployments on the legacy label contract because they
	// have nowhere to provision or report check/ruleset readiness.
	if !guard.panelled {
		return storage.PendingCIModeLabels, nil
	}

	gate, err := guard.store.GetPendingCIRepositoryGate(ctx, guard.repositoryID)
	if err != nil {
		return "", fmt.Errorf("read pending CI readiness: %w", err)
	}
	if gate.Readiness != storage.PendingCIReady ||
		string(gate.EffectiveMode) != string(gate.DesiredMode) {
		return "", fmt.Errorf("merge-after-CI mode is not ready: %s", gate.Reason)
	}
	switch gate.EffectiveMode {
	case storage.PendingCIEffectiveLabels:
		return guard.pendingCILabelMode(ctx, baseBranch)
	case storage.PendingCIEffectiveChecks:
		return guard.pendingCICheckMode(ctx, baseBranch, gate.AppID)
	default:
		return "", fmt.Errorf("merge-after-CI mode is inactive: %s", gate.Reason)
	}
}

func (guard ActivationGuard) pendingCILabelMode(
	ctx context.Context,
	baseBranch string,
) (storage.PendingCIMode, error) {
	required, err := guard.client.GetRequiredStatusChecks(
		ctx, guard.owner, guard.repository, baseBranch,
	)
	if err != nil {
		return "", fmt.Errorf("read merge-after-CI base protection: %w", err)
	}
	for _, check := range required {
		if check.Context == storage.PendingCICheckName {
			return "", fmt.Errorf(
				"merge-after-CI label mode cannot satisfy the required Smyklot check on base branch %s",
				baseBranch,
			)
		}
	}

	return storage.PendingCIModeLabels, nil
}

func (guard ActivationGuard) pendingCICheckMode(
	ctx context.Context,
	baseBranch string,
	appID *int64,
) (storage.PendingCIMode, error) {
	if appID == nil {
		return "", fmt.Errorf("merge-after-CI check identity is unavailable")
	}
	mergeQueue, err := guard.client.IsMergeQueueEnabled(
		ctx, guard.owner, guard.repository, baseBranch,
	)
	if err != nil {
		return "", fmt.Errorf("read merge queue policy: %w", err)
	}
	if mergeQueue {
		return "", fmt.Errorf(
			"merge-after-CI checks do not support the merge queue on base branch %s",
			baseBranch,
		)
	}
	required, err := guard.client.GetRequiredStatusChecks(
		ctx, guard.owner, guard.repository, baseBranch,
	)
	if err != nil {
		return "", fmt.Errorf("read merge-after-CI base protection: %w", err)
	}
	if !requiredContextOwned(required, storage.PendingCICheckName, *appID) {
		return "", fmt.Errorf(
			"merge-after-CI checks do not protect base branch %s",
			baseBranch,
		)
	}

	return storage.PendingCIModeChecks, nil
}

func requiredContextOwned(
	required []github.RequiredCheck,
	name string,
	appID int64,
) bool {
	found := false
	for _, check := range required {
		if check.Context != name {
			continue
		}
		if check.AppID == nil || *check.AppID != appID {
			return false
		}
		found = true
	}

	return found
}

func externalRequiredChecks(
	required []github.RequiredCheck,
	name string,
	appID int64,
) []github.RequiredCheck {
	external := make([]github.RequiredCheck, 0, len(required))
	for _, check := range required {
		if check.Context == name && check.AppID != nil && *check.AppID == appID {
			continue
		}
		external = append(external, check)
	}

	return external
}
