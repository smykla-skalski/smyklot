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
		// Service-owned labels are either backed by an armed request or cleaned
		// by reconcilePendingCIServiceOwnership. They are never legacy input.
		if pullRequestHasLabel(pr, github.LabelPendingCIServiceOwner) {
			continue
		}
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

// reconcilePendingCIServiceOwnership repairs the narrow crash window between
// publishing service-owned artifacts and committing the durable command. An
// armed request keeps its marker. Everything else is already terminal or was
// never committed, so method labels are removed before the marker is released
// to prevent the Action runner from adopting stale work.
func (s *server) reconcilePendingCIServiceOwnership(
	ctx context.Context,
	client *github.Client,
	repository github.Repository,
	prs []map[string]interface{},
) error {
	repositoryID := repositoryStorageID(repository.ID)
	var reconcileErr error
	for _, pr := range prs {
		if !pullRequestHasLabel(pr, github.LabelPendingCIServiceOwner) {
			continue
		}
		pullRequest := extractPRNumber(pr)
		if pullRequest == 0 {
			reconcileErr = errors.Join(
				reconcileErr,
				fmt.Errorf("reconcile pending CI service ownership: invalid pull request number"),
			)

			continue
		}
		err := s.pendingCICoordinator.Exclusive(ctx, repositoryID, func() error {
			_, armedErr := s.store.GetArmed(ctx, repositoryID, pullRequest)
			if armedErr == nil {
				return nil
			}
			if !errors.Is(armedErr, storage.ErrNotFound) {
				return fmt.Errorf("read pending CI service owner: %w", armedErr)
			}
			for _, candidate := range pendingCILabels(pr) {
				if err := cleanupGitHubError(
					"remove orphaned pending CI label",
					client.RemoveLabel(
						ctx, repository.Owner, repository.Name, pullRequest, candidate.label,
					),
				); err != nil {
					return err
				}
			}

			return cleanupGitHubError(
				"remove orphaned pending CI service ownership marker",
				client.RemoveLabel(
					ctx, repository.Owner, repository.Name, pullRequest,
					github.LabelPendingCIServiceOwner,
				),
			)
		})
		if err != nil {
			reconcileErr = errors.Join(reconcileErr, err)
		}
	}

	return reconcileErr
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
