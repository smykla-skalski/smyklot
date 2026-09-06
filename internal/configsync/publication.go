package configsync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

// Proposal is persisted before a branch may be created or advanced. A retry can
// recognize its own immutable commit, but may never adopt an unknown branch tip.
type Proposal struct {
	Branch       string `json:"branch"`
	PreviousHead string `json:"previous_head,omitempty"`
	Commit       string `json:"commit"`
	DefaultHead  string `json:"default_head"`
	Digest       string `json:"digest"`
	Path         string `json:"path"`
	Number       int    `json:"number,omitempty"`
	URL          string `json:"url,omitempty"`
}

func proposalBranch(scope config.PanelFileScope) string {
	if scope == config.PanelFileWorkspace {
		return "smyklot/workspace-configuration"
	}
	return "smyklot/repository-configuration"
}

// PrepareProposal creates unreachable git objects only. The caller must first
// validate/import the semantic document and persist this intent with the panel
// revision before calling PublishProposal. No reference is moved here.
func PrepareProposal(
	ctx context.Context, client *github.Client, file RemoteFile, semantic []byte, previous *Proposal,
) (Proposal, error) {
	content, err := renderPublication(file, semantic)
	if err != nil {
		return Proposal{}, err
	}
	location := file.Location
	if _, err := location.paths(); err != nil {
		return Proposal{}, err
	}
	proposal := Proposal{Branch: proposalBranch(location.Scope), DefaultHead: file.Head, Path: file.WritePath}
	digest := sha256.Sum256(content)
	proposal.Digest = hex.EncodeToString(digest[:])
	proposal.PreviousHead, err = client.GetRef(ctx, location.Owner, location.Repository, "heads/"+proposal.Branch)
	if err != nil {
		return Proposal{}, err
	}
	if proposal.PreviousHead != "" && !validObjectID(proposal.PreviousHead) {
		return Proposal{}, errors.New("GitHub returned an invalid proposal commit ID")
	}
	if proposal.PreviousHead != "" && (previous == nil || previous.Branch != proposal.Branch ||
		(proposal.PreviousHead != previous.Commit && proposal.PreviousHead != previous.PreviousHead)) {
		return Proposal{}, &BlockedError{Code: "proposal_edited", Message: "The configuration proposal was changed on GitHub"}
	}
	pull, err := usableProposal(ctx, client, location, proposal.Branch)
	if err != nil {
		return Proposal{}, err
	}
	if pull != nil && pull.State == github.PullRequestOpen && proposal.PreviousHead == "" {
		return Proposal{}, &BlockedError{Code: "proposal_removed", Message: "The configuration proposal's branch was removed"}
	}
	proposal.Commit, err = prepareCommit(ctx, client, file, proposal, content)
	return proposal, err
}

func prepareCommit(ctx context.Context, client *github.Client, file RemoteFile, proposal Proposal, content []byte) (string, error) {
	location := file.Location
	if !validObjectID(file.Head) || file.WritePath == "" || (file.Migrate && file.Path == "") {
		return "", errors.New("configuration publication needs a complete remote observation")
	}
	base, err := client.GetCommit(ctx, location.Owner, location.Repository, file.Head)
	if err != nil {
		return "", err
	}
	if !validObjectID(base.Tree) {
		return "", errors.New("configuration publication could not read its base tree")
	}
	blob, err := client.CreateBlob(ctx, location.Owner, location.Repository, content)
	if err != nil {
		return "", err
	}
	if !validObjectID(blob) {
		return "", errors.New("GitHub did not return the new configuration blob ID")
	}
	changes := []github.TreeChange{{Path: file.WritePath, Blob: blob}}
	if file.Migrate && file.Path != file.WritePath {
		changes = append(changes, github.TreeChange{Path: file.Path})
	}
	tree, err := client.CreateTree(ctx, location.Owner, location.Repository, base.Tree, changes)
	if err != nil {
		return "", err
	}
	if !validObjectID(tree) {
		return "", errors.New("GitHub did not return the new configuration tree ID")
	}
	parents := []string{file.Head}
	if proposal.PreviousHead != "" && proposal.PreviousHead != file.Head {
		parents = append(parents, proposal.PreviousHead)
	}
	commit, err := client.CreateCommit(ctx, location.Owner, location.Repository,
		"chore(smyklot): sync configuration settings", tree, parents...)
	if err == nil && !validObjectID(commit) {
		return "", errors.New("GitHub did not return the new configuration commit ID")
	}
	return commit, err
}

func validObjectID(value string) bool {
	if len(value) != 40 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func usableProposal(ctx context.Context, client *github.Client, location RemoteLocation, branch string) (*github.PullRequest, error) {
	pull, err := client.FindPullRequestByHead(ctx, location.Owner, location.Repository, branch, location.DefaultBranch)
	if err != nil {
		return nil, err
	}
	if pull != nil && pull.State == github.PullRequestClosed && !pull.Merged {
		return nil, &BlockedError{Code: "proposal_closed", Message: "The configuration pull request was closed without merging"}
	}
	return pull, nil
}

// PublishProposal runs only after its intent has been durably recorded. It
// rechecks both branch tips and uses only non-forced updates. An uncertain push
// or pull-request response can be retried with the same stored proposal.
func PublishProposal(ctx context.Context, client *github.Client, location RemoteLocation, proposal Proposal) (Proposal, error) {
	if _, err := location.paths(); err != nil {
		return proposal, err
	}
	if proposal.Branch != proposalBranch(location.Scope) || !validObjectID(proposal.Commit) ||
		!validObjectID(proposal.DefaultHead) || (proposal.PreviousHead != "" && !validObjectID(proposal.PreviousHead)) {
		return proposal, errors.New("configuration publication does not match its connection")
	}
	head, err := client.GetRef(ctx, location.Owner, location.Repository, "heads/"+location.DefaultBranch)
	if err != nil {
		return proposal, err
	}
	if head != proposal.DefaultHead {
		return proposal, &BlockedError{Code: "source_changed", Message: "The default branch changed while the configuration was being prepared"}
	}
	tip, err := client.GetRef(ctx, location.Owner, location.Repository, "heads/"+proposal.Branch)
	if err != nil {
		return proposal, err
	}
	if tip != proposal.Commit && tip != proposal.PreviousHead {
		return proposal, &BlockedError{Code: "proposal_edited", Message: "The configuration proposal was changed on GitHub"}
	}
	pull, err := usableProposal(ctx, client, location, proposal.Branch)
	if err != nil {
		return proposal, err
	}
	if tip != proposal.Commit {
		if tip == "" {
			err = client.CreateRef(ctx, location.Owner, location.Repository, "heads/"+proposal.Branch, proposal.Commit)
		} else {
			err = client.UpdateRef(ctx, location.Owner, location.Repository, "heads/"+proposal.Branch, proposal.Commit, false)
		}
		if err != nil {
			return proposal, err
		}
	}
	if pull == nil || pull.Merged {
		opened, err := client.CreatePullRequest(ctx, location.Owner, location.Repository, github.NewPullRequest{
			Title: "chore(smyklot): sync configuration settings", Head: proposal.Branch, Base: location.DefaultBranch,
			Body: fmt.Sprintf("Sync `%s` with settings saved in the Smyklot panel.\n\n"+
				"After this merges, changes to either side are synchronized. Overlapping edits need a choice in the panel. "+
				"Closing this pull request pauses publication until it is reopened.", proposal.Path),
		})
		if err != nil {
			return proposal, err
		}
		pull = &opened
	}
	proposal.Number, proposal.URL = pull.Number, pull.URL
	return proposal, nil
}
