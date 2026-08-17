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
		return asCurrentFiles(tree, config), nil
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

// asCurrentFiles reads a repository's tree as what the planner compares
// against, and names what it cannot write.
//
// The conflicts are worked out here rather than in the planner because this is
// where the whole tree is: the planner is handed one entry per path it asked
// about and has no way to see that the path above one of them is a file.
func asCurrentFiles(
	tree github.RepositoryTree,
	config orgsync.FileConfig,
) map[string]orgsync.CurrentFile {
	current := make(map[string]orgsync.CurrentFile, len(tree.Entries))

	for path, entry := range tree.Entries {
		if entry.OrdinaryFile() {
			current[path] = orgsync.CurrentFile{Blob: entry.Blob, Size: entry.Size}
		}
	}

	// Only the paths configuration names. Every other conflict in a repository
	// is somebody else's arrangement of their own files.
	for _, path := range slices.Concat(config.Paths(), config.Retired) {
		if conflict := conflictAt(tree, path); conflict != "" {
			current[path] = orgsync.CurrentFile{Conflict: conflict}
		}
	}

	return current
}

// conflictAt says why a repository cannot hold an ordinary file at a path, or
// nothing.
//
// git will let a commit put a blob where a directory, a link or a submodule is
// and say nothing about what it replaced, and it will let one put a directory
// where a file is. Both are silent destruction, and both are visible here for
// the cost of a map lookup per path segment.
func conflictAt(tree github.RepositoryTree, path string) string {
	if entry, held := tree.Entries[path]; held && !entry.OrdinaryFile() {
		return notAnOrdinaryFile(path, entry.Mode)
	}

	for parent := parentOf(path); parent != ""; parent = parentOf(parent) {
		if entry, held := tree.Entries[parent]; held && !entry.Directory() {
			return blockedByFile(path, parent)
		}
	}

	return ""
}

// The two ways a repository can be unable to hold a file where the
// configuration puts one, said the same way whichever read found it.
func notAnOrdinaryFile(path, mode string) string {
	return fmt.Sprintf(
		"%s is not an ordinary file in this repository; git records it as %s", path, mode)
}

func blockedByFile(path, parent string) string {
	return fmt.Sprintf(
		"%s cannot be written because %s is not a directory in this repository", path, parent)
}

// parentOf is the directory a path sits in, empty at the repository root.
func parentOf(path string) string {
	cut := strings.LastIndex(path, "/")
	if cut < 0 {
		return ""
	}

	return path[:cut]
}

func readFilesOneAtATime(
	ctx context.Context,
	client *github.Client,
	target syncTarget,
	config orgsync.FileConfig,
) (map[string]orgsync.CurrentFile, error) {
	current := map[string]orgsync.CurrentFile{}

	for _, path := range slices.Concat(config.Paths(), config.Retired) {
		found, err := client.ResolveTreePath(
			ctx, target.Owner, target.Name, target.DefaultBranch, path)
		if err != nil {
			return nil, err
		}

		switch {
		case found.Blocked != "":
			current[path] = orgsync.CurrentFile{Conflict: blockedByFile(path, found.Blocked)}

		case !found.Found:

		case !found.Entry.OrdinaryFile():
			current[path] = orgsync.CurrentFile{
				Conflict: notAnOrdinaryFile(path, found.Entry.Mode),
			}

		default:
			current[path] = orgsync.CurrentFile{
				Blob: found.Entry.Blob, Size: found.Entry.Size,
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
	if override == nil || override.AdjustsNothing() {
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

		// Every action of one repository's file work names the same branch,
		// because one planner call wrote them all. Two that disagree is a plan
		// nothing can carry out, and taking whichever came last would split one
		// change across two pull requests without saying so.
		switch {
		case proposal == "":
			proposal = planned.Proposal

		case planned.Proposal != proposal:
			return fmt.Errorf("%w: this repository's file work names two branches, %s and %s",
				orgsync.ErrInvalidPlan, proposal, planned.Proposal)
		}

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
	case pull.Merged && head == pull.Head:
		// Merged, and nothing has been pushed since. Everything the branch says
		// is in the default branch now, so a pull request from it would have no
		// diff at all and GitHub refuses to open one. Taking it away is what
		// lets the next change start from the default branch, which is what an
		// ordinary branch does after a merge.
		if head != "" {
			if err := client.DeleteRef(
				ctx, target.Owner, target.Name, "heads/"+proposal); err != nil {
				return "", nil, err
			}
		}

		return "", nil, nil

	case pull.Merged:
		// Merged, and somebody has pushed to it since. Their commit is the
		// whole of what a pull request from here would carry, and taking the
		// branch away would take it with them. Built on, like every other
		// branch that has something on it.
		logging.From(ctx).Info(
			"the merged proposal branch has moved since; building on it rather than replacing it",
			"branch", proposal)

		return head, nil, nil

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

	wanted, err := s.stillNeeded(ctx, client, target, parentCommit.Tree, files)
	if err != nil {
		return "", err
	}

	changes, err := s.writeBlobs(ctx, client, target, wanted)
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

// stillNeeded leaves out a removal the tree being built on has already made.
//
// The plan is computed against the default branch and the commit is built on
// the proposal branch, which already carries whatever an earlier tick put
// there. A tree entry removing a path that is not in the tree it is built from
// is a 422, so a repository with a retired path would fail every time its
// proposal came round again - which it does, on the reconcile horizon, for as
// long as the pull request sits unmerged.
//
// Only the removals. A blob written for a file the tree already has is a
// request for nothing, and the tree it produces is the one it started from, so
// it costs a request rather than an outcome.
func (s *server) stillNeeded(
	ctx context.Context,
	client *github.Client,
	target syncTarget,
	tree string,
	files []plannedFile,
) ([]plannedFile, error) {
	wanted := make([]plannedFile, 0, len(files))

	for _, file := range files {
		if !file.remove {
			wanted = append(wanted, file)

			continue
		}

		// Walked rather than listed, because the answer has to be exact for
		// exactly the paths being removed, and there are rarely more than a
		// couple of them.
		found, err := client.ResolveTreePath(ctx, target.Owner, target.Name, tree, file.path)
		if err != nil {
			return nil, err
		}

		if found.Found {
			wanted = append(wanted, file)
		}
	}

	return wanted, nil
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
