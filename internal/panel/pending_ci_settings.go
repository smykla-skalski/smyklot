package panel

import (
	"context"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

// updateTargetSettings serializes a target-wide mode change with every
// repository's GitHub gate effects. Otherwise a reconcile can create a
// required-check ruleset after the settings transaction has switched back to
// labels, leaving an external ruleset with no durable ownership record.
func (s *Server) updateTargetSettings(
	ctx context.Context,
	change storage.TargetSettingsChange,
) (storage.Target, error) {
	var updated storage.Target
	err := s.pendingCI.ExclusiveCatalog(ctx, func() error {
		repositories, listErr := s.store.ListRepositories(ctx, change.TargetID)
		if listErr != nil {
			return fmt.Errorf(
				"list repositories for coordinated target settings: %w",
				listErr,
			)
		}
		repositoryIDs := make([]string, 0, len(repositories))
		for _, repository := range repositories {
			repositoryIDs = append(repositoryIDs, repository.ID)
		}

		return s.pendingCI.Exclusive(ctx, repositoryIDs, func() error {
			var updateErr error
			updated, updateErr = s.store.UpdateTargetSettings(ctx, change)

			return updateErr
		})
	})

	return updated, err
}

func (s *Server) updateRepositorySettings(
	ctx context.Context,
	change storage.RepositorySettingsChange,
) (storage.Repository, error) {
	var updated storage.Repository
	err := s.pendingCI.Exclusive(ctx, []string{change.RepositoryID}, func() error {
		var updateErr error
		updated, updateErr = s.store.UpdateRepositorySettings(ctx, change)

		return updateErr
	})

	return updated, err
}
