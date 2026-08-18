package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

type pendingCIGateReconciler struct {
	store  storage.Store
	checks *githubPendingCIChecks
	now    func() time.Time
	wake   func()
}

const (
	pendingCIChecksReadyReason = "Checks and required context are ready"
	pendingCIRulesetActive     = "active"
	pendingCIRulesetBranch     = "branch"
)

type pendingCIGatePolicyError struct{ cause error }

func (err pendingCIGatePolicyError) Error() string { return err.cause.Error() }
func (err pendingCIGatePolicyError) Unwrap() error { return err.cause }

func pendingCIGatePolicy(cause error) error {
	return pendingCIGatePolicyError{cause: cause}
}

func (reconciler *pendingCIGateReconciler) Reconcile(
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
	owner, name, err := parseRepo(repository.FullName)
	if err != nil {
		return reconciler.block(ctx, gate, pendingCIGatePolicy(err))
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
	maintainChecks := pendingCIMustMaintainChecks(gate.DesiredMode, drainingChecks)
	if maintainChecks {
		if !target.Grants("checks") {
			return reconciler.block(
				ctx, gate, pendingCIGatePolicy(errors.New("checks write approval is missing")),
			)
		}
		if !target.CanRead("statuses") {
			return reconciler.block(
				ctx, gate,
				pendingCIGatePolicy(errors.New("commit statuses read approval is missing")),
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
			ctx, gate, pendingCIGatePolicy(errors.New("merge queues read approval is missing")),
		)
	}
	if !target.Grants("administration") {
		return reconciler.block(
			ctx, gate, pendingCIGatePolicy(errors.New("administration write approval is missing")),
		)
	}

	return reconciler.reconcileChecks(ctx, client, target, repository, gate, prs, owner, name)
}

func pendingCIMustMaintainChecks(desired storage.PendingCIMode, draining bool) bool {
	return desired == storage.PendingCIModeChecks || draining
}

func (reconciler *pendingCIGateReconciler) reconcileChecks(
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
	desired := pendingCIRuleset(patterns, appID)
	rulesetID, err := reconcilePendingCIRuleset(ctx, client, owner, name, gate, desired)
	if err != nil {
		return reconciler.block(ctx, gate, err)
	}
	fingerprint, err := pendingCIRulesetFingerprint(desired)
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
		gate.Readiness == storage.PendingCIReady && gate.Reason == pendingCIChecksReadyReason {
		return nil
	}
	_, err = reconciler.store.UpdatePendingCIRepositoryGate(ctx, storage.PendingCIGateChange{
		RepositoryID: repository.ID, ExpectedRevision: gate.Revision,
		EffectiveMode: storage.PendingCIEffectiveChecks, Readiness: storage.PendingCIReady,
		Reason: pendingCIChecksReadyReason, AppID: &appID,
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

func (reconciler *pendingCIGateReconciler) ensureBaselines(
	ctx context.Context,
	target storage.Target,
	repository storage.Repository,
	prs []map[string]interface{},
	patterns storage.PendingCIBranchPatterns,
) error {
	for _, raw := range prs {
		pullRequest, headSHA, baseBranch, err := pendingCIPullRequestHead(raw)
		if err != nil {
			return err
		}
		if !pendingCIBranchIncluded(baseBranch, repository.DefaultBranch, patterns) {
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

func (reconciler *pendingCIGateReconciler) reconcileLabels(
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

func (reconciler *pendingCIGateReconciler) hasArmedCheckRequest(
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

func (reconciler *pendingCIGateReconciler) reconcileInactive(
	ctx context.Context,
	client *github.Client,
	gate storage.PendingCIRepositoryGate,
	repository storage.Repository,
	prs []map[string]interface{},
	owner, name, reason string,
) error {
	if err := removePendingCIRuleset(ctx, client, owner, name, gate); err != nil {
		return reconciler.block(ctx, gate, err)
	}
	cleaning, err := reconciler.store.HasPendingCleanup(ctx, pendingci.CleanupFilter{
		RepositoryID: repository.ID, ArtifactsPendingOnly: true,
	})
	if err != nil {
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

func (reconciler *pendingCIGateReconciler) block(
	ctx context.Context,
	gate storage.PendingCIRepositoryGate,
	cause error,
) error {
	_, err := reconciler.store.UpdatePendingCIRepositoryGate(ctx, storage.PendingCIGateChange{
		RepositoryID: gate.RepositoryID, ExpectedRevision: gate.Revision,
		EffectiveMode: gate.EffectiveMode, Readiness: storage.PendingCIBlocked,
		Reason: cause.Error(), AppID: gate.AppID, RulesetID: gate.RulesetID,
		RulesetFingerprint: gate.RulesetFingerprint, ObservedAt: reconciler.now(),
	})
	if err != nil && !errors.Is(err, storage.ErrConflict) {
		return fmt.Errorf("pending CI readiness failed: %v; persist blocker: %w", cause, err)
	}
	var policy pendingCIGatePolicyError
	if errors.As(cause, &policy) {
		return nil
	}

	return cause
}

func pendingCIRuleset(
	patterns storage.PendingCIBranchPatterns,
	appID int64,
) github.RepositoryRuleset {
	return github.RepositoryRuleset{
		Name: storage.PendingCIRulesetName, Target: pendingCIRulesetBranch,
		Enforcement: pendingCIRulesetActive,
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
	owned := pendingCIOwnedRuleset(summaries, gate.RulesetID)
	if owned == nil {
		return adoptOrCreatePendingCIRuleset(ctx, client, owner, repository, summaries, desired)
	}
	if owned.Source.Inherited() {
		return 0, pendingCIGatePolicy(
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

func pendingCIOwnedRuleset(
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
		return 0, pendingCIGatePolicy(
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
		return 0, pendingCIGatePolicy(
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
			return pendingCIGatePolicy(
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
	branches, err := pendingCIBaseBranches(defaultBranch, prs)
	if err != nil {
		return err
	}
	for _, branch := range branches {
		if !pendingCIBranchIncluded(branch, defaultBranch, patterns) {
			continue
		}
		enabled, err := client.IsMergeQueueEnabled(ctx, owner, repository, branch)
		if err != nil {
			return err
		}
		if enabled {
			return pendingCIGatePolicy(fmt.Errorf(
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
		if summary.Target != pendingCIRulesetBranch ||
			summary.Enforcement != pendingCIRulesetActive {
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
				return pendingCIGatePolicy(
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
	branches, err := pendingCIBaseBranches(defaultBranch, prs)
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
				return pendingCIGatePolicy(fmt.Errorf(
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
			return pendingCIGatePolicy(
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
	if pendingCIBranchIncluded(repository.DefaultBranch, repository.DefaultBranch, patterns) {
		branches[repository.DefaultBranch] = struct{}{}
	}
	for _, raw := range prs {
		_, _, baseBranch, err := pendingCIPullRequestHead(raw)
		if err != nil {
			return err
		}
		if pendingCIBranchIncluded(baseBranch, repository.DefaultBranch, patterns) {
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

func pendingCIBaseBranches(
	defaultBranch string,
	prs []map[string]interface{},
) ([]string, error) {
	branches := map[string]struct{}{defaultBranch: {}}
	for _, raw := range prs {
		_, _, baseBranch, err := pendingCIPullRequestHead(raw)
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

func pendingCIPullRequestHead(raw map[string]interface{}) (int, string, string, error) {
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

func pendingCIBranchIncluded(
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

func pendingCIRulesetFingerprint(ruleset github.RepositoryRuleset) (string, error) {
	content, err := json.Marshal(ruleset)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(content)

	return hex.EncodeToString(sum[:]), nil
}
