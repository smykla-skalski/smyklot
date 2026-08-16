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

	_, repository, err := s.repositoryControls(ctx, targetID, repositoryStorageID(repo.ID))
	if err != nil {
		return err
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
			ctx, targetID, repo, s.openConfigMigration(ctx, client, targetID, repo, file),
		)
	}
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
	if !migratable(file) {
		// The file moved, or was replaced, or stopped parsing. Whatever an
		// open proposal was about, it is not this - and leaving the state at
		// "proposed" would have the panel describing a pull request that has
		// already done its job.
		if repository.ConfigMigration == storage.ConfigMigrationProposed {
			return migrationStepForget
		}

		return migrationStepNothing
	}

	// A repository that told the panel to ignore its file is not one to open a
	// pull request at about the formatting of that file.
	if repository.IgnoreRepositoryFile {
		return migrationStepNothing
	}

	switch repository.ConfigMigration {
	case storage.ConfigMigrationDeclined, storage.ConfigMigrationBlocked:
		return migrationStepNothing

	case storage.ConfigMigrationProposed:
		return migrationStepFollowUp

	default:
		return migrationStepOpen
	}
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
	if pull == nil || pull.State != github.PullRequestClosed || pull.Merged {
		// Still open, or merged and about to stop being migratable anyway.
		return nil
	}

	logging.From(ctx).Info(
		"configuration migration declined",
		"pull_request", pull.Number,
	)

	return s.recordConfigMigration(
		ctx, targetID, repo, storage.ConfigMigrationDeclined, &pull.Number,
	)
}

// openConfigMigration writes the converted file and proposes it.
//
// One commit adds the TOML and deletes the YAML, so the branch never holds a
// state where the repository carries both - which is the state that would make
// Smyklot read one file while whoever is reviewing reads the other.
func (s *server) openConfigMigration(
	ctx context.Context,
	client *github.Client,
	targetID string,
	repo github.Repository,
	file repositoryConfigFile,
) error {
	// One reference lookup answers "have I already done this". A branch that
	// exists means an earlier tick got as far as pushing it, whether or not it
	// got as far as the pull request, so this hands over to the follow-up
	// rather than pushing a second time.
	existing, err := client.GetRef(ctx, repo.Owner, repo.Name, "heads/"+migrationBranch)
	if err != nil {
		return err
	}
	if existing != "" {
		return s.adoptConfigMigration(ctx, client, targetID, repo)
	}

	content, err := config.RenderTOML(file.patch)
	if err != nil {
		return err
	}

	// The branch the file was read from, since that is the branch the
	// configuration takes effect on. A repository whose default GitHub did not
	// report is one this cannot safely guess at.
	branch := repo.DefaultBranch
	if branch == "" {
		return nil
	}

	base, err := client.GetRef(ctx, repo.Owner, repo.Name, "heads/"+branch)
	if err != nil {
		return err
	}
	if base == "" {
		// An empty repository has no configuration file to move.
		return nil
	}

	// The tree the base commit records, because that is what a tree is built
	// from. A reference points at a commit, and CreateTree wants the thing the
	// commit points at.
	baseTree, err := client.GetCommitTree(ctx, repo.Owner, repo.Name, base)
	if err != nil {
		return err
	}

	blob, err := client.CreateBlob(ctx, repo.Owner, repo.Name, content)
	if err != nil {
		return err
	}

	tree, err := client.CreateTree(ctx, repo.Owner, repo.Name, baseTree, []github.TreeChange{
		{Path: migrationTarget, Blob: blob},
		{Path: file.path},
	})
	if err != nil {
		return err
	}

	commit, err := client.CreateCommit(
		ctx, repo.Owner, repo.Name, migrationCommit, tree, base,
	)
	if err != nil {
		return err
	}

	if err := client.CreateRef(
		ctx, repo.Owner, repo.Name, "heads/"+migrationBranch, commit,
	); err != nil {
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

// adoptConfigMigration records a proposal an earlier tick opened and did not
// get to write down.
func (s *server) adoptConfigMigration(
	ctx context.Context,
	client *github.Client,
	targetID string,
	repo github.Repository,
) error {
	pull, err := client.FindPullRequestByHead(ctx, repo.Owner, repo.Name, migrationBranch)
	if err != nil {
		return err
	}
	if pull == nil {
		// The branch is there and nothing was opened from it. Leaving the
		// state alone means the next tick tries again, which is what should
		// happen once whoever is holding the branch has finished with it.
		return nil
	}

	state := storage.ConfigMigrationProposed
	if pull.State == github.PullRequestClosed && !pull.Merged {
		state = storage.ConfigMigrationDeclined
	}

	return s.recordConfigMigration(ctx, targetID, repo, state, &pull.Number)
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
