package main

import (
	"context"
	"fmt"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

type pendingCIActivationGuard interface {
	AllowsActivation(context.Context) (bool, error)
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

	return !serviceStandsDown(ctx, botConfig), nil
}
