package gate

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/logging"
)

const (
	legacyPendingCIDrainComment = "⚠️ This pending CI request was cancelled during the Smyklot service upgrade because its authorized commit could not be recovered. Please reissue the after-CI command."
	orphanPendingCIComment      = "⚠️ This pending CI request was cancelled because Smyklot could not recover its authorized command after a service restart. Please reissue the after-CI command."
)

// DrainLegacyLabels turns pre-durable labels into terminal cleanup
// records. It never guesses an authorized head and never imports a PR that has
// any durable request history.
func (g *Gate) DrainLegacyLabels(
	ctx context.Context,
	client *github.Client,
	targetID string,
	installationID int64,
	repository github.Repository,
	prs []map[string]interface{},
	skip map[int]struct{},
) error {
	for _, pr := range prs {
		candidates := bot.PendingCILabels(pr)
		if len(candidates) == 0 {
			continue
		}
		pullRequest := bot.ExtractPRNumber(pr)
		if pullRequest == 0 {
			return fmt.Errorf("drain legacy pending CI label: invalid pull request number")
		}
		if _, cleaned := skip[pullRequest]; cleaned {
			continue
		}
		headSHA, baseBranch, err := migrationRefs(pr)
		if err != nil {
			return err
		}
		labels := make([]pendingci.LegacyPendingCILabel, 0, len(candidates))
		for _, candidate := range candidates {
			labels = append(labels, pendingci.LegacyPendingCILabel{
				MergeMethod:        pendingci.MergeMethod(candidate.Method),
				RequiredChecksOnly: candidate.RequiredOnly,
				Label:              candidate.Label,
			})
		}

		var result pendingci.LegacyDrainResult
		err = g.coordinator.Exclusive(
			ctx, storage.RepositoryID(repository.ID), func() error {
				var drainErr error
				result, drainErr = g.store.DrainLegacy(ctx, pendingci.LegacyDrainRequest{
					TargetID: targetID, InstallationID: installationID,
					RepositoryID:       storage.RepositoryID(repository.ID),
					RepositoryFullName: bot.RepoFullName(repository.Owner, repository.Name),
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

		g.Wake()
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

// ReconcileServiceArtifacts repairs durable ownership fences and
// cancels artifacts that have no durable authorization. The returned pull
// requests must not be re-imported from the stale GitHub list in this sweep.
func (g *Gate) ReconcileServiceArtifacts(
	ctx context.Context,
	client *github.Client,
	repository github.Repository,
	prs []map[string]interface{},
	inspectAll bool,
) (map[int]struct{}, error) {
	repositoryID := storage.RepositoryID(repository.ID)
	cleaned := make(map[int]struct{})
	var cleanupErr error
	for _, pr := range prs {
		legacy := bot.PullRequestHasLabel(pr, bot.LegacyLabelPendingCIServiceOwner)
		labels := methodLabels(pr)
		if !inspectAll && !legacy && len(labels) == 0 {
			continue
		}
		pullRequest := bot.ExtractPRNumber(pr)
		if pullRequest == 0 {
			cleanupErr = errors.Join(cleanupErr, errors.New(
				"reconcile pending CI service artifacts: invalid pull request number",
			))

			continue
		}
		knownReaction, err := knownServiceReaction(
			ctx, client, repository, pullRequest, g.botUsername,
			inspectAll && !legacy && len(labels) == 0,
		)
		if err != nil {
			cleanupErr = errors.Join(cleanupErr, err)

			continue
		}
		if inspectAll && !legacy && len(labels) == 0 && !knownReaction {
			continue
		}
		wasCleaned, err := g.reconcilePendingCIServiceArtifact(
			ctx, client, repository, repositoryID, pullRequest,
			labels, legacy, knownReaction,
		)
		if err != nil {
			cleanupErr = errors.Join(cleanupErr, err)

			continue
		}
		if wasCleaned {
			cleaned[pullRequest] = struct{}{}
			g.explainOrphanPendingCICleanup(
				ctx, client, repository, pullRequest, len(labels) > 0,
			)
		}
	}

	return cleaned, cleanupErr
}

func methodLabels(pr map[string]interface{}) []string {
	candidates := bot.PendingCILabels(pr)
	labels := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		labels = append(labels, candidate.Label)
	}

	return labels
}

func knownServiceReaction(
	ctx context.Context,
	client *github.Client,
	repository github.Repository,
	pullRequest int,
	botUsername string,
	inspect bool,
) (bool, error) {
	if !inspect {
		return false, nil
	}
	found, err := client.HasPullRequestReaction(
		ctx, repository.Owner, repository.Name, pullRequest,
		botUsername, bot.ReactionPendingCIService,
	)
	if err != nil {
		return false, fmt.Errorf("inspect pending CI service reaction: %w", err)
	}

	return found, nil
}

func (g *Gate) reconcilePendingCIServiceArtifact(
	ctx context.Context,
	client *github.Client,
	repository github.Repository,
	repositoryID string,
	pullRequest int,
	labels []string,
	legacy bool,
	knownReaction bool,
) (bool, error) {
	cleaned := false
	err := g.coordinator.Exclusive(ctx, repositoryID, func() error {
		var err error
		cleaned, err = g.reconcilePendingCIServiceArtifactLocked(
			ctx, client, repository, repositoryID, pullRequest,
			labels, legacy, knownReaction,
		)

		return err
	})

	return cleaned, err
}

func (g *Gate) reconcilePendingCIServiceArtifactLocked(
	ctx context.Context,
	client *github.Client,
	repository github.Repository,
	repositoryID string,
	pullRequest int,
	labels []string,
	legacy bool,
	knownReaction bool,
) (bool, error) {
	request, err := g.store.GetArmed(ctx, repositoryID, pullRequest)
	if err == nil {
		return false, migrateArmedPendingCIServiceArtifact(
			ctx, client, repository, request, legacy,
		)
	}
	if !errors.Is(err, storage.ErrNotFound) {
		return false, fmt.Errorf("read pending CI service request: %w", err)
	}
	pending, err := g.store.HasPendingCleanup(ctx, pendingci.CleanupFilter{
		RepositoryID: repositoryID, PullRequest: pullRequest,
		ArtifactsPendingOnly: true,
	})
	if err != nil {
		return false, fmt.Errorf("read pending CI cleanup: %w", err)
	}
	if pending {
		return false, nil
	}
	serviceOwned := legacy || knownReaction
	if !serviceOwned {
		serviceOwned, err = client.HasPullRequestReaction(
			ctx, repository.Owner, repository.Name, pullRequest,
			g.botUsername, bot.ReactionPendingCIService,
		)
		if err != nil {
			return false, fmt.Errorf("read pending CI service reaction: %w", err)
		}
	}
	if !serviceOwned {
		return false, nil
	}
	if err := cleanupOrphanPendingCIServiceArtifacts(
		ctx, client, repository, pullRequest, labels, legacy, g.botUsername,
	); err != nil {
		return false, err
	}

	return true, nil
}

func migrateArmedPendingCIServiceArtifact(
	ctx context.Context,
	client *github.Client,
	repository github.Repository,
	request pendingci.Request,
	legacy bool,
) error {
	if !legacy {
		return nil
	}
	if err := client.AddPullRequestReaction(
		ctx, repository.Owner, repository.Name,
		request.PullRequest, bot.ReactionPendingCIService,
	); err != nil {
		return fmt.Errorf("migrate pending CI service fence: %w", err)
	}

	return bot.CleanupGitHubError(
		"remove legacy pending CI service label",
		client.RemoveLabel(
			ctx, repository.Owner, repository.Name, request.PullRequest,
			bot.LegacyLabelPendingCIServiceOwner,
		),
	)
}

func cleanupOrphanPendingCIServiceArtifacts(
	ctx context.Context,
	client *github.Client,
	repository github.Repository,
	pullRequest int,
	labels []string,
	legacy bool,
	botUsername string,
) error {
	for _, label := range labels {
		if err := bot.CleanupGitHubError(
			"remove orphan pending CI method label",
			client.RemoveLabel(ctx, repository.Owner, repository.Name, pullRequest, label),
		); err != nil {
			return err
		}
	}
	if err := client.RemovePullRequestReactionByUser(
		ctx, repository.Owner, repository.Name, pullRequest,
		botUsername, bot.ReactionPendingCIService,
	); err != nil {
		return fmt.Errorf("remove orphan pending CI service fence: %w", err)
	}
	if err := client.RemovePullRequestCommentReactionsByUser(
		ctx, repository.Owner, repository.Name, pullRequest,
		botUsername, bot.ReactionPendingCIService,
	); err != nil {
		return fmt.Errorf("remove legacy pending CI service reaction: %w", err)
	}
	if !legacy {
		return nil
	}

	return bot.CleanupGitHubError(
		"remove legacy pending CI service label",
		client.RemoveLabel(
			ctx, repository.Owner, repository.Name, pullRequest,
			bot.LegacyLabelPendingCIServiceOwner,
		),
	)
}

func (g *Gate) explainOrphanPendingCICleanup(
	ctx context.Context,
	client *github.Client,
	repository github.Repository,
	pullRequest int,
	hadMethodLabel bool,
) {
	if !hadMethodLabel {
		return
	}
	if err := client.PostComment(
		ctx, repository.Owner, repository.Name, pullRequest, orphanPendingCIComment,
	); err != nil {
		logging.From(ctx).Warn(
			"failed to explain orphan pending CI cleanup",
			"pr", strconv.Itoa(pullRequest), "error", err,
		)
	}
}

func migrationRefs(pr map[string]interface{}) (string, string, error) {
	head, _ := pr["head"].(map[string]interface{})
	base, _ := pr["base"].(map[string]interface{})
	headSHA, _ := head["sha"].(string)
	baseBranch, _ := base["ref"].(string)
	if headSHA == "" || baseBranch == "" {
		return "", "", fmt.Errorf("drain legacy pending CI label: pull request refs are missing")
	}

	return headSHA, baseBranch, nil
}
