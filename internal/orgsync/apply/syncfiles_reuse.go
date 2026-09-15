package apply

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

const proposalMarker = "<!-- smyklot:file-configuration "

const supersededMarker = "<!-- smyklot:superseded -->"

func proposalFingerprint(body string) string {
	_, tail, found := strings.Cut(body, proposalMarker)
	if !found {
		return ""
	}
	value, _, found := strings.Cut(tail, " -->")
	if !found {
		return ""
	}
	return value
}

// An open PR is the review workspace, not the identity of one rendered result.
// Replan against its tree so a setting changed back to the default also undoes
// the previous proposal's bytes. Other paths and reviewer commits stay intact.
func reuseFileProposal(
	ctx context.Context, client *github.Client, target syncTarget, repositoryID string,
	files orgsync.FileConfig, overrides orgsync.FileOverride,
	formatting config.FormattingPolicy, plan orgsync.FilePlan,
) (repositoryAnswer, error) {
	pulls, err := client.ListPullRequestsByHeadPrefix(ctx, target.Owner, target.Name, orgsync.FileBranchPrefix, target.DefaultBranch)
	if err != nil {
		return repositoryAnswer{}, err
	}
	open, declined, openCount := selectFileProposal(pulls, plan.Proposal)
	if open != nil {
		return replanOpenFiles(ctx, client, target, repositoryID, files, overrides, formatting, plan.Proposal, *open, openCount > 1)
	}
	if len(plan.Actions) == 0 {
		return repositoryAnswer{observation: orgsync.ObservationMatched}, nil
	}
	if declined != nil {
		return repositoryAnswer{observation: orgsync.ObservationDeclined, proposalURL: declined.URL}, nil
	}
	// A reused branch may have been closed for a different configuration.
	// Keep its refusal intact and choose an unused branch for this result.
	fingerprint := plan.Proposal
	plan.Proposal = unusedProposalBranch(pulls, fingerprint)
	if plan.Proposal != fingerprint {
		for index := range plan.Actions {
			file, err := orgsync.DecodeFile(plan.Actions[index].Payload)
			if err != nil {
				return repositoryAnswer{}, err
			}
			file.Proposal, file.Fingerprint = plan.Proposal, fingerprint
			plan.Actions[index].Payload, err = json.Marshal(file)
			if err != nil {
				return repositoryAnswer{}, err
			}
		}
	}
	// Legacy proposals have no configuration marker. Their branch still
	// identifies the configuration that was declined or is under review.
	observation, err := proposalObservation(ctx, client, target, plan.Proposal)
	if err != nil {
		return repositoryAnswer{}, err
	}
	if observation.state != "" {
		return repositoryAnswer{observation: observation.state, proposalURL: observation.proposalURL}, nil
	}
	return compared(plan.Actions), nil
}

func replanOpenFiles(
	ctx context.Context, client *github.Client, target syncTarget, repositoryID string,
	files orgsync.FileConfig, overrides orgsync.FileOverride,
	formatting config.FormattingPolicy, fingerprint string, pull github.PullRequest, consolidate bool,
) (repositoryAnswer, error) {
	current, err := readTreePaths(ctx, client, target, pull.HeadRef, files.Managed())
	if err != nil {
		return repositoryAnswer{}, err
	}
	if current.Missing {
		return repositoryAnswer{}, fmt.Errorf("%w: open proposal branch %s is missing", errSyncFilesUnreadable, pull.HeadRef)
	}
	plan, err := orgsync.PlanFiles(repositoryID, files, overrides, target.DefaultBranch, current.Files, formatting)
	if err != nil {
		return repositoryAnswer{}, err
	}
	recorded := proposalFingerprint(pull.Body)
	if recorded == "" {
		recorded = pull.HeadRef
	}
	if len(plan.Actions) == 0 && (consolidate || recorded != fingerprint) {
		payload, err := json.Marshal(orgsync.ResolvedFile{Proposal: pull.HeadRef, Fingerprint: fingerprint, Consolidate: consolidate, ProposalOnly: true})
		if err != nil {
			return repositoryAnswer{}, err
		}
		return compared([]orgsync.Action{{RepositoryID: repositoryID, Kind: orgsync.KindFiles, Operation: orgsync.OperationUpdate, Subject: "Shared-file pull request", Before: "Proposal needs reconciliation", After: "Update the proposal and close older duplicates", Payload: payload}}), nil
	}
	if len(plan.Actions) == 0 {
		return repositoryAnswer{observation: orgsync.ObservationProposed, proposalURL: pull.URL}, nil
	}
	for index := range plan.Actions {
		file, err := orgsync.DecodeFile(plan.Actions[index].Payload)
		if err != nil {
			return repositoryAnswer{}, err
		}
		file.Proposal, file.Fingerprint = pull.HeadRef, fingerprint
		file.Consolidate = consolidate
		plan.Actions[index].Payload, err = json.Marshal(file)
		if err != nil {
			return repositoryAnswer{}, err
		}
	}
	return compared(plan.Actions), nil
}

func closeOlderFileProposals(ctx context.Context, client *github.Client, target syncTarget, retained github.PullRequest) error {
	pulls, err := client.ListPullRequestsByHeadPrefix(ctx, target.Owner, target.Name, orgsync.FileBranchPrefix, target.DefaultBranch)
	if err != nil {
		return err
	}
	// Recheck the retained PR before closing anything. A concurrent closure or
	// newer proposal requires another plan, not choosing a different survivor.
	found := false
	for _, pull := range pulls {
		if pull.State != github.PullRequestOpen || pull.Merged {
			continue
		}
		if pull.Number > retained.Number {
			return fmt.Errorf("%w: a newer file proposal appeared", orgsync.ErrInvalidPlan)
		}
		if pull.Number == retained.Number {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("%w: the retained proposal is no longer open", errSyncFilesRefused)
	}
	for _, pull := range pulls {
		if pull.State == github.PullRequestOpen && !pull.Merged && pull.Number < retained.Number {
			if err := client.ClosePullRequest(ctx, target.Owner, target.Name, pull.Number, pull.Body+"\n\nSuperseded by "+retained.URL+"\n"+supersededMarker); err != nil {
				return err
			}
		}
	}
	return nil
}

func updateProposalFingerprint(ctx context.Context, client *github.Client, target syncTarget, pull github.PullRequest, fingerprint string) error {
	if fingerprint == "" || proposalFingerprint(pull.Body) == fingerprint {
		return nil
	}
	body := pull.Body
	if old := proposalFingerprint(body); old != "" {
		body = strings.ReplaceAll(body, proposalMarker+old+" -->", "")
	}
	body += "\n" + proposalMarker + fingerprint + " -->\n"
	return client.EditPullRequest(ctx, target.Owner, target.Name, pull.Number, fileProposalTitle, body)
}

func unusedProposalBranch(pulls []github.PullRequest, fingerprint string) string {
	branch := fingerprint
	for range pulls {
		previous := branch
		for _, pull := range pulls {
			if pull.HeadRef == branch && pull.State == github.PullRequestClosed && !pull.Merged {
				branch += "-r" + strconv.Itoa(pull.Number)
			}
		}
		if previous == branch {
			break
		}
	}
	return branch
}

func selectFileProposal(pulls []github.PullRequest, fingerprint string) (*github.PullRequest, *github.PullRequest, int) {
	var open *github.PullRequest
	openCount := 0
	var latest *github.PullRequest
	for index := range pulls {
		pull := &pulls[index]
		if pull.State == github.PullRequestOpen && !pull.Merged {
			openCount++
			if open == nil || pull.Number > open.Number {
				open = pull
			}
		}
		recorded := proposalFingerprint(pull.Body)
		if recorded == "" {
			recorded = pull.HeadRef
		}
		if recorded == fingerprint && !strings.Contains(pull.Body, supersededMarker) && (latest == nil || pull.Number > latest.Number) {
			latest = pull
		}
	}
	if latest != nil && latest.State == github.PullRequestClosed && !latest.Merged {
		return open, latest, openCount
	}
	return open, nil, openCount
}
