package main

import (
	"context"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

type pendingCIActivationGuard interface {
	AllowsActivation(context.Context) (bool, error)
}

type pendingCIModeResolver interface {
	PendingCIMode(context.Context) (storage.PendingCIMode, error)
}

// githubPendingCIActivationGuard revalidates the repository's runner only
// after activation owns its repository coordinator. This fences deliveries
// that read service ownership before a completed handoff to the Action.
type githubPendingCIActivationGuard struct {
	server       *server
	client       *github.Client
	targetID     string
	repositoryID string
	owner        string
	repository   string
}

func (guard githubPendingCIActivationGuard) AllowsActivation(
	ctx context.Context,
) (bool, error) {
	botConfig, err := guard.server.serviceConfig(
		ctx, guard.client, guard.targetID, guard.repositoryID, guard.owner, guard.repository,
	)
	if err != nil {
		return false, fmt.Errorf("read repository configuration: %w", err)
	}

	if serviceStandsDown(ctx, botConfig) {
		return false, nil
	}
	_, err = guard.PendingCIMode(ctx)
	if err != nil {
		return false, err
	}

	return true, nil
}

func (guard githubPendingCIActivationGuard) PendingCIMode(
	ctx context.Context,
) (storage.PendingCIMode, error) {
	// Merge-after-CI mode is installation policy owned by the panel. Keep
	// panel-less service deployments on the legacy label contract because they
	// have nowhere to provision or report check/ruleset readiness.
	if guard.server.panel == nil {
		return storage.PendingCIModeLabels, nil
	}

	gate, err := guard.server.store.GetPendingCIRepositoryGate(ctx, guard.repositoryID)
	if err != nil {
		return "", fmt.Errorf("read pending CI readiness: %w", err)
	}
	if gate.Readiness != storage.PendingCIReady ||
		string(gate.EffectiveMode) != string(gate.DesiredMode) {
		return "", fmt.Errorf("merge-after-CI mode is not ready: %s", gate.Reason)
	}
	switch gate.EffectiveMode {
	case storage.PendingCIEffectiveLabels:
		return storage.PendingCIModeLabels, nil
	case storage.PendingCIEffectiveChecks:
		return storage.PendingCIModeChecks, nil
	default:
		return "", fmt.Errorf("merge-after-CI mode is inactive: %s", gate.Reason)
	}
}
