package apply

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

// readTreePaths reads what a ref holds at each of several paths.
//
// One request. git names an object by hashing its contents, so a listing of the
// tree answers whether each managed file already says what it should - where
// the tool this replaces downloaded every file from every repository on every
// run, and treated a failed download as a file to skip.
//
// One function for both refs a file sync asks about. The default branch is what
// the plan is computed from and the proposal branch is what the commit is built
// on, and they are the same question about a different ref: reading them two
// ways is how they came to two answers.
func readTreePaths(
	ctx context.Context,
	client *github.Client,
	target syncTarget,
	ref string,
	paths []string,
) (treeContents, error) {
	tree, err := client.ListRepositoryTree(ctx, target.Owner, target.Name, ref)
	if err != nil {
		return treeContents{}, err
	}

	if tree.Missing {
		// No tree at all, which is not the same as a tree holding none of these
		// paths. Said rather than flattened into "every file is absent",
		// because the caller deciding what to do about it needs the difference.
		return treeContents{Missing: true}, nil
	}

	if tree.Truncated {
		// GitHub declines to list a tree past a hundred thousand entries, and a
		// path missing from a listing that stopped early is not a path a
		// repository does not have. Walking each path settles it: a handful of
		// requests, for the handful of repositories that are that large.
		logging.From(ctx).Info(
			"this tree is too large to list, so its paths are walked a level at a time",
			"ref", ref, "paths", len(paths))

		return walkTreePaths(ctx, client, target, ref, paths)
	}

	// Only the paths asked about. Every other conflict in a repository is
	// somebody else's arrangement of their own files, and carrying the rest of
	// the tree here handed the planner a map four orders of magnitude larger
	// than the walk hands it for the same repository.
	current := make(map[string]orgsync.CurrentFile, len(paths))

	for _, path := range paths {
		if held, has := currentFileAt(path, tree.At(path)); has {
			current[path] = held
		}
	}

	return treeContents{Files: current}, nil
}

// treeContents is what a ref holds at the paths asked about, and whether it
// holds anything at all.
type treeContents struct {
	Files map[string]orgsync.CurrentFile

	// Missing is there being no tree at that ref: a repository with no commits,
	// or a branch nothing has pushed to.
	Missing bool
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

func walkTreePaths(
	ctx context.Context,
	client *github.Client,
	target syncTarget,
	ref string,
	paths []string,
) (treeContents, error) {
	resolved, err := client.ResolveTreePaths(
		ctx, target.Owner, target.Name, ref, paths)
	if err != nil {
		return treeContents{}, err
	}

	current := make(map[string]orgsync.CurrentFile, len(resolved))

	for path, found := range resolved {
		if held, has := currentFileAt(path, found); has {
			current[path] = held
		}
	}

	return treeContents{Files: current}, nil
}

// currentFileAt reads one resolved tree path as what the planner compares
// against, answering false where the repository has nothing there at all.
//
// One function, because a path has to mean the same thing wherever it was
// read: from a whole listing, walked a level at a time, or looked at again on
// the branch a commit is built on. Written out three times it came to two
// answers - a retired path whose parent had become a file refused the whole
// repository at plan time and was quietly skipped at apply time.
func currentFileAt(filePath string, found github.TreePath) (orgsync.CurrentFile, bool) {
	switch {
	case found.Blocked != "":
		return orgsync.CurrentFile{
			Conflict: blockedByFile(filePath, found.Blocked),
			Blocked:  true,
		}, true

	case !found.Found:
		return orgsync.CurrentFile{}, false

	case !found.Entry.OrdinaryFile():
		return orgsync.CurrentFile{
			Conflict: notAnOrdinaryFile(filePath, found.Entry.Mode),
		}, true

	default:
		return orgsync.CurrentFile{Blob: found.Entry.Blob, Size: found.Entry.Size}, true
	}
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
func applyFileActions(
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

	return proposeFiles(ctx, client, target, proposal, files)
}

// proposeFiles builds the commit and the pull request that carries it.
//
// Nothing is ever force-pushed. A commit is built on whatever the branch
// already points at and the reference is moved forward, so a reviewer's fixup
// is a commit this one descends from rather than something that disappears. The
// tool this replaces rebuilt the branch from the default branch on every run
// and force-updated the reference, which destroyed anything anybody had pushed
// to it - with no error and no trace.
func proposeFiles(
	ctx context.Context,
	client *github.Client,
	target syncTarget,
	proposal string,
	files []plannedFile,
) error {
	if target.DefaultBranch == "" {
		return fmt.Errorf("%w: GitHub named no default branch", errSyncFilesUnreadable)
	}

	branch, err := readProposal(ctx, client, target, proposal)
	if err != nil {
		return err
	}

	changed, err := commitFiles(ctx, client, target, proposal, branch, files)
	if err != nil {
		return err
	}

	if !changed && branch.BuildOn == "" {
		// Built on the default branch and changed nothing, so the default
		// branch already says what it should: somebody landed the same change
		// between the plan and now, or a merged proposal is being replayed.
		// There is nothing to propose and nothing to open.
		logging.From(ctx).Info("the files already say what they should; nothing proposed")

		return nil
	}

	return openOrUpdateProposal(ctx, client, target, proposal, branch.Pull, files)
}

// proposalBranch is where a repository's file work stands.
type proposalBranch struct {
	// Head is what the branch points at, empty where there is no branch. It is
	// what the move is made against, so a push that lands in between is
	// refused rather than overwritten.
	Head string

	// BuildOn is the commit the next one descends from, empty for the default
	// branch.
	//
	// The branch's own tip wherever there is work on it to keep, and the
	// default branch wherever the branch has been merged. What merged is in
	// the default branch already, so building on the tip again produces a
	// commit carrying nothing and a pull request GitHub will not open - and
	// reading that refusal as "the files are right" recorded a repository as
	// matching while its default branch had since been changed back.
	BuildOn string

	// Pull is the open pull request to keep describing, if there is one.
	Pull *github.PullRequest
}

// readProposal answers where a repository's proposal branch stands, resolving
// the states a previous run can have left behind.
func readProposal(
	ctx context.Context,
	client *github.Client,
	target syncTarget,
	proposal string,
) (proposalBranch, error) {
	head, err := client.GetRef(ctx, target.Owner, target.Name, "heads/"+proposal)
	if err != nil {
		return proposalBranch{}, err
	}

	pull, err := client.FindPullRequestByHead(
		ctx, target.Owner, target.Name, proposal, target.DefaultBranch)
	if err != nil {
		return proposalBranch{}, err
	}

	if pull == nil {
		// A branch with no pull request is what an earlier run that pushed and
		// then died leaves behind. It is built on rather than abandoned.
		return proposalBranch{Head: head, BuildOn: head}, nil
	}

	switch {
	case pull.State == github.PullRequestClosed && !pull.Merged:
		return proposalBranch{}, fmt.Errorf(
			"%w: pull request %d", errSyncFilesRefused, pull.Number)

	case pull.Merged:
		// The branch stays where it is - nothing here removes one. GitHub's
		// delete has no compare-and-swap, unlike the move below, which it
		// refuses when it is not a fast-forward, so a commit landing between
		// reading a branch and removing it would be gone with no error and no
		// trace. It is left alone and built past instead: a commit on the
		// default branch tip still descends from a merged tip, so the move is
		// a fast-forward and nothing on the branch is lost.
		return proposalBranch{Head: head}, nil

	default:
		return proposalBranch{Head: head, BuildOn: head, Pull: pull}, nil
	}
}

// commitFiles writes the change and moves the branch to it, answering whether
// there was anything to write - what it would build on may already say it.
func commitFiles(
	ctx context.Context,
	client *github.Client,
	target syncTarget,
	proposal string,
	branch proposalBranch,
	files []plannedFile,
) (changed bool, err error) {
	parent := branch.BuildOn
	if parent == "" {
		base, err := client.GetRef(
			ctx, target.Owner, target.Name, "heads/"+target.DefaultBranch)
		if err != nil {
			return false, err
		}

		if base == "" {
			return false, fmt.Errorf(
				"%w: the default branch has no commits", errSyncFilesUnreadable)
		}

		parent = base
	}

	// A tree is built from a tree, and a reference points at a commit. Peeling
	// one to the other is a request rather than a guess at what GitHub accepts.
	parentCommit, err := client.GetCommit(ctx, target.Owner, target.Name, parent)
	if err != nil {
		return false, err
	}

	wanted, err := stillNeeded(ctx, client, target, parentCommit.Tree, files)
	if err != nil {
		return false, err
	}

	changes, err := writeBlobs(ctx, client, target, wanted)
	if err != nil {
		return false, err
	}

	if len(changes) == 0 {
		// Nothing left to write: every path this would change already says what
		// it should on the tree being built from, which is what a proposal
		// branch that already carries the whole change looks like.
		//
		// Answered here rather than by asking. GitHub documents the entry list
		// as required, so a tree built from none of them either fails or hands
		// back the tree it was given, and a repository's proposal should not
		// turn on which - the answer is known without the request.
		return false, nil
	}

	tree, err := client.CreateTree(
		ctx, target.Owner, target.Name, parentCommit.Tree, changes)
	if err != nil {
		return false, err
	}

	if tree == parentCommit.Tree {
		// Everything this would write is already there. Committing anyway would
		// add an empty commit to somebody's branch on every reconcile.
		return false, nil
	}

	// Both, where a spent branch is being built past. GitHub squashes and
	// rebases as well as merging, and after either of those the branch's tip is
	// not in the default branch at all - so a commit built on the default
	// branch does not descend from it, and moving the branch there is not a
	// fast-forward. GitHub refuses that, and the repository is then stuck for
	// ever, re-planning and re-failing every horizon. Naming the old tip as a
	// second parent is what makes the move a fast-forward whichever way the
	// repository merged, without forcing anything or removing anything.
	parents := []string{parent}
	if branch.BuildOn == "" && branch.Head != "" {
		parents = append(parents, branch.Head)
	}

	commit, err := client.CreateCommit(
		ctx, target.Owner, target.Name, fileCommitMessage, tree, parents...)
	if err != nil {
		return false, err
	}

	return true, moveProposal(ctx, client, target, proposal, branch.Head, commit)
}

// moveProposal points the branch at the new commit.
//
// Never forced. The commit descends from what the branch pointed at when this
// read it - directly where it was built on the branch, and through the default
// branch where a merged tip was built past, since the default branch holds that
// tip already. So GitHub accepts the move, and refuses it if somebody pushed in
// between, which is exactly the answer that should stop this from overwriting
// them. The next reconcile builds on what they pushed.
func moveProposal(
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

// stillNeeded narrows the plan to what the tree being built on can still take.
//
// The plan is computed against the default branch and the commit is built on
// the proposal branch, which is a different tree: it carries whatever an
// earlier tick put there, and whatever anybody with push rights put there
// afterwards. So every question the plan asked of the default branch is asked
// again here, of the tree the commit is actually made from.
//
// Read the same way the planner reads the default branch - one listing, walked
// a level at a time only where GitHub declines to finish it - so the two cannot
// come to different answers about the same tree.
func stillNeeded(
	ctx context.Context,
	client *github.Client,
	target syncTarget,
	tree string,
	files []plannedFile,
) ([]plannedFile, error) {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.path)
	}

	// Read the same way the planner reads the default branch, so the two
	// cannot come to different answers about one tree state.
	current, err := readTreePaths(ctx, client, target, tree, paths)
	if err != nil {
		return nil, err
	}

	wanted := make([]plannedFile, 0, len(files))

	for _, file := range files {
		held, has := current.Files[file.path]

		// A removal has nothing to remove where the path is gone or unreachable,
		// and it is left out rather than refused: a tree entry removing a path
		// that is not in the tree it is built from is a 422, so a repository
		// with a retired path an earlier tick removed would fail every time its
		// proposal came round again. A write in the same place is refused,
		// because writing there would take what is in the way with it.
		if file.remove && (!has || held.Blocked) {
			continue
		}

		if held.Conflict != "" {
			return nil, fmt.Errorf("%w: %s", orgsync.ErrRepositoryConflict, held.Conflict)
		}

		wanted = append(wanted, file)
	}

	return wanted, nil
}

func writeBlobs(
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

func openOrUpdateProposal(
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
		// A branch carrying nothing the default branch does not is refused
		// here, and that refusal is a failure rather than an answer. Reaching
		// this means the planner found the default branch wanting, so "there
		// is nothing between them" says the branch is stale, never that the
		// files are right - reading it as success recorded a repository as
		// matching while what it should hold was missing, and the branch is
		// named after the outcome, so nothing would ever ask again.
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
