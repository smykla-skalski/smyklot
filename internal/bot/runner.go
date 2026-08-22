package bot

import (
	"context"
	"errors"
	"fmt"

	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/logging"
)

// This file decides which entry point acts on a repository.
//
// A repository that has the workflow file and is installed on a running service
// is seen by both, and both would approve, merge and comment on the same
// comment. The repository's own .github/smyklot.yaml names one of them, and
// whichever is not named stands down.

// standDownSummary is what the Action leaves behind in the job summary when it
// stands down, so a repository whose service is not running has somewhere to
// look other than a green run that did nothing.
const standDownSummary = "\n## Smyklot\n\nStood down: `runner` is `%[1]s`, so the %[1]s handles this repository.\n"

// standsDown reports whether the entry point asking should leave this
// repository to the other one, and says so in the log either way it is read.
func standsDown(ctx context.Context, bc *config.Config, self config.Runner) bool {
	if bc.RunBy(self) {
		return false
	}

	logging.From(ctx).Info("standing down", "runner", string(bc.EffectiveRunner()))

	return true
}

// ActionStandsDown reports whether the Action should leave this comment alone.
//
// Nothing is posted to the pull request. If the service is running it has
// already reacted, and a second reaction is the duplicate this exists to
// prevent. The job summary carries the reason instead.
func ActionStandsDown(ctx context.Context, bc *config.Config) bool {
	if !standsDown(ctx, bc, config.RunnerAction) {
		return false
	}

	if err := appendStepSummary(fmt.Sprintf(standDownSummary, bc.EffectiveRunner())); err != nil {
		logging.From(ctx).Warn("failed to write step summary", "error", err)
	}

	return true
}

// unusableConfigSummary is what a sweep leaves behind when the repository's
// file cannot be read, since a sweep has no one comment to answer.
const unusableConfigSummary = "\n## Smyklot\n\nStopped: `.github/smyklot.yaml` could not be used.\n\n```text\n%s\n```\n"

// ReportUnusableRepoConfig stops a sweep and says why where the run can be
// read.
//
// A broken file stops the sweep rather than letting it fall back to defaults,
// because the file is where a repository narrows what is allowed and carrying
// on would restore commands it had turned off. The comment path answers the
// pull request that asked for something; a sweep is looking at every open pull
// request at once, so commenting on all of them to report one broken file would
// be the wrong kind of loud. The job summary carries it instead, and the run
// fails so a repository does not lose reaction commands quietly.
func ReportUnusableRepoConfig(ctx context.Context, cause error) error {
	if !errors.Is(cause, ErrRepoConfigInvalid) {
		return cause
	}

	if err := appendStepSummary(fmt.Sprintf(unusableConfigSummary, cause)); err != nil {
		logging.From(ctx).Warn("failed to write step summary", "error", err)
	}

	logging.From(ctx).Error("stopping: repository configuration is unusable", "error", cause)

	return cause
}

// ServiceStandsDown reports whether the service should leave a repository to
// the Action.
//
// This is the rollback path: a repository sets runner to action and the service
// stops touching it, without the service being redeployed or reconfigured.
func ServiceStandsDown(ctx context.Context, bc *config.Config) bool {
	return standsDown(ctx, bc, config.RunnerService)
}
