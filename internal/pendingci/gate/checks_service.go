package gate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

const (
	ReauthorizeAction   = "reauthorize"
	checkRenewAfter     = 13 * 24 * time.Hour
	completedRenewAfter = 365 * 24 * time.Hour
	checkInProgress     = "in_progress"
	checkCompleted      = "completed"
	checkSuccess        = "success"
)

type tokens interface {
	AppToken() (string, error)
	InstallationToken(int64) (string, error)
}

type Checks struct {
	store      pendingci.CheckStore
	tokens     tokens
	apiBaseURL string
	now        func() time.Time
	syncer     bot.Exclusive

	appMu sync.Mutex
	appID int64
}

type checkDesired struct {
	Status     string
	Conclusion string
	Title      string
	Summary    string
	Actions    []pendingci.CheckAction
}

func (checks *Checks) AppID(ctx context.Context) (int64, error) {
	checks.appMu.Lock()
	defer checks.appMu.Unlock()
	if checks.appID > 0 {
		return checks.appID, nil
	}
	token, err := checks.tokens.AppToken()
	if err != nil {
		return 0, bot.NewGitHubError(bot.ErrGitHubAppAuth, err)
	}
	client, err := github.NewAppClient(token, checks.apiBaseURL)
	if err != nil {
		return 0, bot.NewGitHubError(bot.ErrGitHubClient, err)
	}
	checks.appID, err = client.AppID(ctx)
	if err != nil {
		return 0, fmt.Errorf("read GitHub App identity: %w", err)
	}

	return checks.appID, nil
}

func (checks *Checks) CheckSlot(
	ctx context.Context,
	id int64,
) (pendingci.CheckSlot, error) {
	return checks.store.GetCheckSlot(ctx, id)
}

func (checks *Checks) EnsureBaseline(
	ctx context.Context,
	target storage.Target,
	repository storage.Repository,
	pullRequest int,
	headSHA string,
) (pendingci.CheckSlot, error) {
	return checks.ensure(ctx, target, repository, pullRequest, headSHA, checkDesired{
		Status: checkCompleted, Conclusion: checkSuccess,
		Title:   "Merge after CI is ready",
		Summary: "No merge-after-CI request is currently authorized for this commit.",
	})
}

func (checks *Checks) EnsureAuthorized(
	ctx context.Context,
	target storage.Target,
	repository storage.Repository,
	pullRequest int,
	headSHA string,
	method pendingci.MergeMethod,
	requester string,
) (pendingci.CheckSlot, error) {
	return checks.ensure(ctx, target, repository, pullRequest, headSHA, checkDesired{
		Status: checkInProgress,
		Title:  "Waiting for CI",
		Summary: fmt.Sprintf(
			"%s authorized a %s merge when the other CI checks pass and remain stable.",
			requester,
			method,
		),
	})
}

func (checks *Checks) EnsureReauthorization(
	ctx context.Context,
	target storage.Target,
	repository storage.Repository,
	pullRequest int,
	headSHA string,
) (pendingci.CheckSlot, error) {
	return checks.ensure(ctx, target, repository, pullRequest, headSHA, checkDesired{
		Status: checkCompleted, Conclusion: "action_required",
		Title:   "Reauthorization required",
		Summary: "The pull request head or base changed. Reauthorize the merge-after-CI request for the current revision.",
		Actions: []pendingci.CheckAction{{
			Label: "Reauthorize", Description: "Authorize merge after CI for this commit",
			Identifier: ReauthorizeAction,
		}},
	})
}

func (checks *Checks) EnsureMergeReady(
	ctx context.Context,
	slot pendingci.CheckSlot,
) (pendingci.CheckSlot, error) {
	target := storage.Target{ID: slot.TargetID, InstallationID: fmt.Sprint(slot.InstallationID)}
	repository := storage.Repository{ID: slot.RepositoryID, FullName: slot.RepositoryFullName}

	return checks.ensure(ctx, target, repository, slot.PullRequest, slot.HeadSHA, checkDesired{
		Status: checkCompleted, Conclusion: checkSuccess,
		Title:   "CI passed",
		Summary: "The authorized commit passed CI and Smyklot is completing the exact-head merge.",
	})
}

func (checks *Checks) ensure(
	ctx context.Context,
	target storage.Target,
	repository storage.Repository,
	pullRequest int,
	headSHA string,
	desired checkDesired,
) (pendingci.CheckSlot, error) {
	if checks.syncer == nil {
		return pendingci.CheckSlot{}, errors.New("pending CI check coordinator is unavailable")
	}
	var slot pendingci.CheckSlot
	err := checks.syncer.Exclusive(
		ctx,
		repository.ID+":"+headSHA,
		func() error {
			var ensureErr error
			slot, ensureErr = checks.ensureExclusive(
				ctx,
				target,
				repository,
				pullRequest,
				headSHA,
				desired,
			)

			return ensureErr
		},
	)

	return slot, err
}

func (checks *Checks) ensureExclusive(
	ctx context.Context,
	target storage.Target,
	repository storage.Repository,
	pullRequest int,
	headSHA string,
	desired checkDesired,
) (pendingci.CheckSlot, error) {
	appID, err := checks.AppID(ctx)
	if err != nil {
		return pendingci.CheckSlot{}, err
	}
	installationID, err := parsePositiveInt64(target.InstallationID)
	if err != nil {
		return pendingci.CheckSlot{}, err
	}
	digest, err := checkDigest(desired)
	if err != nil {
		return pendingci.CheckSlot{}, err
	}
	request := pendingci.EnsureCheckSlotRequest{
		TargetID: target.ID, InstallationID: installationID,
		RepositoryID: repository.ID, RepositoryFullName: repository.FullName,
		PullRequest: pullRequest, HeadSHA: headSHA, AppID: appID,
		Name:          storage.PendingCICheckName,
		ExternalID:    fmt.Sprintf("smyklot:merge-after-ci:%s:%s", repository.ID, headSHA),
		DesiredStatus: desired.Status, DesiredConclusion: desired.Conclusion,
		DesiredTitle: desired.Title, DesiredSummary: desired.Summary,
		DesiredActions: desired.Actions, DesiredDigest: digest, ChangedAt: checks.now(),
	}
	slot, err := checks.store.EnsureCheckSlot(ctx, request)
	if errors.Is(err, pendingci.ErrSharedHead) {
		slot, err = checks.reassignClosedPullRequestSlot(ctx, request)
	}
	if err != nil {
		return pendingci.CheckSlot{}, err
	}
	if checkNeedsRenewal(slot, checks.now()) {
		slot, err = checks.store.RenewCheckSlot(ctx, pendingci.RenewCheckSlotRequest{
			ID: slot.ID, ExpectedRevision: slot.Revision,
			ExternalID: renewedExternalID(slot),
			RenewedAt:  checks.now(),
		})
		if err != nil {
			return pendingci.CheckSlot{}, fmt.Errorf("renew pending CI check: %w", err)
		}
	}
	if slot.AppliedDigest == slot.DesiredDigest && slot.State == pendingci.CheckSlotReady {
		return slot, nil
	}

	return checks.sync(ctx, slot)
}

func (checks *Checks) reassignClosedPullRequestSlot(
	ctx context.Context,
	request pendingci.EnsureCheckSlotRequest,
) (pendingci.CheckSlot, error) {
	current, err := checks.store.GetCheckSlotByHead(
		ctx, request.RepositoryID, request.HeadSHA,
	)
	if err != nil {
		return pendingci.CheckSlot{}, err
	}
	client, owner, repository, err := checks.client(current)
	if err != nil {
		return pendingci.CheckSlot{}, err
	}
	state, err := client.GetPullRequestState(ctx, owner, repository, current.PullRequest)
	if err != nil {
		return pendingci.CheckSlot{}, fmt.Errorf("read prior check pull request: %w", err)
	}
	if state.Open {
		return pendingci.CheckSlot{}, pendingci.ErrSharedHead
	}
	_, err = checks.store.ReassignCheckSlot(ctx, pendingci.ReassignCheckSlotRequest{
		ID: current.ID, ExpectedRevision: current.Revision,
		PullRequest: request.PullRequest, ReassignedAt: request.ChangedAt,
	})
	if err != nil {
		return pendingci.CheckSlot{}, fmt.Errorf("reassign closed pull request check: %w", err)
	}

	return checks.store.EnsureCheckSlot(ctx, request)
}

func (checks *Checks) RefreshRerequest(
	ctx context.Context,
	repositoryID, headSHA string,
	appID, checkRunID int64,
	name, externalID string,
	exactRun bool,
) (bool, error) {
	if checks == nil || checks.syncer == nil {
		return false, nil
	}
	refreshed := false
	err := checks.syncer.Exclusive(ctx, repositoryID+":"+headSHA, func() error {
		slot, err := checks.store.GetCheckSlotByHead(ctx, repositoryID, headSHA)
		if errors.Is(err, storage.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if slot.AppID != appID {
			return nil
		}
		if exactRun && (slot.CheckRunID == nil || *slot.CheckRunID != checkRunID ||
			slot.Name != name || slot.ExternalID != externalID) {
			return nil
		}
		slot, err = checks.store.RefreshCheckSlot(ctx, pendingci.RefreshCheckSlotRequest{
			ID: slot.ID, ExpectedRevision: slot.Revision, RefreshedAt: checks.now(),
		})
		if err != nil {
			return fmt.Errorf("refresh rerequested pending CI check: %w", err)
		}
		if _, err := checks.sync(ctx, slot); err != nil {
			return err
		}
		refreshed = true

		return nil
	})

	return refreshed, err
}

func checkNeedsRenewal(slot pendingci.CheckSlot, now time.Time) bool {
	age := now.Sub(slot.UpdatedAt)

	return (slot.DesiredStatus == checkInProgress && age >= checkRenewAfter) ||
		(slot.DesiredStatus == checkCompleted && age >= completedRenewAfter)
}

func renewedExternalID(slot pendingci.CheckSlot) string {
	base := slot.ExternalID
	if marker := strings.LastIndex(base, ":g"); marker >= 0 {
		if generation, err := strconv.ParseInt(base[marker+2:], 10, 64); err == nil && generation > 0 {
			base = base[:marker]
		}
	}

	return fmt.Sprintf("%s:g%d", base, slot.Generation+1)
}

func (checks *Checks) sync(
	ctx context.Context,
	slot pendingci.CheckSlot,
) (pendingci.CheckSlot, error) {
	client, owner, repository, err := checks.client(slot)
	if err != nil {
		return pendingci.CheckSlot{}, err
	}
	var run github.CheckRun
	if slot.CheckRunID == nil {
		run, err = checks.adoptOrCreate(ctx, client, owner, repository, slot)
	} else {
		run, err = client.UpdateCheckRun(
			ctx, owner, repository, *slot.CheckRunID, checkRunWrite(slot, checks.now()),
		)
	}
	if err != nil {
		return pendingci.CheckSlot{}, checks.retry(ctx, slot, err)
	}
	if err := validateOwnedCheckRun(run, slot); err != nil {
		return pendingci.CheckSlot{}, checks.retry(ctx, slot, err)
	}
	if slot.CheckRunID == nil {
		slot, err = checks.store.BindCheckRun(ctx, pendingci.BindCheckRunRequest{
			ID: slot.ID, ExpectedRevision: slot.Revision,
			CheckRunID: run.ID, CheckURL: run.HTMLURL, BoundAt: checks.now(),
		})
		if err != nil {
			return pendingci.CheckSlot{}, fmt.Errorf("bind pending CI check run: %w", err)
		}
	}
	applied, err := checks.store.ApplyCheckSlot(ctx, pendingci.ApplyCheckSlotRequest{
		ID: slot.ID, ExpectedRevision: slot.Revision, AppliedDigest: slot.DesiredDigest,
		CheckRunID: run.ID, CheckURL: run.HTMLURL, AppliedAt: checks.now(),
	})
	if err != nil {
		return pendingci.CheckSlot{}, fmt.Errorf("record applied pending CI check: %w", err)
	}

	return applied, nil
}

func (checks *Checks) adoptOrCreate(
	ctx context.Context,
	client *github.Client,
	owner, repository string,
	slot pendingci.CheckSlot,
) (github.CheckRun, error) {
	runs, err := client.ListCheckRunsForRef(ctx, owner, repository, slot.HeadSHA)
	if err != nil {
		return github.CheckRun{}, err
	}
	matches := make([]github.CheckRun, 0, 1)
	for _, run := range runs {
		if run.AppID == slot.AppID && run.Name == slot.Name && run.ExternalID == slot.ExternalID {
			matches = append(matches, run)
		}
	}
	if len(matches) > 1 {
		return github.CheckRun{}, fmt.Errorf("multiple Smyklot check runs own %s", slot.HeadSHA)
	}
	if len(matches) == 1 {
		return client.UpdateCheckRun(
			ctx, owner, repository, matches[0].ID, checkRunWrite(slot, checks.now()),
		)
	}

	return client.CreateCheckRun(ctx, owner, repository, checkRunWrite(slot, checks.now()))
}

func (checks *Checks) retry(
	ctx context.Context,
	slot pendingci.CheckSlot,
	cause error,
) error {
	_, retryErr := checks.store.RetryCheckSlot(ctx, pendingci.RetryCheckSlotRequest{
		ID: slot.ID, ExpectedRevision: slot.Revision,
		RetryAt: checks.now().Add(RetryDelay), Error: cause.Error(), FailedAt: checks.now(),
	})
	if retryErr != nil && !errors.Is(retryErr, storage.ErrConflict) {
		return fmt.Errorf("sync pending CI check: %v; record retry: %w", cause, retryErr)
	}

	return fmt.Errorf("sync pending CI check: %w", cause)
}

func (checks *Checks) client(
	slot pendingci.CheckSlot,
) (*github.Client, string, string, error) {
	token, err := checks.tokens.InstallationToken(slot.InstallationID)
	if err != nil {
		return nil, "", "", bot.NewGitHubError(bot.ErrGitHubAppAuth, err)
	}
	client, err := github.NewClient(token, checks.apiBaseURL)
	if err != nil {
		return nil, "", "", bot.NewGitHubError(bot.ErrGitHubClient, err)
	}
	owner, repository, found := strings.Cut(slot.RepositoryFullName, "/")
	if !found || owner == "" || repository == "" || strings.Contains(repository, "/") {
		return nil, "", "", fmt.Errorf("invalid repository name %q", slot.RepositoryFullName)
	}

	return client, owner, repository, nil
}

func checkRunWrite(slot pendingci.CheckSlot, now time.Time) github.CheckRunWrite {
	actions := make([]github.CheckRunAction, 0, len(slot.DesiredActions))
	for _, action := range slot.DesiredActions {
		actions = append(actions, github.CheckRunAction(action))
	}
	write := github.CheckRunWrite{
		Name: slot.Name, HeadSHA: slot.HeadSHA, ExternalID: slot.ExternalID,
		Status: slot.DesiredStatus, Conclusion: slot.DesiredConclusion,
		Output:  github.CheckRunOutput{Title: slot.DesiredTitle, Summary: slot.DesiredSummary},
		Actions: actions, StartedAt: now,
	}
	if slot.DesiredStatus == checkCompleted {
		write.CompletedAt = now
	}

	return write
}

func validateOwnedCheckRun(run github.CheckRun, slot pendingci.CheckSlot) error {
	if run.ID <= 0 || run.AppID != slot.AppID || run.Name != slot.Name ||
		run.HeadSHA != slot.HeadSHA || run.ExternalID != slot.ExternalID {
		return fmt.Errorf("GitHub returned a Check Run outside the durable Smyklot slot")
	}

	return nil
}

func checkDigest(desired checkDesired) (string, error) {
	content, err := json.Marshal(desired)
	if err != nil {
		return "", fmt.Errorf("encode pending CI check desire: %w", err)
	}
	sum := sha256.Sum256(content)

	return hex.EncodeToString(sum[:]), nil
}

func parsePositiveInt64(value string) (int64, error) {
	number, err := strconv.ParseInt(value, 10, 64)
	if err != nil || number <= 0 {
		return 0, fmt.Errorf("invalid positive integer %q", value)
	}

	return number, nil
}
