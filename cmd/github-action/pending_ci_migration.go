package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/logging"
)

const legacyPendingCIDrainComment = "⚠️ This pending CI request was cancelled during the Smyklot service upgrade because its authorized commit could not be recovered. Please reissue the after-CI command."

// drainLegacyPendingCILabels turns pre-durable labels into terminal cleanup
// records. It never guesses an authorized head and never imports a PR that has
// any durable request history.
func (s *server) drainLegacyPendingCILabels(
	ctx context.Context,
	client *github.Client,
	targetID string,
	installationID int64,
	repository github.Repository,
	prs []map[string]interface{},
) error {
	for _, candidate := range filterPendingCIPRs(prs) {
		pullRequest := extractPRNumber(candidate.prData)
		if pullRequest == 0 {
			return fmt.Errorf("drain legacy pending CI label: invalid pull request number")
		}
		headSHA, baseBranch, err := pendingCIMigrationRefs(candidate.prData)
		if err != nil {
			return err
		}

		var result pendingci.LegacyDrainResult
		err = s.pendingCICoordinator.Exclusive(
			ctx, repositoryStorageID(repository.ID), func() error {
				var drainErr error
				result, drainErr = s.store.DrainLegacy(ctx, pendingci.LegacyDrainRequest{
					TargetID: targetID, InstallationID: installationID,
					RepositoryID:       repositoryStorageID(repository.ID),
					RepositoryFullName: repoFullName(repository.Owner, repository.Name),
					PullRequest:        pullRequest, HeadSHA: headSHA,
					BaseBranch: baseBranch, MergeMethod: pendingci.MergeMethod(candidate.method),
					RequiredChecksOnly: candidate.requiredOnly, Label: candidate.label,
					DrainedAt: time.Now().UTC(),
				})

				return drainErr
			},
		)
		if err != nil {
			return fmt.Errorf("persist legacy pending CI drain: %w", err)
		}
		if result.Request == nil {
			continue
		}

		s.pendingCI.Wake()
		if err := client.PostComment(
			ctx, repository.Owner, repository.Name, pullRequest, legacyPendingCIDrainComment,
		); err != nil {
			logging.From(ctx).Warn(
				"failed to explain legacy pending CI drain",
				"pr", strconv.Itoa(pullRequest), "error", err,
			)
		}
	}

	return nil
}

func pendingCIMigrationRefs(pr map[string]interface{}) (string, string, error) {
	head, _ := pr["head"].(map[string]interface{})
	base, _ := pr["base"].(map[string]interface{})
	headSHA, _ := head["sha"].(string)
	baseBranch, _ := base["ref"].(string)
	if headSHA == "" || baseBranch == "" {
		return "", "", fmt.Errorf("drain legacy pending CI label: pull request refs are missing")
	}

	return headSHA, baseBranch, nil
}
