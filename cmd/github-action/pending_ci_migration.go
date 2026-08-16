package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
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
	for _, pr := range prs {
		candidates := pendingCILabels(pr)
		if len(candidates) == 0 {
			continue
		}
		pullRequest := extractPRNumber(pr)
		if pullRequest == 0 {
			return fmt.Errorf("drain legacy pending CI label: invalid pull request number")
		}
		headSHA, baseBranch, err := pendingCIMigrationRefs(pr)
		if err != nil {
			return err
		}
		labels := make([]pendingci.LegacyPendingCILabel, 0, len(candidates))
		for _, candidate := range candidates {
			labels = append(labels, pendingci.LegacyPendingCILabel{
				MergeMethod:        pendingci.MergeMethod(candidate.method),
				RequiredChecksOnly: candidate.requiredOnly,
				Label:              candidate.label,
			})
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
					BaseBranch: baseBranch, Labels: labels, DrainedAt: time.Now().UTC(),
				})

				return drainErr
			},
		)
		if err != nil {
			return fmt.Errorf("persist legacy pending CI drain: %w", err)
		}
		if len(result.Requests) == 0 {
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

// migrateLegacyPendingCIServiceLabels replaces the old ownership label on an
// armed request with the service handoff reaction. Terminal cleanup retains
// the label until its method label is gone; orphaned labels are simply removed.
func (s *server) migrateLegacyPendingCIServiceLabels(
	ctx context.Context,
	client *github.Client,
	repository github.Repository,
	prs []map[string]interface{},
) error {
	repositoryID := repositoryStorageID(repository.ID)
	var cleanupErr error
	for _, pr := range prs {
		if !pullRequestHasLabel(pr, github.LegacyLabelPendingCIServiceOwner) {
			continue
		}
		pullRequest := extractPRNumber(pr)
		if pullRequest == 0 {
			cleanupErr = errors.Join(
				cleanupErr,
				fmt.Errorf("remove legacy pending CI service label: invalid pull request number"),
			)

			continue
		}
		err := s.pendingCICoordinator.Exclusive(ctx, repositoryID, func() error {
			request, armedErr := s.store.GetArmed(ctx, repositoryID, pullRequest)
			if armedErr == nil {
				if request.SourceCommentID > 0 {
					if err := client.AddReaction(
						ctx, repository.Owner, repository.Name,
						int(request.SourceCommentID), github.ReactionPendingCIService,
					); err != nil {
						return fmt.Errorf("migrate pending CI service reaction: %w", err)
					}
				}
			} else if !errors.Is(armedErr, storage.ErrNotFound) {
				return fmt.Errorf("read pending CI service request: %w", armedErr)
			} else {
				pending, pendingErr := s.store.HasPendingCleanup(
					ctx, pendingci.CleanupFilter{
						RepositoryID: repositoryID, PullRequest: pullRequest,
						ArtifactsPendingOnly: true,
					},
				)
				if pendingErr != nil {
					return fmt.Errorf("read pending CI cleanup: %w", pendingErr)
				}
				if pending {
					return nil
				}
			}

			return cleanupGitHubError(
				"remove legacy pending CI service label",
				client.RemoveLabel(
					ctx, repository.Owner, repository.Name, pullRequest,
					github.LegacyLabelPendingCIServiceOwner,
				),
			)
		})
		if err != nil {
			cleanupErr = errors.Join(cleanupErr, err)
		}
	}

	return cleanupErr
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
