package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type ciOutcome uint8

const (
	ciOutcomePassing ciOutcome = iota
	ciOutcomePending
	ciOutcomeFailing
	ciOutcomeUnknown
)

type checkRunSignal struct {
	context    string
	appID      int64
	externalID string
	outcome    ciOutcome
}

type commitStatusSignal struct {
	context string
	outcome ciOutcome
}

type checkRunResponse struct {
	TotalCount *int `json:"total_count"`
	CheckRuns  []struct {
		Name       string `json:"name"`
		ExternalID string `json:"external_id"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
		App        struct {
			ID int64 `json:"id"`
		} `json:"app"`
	} `json:"check_runs"`
}

type commitStatusResponse struct {
	TotalCount *int `json:"total_count"`
	Statuses   []struct {
		Context string `json:"context"`
		State   string `json:"state"`
	} `json:"statuses"`
}

type branchRuleResponse struct {
	Type       string `json:"type"`
	Parameters struct {
		RequiredStatusChecks []struct {
			Context       string `json:"context"`
			IntegrationID *int64 `json:"integration_id"`
		} `json:"required_status_checks"`
	} `json:"parameters"`
}

// RequiredCIRequirements is the effective required-CI policy for one branch.
// Required workflows are separate rules, not status-check contexts, and cannot
// be inferred safely from a Check Run name.
type RequiredCIRequirements struct {
	StatusChecks     []RequiredCheck
	RequiredWorkflow bool
}

// GetRequiredStatusChecks retrieves required status checks from branch protection.
func (c *Client) GetRequiredStatusChecks(
	ctx context.Context,
	owner, repo, branch string,
) ([]RequiredCheck, error) {
	requirements, err := c.GetRequiredCIRequirements(ctx, owner, repo, branch)

	return requirements.StatusChecks, err
}

// GetRequiredCIRequirements retrieves status-check and required-workflow
// policy. Callers implementing "required CI only" must reject required
// workflows until they can identify their runs without guessing by name.
func (c *Client) GetRequiredCIRequirements(
	ctx context.Context,
	owner, repo, branch string,
) (RequiredCIRequirements, error) {
	classic, err := c.getClassicRequiredStatusChecks(ctx, owner, repo, branch)
	if err != nil {
		return RequiredCIRequirements{}, err
	}
	ruleset, requiredWorkflow, err := c.getRulesetRequiredStatusChecks(ctx, owner, repo, branch)
	if err != nil {
		return RequiredCIRequirements{}, err
	}

	return RequiredCIRequirements{
		StatusChecks:     mergeRequiredChecks(classic, ruleset),
		RequiredWorkflow: requiredWorkflow,
	}, nil
}

func (c *Client) getClassicRequiredStatusChecks(
	ctx context.Context,
	owner, repo, branch string,
) ([]RequiredCheck, error) {
	path := fmt.Sprintf(
		"/repos/%s/%s/branches/%s/protection/required_status_checks",
		owner,
		repo,
		url.PathEscape(branch),
	)
	response, err := doJSON[struct {
		Contexts []string `json:"contexts"`
		Checks   []struct {
			Context string `json:"context"`
			AppID   *int64 `json:"app_id"`
		} `json:"checks"`
	}](ctx, c, http.MethodGet, path, nil)
	if err != nil {
		// An unprotected branch has no required checks, which is an answer
		// rather than a failure.
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return []RequiredCheck{}, nil
		}

		return nil, err
	}

	required := make([]RequiredCheck, 0, len(response.Contexts)+len(response.Checks))
	contextsWithChecks := make(map[string]struct{}, len(response.Checks))
	seen := make(map[string]struct{}, len(response.Contexts)+len(response.Checks))
	for _, check := range response.Checks {
		appID := normalizeRequiredAppID(check.AppID)
		key := requiredCheckKey(check.Context, appID)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		contextsWithChecks[check.Context] = struct{}{}
		required = append(required, RequiredCheck{Context: check.Context, AppID: appID})
	}
	for _, contextName := range response.Contexts {
		if _, represented := contextsWithChecks[contextName]; represented {
			continue
		}
		key := requiredCheckKey(contextName, nil)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		required = append(required, RequiredCheck{Context: contextName})
	}

	return required, nil
}

func (c *Client) getRulesetRequiredStatusChecks(
	ctx context.Context,
	owner, repo, branch string,
) ([]RequiredCheck, bool, error) {
	required := make([]RequiredCheck, 0)
	requiredWorkflow := false
	for page := 1; page <= maxPages; page++ {
		path := fmt.Sprintf(
			"/repos/%s/%s/rules/branches/%s?per_page=%d&page=%d",
			owner,
			repo,
			url.PathEscape(branch),
			pageSize,
			page,
		)
		rules, err := doJSON[[]branchRuleResponse](ctx, c, http.MethodGet, path, nil)
		if err != nil {
			var apiErr *APIError
			if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
				return required, requiredWorkflow, nil
			}

			return nil, false, err
		}
		for _, rule := range rules {
			if rule.Type == rulesetWorkflows {
				requiredWorkflow = true
			}
			if rule.Type != "required_status_checks" {
				continue
			}
			for _, check := range rule.Parameters.RequiredStatusChecks {
				required = append(required, RequiredCheck{
					Context: check.Context,
					AppID:   normalizeRequiredAppID(check.IntegrationID),
				})
			}
		}
		if len(rules) < pageSize {
			return required, requiredWorkflow, nil
		}
	}

	return nil, false, NewAPIError(
		ErrIncompletePagination,
		0,
		http.MethodGet,
		fmt.Sprintf("/repos/%s/%s/rules/branches/%s", owner, repo, url.PathEscape(branch)),
		nil,
	)
}

func mergeRequiredChecks(groups ...[]RequiredCheck) []RequiredCheck {
	bound := make(map[string]bool)
	for _, group := range groups {
		for _, check := range group {
			if check.AppID != nil {
				bound[check.Context] = true
			}
		}
	}
	seen := make(map[string]struct{})
	merged := make([]RequiredCheck, 0)
	for _, group := range groups {
		for _, check := range group {
			if check.Context == "" || (check.AppID == nil && bound[check.Context]) {
				continue
			}
			key := requiredCheckKey(check.Context, check.AppID)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			merged = append(merged, check)
		}
	}

	return merged
}

func normalizeRequiredAppID(appID *int64) *int64 {
	if appID == nil || *appID == -1 {
		return nil
	}

	return appID
}

func requiredCheckKey(contextName string, appID *int64) string {
	if appID == nil {
		return contextName + ":any"
	}

	return fmt.Sprintf("%s:%d", contextName, *appID)
}

// GetCheckStatus aggregates Check Runs and legacy commit statuses for a commit.
// A nil requiredOnly slice means all contexts; a non-nil slice means exactly
// the branch-protection requirements supplied by the caller.
func (c *Client) GetCheckStatus(
	ctx context.Context,
	owner, repo, ref string,
	requiredOnly []RequiredCheck,
) (*CheckStatus, error) {
	return c.getCheckStatus(ctx, owner, repo, ref, requiredOnly, nil)
}

// OwnedCheck identifies the one durable Check Run Smyklot may exclude from CI
// aggregation while deciding whether that run can become successful.
type OwnedCheck struct {
	Name       string
	AppID      int64
	ExternalID string
}

// GetCheckStatusExcludingCheck ignores exactly one App-bound Check Run. A
// required Smyklot gate must not wait on itself while deciding whether CI is
// ready to satisfy that gate.
func (c *Client) GetCheckStatusExcludingCheck(
	ctx context.Context,
	owner, repo, ref string,
	requiredOnly []RequiredCheck,
	excluded OwnedCheck,
) (*CheckStatus, error) {
	return c.getCheckStatus(ctx, owner, repo, ref, requiredOnly, &excluded)
}

func (c *Client) getCheckStatus(
	ctx context.Context,
	owner, repo, ref string,
	requiredOnly []RequiredCheck,
	excluded *OwnedCheck,
) (*CheckStatus, error) {
	checkRuns, err := c.listCheckRuns(ctx, owner, repo, ref, "latest")
	if err != nil {
		return nil, err
	}
	conflictingOwnedContext := false
	if excluded != nil {
		allRuns, allErr := c.listCheckRuns(ctx, owner, repo, ref, "all")
		if allErr != nil {
			return nil, allErr
		}
		owned := 0
		for _, signal := range allRuns {
			if signal.context == excluded.Name && signal.appID == excluded.AppID &&
				signal.externalID == excluded.ExternalID {
				owned++
			}
		}
		conflictingOwnedContext = owned != 1
		var latestConflict bool
		checkRuns, latestConflict = excludeCheckRun(checkRuns, *excluded)
		conflictingOwnedContext = conflictingOwnedContext || latestConflict
		if requiredOnly != nil {
			requiredOnly = excludeRequiredCheck(requiredOnly, *excluded)
		}
	}
	commitStatuses, err := c.listCommitStatuses(ctx, owner, repo, ref)
	if err != nil {
		return nil, err
	}

	if requiredOnly != nil {
		status := aggregateRequiredChecks(requiredOnly, checkRuns, commitStatuses)
		if conflictingOwnedContext {
			markCheckStatusBlocked(status)
		}

		return status, nil
	}

	status := aggregateAllChecks(checkRuns, commitStatuses)
	if conflictingOwnedContext {
		markCheckStatusBlocked(status)
	}

	return status, nil
}

func excludeCheckRun(signals []checkRunSignal, excluded OwnedCheck) ([]checkRunSignal, bool) {
	filtered := signals[:0]
	conflict := false
	for _, signal := range signals {
		if signal.context != excluded.Name {
			filtered = append(filtered, signal)
			continue
		}
		if signal.appID == excluded.AppID && signal.externalID == excluded.ExternalID {
			continue
		}
		if signal.appID == excluded.AppID &&
			checkExternalIDFamily(signal.externalID) == checkExternalIDFamily(excluded.ExternalID) {
			continue
		}
		conflict = true
		filtered = append(filtered, signal)
	}

	return filtered, conflict
}

func checkExternalIDFamily(externalID string) string {
	marker := strings.LastIndex(externalID, ":g")
	if marker < 0 {
		return externalID
	}
	generation, err := strconv.ParseInt(externalID[marker+2:], 10, 64)
	if err != nil || generation <= 0 {
		return externalID
	}

	return externalID[:marker]
}

func excludeRequiredCheck(required []RequiredCheck, excluded OwnedCheck) []RequiredCheck {
	filtered := make([]RequiredCheck, 0, len(required))
	for _, check := range required {
		if check.Context == excluded.Name && check.AppID != nil && *check.AppID == excluded.AppID {
			continue
		}
		filtered = append(filtered, check)
	}

	return filtered
}

func markCheckStatusBlocked(status *CheckStatus) {
	status.State = CIStatePending
	status.AllPassing = false
	status.Pending = true
}

func (c *Client) listCheckRuns(
	ctx context.Context,
	owner, repo, ref, filter string,
) ([]checkRunSignal, error) {
	var signals []checkRunSignal
	for page := 1; page <= maxPages; page++ {
		path := fmt.Sprintf(
			"/repos/%s/%s/commits/%s/check-runs?filter=%s&per_page=%d&page=%d",
			owner,
			repo,
			url.PathEscape(ref),
			url.QueryEscape(filter),
			pageSize,
			page,
		)
		response, err := doJSON[checkRunResponse](ctx, c, http.MethodGet, path, nil)
		if err != nil {
			return nil, err
		}

		for _, run := range response.CheckRuns {
			signals = append(signals, checkRunSignal{
				context: run.Name, appID: run.App.ID, externalID: run.ExternalID,
				outcome: classifyCheckRun(run.Status, run.Conclusion),
			})
		}
		complete, err := paginationComplete(len(response.CheckRuns), len(signals), response.TotalCount, page)
		if err != nil {
			return nil, NewAPIError(ErrIncompletePagination, 0, http.MethodGet, path, err)
		}
		if complete {
			return signals, nil
		}
	}

	return signals, nil
}

func (c *Client) listCommitStatuses(
	ctx context.Context,
	owner, repo, ref string,
) ([]commitStatusSignal, error) {
	var signals []commitStatusSignal
	seen := make(map[string]struct{})
	received := 0
	for page := 1; page <= maxPages; page++ {
		path := fmt.Sprintf(
			"/repos/%s/%s/commits/%s/status?per_page=%d&page=%d",
			owner,
			repo,
			url.PathEscape(ref),
			pageSize,
			page,
		)
		response, err := doJSON[commitStatusResponse](ctx, c, http.MethodGet, path, nil)
		if err != nil {
			return nil, err
		}

		received += len(response.Statuses)
		for _, status := range response.Statuses {
			if _, exists := seen[status.Context]; exists {
				continue
			}
			seen[status.Context] = struct{}{}
			signals = append(signals, commitStatusSignal{
				context: status.Context,
				outcome: classifyCommitStatus(status.State),
			})
		}
		complete, err := paginationComplete(len(response.Statuses), received, response.TotalCount, page)
		if err != nil {
			return nil, NewAPIError(ErrIncompletePagination, 0, http.MethodGet, path, err)
		}
		if complete {
			return signals, nil
		}
	}

	return signals, nil
}

func paginationComplete(pageItems, received int, total *int, page int) (bool, error) {
	if total != nil && received >= *total {
		return true, nil
	}
	if pageItems < pageSize {
		if total != nil {
			return false, fmt.Errorf("received %d of %d items", received, *total)
		}

		return true, nil
	}
	if page == maxPages {
		return false, fmt.Errorf("list still incomplete after %d items", received)
	}

	return false, nil
}

func classifyCheckRun(status, conclusion string) ciOutcome {
	switch status {
	case "queued", "in_progress", "pending", "waiting", "requested":
		return ciOutcomePending
	case "completed":
		switch conclusion {
		case "success", "skipped", "neutral":
			return ciOutcomePassing
		case "failure", "cancelled", "timed_out", "action_required", "startup_failure", "stale":
			return ciOutcomeFailing
		default:
			return ciOutcomeUnknown
		}
	default:
		return ciOutcomeUnknown
	}
}

func classifyCommitStatus(state string) ciOutcome {
	switch state {
	case "success":
		return ciOutcomePassing
	case "pending":
		return ciOutcomePending
	case "failure", "error":
		return ciOutcomeFailing
	default:
		return ciOutcomeUnknown
	}
}

func aggregateAllChecks(checkRuns []checkRunSignal, statuses []commitStatusSignal) *CheckStatus {
	result := &CheckStatus{Total: len(checkRuns) + len(statuses)}
	for _, run := range checkRuns {
		countOutcome(result, run.outcome)
	}
	for _, status := range statuses {
		countOutcome(result, status.outcome)
	}

	return finishCheckStatus(result)
}

func aggregateRequiredChecks(
	required []RequiredCheck,
	checkRuns []checkRunSignal,
	statuses []commitStatusSignal,
) *CheckStatus {
	result := &CheckStatus{Total: len(required)}
	for _, requirement := range required {
		outcomes := matchingOutcomes(requirement, checkRuns, statuses)
		if len(outcomes) == 0 {
			result.Missing++
			countOutcome(result, ciOutcomePending)
			continue
		}
		countOutcome(result, safestOutcome(outcomes))
	}

	return finishCheckStatus(result)
}

func matchingOutcomes(
	required RequiredCheck,
	checkRuns []checkRunSignal,
	statuses []commitStatusSignal,
) []ciOutcome {
	var outcomes []ciOutcome
	for _, run := range checkRuns {
		if run.context == required.Context && (required.AppID == nil || run.appID == *required.AppID) {
			outcomes = append(outcomes, run.outcome)
		}
	}
	if required.AppID != nil {
		return outcomes
	}
	for _, status := range statuses {
		if status.context == required.Context {
			outcomes = append(outcomes, status.outcome)
		}
	}

	return outcomes
}

func safestOutcome(outcomes []ciOutcome) ciOutcome {
	result := ciOutcomePassing
	for _, outcome := range outcomes {
		if outcome > result {
			result = outcome
		}
	}

	return result
}

func countOutcome(result *CheckStatus, outcome ciOutcome) {
	switch outcome {
	case ciOutcomePassing:
		result.Passed++
	case ciOutcomePending:
		result.InProgress++
	case ciOutcomeFailing:
		result.Failed++
	case ciOutcomeUnknown:
		result.Unknown++
	}
}

func finishCheckStatus(result *CheckStatus) *CheckStatus {
	switch {
	case result.Total == 0:
		result.State = CIStateNoChecks
	case result.Unknown > 0:
		result.State = CIStateIndeterminate
	case result.Failed > 0:
		result.State = CIStateFailing
	case result.InProgress > 0:
		result.State = CIStatePending
	case result.Passed == result.Total:
		result.State = CIStatePassing
	default:
		result.State = CIStateIndeterminate
	}

	result.AllPassing = result.State == CIStatePassing
	result.Pending = result.State == CIStatePending
	result.Failing = result.Failed > 0
	result.Summary = fmt.Sprintf("%d/%d checks passing", result.Passed, result.Total)
	if result.InProgress > 0 {
		result.Summary += fmt.Sprintf(", %d in progress", result.InProgress)
	}
	if result.Failed > 0 {
		result.Summary += fmt.Sprintf(", %d failed", result.Failed)
	}
	if result.Missing > 0 {
		result.Summary += fmt.Sprintf(", %d missing", result.Missing)
	}
	if result.Unknown > 0 {
		result.Summary += fmt.Sprintf(", %d indeterminate", result.Unknown)
	}

	return result
}
