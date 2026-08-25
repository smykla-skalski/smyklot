package gate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

type gateStore interface {
	GetArmed(context.Context, string, int) (pendingci.Request, error)
	ListQueue(context.Context, pendingci.QueueFilter) ([]pendingci.Request, error)
	HasPendingCleanup(context.Context, pendingci.CleanupFilter) (bool, error)
	GetPendingCIRepositoryGate(context.Context, string) (storage.PendingCIRepositoryGate, error)
	UpdatePendingCIRepositoryGate(
		context.Context,
		storage.PendingCIGateChange,
	) (storage.PendingCIRepositoryGate, error)
}

type GateReconciler struct {
	store  gateStore
	checks *Checks
	now    func() time.Time
	wake   func()
}

const (
	checksReadyReason = "Checks and required context are ready"
	rulesetActive     = "active"
	rulesetBranch     = "branch"
)

type gatePolicyError struct{ cause error }

func (err gatePolicyError) Error() string { return err.cause.Error() }
func (err gatePolicyError) Unwrap() error { return err.cause }

func gatePolicy(cause error) error {
	return gatePolicyError{cause: cause}
}

func (reconciler *GateReconciler) Reconcile(
	ctx context.Context,
	client *github.Client,
	target storage.Target,
	repository storage.Repository,
	prs []map[string]interface{},
	serviceEnabled bool,
) error {
	gate, err := reconciler.store.GetPendingCIRepositoryGate(ctx, repository.ID)
	if err != nil {
		return fmt.Errorf("read pending CI repository gate: %w", err)
	}
	owner, name, err := bot.ParseRepo(repository.FullName)
	if err != nil {
		return reconciler.block(ctx, gate, gatePolicy(err))
	}
	if !serviceEnabled {
		return reconciler.reconcileInactive(
			ctx,
			client,
			gate,
			repository,
			prs,
			owner,
			name,
			"Repository is not active on the service",
		)
	}
	drainingChecks := false
	if gate.DesiredMode == storage.PendingCIModeLabels {
		drainingChecks, err = reconciler.hasArmedCheckRequest(ctx, repository.ID)
		if err != nil {
			return reconciler.block(ctx, gate, err)
		}
	}
	maintainChecks := mustMaintainChecks(gate.DesiredMode, drainingChecks)
	if maintainChecks {
		if !target.Grants("checks") {
			return reconciler.block(
				ctx, gate, gatePolicy(errors.New("checks write approval is missing")),
			)
		}
		if !target.CanRead("statuses") {
			return reconciler.block(
				ctx, gate,
				gatePolicy(errors.New("commit statuses read approval is missing")),
			)
		}
		patterns := target.PendingCIBranchPatternsDefault
		if repository.PendingCIBranchPatternsOverride != nil {
			patterns = *repository.PendingCIBranchPatternsOverride
		}
		if err := reconciler.ensureBaselines(ctx, target, repository, prs, patterns); err != nil {
			return reconciler.block(ctx, gate, err)
		}
	}
	if gate.DesiredMode == storage.PendingCIModeLabels {
		return reconciler.reconcileLabels(
			ctx, client, gate, repository, prs, owner, name, drainingChecks,
		)
	}
	if !target.CanRead("merge_queues") {
		return reconciler.block(
			ctx, gate, gatePolicy(errors.New("merge queues read approval is missing")),
		)
	}
	if !target.Grants("administration") {
		return reconciler.block(
			ctx, gate, gatePolicy(errors.New("administration write approval is missing")),
		)
	}

	return reconciler.reconcileChecks(ctx, client, target, repository, gate, prs, owner, name)
}

func mustMaintainChecks(desired storage.PendingCIMode, draining bool) bool {
	return desired == storage.PendingCIModeChecks || draining
}

func (reconciler *GateReconciler) reconcileChecks(
	ctx context.Context,
	client *github.Client,
	target storage.Target,
	repository storage.Repository,
	gate storage.PendingCIRepositoryGate,
	prs []map[string]interface{},
	owner, name string,
) error {
	appID, err := reconciler.checks.AppID(ctx)
	if err != nil {
		return reconciler.block(ctx, gate, err)
	}
	patterns := target.PendingCIBranchPatternsDefault
	if repository.PendingCIBranchPatternsOverride != nil {
		patterns = *repository.PendingCIBranchPatternsOverride
	}
	if err := ensureNoMergeQueue(
		ctx, client, owner, name, repository.DefaultBranch, prs, patterns,
	); err != nil {
		return reconciler.block(ctx, gate, err)
	}
	desired := ruleset(patterns, appID)
	rulesetID, err := reconcilePendingCIRuleset(ctx, client, owner, name, gate, desired)
	if err != nil {
		return reconciler.block(ctx, gate, err)
	}
	fingerprint, err := rulesetFingerprint(desired)
	if err != nil {
		return reconciler.block(ctx, gate, err)
	}
	if gate.AppID == nil || *gate.AppID != appID || gate.RulesetID == nil ||
		*gate.RulesetID != rulesetID || gate.RulesetFingerprint != fingerprint {
		gate, err = reconciler.store.UpdatePendingCIRepositoryGate(ctx, storage.PendingCIGateChange{
			RepositoryID: repository.ID, ExpectedRevision: gate.Revision,
			EffectiveMode: gate.EffectiveMode, Readiness: storage.PendingCIProvisioning,
			Reason: "Verifying the required Smyklot context", AppID: &appID,
			RulesetID: &rulesetID, RulesetFingerprint: fingerprint, ObservedAt: reconciler.now(),
		})
		if err != nil {
			if errors.Is(err, storage.ErrConflict) {
				return nil
			}

			return fmt.Errorf("record pending CI ruleset ownership: %w", err)
		}
	}
	if err := ensurePendingCIRequiredContexts(
		ctx, client, owner, name, repository, prs, patterns, appID,
	); err != nil {
		return reconciler.block(ctx, gate, err)
	}
	if gate.EffectiveMode == storage.PendingCIEffectiveChecks &&
		gate.Readiness == storage.PendingCIReady && gate.Reason == checksReadyReason {
		return nil
	}
	_, err = reconciler.store.UpdatePendingCIRepositoryGate(ctx, storage.PendingCIGateChange{
		RepositoryID: repository.ID, ExpectedRevision: gate.Revision,
		EffectiveMode: storage.PendingCIEffectiveChecks, Readiness: storage.PendingCIReady,
		Reason: checksReadyReason, AppID: &appID,
		RulesetID: &rulesetID, RulesetFingerprint: fingerprint, ObservedAt: reconciler.now(),
	})
	if err != nil && !errors.Is(err, storage.ErrConflict) {
		return fmt.Errorf("mark pending CI checks ready: %w", err)
	}
	if err == nil && reconciler.wake != nil {
		reconciler.wake()
	}

	return nil
}

func (reconciler *GateReconciler) ensureBaselines(
	ctx context.Context,
	target storage.Target,
	repository storage.Repository,
	prs []map[string]interface{},
	patterns storage.PendingCIBranchPatterns,
) error {
	for _, raw := range prs {
		pullRequest, headSHA, baseBranch, err := pullRequestHead(raw)
		if err != nil {
			return err
		}
		if !branchIncluded(baseBranch, repository.DefaultBranch, patterns) {
			continue
		}
		armed, err := reconciler.store.GetArmed(ctx, repository.ID, pullRequest)
		if err == nil && armed.ArtifactKind == pendingci.ArtifactCheck {
			continue
		}
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			return fmt.Errorf("read pending CI baseline owner for pull request %d: %w", pullRequest, err)
		}
		if _, err := reconciler.checks.EnsureBaseline(
			ctx, target, repository, pullRequest, headSHA,
		); err != nil {
			return fmt.Errorf("ensure baseline check for pull request %d: %w", pullRequest, err)
		}
	}

	return nil
}

func (reconciler *GateReconciler) reconcileLabels(
	ctx context.Context,
	client *github.Client,
	gate storage.PendingCIRepositoryGate,
	repository storage.Repository,
	prs []map[string]interface{},
	owner, name string,
	draining bool,
) error {
	if draining {
		const reason = "Waiting for existing check-mode authorizations to finish"
		if gate.EffectiveMode == storage.PendingCIEffectiveChecks &&
			gate.Readiness == storage.PendingCIDraining && gate.Reason == reason {
			return nil
		}
		_, err := reconciler.store.UpdatePendingCIRepositoryGate(ctx, storage.PendingCIGateChange{
			RepositoryID: gate.RepositoryID, ExpectedRevision: gate.Revision,
			EffectiveMode: storage.PendingCIEffectiveChecks, Readiness: storage.PendingCIDraining,
			Reason: reason, AppID: gate.AppID, RulesetID: gate.RulesetID,
			RulesetFingerprint: gate.RulesetFingerprint, ObservedAt: reconciler.now(),
		})
		if err != nil && !errors.Is(err, storage.ErrConflict) {
			return fmt.Errorf("mark pending CI checks draining: %w", err)
		}

		return nil
	}
	if err := removePendingCIRuleset(ctx, client, owner, name, gate); err != nil {
		return reconciler.block(ctx, gate, err)
	}
	if err := ensureNoPendingCIRequiredRulesets(ctx, client, owner, name); err != nil {
		return reconciler.block(ctx, gate, err)
	}
	if err := ensureNoPendingCIRequiredContextOnBranches(
		ctx, client, owner, name, repository.DefaultBranch, prs,
	); err != nil {
		return reconciler.block(ctx, gate, err)
	}
	_, err := reconciler.store.UpdatePendingCIRepositoryGate(ctx, storage.PendingCIGateChange{
		RepositoryID: gate.RepositoryID, ExpectedRevision: gate.Revision,
		EffectiveMode: storage.PendingCIEffectiveLabels, Readiness: storage.PendingCIReady,
		Reason: "Label mode is ready", ObservedAt: reconciler.now(),
	})
	if err != nil && !errors.Is(err, storage.ErrConflict) {
		return fmt.Errorf("mark pending CI labels ready: %w", err)
	}

	return nil
}

func (reconciler *GateReconciler) hasArmedCheckRequest(
	ctx context.Context,
	repositoryID string,
) (bool, error) {
	requests, err := reconciler.store.ListQueue(ctx, pendingci.QueueFilter{
		RepositoryID: repositoryID,
		ArtifactKind: pendingci.ArtifactCheck,
		Limit:        1,
	})
	if err != nil {
		return false, fmt.Errorf("read draining check requests: %w", err)
	}

	return len(requests) > 0, nil
}

func (reconciler *GateReconciler) reconcileInactive(
	ctx context.Context,
	client *github.Client,
	gate storage.PendingCIRepositoryGate,
	repository storage.Repository,
	prs []map[string]interface{},
	owner, name, reason string,
) error {
	cleaning, err := reconciler.store.HasPendingCleanup(ctx, pendingci.CleanupFilter{
		RepositoryID: repository.ID, ArtifactsPendingOnly: true,
	})
	if err != nil {
		return reconciler.block(ctx, gate, err)
	}
	if !cleaning && !gateOwnsServiceArtifacts(gate) {
		_, err := reconciler.store.UpdatePendingCIRepositoryGate(
			ctx,
			storage.PendingCIGateChange{
				RepositoryID: gate.RepositoryID, ExpectedRevision: gate.Revision,
				EffectiveMode: storage.PendingCIEffectiveNone, Readiness: storage.PendingCIReady,
				Reason: reason, ObservedAt: reconciler.now(),
			},
		)
		if err != nil && !errors.Is(err, storage.ErrConflict) {
			return err
		}

		return nil
	}
	if err := removePendingCIRuleset(ctx, client, owner, name, gate); err != nil {
		return reconciler.block(ctx, gate, err)
	}
	if cleaning {
		const drainingReason = "Waiting for existing service artifacts to be cleaned"
		if gate.Readiness == storage.PendingCIDraining && gate.Reason == drainingReason {
			return nil
		}
		_, err := reconciler.store.UpdatePendingCIRepositoryGate(
			ctx,
			storage.PendingCIGateChange{
				RepositoryID: gate.RepositoryID, ExpectedRevision: gate.Revision,
				EffectiveMode: gate.EffectiveMode, Readiness: storage.PendingCIDraining,
				Reason: drainingReason, ObservedAt: reconciler.now(),
			},
		)
		if err != nil && !errors.Is(err, storage.ErrConflict) {
			return fmt.Errorf("mark pending CI service artifacts draining: %w", err)
		}

		return nil
	}
	if err := ensureNoPendingCIRequiredRulesets(ctx, client, owner, name); err != nil {
		return reconciler.block(ctx, gate, err)
	}
	if err := ensureNoPendingCIRequiredContextOnBranches(
		ctx, client, owner, name, repository.DefaultBranch, prs,
	); err != nil {
		return reconciler.block(ctx, gate, err)
	}
	_, err = reconciler.store.UpdatePendingCIRepositoryGate(ctx, storage.PendingCIGateChange{
		RepositoryID: gate.RepositoryID, ExpectedRevision: gate.Revision,
		EffectiveMode: storage.PendingCIEffectiveNone, Readiness: storage.PendingCIReady,
		Reason: reason, ObservedAt: reconciler.now(),
	})
	if err != nil && !errors.Is(err, storage.ErrConflict) {
		return err
	}

	return nil
}

func gateOwnsServiceArtifacts(gate storage.PendingCIRepositoryGate) bool {
	return gate.EffectiveMode != storage.PendingCIEffectiveNone || gate.AppID != nil ||
		gate.RulesetID != nil || gate.RulesetFingerprint != "" ||
		gate.Readiness == storage.PendingCIProvisioning ||
		gate.Readiness == storage.PendingCIDraining
}

func (reconciler *GateReconciler) block(
	ctx context.Context,
	gate storage.PendingCIRepositoryGate,
	cause error,
) error {
	_, err := reconciler.store.UpdatePendingCIRepositoryGate(ctx, storage.PendingCIGateChange{
		RepositoryID: gate.RepositoryID, ExpectedRevision: gate.Revision,
		EffectiveMode: gate.EffectiveMode, Readiness: storage.PendingCIBlocked,
		Reason: gateBlockReason(cause), AppID: gate.AppID, RulesetID: gate.RulesetID,
		RulesetFingerprint: gate.RulesetFingerprint, ObservedAt: reconciler.now(),
	})
	if err != nil && !errors.Is(err, storage.ErrConflict) {
		return fmt.Errorf("pending CI readiness failed: %v; persist blocker: %w", cause, err)
	}
	var policy gatePolicyError
	if errors.As(cause, &policy) {
		return nil
	}

	return cause
}

// gateBlockReason turns provider failures into recovery guidance before the
// reason is persisted for the panel. The original error still returns from
// block, so logs and retry decisions keep GitHub's status, path, and detail.
func gateBlockReason(cause error) string {
	var apiErr *github.APIError
	if !errors.As(cause, &apiErr) {
		return cause.Error()
	}

	operation := repositoryOperation(apiErr.Path)
	if isCheckRunOperation(operation) {
		return checkRunBlockReason(apiErr)
	}
	if !isRulesetOperation(operation) && !isBranchProtectionOperation(operation) {
		return cause.Error()
	}

	detail := strings.ToLower(apiErr.Detail)
	switch {
	case apiErr.Retryable():
		return "GitHub is temporarily unavailable while Smyklot checks repository protection. " +
			"Smyklot will retry."
	case apiErr.StatusCode == http.StatusForbidden &&
		isRulesetOperation(operation) &&
		strings.Contains(detail, "upgrade to github pro"):
		return "GitHub rulesets require GitHub Pro for private repositories. " +
			"Upgrade the account or make this repository public."
	case apiErr.StatusCode == http.StatusForbidden && isRulesetOperation(operation):
		return "Smyklot cannot read this repository's rulesets. " +
			"Check the GitHub App's administration access and the repository owner's GitHub plan."
	case apiErr.StatusCode == http.StatusForbidden &&
		isRequiredStatusChecksOperation(operation):
		return "Smyklot cannot read this repository's required status checks. " +
			"Check the GitHub App's administration access and the repository owner's GitHub plan."
	case apiErr.StatusCode == http.StatusForbidden:
		return "GitHub refused Smyklot access while checking repository protection. " +
			"Check the GitHub App's repository access and permissions."
	default:
		return "Smyklot could not check this repository's protection settings. " +
			"Check the GitHub App's access and try again."
	}
}

func repositoryOperation(path string) []string {
	path, _, _ = strings.Cut(path, "?")
	segments := strings.Split(strings.Trim(path, "/"), "/")
	for index := 0; index+3 < len(segments); index++ {
		if segments[index] == "repos" {
			return segments[index+3:]
		}
	}

	return segments
}

func isCheckRunOperation(operation []string) bool {
	return len(operation) > 0 && operation[0] == "check-runs" ||
		len(operation) > 2 && operation[0] == "commits" && operation[2] == "check-runs"
}

func isRulesetOperation(operation []string) bool {
	return len(operation) > 0 && operation[0] == "rulesets" ||
		len(operation) > 1 && operation[0] == "rules" && operation[1] == "branches"
}

func isBranchProtectionOperation(operation []string) bool {
	return len(operation) > 2 && operation[0] == "branches" && operation[2] == "protection"
}

func isRequiredStatusChecksOperation(operation []string) bool {
	return isBranchProtectionOperation(operation) && len(operation) > 3 &&
		operation[3] == "required_status_checks"
}

func checkRunBlockReason(apiErr *github.APIError) string {
	switch {
	case apiErr.Retryable():
		return "GitHub is temporarily unavailable while Smyklot prepares baseline checks. " +
			"Smyklot will retry."
	case apiErr.StatusCode == http.StatusForbidden:
		return "Smyklot cannot manage this repository's baseline checks. " +
			"Check the GitHub App's checks access."
	default:
		return "Smyklot could not prepare a baseline check for this pull request. " +
			"Refresh the repository and try again."
	}
}

func ruleset(
	patterns storage.PendingCIBranchPatterns,
	appID int64,
) github.RepositoryRuleset {
	return github.RepositoryRuleset{
		Name: storage.PendingCIRulesetName, Target: rulesetBranch,
		Enforcement: rulesetActive,
		Conditions: github.RulesetConditions{
			IncludeRefs: append([]string(nil), patterns.Include...),
			ExcludeRefs: append([]string(nil), patterns.Exclude...),
		},
		Rules: github.RulesetRules{RequiredStatusChecks: &github.RulesetStatusChecksRule{
			Checks: []github.RulesetStatusCheck{{
				Context: storage.PendingCICheckName, IntegrationID: appID,
			}},
			DoNotEnforceOnCreate: true,
		}},
	}
}

func reconcilePendingCIRuleset(
	ctx context.Context,
	client *github.Client,
	owner, repository string,
	gate storage.PendingCIRepositoryGate,
	desired github.RepositoryRuleset,
) (int64, error) {
	summaries, err := client.ListRepositoryRulesets(ctx, owner, repository)
	if err != nil {
		return 0, err
	}
	owned := ownedRuleset(summaries, gate.RulesetID)
	if owned == nil {
		return adoptOrCreatePendingCIRuleset(ctx, client, owner, repository, summaries, desired)
	}
	if owned.Source.Inherited() {
		return 0, gatePolicy(
			errors.New("the recorded Smyklot ruleset is inherited and cannot be managed"),
		)
	}
	actual, err := client.GetRepositoryRuleset(ctx, owner, repository, owned.ID)
	if err != nil {
		return 0, err
	}
	if !samePendingCIRuleset(actual, desired) {
		if err := client.UpdateRepositoryRuleset(ctx, owner, repository, owned.ID, desired); err != nil {
			return 0, err
		}
	}

	return owned.ID, nil
}

func ownedRuleset(
	summaries []github.RulesetSummary,
	rulesetID *int64,
) *github.RulesetSummary {
	if rulesetID == nil {
		return nil
	}
	for index := range summaries {
		if summaries[index].ID == *rulesetID {
			return &summaries[index]
		}
	}

	return nil
}

func adoptOrCreatePendingCIRuleset(
	ctx context.Context,
	client *github.Client,
	owner, repository string,
	summaries []github.RulesetSummary,
	desired github.RepositoryRuleset,
) (int64, error) {
	named := make([]github.RulesetSummary, 0, 1)
	for _, summary := range summaries {
		if summary.Name == storage.PendingCIRulesetName {
			named = append(named, summary)
		}
	}
	if len(named) > 1 || (len(named) == 1 && named[0].Source.Inherited()) {
		return 0, gatePolicy(
			errors.New("another ruleset already uses Smyklot's managed name"),
		)
	}
	if len(named) == 0 {
		return client.CreateRepositoryRulesetWithID(ctx, owner, repository, desired)
	}
	actual, err := client.GetRepositoryRuleset(ctx, owner, repository, named[0].ID)
	if err != nil {
		return 0, err
	}
	if !samePendingCIRuleset(actual, desired) {
		return 0, gatePolicy(
			errors.New("an unmanaged ruleset already uses Smyklot's managed name"),
		)
	}

	return named[0].ID, nil
}

func samePendingCIRuleset(left, right github.RepositoryRuleset) bool {
	normalize := func(ruleset github.RepositoryRuleset) github.RepositoryRuleset {
		if ruleset.Conditions.IncludeRefs == nil {
			ruleset.Conditions.IncludeRefs = []string{}
		}
		if ruleset.Conditions.ExcludeRefs == nil {
			ruleset.Conditions.ExcludeRefs = []string{}
		}
		if ruleset.BypassActors == nil {
			ruleset.BypassActors = []github.RulesetBypassActor{}
		}
		if ruleset.OtherRules == nil {
			ruleset.OtherRules = []string{}
		}
		if ruleset.Rules.RequiredStatusChecks != nil &&
			ruleset.Rules.RequiredStatusChecks.Checks == nil {
			ruleset.Rules.RequiredStatusChecks.Checks = []github.RulesetStatusCheck{}
		}

		return ruleset
	}

	return reflect.DeepEqual(normalize(left), normalize(right))
}

func removePendingCIRuleset(
	ctx context.Context,
	client *github.Client,
	owner, repository string,
	gate storage.PendingCIRepositoryGate,
) error {
	if gate.RulesetID != nil {
		if err := client.DeleteRepositoryRuleset(ctx, owner, repository, *gate.RulesetID); err != nil {
			var apiErr *github.APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != 404 {
				return err
			}
		}
		return nil
	}
	summaries, err := client.ListRepositoryRulesets(ctx, owner, repository)
	if err != nil {
		return err
	}
	for _, summary := range summaries {
		if summary.Name == storage.PendingCIRulesetName {
			return gatePolicy(
				errors.New("a same-named ruleset is not recorded as Smyklot-owned"),
			)
		}
	}

	return nil
}

func ensureNoMergeQueue(
	ctx context.Context,
	client *github.Client,
	owner, repository, defaultBranch string,
	prs []map[string]interface{},
	patterns storage.PendingCIBranchPatterns,
) error {
	branches, err := baseBranches(defaultBranch, prs)
	if err != nil {
		return err
	}
	for _, branch := range branches {
		if !branchIncluded(branch, defaultBranch, patterns) {
			continue
		}
		enabled, err := client.IsMergeQueueEnabled(ctx, owner, repository, branch)
		if err != nil {
			return err
		}
		if enabled {
			return gatePolicy(fmt.Errorf(
				"merge queue on branch %s is not supported by merge-after-CI checks",
				branch,
			))
		}
	}
	return nil
}

func ensureNoPendingCIRequiredRulesets(
	ctx context.Context,
	client *github.Client,
	owner, repository string,
) error {
	summaries, err := client.ListRepositoryRulesets(ctx, owner, repository)
	if err != nil {
		return err
	}
	for _, summary := range summaries {
		if summary.Target != rulesetBranch ||
			summary.Enforcement != rulesetActive {
			continue
		}
		ruleset, err := client.GetRepositoryRulesetIncludingParents(
			ctx, owner, repository, summary.ID,
		)
		if err != nil {
			return err
		}
		if ruleset.Rules.RequiredStatusChecks == nil {
			continue
		}
		for _, check := range ruleset.Rules.RequiredStatusChecks.Checks {
			if check.Context == storage.PendingCICheckName {
				return gatePolicy(
					errors.New("remove the required Smyklot check before enabling label mode"),
				)
			}
		}
	}

	return nil
}

func ensureNoPendingCIRequiredContextOnBranches(
	ctx context.Context,
	client *github.Client,
	owner, repository, defaultBranch string,
	prs []map[string]interface{},
) error {
	branches, err := baseBranches(defaultBranch, prs)
	if err != nil {
		return err
	}
	for _, branch := range branches {
		required, err := client.GetRequiredStatusChecks(ctx, owner, repository, branch)
		if err != nil {
			return err
		}
		for _, check := range required {
			if check.Context == storage.PendingCICheckName {
				return gatePolicy(fmt.Errorf(
					"remove the required Smyklot check from branch %s before enabling label mode",
					branch,
				))
			}
		}
	}

	return nil
}

func ensureNoConflictingRequiredContext(
	ctx context.Context,
	client *github.Client,
	owner, repository, branch string,
	appID int64,
) error {
	required, err := client.GetRequiredStatusChecks(ctx, owner, repository, branch)
	if err != nil {
		return err
	}
	found := false
	for _, check := range required {
		if check.Context != storage.PendingCICheckName {
			continue
		}
		if check.AppID == nil || *check.AppID != appID {
			return gatePolicy(
				errors.New("the Smyklot required context is not bound to this GitHub App"),
			)
		}
		found = true
	}
	if !found {
		return fmt.Errorf("the Smyklot required context is not active on branch %s", branch)
	}

	return nil
}

func ensurePendingCIRequiredContexts(
	ctx context.Context,
	client *github.Client,
	owner, name string,
	repository storage.Repository,
	prs []map[string]interface{},
	patterns storage.PendingCIBranchPatterns,
	appID int64,
) error {
	branches := make(map[string]struct{})
	if branchIncluded(repository.DefaultBranch, repository.DefaultBranch, patterns) {
		branches[repository.DefaultBranch] = struct{}{}
	}
	for _, raw := range prs {
		_, _, baseBranch, err := pullRequestHead(raw)
		if err != nil {
			return err
		}
		if branchIncluded(baseBranch, repository.DefaultBranch, patterns) {
			branches[baseBranch] = struct{}{}
		}
	}
	for branch := range branches {
		if err := ensureNoConflictingRequiredContext(
			ctx,
			client,
			owner,
			name,
			branch,
			appID,
		); err != nil {
			return err
		}
	}

	return nil
}

func baseBranches(
	defaultBranch string,
	prs []map[string]interface{},
) ([]string, error) {
	branches := map[string]struct{}{defaultBranch: {}}
	for _, raw := range prs {
		_, _, baseBranch, err := pullRequestHead(raw)
		if err != nil {
			return nil, err
		}
		branches[baseBranch] = struct{}{}
	}
	result := make([]string, 0, len(branches))
	for branch := range branches {
		if branch != "" {
			result = append(result, branch)
		}
	}
	slices.Sort(result)

	return result, nil
}

func pullRequestHead(raw map[string]interface{}) (int, string, string, error) {
	number, ok := raw["number"].(float64)
	if !ok || number <= 0 {
		return 0, "", "", errors.New("open pull request has no number")
	}
	head, ok := raw["head"].(map[string]interface{})
	if !ok {
		return 0, "", "", errors.New("open pull request has no head")
	}
	base, ok := raw["base"].(map[string]interface{})
	if !ok {
		return 0, "", "", errors.New("open pull request has no base")
	}
	headSHA, headOK := head["sha"].(string)
	baseBranch, baseOK := base["ref"].(string)
	if !headOK || headSHA == "" || !baseOK || baseBranch == "" {
		return 0, "", "", errors.New("open pull request has incomplete head or base")
	}

	return int(number), headSHA, baseBranch, nil
}

func branchIncluded(
	branch, defaultBranch string,
	patterns storage.PendingCIBranchPatterns,
) bool {
	ref := "refs/heads/" + branch
	matches := func(pattern string) bool {
		switch pattern {
		case "~ALL":
			return true
		case "~DEFAULT_BRANCH":
			return branch == defaultBranch
		default:
			return githubRefPatternMatches(pattern, ref)
		}
	}
	included := false
	for _, pattern := range patterns.Include {
		included = included || matches(pattern)
	}
	for _, pattern := range patterns.Exclude {
		if matches(pattern) {
			return false
		}
	}

	return included
}

func githubRefPatternMatches(pattern, ref string) bool {
	patterns := strings.Split(pattern, "/")
	parts := strings.Split(ref, "/")
	type position struct{ pattern, part int }
	seen := make(map[position]bool)
	known := make(map[position]bool)
	var match func(int, int) bool
	match = func(patternIndex, partIndex int) bool {
		key := position{pattern: patternIndex, part: partIndex}
		if known[key] {
			return seen[key]
		}
		known[key] = true
		var matched bool
		switch {
		case patternIndex == len(patterns):
			matched = partIndex == len(parts)
		case patterns[patternIndex] == "**" && patternIndex+1 < len(patterns):
			matched = match(patternIndex+1, partIndex) ||
				(partIndex < len(parts) && !strings.HasPrefix(parts[partIndex], ".") &&
					match(patternIndex, partIndex+1))
		case partIndex < len(parts):
			segmentMatches, err := path.Match(
				goPathPattern(patterns[patternIndex]),
				parts[partIndex],
			)
			if strings.HasPrefix(parts[partIndex], ".") &&
				!githubPatternExplicitlyMatchesDot(patterns[patternIndex]) {
				segmentMatches = false
			}
			matched = err == nil && segmentMatches && match(patternIndex+1, partIndex+1)
		}
		seen[key] = matched

		return matched
	}

	return match(0, 0)
}

func githubPatternExplicitlyMatchesDot(pattern string) bool {
	return strings.HasPrefix(pattern, ".") || strings.HasPrefix(pattern, `\.`)
}

func goPathPattern(pattern string) string {
	var translated strings.Builder
	translated.Grow(len(pattern))
	for index := 0; index < len(pattern); index++ {
		if pattern[index] == '\\' && index+1 < len(pattern) {
			translated.WriteByte(pattern[index])
			index++
			translated.WriteByte(pattern[index])
			continue
		}
		if pattern[index] == '[' && index+1 < len(pattern) && pattern[index+1] == '!' {
			translated.WriteString("[^")
			index++
			continue
		}
		translated.WriteByte(pattern[index])
	}

	return translated.String()
}

func rulesetFingerprint(ruleset github.RepositoryRuleset) (string, error) {
	content, err := json.Marshal(ruleset)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(content)

	return hex.EncodeToString(sum[:]), nil
}
