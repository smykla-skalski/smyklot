package main

import (
	"context"

	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

// repositoryConfigAdapter answers the one question about a repository the
// pending CI runtime cannot answer for itself.
//
// An adapter rather than renaming the server's own method: Config is the
// runtime's vocabulary and would read as nothing in particular on a struct this
// size. This file is also where somebody looks to see what the runtime is
// allowed to ask the service for, which a method buried among the other
// seventy-odd would not be.
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
