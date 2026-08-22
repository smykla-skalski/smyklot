package main

import (
	"context"

	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

type repositoryConfigAdapter struct{ server *server }

func (a repositoryConfigAdapter) Config(
	ctx context.Context,
	client *github.Client,
	targetID, repositoryID, owner, repository string,
) (*config.Config, error) {
	return a.server.serviceConfigWithoutCatalogRefresh(
		ctx, client, targetID, repositoryID, owner, repository,
	)
}
