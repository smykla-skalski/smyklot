package apply

import (
	"context"
	"fmt"
	"strings"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	appconfig "github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/logging"
)

// repositoryAnswer separates work to do from evidence that needs no action.
// No actions alone cannot establish agreement: files may already be proposed
// or their proposal declined without changing the default branch.
type repositoryAnswer struct {
	actions     []orgsync.Action
	problem     string
	observation orgsync.Observation
}

func compared(actions []orgsync.Action) repositoryAnswer {
	if len(actions) == 0 {
		return repositoryAnswer{observation: orgsync.ObservationMatched}
	}
	return repositoryAnswer{actions: actions}
}

// repositoryQuestion asks one repository what one kind would take.
//
// problem is empty where the answer covers the whole of what this kind
// configures, which is nearly always, and for labels and settings is always. A
// ruleset a repository holds twice is one exception: nothing can say which one
// the configuration meant, so part of the kind is unresolved however much of
// the rest was worked out. A file sync has three of its own.
//
// A problem throws the actions away with it, because the executor records a
// kind settled once its every action applied - so acting on the resolved part
// would mark the unresolved part up to date too. It is words rather than a
// flag because it is the only account of why this repository is not being
// synced that anybody outside the service log ever sees.
//
// An error is different: the repository could not be read at all, which is
// nobody's mistake to fix and is retried on the next tick.
type repositoryQuestion func(
	context.Context, storage.Repository,
) (repositoryAnswer, error)

// repositoryPlanner reads a kind's stored document and returns what to ask each
// repository with it.
//
// The one place a kind's stored document meets its planner, and it is read once
// for the whole kind rather than once per repository: the document is the same
// for all of them, so decoding it inside the loop would decode it a hundred
// times over and report a document nobody can read a hundred times too.
//
// Validated here as well as in the panel. The panel covers what somebody typed;
// this covers a row written before a rule existed, or by a hand on the database,
// and every rule it checks is one GitHub answers with a 422. A kind this version
// does not know is refused rather than skipped, because skipping would record
// the repository as settled for work nothing did.
func repositoryPlanner(
	client *github.Client,
	syncConfig orgsync.Config,
	overrides map[string]*orgsync.RepositoryOverride,
	formatting appconfig.FormattingPolicy,
	targetPatch appconfig.Patch,
) (repositoryQuestion, error) {
	switch syncConfig.Kind {
	case orgsync.KindLabels:
		return labelPlanner(client, syncConfig)

	case orgsync.KindSettings:
		return settingsPlanner(client, syncConfig)

	case orgsync.KindRulesets:
		return rulesetPlanner(client, syncConfig)

	case orgsync.KindFiles:
		return filePlanner(client, syncConfig, overrides, formatting, targetPatch)

	default:
		return nil, fmt.Errorf("%w: %s", errSyncKindUnsupported, syncConfig.Kind)
	}
}

func labelPlanner(client *github.Client, config orgsync.Config) (repositoryQuestion, error) {
	labels, err := decodeSyncDocument[orgsync.LabelConfig](config)
	if err != nil {
		return nil, err
	}

	return func(
		ctx context.Context, repository storage.Repository,
	) (repositoryAnswer, error) {
		owner, name := splitFullName(repository.FullName)

		current, err := client.ListRepositoryLabels(ctx, owner, name)
		if err != nil {
			return repositoryAnswer{}, err
		}

		return compared(orgsync.PlanLabels(
			repository.ID, labels, asCurrentLabels(current), labels.Exclusions(),
		)), nil
	}, nil
}

func settingsPlanner(client *github.Client, config orgsync.Config) (repositoryQuestion, error) {
	settings, err := decodeSyncDocument[orgsync.SettingsConfig](config)
	if err != nil {
		return nil, err
	}

	return func(
		ctx context.Context, repository storage.Repository,
	) (repositoryAnswer, error) {
		owner, name := splitFullName(repository.FullName)

		current, err := client.GetRepositorySettings(ctx, owner, name)
		if err != nil {
			return repositoryAnswer{}, err
		}

		return compared(orgsync.PlanSettings(
			repository.ID, settings, asCurrentSettings(current),
		)), nil
	}, nil
}

func rulesetPlanner(client *github.Client, config orgsync.Config) (repositoryQuestion, error) {
	rulesets, err := decodeSyncDocument[orgsync.RulesetConfig](config)
	if err != nil {
		return nil, err
	}

	return func(
		ctx context.Context, repository storage.Repository,
	) (repositoryAnswer, error) {
		owner, name := splitFullName(repository.FullName)

		current, err := readRulesets(ctx, client, owner, name, rulesets)
		if err != nil {
			return repositoryAnswer{}, err
		}

		actions, ambiguous := orgsync.PlanRulesets(
			repository.ID, rulesets, current, rulesets.Exclusions())
		if len(ambiguous) > 0 {
			// A ruleset nothing can address produces no action, so a plan
			// cannot carry it and a person reading one would see a repository
			// that looks finished.
			return repositoryAnswer{problem: "more than one ruleset here carries a configured name (" +
				strings.Join(ambiguous, ", ") +
				"), so nothing can say which one the configuration means"}, nil
		}

		return compared(actions), nil
	}, nil
}

func filePlanner(
	client *github.Client,
	syncConfig orgsync.Config,
	overrides map[string]*orgsync.RepositoryOverride,
	formatting appconfig.FormattingPolicy,
	targetPatch appconfig.Patch,
) (repositoryQuestion, error) {
	files, err := decodeSyncDocument[orgsync.FileConfig](syncConfig)
	if err != nil {
		return nil, err
	}

	return func(
		ctx context.Context, repository storage.Repository,
	) (repositoryAnswer, error) {
		policy := repositoryFormattingPolicy(formatting, targetPatch, repository)
		return planRepositoryFiles(
			ctx, client, repository, files, overrides[repository.ID], policy,
		)
	}, nil
}

// planRepositoryFiles answers what one repository's files would take, and where
// it cannot answer, why.
//
// Three ways not to: a repository with nowhere to propose against, adjustments
// that cannot be used, and files that cannot be composed. The last two are
// somebody's to fix, and recording a digest against either would say the
// repository matches for six hours when nothing has looked at it. All three are
// returned in words, because the alternative is a repository that is quietly
// receiving none of the organization's files and nothing anybody can read that
// says so.
func planRepositoryFiles(
	ctx context.Context,
	client *github.Client,
	repository storage.Repository,
	config orgsync.FileConfig,
	override *orgsync.RepositoryOverride,
	formatting appconfig.FormattingPolicy,
) (repositoryAnswer, error) {
	target := syncTargetFor(repository)

	if target.DefaultBranch == "" {
		// A repository with no commits has nowhere to propose against, and
		// GitHub names no branch for one. Said here rather than discovered
		// against the API, which would spend a request per repository per tick
		// learning it again.
		return repositoryAnswer{problem: "this repository has no default branch, " +
			"so there is nowhere to propose a change"}, nil
	}
	if !repository.IgnoreRepositoryFile && repository.ConfigFileError != nil {
		return repositoryAnswer{problem: "the repository configuration cannot be used: " +
			*repository.ConfigFileError}, nil
	}

	adjustments, err := decodeFileOverride(override, config)
	if err != nil {
		return repositoryAnswer{problem: "the adjustments saved for this repository cannot be used: " +
			err.Error()}, nil
	}

	current, err := readTreePaths(
		ctx, client, target, target.DefaultBranch, config.Managed())
	if err != nil {
		return repositoryAnswer{}, err
	}

	if current.Missing {
		// There is no tree at that branch. GitHub names a default branch
		// whatever the case - the name is configuration, and it is there long
		// before the branch is - so the name says nothing about whether there
		// is anything to propose against. The tree read does, and it is a read
		// the planner makes already.
		//
		// Said rather than planned. Every managed path is absent from a
		// repository with no tree, so the planner would emit a create for each,
		// a person would approve them, and the apply would refuse for want of a
		// branch to build on - which spends the installation's one live plan
		// slot and marks every plan riding with it failed.
		//
		// The reason lists the causes rather than picking one. GitHub answers
		// 404 for a repository with no commits, for a branch that was renamed
		// since the catalog last looked, and for one this installation can no
		// longer read, and the read cannot tell them apart.
		return repositoryAnswer{problem: "there is nothing at " + target.DefaultBranch +
			" to propose against: this repository has no commits, the branch was " +
			"renamed, or Smyklot can no longer read it"}, nil
	}

	plan, err := orgsync.PlanFiles(
		repository.ID, config, adjustments, target.DefaultBranch, current.Files, formatting)
	if err != nil {
		// A merge that cannot be applied. Fail-closed: no actions, and no
		// digest, so the repository is asked again once somebody fixes it.
		return repositoryAnswer{problem: "these files cannot be composed: " + err.Error()}, nil
	}

	if len(plan.Actions) == 0 {
		return repositoryAnswer{observation: orgsync.ObservationMatched}, nil
	}

	observation, err := proposalObservation(ctx, client, target, plan.Proposal)
	if err != nil {
		return repositoryAnswer{}, err
	}

	if observation != "" {
		// Already asked, so there is nothing to plan. Preserve whether the
		// proposal is open or declined instead of claiming the files match. This is the whole
		// of what a file sync can do: propose. The branch is named after what
		// the files should end up saying, so a configuration that changes is a
		// different branch and the question is put once more.
		logging.From(ctx).Info(
			"this repository already has this change in front of it, so it is left alone",
			"repo", repository.FullName, "branch", plan.Proposal)

		return repositoryAnswer{observation: observation}, nil
	}

	return compared(plan.Actions), nil
}

// proposalObservation distinguishes an open proposal from a rejected one.
//
// Whatever state, because the answer decides whether to propose again. An open
// one is being considered and a closed one was refused, and both mean the
// asking is done - a plan computed for either would be the same plan, approved
// again, adopting the same pull request, once every horizon for as long as it
// sat there. A merged one is not outstanding: the change landed, and files that
// still differ after it are a new question.
func proposalObservation(
	ctx context.Context,
	client *github.Client,
	target syncTarget,
	proposal string,
) (orgsync.Observation, error) {
	pull, err := client.FindPullRequestByHead(
		ctx, target.Owner, target.Name, proposal, target.DefaultBranch)
	if err != nil || pull == nil {
		return "", err
	}

	if pull.Merged {
		return "", nil
	}
	if pull.State == github.PullRequestClosed {
		return orgsync.ObservationDeclined, nil
	}

	return orgsync.ObservationProposed, nil
}
