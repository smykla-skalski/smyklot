package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/logging"
)

const (
	// fileCommitMessage is what the sync commit says. Conventional, because the
	// repositories it lands in check that their commits are.
	fileCommitMessage = "chore(sync): update the shared files"

	// fileProposalTitle is what the pull request is called. It becomes the
	// commit subject wherever the repository squashes.
	fileProposalTitle = "chore(sync): update the shared files"
)

var (
	// errSyncFilesUnreadable is a repository whose files cannot be compared:
	// GitHub named no default branch for it, or declined to list its tree.
	errSyncFilesUnreadable = errors.New("this repository's files cannot be read")

	// errSyncFilesRefused is a proposal the repository closed between the plan
	// being computed and the work running.
	//
	// A failure rather than a silence. Nothing happened, so recording the
	// actions as applied would say a repository matches when it does not; the
	// next reconcile finds the closed pull request where the planner looks for
	// it and settles the repository there, which is where that answer belongs.
	errSyncFilesRefused = errors.New("this repository closed the pull request for this change")
)

// readRepositoryFiles reads what a repository currently has at every path the
// configuration cares about.
//
// One request. git names an object by hashing its contents, so a listing of the
// tree answers whether each managed file already says what it should - where
// the tool this replaces downloaded every file from every repository on every
// run, and treated a failed download as a file to skip.
func readRepositoryFiles(
	ctx context.Context,
	client *github.Client,
	target syncTarget,
	config orgsync.FileConfig,
) (map[string]orgsync.CurrentFile, error) {
	tree, err := client.ListRepositoryTree(
		ctx, target.Owner, target.Name, target.DefaultBranch)
	if err != nil {
		return nil, err
	}

	if !tree.Truncated {
		return asCurrentFiles(tree.Blobs), nil
	}

	// GitHub declines to list a tree past a hundred thousand entries, and a
	// path missing from a listing that stopped early is not a path a repository
	// does not have. Reading each managed path settles it: a handful of
	// requests, for the handful of repositories that are that large.
	logging.From(ctx).Info(
		"this repository's tree is too large to list, so its files are read one at a time",
		"paths", len(config.Files)+len(config.Retired))

	return readFilesOneAtATime(ctx, client, target, config)
}

func asCurrentFiles(blobs map[string]github.TreeBlob) map[string]orgsync.CurrentFile {
	current := make(map[string]orgsync.CurrentFile, len(blobs))
	for path, blob := range blobs {
		current[path] = orgsync.CurrentFile{Blob: blob.Blob, Size: blob.Size}
	}

	return current
}

func readFilesOneAtATime(
	ctx context.Context,
	client *github.Client,
	target syncTarget,
	config orgsync.FileConfig,
) (map[string]orgsync.CurrentFile, error) {
	current := map[string]orgsync.CurrentFile{}

	for _, path := range slices.Concat(config.Paths(), config.Retired) {
		content, found, err := client.GetRepositoryFile(
			ctx, target.Owner, target.Name, target.DefaultBranch, path)
		if err != nil {
			return nil, err
		}

		if found {
			current[path] = orgsync.CurrentFile{
				Blob: orgsync.BlobID(content), Size: len(content),
			}
		}
	}

	return current, nil
}

// decodeFileOverride reads what one repository adjusts about its files.
//
// Validated against the installation's configuration, not only decoded. The
// panel checks what somebody types; this covers a row written before a rule
// existed, and an adjustment naming a file nobody syncs is the same silence as
// a mistyped path - it reads as configured and quietly leaves the repository
// with the plain template.
func decodeFileOverride(
	override *orgsync.RepositoryOverride,
	config orgsync.FileConfig,
) (orgsync.FileOverride, error) {
	if override == nil || len(override.Document) == 0 {
		return orgsync.FileOverride{}, nil
	}

	var adjustments orgsync.FileOverride
	if err := json.Unmarshal(override.Document, &adjustments); err != nil {
		return orgsync.FileOverride{}, fmt.Errorf("decode file adjustments: %w", err)
	}

	if err := adjustments.Validate(config); err != nil {
		return orgsync.FileOverride{}, err
	}

	return adjustments, nil
}

// plannedFile is one path a proposal writes or removes.
//
// Whether it is a removal comes off the action rather than off an empty
// payload: a file with nothing in it and a file that should not be there are
// different requests, and telling them apart by whether some bytes are missing
// is how they come to be the same one.
type plannedFile struct {
	path    string
	content []byte
	remove  bool
}

// applyFileActions puts one repository's whole file change behind one pull
// request.
//
// One commit for all of it, which is what makes the ordering hazards go away
// rather than have to be handled: the tool this replaces scheduled its
// deletions before it had fetched what would replace them, so a fetch that
// failed left a repository with neither. A commit lands whole or not at all.
func (s *server) applyFileActions(
	ctx context.Context,
	client *github.Client,
	target syncTarget,
	actions []orgsync.Action,
) error {
	var (
		files    []plannedFile
		proposal string
	)

	for _, action := range actions {
		planned, err := orgsync.DecodeFile(action.Payload)
		if err != nil {
			return err
		}

		proposal = planned.Proposal
		files = append(files, plannedFile{
			path:    planned.Path,
			content: planned.Content,
			remove:  action.Operation == orgsync.OperationDelete,
		})
	}

	if proposal == "" {
		return fmt.Errorf("%w: no branch to propose the files on", orgsync.ErrInvalidPlan)
	}

	return s.proposeFiles(ctx, client, target, proposal, files)
}

// proposeFiles builds the commit and the pull request that carries it.
//
// Nothing is ever force-pushed. A commit is built on whatever the branch
// already points at and the reference is moved forward, so a reviewer's fixup
// is a commit this one descends from rather than something that disappears. The
// tool this replaces rebuilt the branch from the default branch on every run
// and force-updated the reference, which destroyed anything anybody had pushed
// to it - with no error and no trace.
func (s *server) proposeFiles(
	ctx context.Context,
	client *github.Client,
	target syncTarget,
	proposal string,
	files []plannedFile,
) error {
	if target.DefaultBranch == "" {
		return fmt.Errorf("%w: GitHub named no default branch", errSyncFilesUnreadable)
	}

	head, pull, err := s.readProposal(ctx, client, target, proposal)
	if err != nil {
		return err
	}

	commit, err := s.commitFiles(ctx, client, target, proposal, head, files)
	if err != nil {
		return err
	}

	if commit == "" && head == "" {
		// Somebody landed the same change on the default branch between the
		// plan and now, so there is nothing to propose and nothing to open.
		logging.From(ctx).Info("the files already say what they should; nothing proposed")

		return nil
	}

	return s.openOrUpdateProposal(ctx, client, target, proposal, pull, files)
}

// readProposal answers where a repository's proposal branch stands, resolving
// the states a previous run can have left behind.
func (s *server) readProposal(
	ctx context.Context,
	client *github.Client,
	target syncTarget,
	proposal string,
) (head string, pull *github.PullRequest, err error) {
	head, err = client.GetRef(ctx, target.Owner, target.Name, "heads/"+proposal)
	if err != nil {
		return "", nil, err
	}

	pull, err = client.FindPullRequestByHead(ctx, target.Owner, target.Name, proposal)
	if err != nil {
		return "", nil, err
	}

	if pull == nil {
		// A branch with no pull request is what an earlier run that pushed and
		// then died leaves behind. It is built on rather than abandoned.
		return head, nil, nil
	}

	switch {
	case pull.Merged:
		// The branch is already in the default branch, so building on it would
		// propose a diff against something that has moved. Starting again from
		// the default branch is what an ordinary branch does after a merge.
		if head != "" {
			if err := client.DeleteRef(
				ctx, target.Owner, target.Name, "heads/"+proposal); err != nil {
				return "", nil, err
			}
		}

		return "", nil, nil

	case pull.State == github.PullRequestClosed:
		return "", nil, fmt.Errorf("%w: pull request %d", errSyncFilesRefused, pull.Number)

	default:
		return head, pull, nil
	}
}

// commitFiles writes the change and moves the branch to it, answering with the
// commit or with nothing where the branch already said what it should.
func (s *server) commitFiles(
	ctx context.Context,
	client *github.Client,
	target syncTarget,
	proposal, head string,
	files []plannedFile,
) (string, error) {
	// Built on the branch where there is one, so nothing that was pushed to it
	// is lost, and on the default branch where there is not.
	parent := head
	if parent == "" {
		base, err := client.GetRef(
			ctx, target.Owner, target.Name, "heads/"+target.DefaultBranch)
		if err != nil {
			return "", err
		}

		if base == "" {
			return "", fmt.Errorf("%w: the default branch has no commits", errSyncFilesUnreadable)
		}

		parent = base
	}

	// A tree is built from a tree, and a reference points at a commit. Peeling
	// one to the other is a request rather than a guess at what GitHub accepts.
	parentCommit, err := client.GetCommit(ctx, target.Owner, target.Name, parent)
	if err != nil {
		return "", err
	}

	changes, err := s.writeBlobs(ctx, client, target, files)
	if err != nil {
		return "", err
	}

	tree, err := client.CreateTree(
		ctx, target.Owner, target.Name, parentCommit.Tree, changes)
	if err != nil {
		return "", err
	}

	if tree == parentCommit.Tree {
		// Everything this would write is already there. Committing anyway would
		// add an empty commit to somebody's branch on every reconcile.
		return "", nil
	}

	commit, err := client.CreateCommit(
		ctx, target.Owner, target.Name, fileCommitMessage, tree, parent)
	if err != nil {
		return "", err
	}

	return commit, s.moveProposal(ctx, client, target, proposal, head, commit)
}

// moveProposal points the branch at the new commit.
//
// Never forced. The commit descends from what the branch pointed at when this
// read it, so GitHub accepts the move - and refuses it if somebody pushed in
// between, which is exactly the answer that should stop this from overwriting
// them. The next reconcile builds on what they pushed.
func (s *server) moveProposal(
	ctx context.Context,
	client *github.Client,
	target syncTarget,
	proposal, head, commit string,
) error {
	if head == "" {
		return client.CreateRef(ctx, target.Owner, target.Name, "heads/"+proposal, commit)
	}

	return client.UpdateRef(ctx, target.Owner, target.Name, "heads/"+proposal, commit, false)
}

func (s *server) writeBlobs(
	ctx context.Context,
	client *github.Client,
	target syncTarget,
	files []plannedFile,
) ([]github.TreeChange, error) {
	changes := make([]github.TreeChange, 0, len(files))

	for _, file := range files {
		if file.remove {
			// An empty blob is how a tree entry spells a deletion.
			changes = append(changes, github.TreeChange{Path: file.path})

			continue
		}

		blob, err := client.CreateBlob(ctx, target.Owner, target.Name, file.content)
		if err != nil {
			return nil, err
		}

		changes = append(changes, github.TreeChange{Path: file.path, Blob: blob})
	}

	return changes, nil
}

func (s *server) openOrUpdateProposal(
	ctx context.Context,
	client *github.Client,
	target syncTarget,
	proposal string,
	pull *github.PullRequest,
	files []plannedFile,
) error {
	body := fileProposalBody(files)

	if pull != nil {
		// Kept current rather than left as it was written. A proposal sits
		// until somebody merges it, and what it would do moves as the
		// repository does.
		return client.EditPullRequest(
			ctx, target.Owner, target.Name, pull.Number, fileProposalTitle, body)
	}

	opened, err := client.CreatePullRequest(
		ctx, target.Owner, target.Name, github.NewPullRequest{
			Title: fileProposalTitle,
			Body:  body,
			Head:  proposal,
			Base:  target.DefaultBranch,
		})
	if err != nil {
		return err
	}

	logging.From(ctx).Info("files proposed", "pull_request", opened.Number)

	return nil
}

// fileProposalBody says what the proposal does, and what closing it means.
//
// What closing it means, because that is the only way to refuse: the branch is
// named after what the files should end up saying, so a closed pull request
// stops this asking again - and a configuration that changes is a different
// branch, which asks once more.
func fileProposalBody(files []plannedFile) string {
	var body strings.Builder

	body.WriteString("Smyklot keeps these files in step across the organization.\n")

	removed := paths(files, func(file plannedFile) bool { return file.remove })
	written := paths(files, func(file plannedFile) bool { return !file.remove })

	writeProposalSection(&body, "Written", written)
	writeProposalSection(&body, "Removed", removed)

	if len(removed) > 0 {
		body.WriteString("\n> [!CAUTION]\n")
		body.WriteString("> This removes files. Read it before merging.\n")
	}

	body.WriteString(
		"\nClosing this tells Smyklot not to propose this change again. " +
			"Change the configuration in the panel and it will propose the new one.\n")

	return body.String()
}

func writeProposalSection(body *strings.Builder, heading string, files []string) {
	if len(files) == 0 {
		return
	}

	fmt.Fprintf(body, "\n## %s\n\n", heading)

	for _, path := range files {
		fmt.Fprintf(body, "- `%s`\n", path)
	}
}

func paths(files []plannedFile, want func(plannedFile) bool) []string {
	found := make([]string, 0, len(files))

	for _, file := range files {
		if want(file) {
			found = append(found, file.path)
		}
	}

	slices.Sort(found)

	return found
}
