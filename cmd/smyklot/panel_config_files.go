package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/smykla-skalski/smyklot/internal/configsync"
	adminpanel "github.com/smykla-skalski/smyklot/internal/panel"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

var _ adminpanel.ConfigFileController = (*server)(nil)

func (s *server) PreviewConfigurationFile(ctx context.Context, targetID, repositoryID string) (configsync.ConnectionPreview, error) {
	var preview configsync.ConnectionPreview
	err := s.withConfigurationFileReview(ctx, targetID, repositoryID, func(engine configsync.Engine, client *github.Client) error {
		var err error
		preview, err = engine.Preview(ctx, client, targetID, repositoryID)
		return err
	})
	return preview, err
}

func (s *server) ResolveConfigurationFile(ctx context.Context, request adminpanel.ConfigFileResolutionRequest) error {
	return s.withConfigurationFileReview(ctx, request.TargetID, request.RepositoryID, func(engine configsync.Engine, client *github.Client) error {
		change, err := engine.PrepareResolution(ctx, client, request.TargetID, request.RepositoryID, request.ReviewToken, request.Side)
		if err != nil {
			return err
		}
		_, err = s.store.SaveConfigFileResolution(ctx, storage.ConfigFileResolution{
			State: change, Side: request.Side, ActorAccountID: request.ActorAccountID,
			ElevationID: request.ElevationID, SessionTokenHash: request.SessionTokenHash,
		})
		return err
	})
}

// Keep the fresh read and choice transaction under the same exclusion as the
// worker and panel settings saves. Storage still compares revisions and validates
// an elevation at the actual write, after the possibly slow GitHub observation.
func (s *server) withConfigurationFileReview(ctx context.Context, targetID, repositoryID string, operation func(configsync.Engine, *github.Client) error) error {
	return s.withConfigurationFileScope(ctx, targetID, repositoryID, func() error {
		engine := configsync.Engine{Store: s.store, QuietPeriod: s.cfg.pendingCIQuietPeriod}
		snapshot, err := engine.Snapshot(ctx, targetID, repositoryID)
		if err != nil {
			return err
		}
		enabled := snapshot.Target.Available && snapshot.Target.ConfigFileSyncEnabled
		if snapshot.Repository != nil {
			enabled = snapshot.Target.Available && snapshot.Repository.Available &&
				snapshot.Repository.ConfigFileSyncEnabled && !snapshot.Repository.IgnoreRepositoryFile
		}
		if !enabled {
			// Explain inactive connections without acquiring installation credentials.
			return operation(engine, nil)
		}
		installationID, err := strconv.ParseInt(snapshot.Target.InstallationID, 10, 64)
		if err != nil || installationID <= 0 {
			return fmt.Errorf("invalid configuration file installation id")
		}
		client, err := s.queuedInstallationClient(installationID)
		if err != nil {
			return err
		}
		return operation(engine, client)
	})
}
