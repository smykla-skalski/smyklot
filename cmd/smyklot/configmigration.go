package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/logging"
)

const (
	// migrationBranch prefixes immutable, content-addressed proposal branches.
	// A retry built from a newer default branch gets a new name rather than
	// replacing the branch a maintainer may still be working on.
	migrationBranch = "smyklot/config-toml-migration"

	// migrationTarget is where the converted file goes: the first path
	// discovery looks in, and the one the documentation asks for.
	migrationTarget = ".smyklot.toml"

	migrationTitle = "Move Smyklot's configuration to TOML"

	migrationCommit = "chore(smyklot): move configuration to TOML"

	// migrationHeader is what the file says about itself, months later, to
	// somebody who was not here when the pull request arrived.
	//
	// It goes on the file the migration writes rather than into RenderTOML,
	// because it is context for a human reading an unsolicited change, not part
	// of serialising settings.
	//
	// The schema directive is first because that is the only line taplo reads
	// it on, and it names the published URL rather than this deployment's own:
	// the file is somebody else's, and it has to resolve from a laptop that has
	// never heard of the installation that wrote it.
	migrationHeader = "#:schema " + config.SchemaURL + "\n" +
		"# Smyklot reads this file from the default branch.\n" +
		"# https://github.com/smykla-skalski/smyklot#bot-configuration\n\n"
)

// migrationBody is what the pull request says, with the file it is moving.
//
// It says what happens if nobody does anything, because that is the first thing
// somebody reads an unsolicited pull request to find out.
const migrationBody = "Smyklot reads its configuration from TOML now. " +
	"This moves `%s` to `" + migrationTarget + "` without changing what it says.\n\n" +
	"Smyklot keeps reading the old file until this merges, so nothing changes " +
	"while this sits here. If you close it, Smyklot will not ask again."

// proposeConfigMigration opens, or follows up, the pull request that moves a
// repository's configuration file to TOML.
//
// It runs from the sweep and from nowhere else. A webhook worker answering
// "/approve" must not open a pull request as a side effect: the person who
// asked for an approval did not ask for this, and a repository whose comments
// arrive in a burst would get one proposal per comment.
func (s *server) proposeConfigMigration(
	ctx context.Context,
	client *github.Client,
	targetID string,
	repo github.Repository,
	file repositoryConfigFile,
) error {
	if s.panel == nil {
		// Nowhere to remember a refusal, so asking would mean asking forever.
		return nil
	}

	target, repository, err := s.repositoryControls(ctx, targetID, storage.RepositoryID(repo.ID))
	if err != nil {
		return err
	}

	// Its own precondition rather than the caller's. A repository somebody
	// turned off is not one to open a pull request at, and this no longer sits
	// behind the sweep's own enablement check - it runs before the stand-down
	// that check follows.
	if !repositoryEnabled(target, repository) {
		return nil
	}

	switch nextMigrationStep(repository, file) {
	case migrationStepNothing:
		return nil

	case migrationStepForget:
		return s.recordConfigMigration(
			ctx, targetID, repo, storage.ConfigMigrationNone, nil,
		)

	case migrationStepFollowUp:
		return s.followUpConfigMigration(ctx, client, targetID, repo, repository.ConfigMigrationPR)

	default:
		return s.stopIfRefused(
			ctx, targetID, repo, s.openConfigMigration(ctx, targetID, client, repo),
		)
	}
}

// repositoryEnabled reports a repository Smyklot has been left switched on for.
func repositoryEnabled(target storage.Target, repository storage.Repository) bool {
	return target.Available && repository.Available &&
		storage.RepositoryEnabled(target, repository)
}

// stopIfRefused turns a refusal from GitHub into a durable state rather than
// something to retry on a timer.
//
// A proposal costs seven requests to build, and the last of them - pushing the
// branch - is the first that needs write access. An App that was never granted
// it fails there every tick, forever, spending all seven each time. A
// permission nobody has granted will not appear because the bot asked again
// twelve times an hour, so this stops asking and says so in the panel, where
// somebody can grant it and clear the state.
//
// Only a refusal. A rate limit, a timeout or a 5xx is the same request worth
// making again, and APIError.Retryable is the one place that distinction is
// drawn.
func (s *server) stopIfRefused(
	ctx context.Context,
	targetID string,
	repo github.Repository,
	err error,
) error {
	var apiErr *github.APIError
	if err == nil || !errors.As(err, &apiErr) || apiErr.Retryable() {
		return err
	}

	logging.From(ctx).Warn(
		"configuration migration refused by GitHub; not asking again",
		"status", apiErr.StatusCode,
		"error", err,
	)

	// The refusal is swallowed once it is written down. Returning it as well
	// would have the sweep log the same thing twice, and there is nothing left
	// to retry.
	return s.recordConfigMigration(ctx, targetID, repo, storage.ConfigMigrationBlocked, nil)
}

type migrationStep int

const (
	migrationStepNothing migrationStep = iota
	migrationStepForget
	migrationStepFollowUp
	migrationStepOpen
)

// nextMigrationStep decides what this sweep tick owes the repository, from what
// is already known rather than by asking GitHub anything.
func nextMigrationStep(
	repository storage.Repository,
	file repositoryConfigFile,
) migrationStep {
	// An open proposal is followed up whatever the file looks like now, because
	// GitHub is the one that knows what became of it. Reading the file instead
	// would leave the panel describing a pull request somebody has since closed.
	if repository.ConfigMigration == storage.ConfigMigrationProposed {
		return migrationStepFollowUp
	}

	// The repository is on TOML, so every answer this state could hold is about
	// something that is over. A refusal in particular has to stop being shown:
	// the file it refused to move is not there any more.
	if migrated(file) {
		if repository.ConfigMigration == storage.ConfigMigrationNone {
			return migrationStepNothing
		}

		return migrationStepForget
	}

	if !migratable(file) {
		// Deliberately not forgetting a refusal here. A file that stopped
		// parsing is not a repository that changed its mind, and clearing the
		// state on it would have Smyklot ask again the moment the file was
		// fixed - which is the nagging the refusal exists to prevent.
		return migrationStepNothing
	}

	// A repository that told the panel to ignore its file is not one to open a
	// pull request at about the formatting of that file.
	if repository.IgnoreRepositoryFile {
		return migrationStepNothing
	}

	if repository.ConfigMigration == storage.ConfigMigrationNone {
		return migrationStepOpen
	}

	// Refused, or refused by GitHub. Both are durable and both are cleared from
	// the panel rather than by another tick.
	return migrationStepNothing
}

// migrated reports a repository reading a TOML file, which is where all of this
// was trying to get it.
func migrated(file repositoryConfigFile) bool {
	if file.status != storage.RepositoryFileValid {
		return false
	}

	format, err := config.FormatOf(file.path)

	return err == nil && format == config.FormatTOML
}

// migratable reports a repository whose configuration can be converted without
// anyone having to decide anything.
func migratable(file repositoryConfigFile) bool {
	if file.status != storage.RepositoryFileValid {
		// Converting a file that does not parse would launder a broken file
		// into a valid-looking one saying something nobody wrote.
		return false
	}

	format, err := config.FormatOf(file.path)
	if err != nil || format != config.FormatYAML {
		return false
	}

	// A repository already carrying a TOML file somewhere has made a choice
	// this cannot second-guess: the YAML only won because an operator pointed
	// the panel at it.
	for _, other := range file.superseded {
		if other, err := config.FormatOf(other); err == nil && other == config.FormatTOML {
			return false
		}
	}

	return true
}

// followUpConfigMigration reads what became of a proposal that was open when
// this last looked.
func (s *server) followUpConfigMigration(
	ctx context.Context,
	client *github.Client,
	targetID string,
	repo github.Repository,
	pullRequest *int,
) error {
	if pullRequest == nil {
		// A proposed state without the proposal number cannot ever be followed.
		// Heal it so the next tick can create a proposal it can track.
		return s.recordConfigMigration(ctx, targetID, repo, storage.ConfigMigrationNone, nil)
	}

	pull, err := client.GetPullRequestState(ctx, repo.Owner, repo.Name, *pullRequest)
	if err != nil {
		return err
	}

	switch {
	case pull.Open:
		// Still waiting on the repository.
		return nil

	case pull.Merged:
		// Done. The state goes back to nothing rather than staying at
		// "proposed" describing a pull request that has already landed.
		logging.From(ctx).Info("configuration migration merged", "pull_request", pull.Number)

		return s.recordConfigMigration(ctx, targetID, repo, storage.ConfigMigrationNone, nil)

	default:
		logging.From(ctx).Info("configuration migration declined", "pull_request", pull.Number)

		return s.recordConfigMigration(
			ctx, targetID, repo, storage.ConfigMigrationDeclined, &pull.Number,
		)
	}
}

// openConfigMigration writes the converted file and proposes it.
//
// One commit adds the TOML and deletes the YAML, so the branch never holds a
// state where the repository carries both - which is the state that would make
// Smyklot read one file while whoever is reviewing reads the other.
func (s *server) openConfigMigration(
	ctx context.Context,
	targetID string,
	client *github.Client,
	repo github.Repository,
) error {
	built, err := s.buildConfigMigration(ctx, client, repo)
	if err != nil || built.commit == "" {
		return err
	}
	head := migrationBranchFor(built.tree)

	existing, err := client.GetRef(ctx, repo.Owner, repo.Name, "heads/"+head)
	if err != nil {
		return err
	}
	if existing == "" {
		if err := s.pushConfigMigration(ctx, client, repo, head, built.commit); err != nil {
			return err
		}
	} else {
		tip, err := client.GetCommit(ctx, repo.Owner, repo.Name, existing)
		if err != nil {
			return err
		}
		if tip.Tree != built.tree {
			logging.From(ctx).Info(
				"configuration migration branch changed after creation; leaving it alone",
				"branch", head,
				"commit", existing,
			)

			return nil
		}

		adopted, err := s.adoptConfigMigration(ctx, targetID, client, repo, head)
		if err != nil || adopted {
			return err
		}
	}

	return s.createConfigMigrationPull(
		ctx, targetID, client, repo, head, built.branch, built.source,
	)
}

func (s *server) createConfigMigrationPull(
	ctx context.Context,
	targetID string,
	client *github.Client,
	repo github.Repository,
	head, branch, source string,
) error {
	pull, err := client.CreatePullRequest(ctx, repo.Owner, repo.Name, github.NewPullRequest{
		Title: migrationTitle,
		Body:  fmt.Sprintf(migrationBody, source),
		Head:  head,
		Base:  branch,
	})
	if err != nil {
		return err
	}

	logging.From(ctx).Info(
		"configuration migration proposed",
		"pull_request", pull.Number,
		"from", source,
		"to", migrationTarget,
	)

	return s.recordConfigMigration(
		ctx, targetID, repo, storage.ConfigMigrationProposed, &pull.Number,
	)
}

// buildConfigMigration reads and converts one immutable default-branch commit,
// then reports the new commit, its base branch, and the source file it moved.
//
// An empty commit means there is nothing to build on - a repository whose
// default branch GitHub did not name, or which has no commits at all.
type configMigrationBuild struct {
	commit string
	branch string
	source string
	tree   string
}

func (s *server) buildConfigMigration(
	ctx context.Context,
	client *github.Client,
	repo github.Repository,
) (configMigrationBuild, error) {
	// The branch the file was read from, since that is the branch the
	// configuration takes effect on. A repository whose default GitHub did not
	// report is one this cannot safely guess at.
	branch := repo.DefaultBranch
	if branch == "" {
		return configMigrationBuild{}, nil
	}

	base, err := client.GetRef(ctx, repo.Owner, repo.Name, "heads/"+branch)
	if err != nil || base == "" {
		// An empty repository has no configuration file to move.
		return configMigrationBuild{}, err
	}

	// Re-read from the immutable commit this tree is built from. The sweep's
	// cached file may describe an older default-branch tip; using those bytes
	// here would delete a maintainer's newer YAML while writing stale TOML.
	found, err := client.FindRepoConfigAtCommit(ctx, repo.Owner, repo.Name, "", base)
	if err != nil {
		return configMigrationBuild{}, err
	}
	file := repositoryConfigFileFrom(found, "")
	if !migratable(file) {
		return configMigrationBuild{}, nil
	}
	content, err := config.RenderTOML(file.patch)
	if err != nil {
		return configMigrationBuild{}, err
	}

	// The tree the base commit records, because that is what a tree is built
	// from. A reference points at a commit, and CreateTree wants the thing the
	// commit points at.
	baseCommit, err := client.GetCommit(ctx, repo.Owner, repo.Name, base)
	if err != nil {
		return configMigrationBuild{}, err
	}

	blob, err := client.CreateBlob(
		ctx, repo.Owner, repo.Name, append([]byte(migrationHeader), content...),
	)
	if err != nil {
		return configMigrationBuild{}, err
	}

	tree, err := client.CreateTree(ctx, repo.Owner, repo.Name, baseCommit.Tree, []github.TreeChange{
		{Path: migrationTarget, Blob: blob},
		{Path: file.path},
	})
	if err != nil {
		return configMigrationBuild{}, err
	}

	commit, err := client.CreateCommit(ctx, repo.Owner, repo.Name, migrationCommit, tree, base)

	return configMigrationBuild{
		commit: commit, branch: branch, source: file.path, tree: tree,
	}, err
}

// pushConfigMigration puts the commit on the migration branch.
func (s *server) pushConfigMigration(
	ctx context.Context,
	client *github.Client,
	repo github.Repository,
	branch string,
	commit string,
) error {
	return client.CreateRef(ctx, repo.Owner, repo.Name, "heads/"+branch, commit)
}

func migrationBranchFor(tree string) string { return migrationBranch + "-" + tree }

// adoptConfigMigration records a proposal that is still open, and reports
// whether there was one.
//
// Only an open one. A closed proposal is not something to adopt: either an
// operator has cleared the refusal and is asking for it again, or nobody ever
// opened anything from the branch. Reading a closed one as "declined" here is
// what made the panel's only way back from a refusal silently undo itself on
// the next sweep tick.
func (s *server) adoptConfigMigration(
	ctx context.Context,
	targetID string,
	client *github.Client,
	repo github.Repository,
	branch string,
) (bool, error) {
	pull, err := client.FindPullRequestByHead(ctx, repo.Owner, repo.Name, branch, repo.DefaultBranch)
	if err != nil || pull == nil || pull.State != github.PullRequestOpen {
		return false, err
	}

	return true, s.recordConfigMigration(
		ctx, targetID, repo, storage.ConfigMigrationProposed, &pull.Number,
	)
}

func (s *server) recordConfigMigration(
	ctx context.Context,
	targetID string,
	repo github.Repository,
	state storage.ConfigMigrationState,
	pull *int,
) error {
	if err := s.store.SetRepositoryConfigMigration(ctx, storage.RepositoryConfigMigration{
		TargetID:     targetID,
		RepositoryID: storage.RepositoryID(repo.ID),
		State:        state,
		PullRequest:  pull,
	}); err != nil {
		return fmt.Errorf("record configuration migration: %w", err)
	}

	s.panel.Announce(targetID, storage.RepositoryID(repo.ID))

	return nil
}
