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
	// migrationBranch is fixed rather than derived from the content.
	//
	// It is what makes "have I already asked?" one reference lookup instead of
	// a pull request search, and it is what stops a second proposal appearing
	// beside the first when the file changes while one is open.
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

	target, repository, err := s.repositoryControls(ctx, targetID, repositoryStorageID(repo.ID))
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
		return s.followUpConfigMigration(ctx, client, targetID, repo)

	default:
		return s.stopIfRefused(
			ctx, targetID, repo, s.openConfigMigration(ctx, targetID, client, repo, file),
		)
	}
}

// repositoryEnabled reports a repository Smyklot has been left switched on for.
func repositoryEnabled(target storage.Target, repository storage.Repository) bool {
	if !target.Available || !repository.Available {
		return false
	}
	if repository.EnabledOverride != nil {
		return *repository.EnabledOverride
	}

	return target.RepositoryDefaultEnabled
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
) error {
	pull, err := client.FindPullRequestByHead(ctx, repo.Owner, repo.Name, migrationBranch)
	if err != nil {
		return err
	}

	switch {
	case pull == nil || pull.State != github.PullRequestClosed:
		// Nothing to report, or still open and waiting on the repository.
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
	file repositoryConfigFile,
) error {
	// One reference lookup answers "have I already done this". A branch that
	// exists means an earlier tick got as far as pushing it, and what to do
	// about that depends on whether anything is still open against it.
	existing, err := client.GetRef(ctx, repo.Owner, repo.Name, "heads/"+migrationBranch)
	if err != nil {
		return err
	}

	if existing != "" {
		adopted, err := s.adoptConfigMigration(ctx, targetID, client, repo)
		if err != nil || adopted {
			return err
		}

		// Nothing open against the branch: an earlier tick pushed it and never
		// opened anything, or the proposal was closed and an operator has since
		// asked for it again. Either way the branch is stale - but rebuilding it
		// replaces its history, so it is only stale if Smyklot is still the last
		// thing to have written to it.
		own, err := s.ownsMigrationBranch(ctx, client, repo, existing)
		if err != nil || !own {
			return err
		}
	}

	commit, branch, err := s.buildConfigMigration(ctx, client, repo, file)
	if err != nil || commit == "" {
		return err
	}

	if err := s.pushConfigMigration(ctx, client, repo, commit, existing != ""); err != nil {
		return err
	}

	pull, err := client.CreatePullRequest(ctx, repo.Owner, repo.Name, github.NewPullRequest{
		Title: migrationTitle,
		Body:  fmt.Sprintf(migrationBody, file.path),
		Head:  migrationBranch,
		Base:  branch,
	})
	if err != nil {
		return err
	}

	logging.From(ctx).Info(
		"configuration migration proposed",
		"pull_request", pull.Number,
		"from", file.path,
		"to", migrationTarget,
	)

	return s.recordConfigMigration(
		ctx, targetID, repo, storage.ConfigMigrationProposed, &pull.Number,
	)
}

// buildConfigMigration writes the converted file as a commit on the default
// branch, and reports the commit and the branch it was built on.
//
// An empty commit means there is nothing to build on - a repository whose
// default branch GitHub did not name, or which has no commits at all.
func (s *server) buildConfigMigration(
	ctx context.Context,
	client *github.Client,
	repo github.Repository,
	file repositoryConfigFile,
) (commit, branch string, err error) {
	content, err := config.RenderTOML(file.patch)
	if err != nil {
		return "", "", err
	}

	// The branch the file was read from, since that is the branch the
	// configuration takes effect on. A repository whose default GitHub did not
	// report is one this cannot safely guess at.
	branch = repo.DefaultBranch
	if branch == "" {
		return "", "", nil
	}

	base, err := client.GetRef(ctx, repo.Owner, repo.Name, "heads/"+branch)
	if err != nil || base == "" {
		// An empty repository has no configuration file to move.
		return "", "", err
	}

	// The tree the base commit records, because that is what a tree is built
	// from. A reference points at a commit, and CreateTree wants the thing the
	// commit points at.
	baseCommit, err := client.GetCommit(ctx, repo.Owner, repo.Name, base)
	if err != nil {
		return "", "", err
	}

	blob, err := client.CreateBlob(
		ctx, repo.Owner, repo.Name, append([]byte(migrationHeader), content...),
	)
	if err != nil {
		return "", "", err
	}

	tree, err := client.CreateTree(ctx, repo.Owner, repo.Name, baseCommit.Tree, []github.TreeChange{
		{Path: migrationTarget, Blob: blob},
		{Path: file.path},
	})
	if err != nil {
		return "", "", err
	}

	commit, err = client.CreateCommit(ctx, repo.Owner, repo.Name, migrationCommit, tree, base)

	return commit, branch, err
}

// ownsMigrationBranch reports a branch still carrying the commit Smyklot put
// there, and nothing on top of it.
//
// A branch named after the bot is the bot's by convention and by nothing
// stronger - it is a place anybody can push to. So the tip is checked rather
// than assumed, because the alternative is that a fixup somebody pushed after
// closing the proposal disappears on the next tick with no error and no trace:
// the failure that made file sync's force-push dangerous in the tool this
// replaces.
//
// Somebody else's branch is left alone rather than written down as blocked,
// because the state resolves itself. Whoever pushed opens a pull request, and
// the next tick adopts it; a refusal recorded here would be a refusal to.
func (s *server) ownsMigrationBranch(
	ctx context.Context,
	client *github.Client,
	repo github.Repository,
	head string,
) (bool, error) {
	tip, err := client.GetCommit(ctx, repo.Owner, repo.Name, head)
	if err != nil {
		return false, err
	}

	if tip.Message == migrationCommit {
		return true, nil
	}

	logging.From(ctx).Info(
		"configuration migration branch carries somebody else's work; leaving it alone",
		"branch", migrationBranch,
		"commit", head,
	)

	return false, nil
}

// pushConfigMigration puts the commit on the migration branch.
//
// Replacing an existing branch is a force, and what makes that safe is checked
// rather than assumed. A branch with an open proposal is adopted instead of
// pushed over, and a branch whose tip somebody else wrote is left alone, so
// what reaches here is a branch holding Smyklot's own commit and nothing else.
func (s *server) pushConfigMigration(
	ctx context.Context,
	client *github.Client,
	repo github.Repository,
	commit string,
	exists bool,
) error {
	if !exists {
		return client.CreateRef(ctx, repo.Owner, repo.Name, "heads/"+migrationBranch, commit)
	}

	return client.UpdateRef(ctx, repo.Owner, repo.Name, "heads/"+migrationBranch, commit, true)
}

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
) (bool, error) {
	pull, err := client.FindPullRequestByHead(ctx, repo.Owner, repo.Name, migrationBranch)
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
		RepositoryID: repositoryStorageID(repo.ID),
		State:        state,
		PullRequest:  pull,
	}); err != nil {
		return fmt.Errorf("record configuration migration: %w", err)
	}

	s.panel.Announce(targetID, repositoryStorageID(repo.ID))

	return nil
}
