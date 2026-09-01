# Smyklot

> GitHub App for automated PR approvals and merges based on CODEOWNERS

## Overview

Smyklot is a GitHub App that automates pull request approvals and merges by validating permissions against your repository's CODEOWNERS file. Comment on a PR with commands or add emoji reactions, and Smyklot handles it.

## Features

- CODEOWNERS-based permissions - only repository owners in `.github/CODEOWNERS` can approve/merge
- Multiple command formats - slash commands (`/approve`), mentions (`@smyklot approve`), and bare commands (`lgtm`, `merge`)
- Merge method control - explicit `squash` and `rebase` commands with intelligent fallback for `merge`
- Reaction-based commands - use 👍 to approve, 🚀 to merge, or ❤️ to cleanup
- Cleanup command - removes all bot reactions, approvals, and comments with a single command
- Approval deduplication - prevents duplicate approvals with smart reaction handling
- Flexible configuration - a TOML file in the repository, one `SMYKLOT_CONFIG` document, individual variables, or flags
- Emoji feedback - instant visual confirmation with ✅ (success), ❌ (error), or ⚠️ (warning)
- Comment edit/delete handling - reacts to command edits and posts a notice when an approve/merge command comment is deleted
- Reaction removal tracking - automatically removes approvals/merges when reactions are removed
- Multi-command support - execute multiple commands in a single comment
- Minimal permissions - follows GitHub Actions best practices
- Runs either way - as a GitHub Action, or as a [webhook service](#running-as-a-service) covering every repository the App is installed on
- 480+ passing tests

## Quick start

### Prerequisites

- GitHub repository with Actions enabled
- `.github/CODEOWNERS` file in your repository

### Installation

#### Option 1: GitHub App (recommended)

1. Install the [Smyklot GitHub App](https://github.com/apps/smyklot) on your repository
2. Create `.github/CODEOWNERS`:

   ```text
   * @username
   ```

3. Comment on PRs with commands or add reactions

#### Option 2: Manual workflow

Copy the workflow file to your repository:

```bash
cp .github/workflows/pr-commands.yaml your-repo/.github/workflows/
```

## Usage

### Comment commands

Smyklot responds to these commands in PR comments:

| Command     | Aliases          | Format                                    | Description                                       |
| ----------- | ---------------- | ----------------------------------------- | ------------------------------------------------- |
| `approve`   | `lgtm`, `accept` | `/approve`, `@smyklot approve`, `approve` | Approve the pull request                          |
| `merge`     | -                | `/merge`, `@smyklot merge`, `merge`       | Merge the pull request (with fallback)            |
| `squash`    | -                | `/squash`, `@smyklot squash`, `squash`    | Squash merge the pull request                     |
| `rebase`    | -                | `/rebase`, `@smyklot rebase`, `rebase`    | Rebase merge the pull request                     |
| `unapprove` | -                | `/unapprove`, `@smyklot unapprove`        | Remove approval                                   |
| `cleanup`   | -                | `/cleanup`, `@smyklot cleanup`, `cleanup` | Remove all bot reactions, approvals, and comments |
| `help`      | -                | `/help`, `@smyklot help`                  | Show help information                             |

Command formats:

- Slash commands: `/approve`, `/merge`, `/squash`, `/rebase`, `/unapprove`, `/cleanup`, `/help`
- Mention commands: `@smyklot approve`, `@smyklot merge`, `@smyklot squash`, `@smyklot rebase`, `@smyklot unapprove`, `@smyklot cleanup`
- Bare commands: `approve`, `accept`, `lgtm`, `merge`, `squash`, `rebase`, `cleanup` (exact match only)

All commands are case-insensitive.

### Reaction commands

| Reaction | Action                                                      |
| -------- | ----------------------------------------------------------- |
| 👍       | Approve the pull request                                    |
| 🚀       | Merge the pull request                                      |
| ❤️       | Cleanup (remove all bot reactions, approvals, and comments) |

Reactions must be added to the PR description, not to comments. Removing a reaction automatically undoes the corresponding action (removes approval/merge labels).

### Multiple commands

You can use multiple non-contradicting commands in a single comment:

```text
/approve
/merge
```

or

```text
lgtm merge
```

Commands will be executed in order: approve first, then merge.

### Examples

#### Approving a PR

Any of these will approve the PR:

```text
/approve
```

```text
@smyklot approve
```

```text
lgtm
```

Or add a 👍 reaction to the PR description.

#### Merging a PR

Merge with default method (with fallback to squash/rebase if merge commits disallowed):

```text
/merge
```

Or add a 🚀 reaction to the PR description.

Draft pull requests remain protected by default. Set `allow_draft_merges = true` to let any merge command or 🚀 reaction mark a draft ready for review before continuing. A merge-after-CI command publishes the pull request when accepted; converting it back to draft cancels that pending merge and requires a new command.

#### Squash merging

```text
/squash
```

#### Rebase merging

```text
/rebase
```

#### Removing approval

```text
/unapprove
```

Or remove your 👍 reaction from the PR description.

#### Cleanup

Remove all bot reactions, approvals, and comments:

```text
/cleanup
```

Or add a ❤️ reaction to the PR description.

Cleanup cannot be combined with other commands.

## Configuration

### CODEOWNERS setup

Create `.github/CODEOWNERS` in your repository:

```text
# Global owners can approve/merge any PR
* @username1 @username2
```

Only the global `*` pattern is read. Path-specific owners are not implemented, so a line naming a path is ignored.

### Bot configuration

Smyklot can be configured via repository variables (Settings → Secrets and variables → Actions → Variables) or a config file checked into the repository.

A setting can be given a value in eight places. They are resolved lowest first, so a later layer replaces what an earlier one said and a setting nobody names keeps its default:

```text
1. defaults             built into Smyklot
2. process file         --config-file, or SMYKLOT_CONFIG_FILE
3. process document     SMYKLOT_CONFIG
4. process environment  SMYKLOT_* variables, one per setting
5. process flags        the command line
6. account settings     the panel, for every repository
7. repository file      .smyklot.toml in the repository
8. repository settings  the panel, for one repository
```

The environment document sits below the individual variables so that changing one setting means adding one variable rather than rewriting the whole document.

That block is generated from `config.PrecedenceDoc`, and a test fails if this copy of it goes stale.

#### Option 1: A whole configuration in one variable

Set a `SMYKLOT_CONFIG` repository variable to a TOML document:

```toml
quiet_success = false
quiet_reactions = false
quiet_pending = false
allowed_commands = ["approve", "merge"]
command_aliases = { ok = "approve", ship = "merge" }
command_prefix = "/"
disable_mentions = false
disable_bare_commands = false
disable_unapprove = false
disable_reactions = false
disable_deleted_comments = false
allow_self_approval = false
allow_draft_merges = false
```

A variable still holding the JSON object this used to be is read as before, and Smyklot warns at startup. It cannot open a pull request migrating that one: the App has no permission to write Actions variables.

An unknown key refuses to start. A typo in `allowed_commands` used to be dropped without a word, which left every command allowed.

#### Option 2: Repository config file

Check `.github/smyklot.yaml` into the repository:

```yaml
quiet_success: true
command_prefix: "!"
allowed_commands:
  - approve
  - merge

# Which entry point handles this repository, if not the service
runner: action
```

Settings the file omits keep whatever the workflow or the service was started with, so a file need only list what it changes.

TOML is what Smyklot asks for now, at `.smyklot.toml`, `.smyklot/config.toml` or `.github/.smyklot.toml`. `.github/smyklot.yaml` is still read, and the service opens a pull request moving it across - one commit that adds the TOML and removes the YAML, so the repository never carries both. Close that pull request and Smyklot will not ask again; the panel is where an operator puts it back on the table.

When a repository carries more than one of these, the first in that order wins and the panel names the others.

Editors complete and check the file from a published JSON Schema. Point one at it with a directive on the first line, which is what the migration pull request writes:

```toml
#:schema https://smyklot.com/schema/repository-v1.json
```

Smyklot serves that document itself, from the same build that reads the file, so the two cannot describe different settings.

This is the only per-repository configuration the [service](#running-as-a-service) can see - it has no access to a repository's Actions variables. The Action reads the same file, so a repository gets the same behaviour whichever one handles the comment.

The file is read from the **default branch**, so a pull request cannot change how its own commands are handled.

If it cannot be parsed, no command runs and the bot replies saying so. It does not fall back to defaults: this file is where a repository narrows `allowed_commands`, and ignoring a broken one would quietly restore commands the repository had turned off.

#### Option 3: Individual variables

Configure individual settings via repository variables or environment variables with `SMYKLOT_` prefix:

| Variable                           | Type    | Default        | Description                                                                |
| ---------------------------------- | ------- | -------------- | -------------------------------------------------------------------------- |
| `SMYKLOT_QUIET_SUCCESS`            | boolean | `false`        | Disable success feedback comments                                          |
| `SMYKLOT_QUIET_REACTIONS`          | boolean | `false`        | Disable reaction-based approval/merge comments                             |
| `SMYKLOT_QUIET_PENDING`            | boolean | `false`        | Disable pending CI comments (reactions only for "merge after CI")          |
| `SMYKLOT_ALLOWED_COMMANDS`         | list    | all            | Limit which commands are allowed                                           |
| `SMYKLOT_COMMAND_ALIASES`          | map     | default        | Define custom command aliases                                              |
| `SMYKLOT_COMMAND_PREFIX`           | string  | `/`            | Custom command prefix                                                      |
| `SMYKLOT_DISABLE_MENTIONS`         | boolean | `false`        | Disable mention commands                                                   |
| `SMYKLOT_DISABLE_BARE_COMMANDS`    | boolean | `false`        | Disable bare commands                                                      |
| `SMYKLOT_DISABLE_UNAPPROVE`        | boolean | `false`        | Disable unapprove command                                                  |
| `SMYKLOT_DISABLE_REACTIONS`        | boolean | `false`        | Disable reaction-based approvals/merges                                    |
| `SMYKLOT_DISABLE_DELETED_COMMENTS` | boolean | `false`        | Disable the notice posted when a command comment is deleted                |
| `SMYKLOT_ALLOW_SELF_APPROVAL`      | boolean | `false`        | Allow PR authors to approve their own PRs                                  |
| `SMYKLOT_ALLOW_DRAFT_MERGES`       | boolean | `false`        | Mark draft PRs ready for review before executing merge commands            |
| `SMYKLOT_RUNNER`                   | string  | `service`      | Which entry point acts: `service` or `action`; the other stands down       |
| `SMYKLOT_BOT_USERNAME`             | string  | `smyklot[bot]` | Bot username for cleanup operations (GitHub App format: `{app-slug}[bot]`) |
| `SMYKLOT_GITHUB_API_URL`           | string  | public API     | REST API base URL for a proxy or mirror (Enterprise is not supported)      |

A list is separated by commas or whitespace, and a map is written `name=value` pairs the same way: `SMYKLOT_COMMAND_ALIASES="ok=approve,ship=merge"`. A variable set to nothing counts as unset, so a workflow forwarding an input nobody filled in changes nothing.

Every setting except `runner` also takes a command-line flag named after it, and `--config-file` names a TOML file of them.

#### Configuration examples

##### Example 1: Quiet mode

Only show emoji reactions, no success comments:

```yaml
# In workflow or as repository variable
env:
  SMYKLOT_QUIET_SUCCESS: "true"
```

Result: User sees only ✅ reaction, no "PR Approved" comment.

##### Example 2: Custom prefix

Use `!` instead of `/` for commands:

```yaml
env:
  SMYKLOT_COMMAND_PREFIX: "!"
```

Users can now use `!approve` and `!merge`.

##### Example 3: Command aliases

Create shortcuts for commands:

```yaml
env:
  SMYKLOT_COMMAND_ALIASES: "app=approve,a=approve,m=merge"
```

Users can use `/app`, `/a`, or `/m` as shortcuts.

##### Example 4: Reactions only

Disable comment-based commands, only allow reactions:

```json
{
  "disable_mentions": true,
  "disable_bare_commands": true,
  "command_prefix": "disabled"
}
```

Only 👍 and 🚀 reactions will work.

##### Example 5: Disable reaction tracking

Don't remove approvals/merges when reactions are removed:

```yaml
env:
  SMYKLOT_DISABLE_DELETED_COMMENTS: "true"
```

##### Example 6: Allow self-approval

⚠️ Not recommended for production - allows PR authors to approve their own PRs:

```yaml
env:
  SMYKLOT_ALLOW_SELF_APPROVAL: "true"
```

or via JSON:

```json
{
  "allow_self_approval": true
}
```

By default, Smyklot prevents self-approval to enforce separation of duties. Only enable this in development/testing environments.

##### Example 7: Merge draft pull requests

Allow every merge command and 🚀 reaction to publish a draft before continuing:

```yaml
env:
  SMYKLOT_ALLOW_DRAFT_MERGES: "true"
```

This does not bypass reviews or required checks. If the pull request is converted back to draft while waiting for CI, Smyklot cancels the pending merge.

Action workflows that enable this setting must pass the immutable event revision as `COMMENT_UPDATED_AT: ${{ github.event.comment.updated_at }}` alongside `COMMENT_BODY`. Smyklot rejects a delayed workflow when the live comment no longer matches both values.

## Running as a service

Smyklot can run as a long-running process instead of a per-comment workflow. One process serves every repository the App is installed on, so no repository needs a workflow file, and a command takes effect without a workflow run being queued first.

```bash
smyklot serve
```

Point the GitHub App's webhook at `https://your-host/webhook` and set the same secret the process reads. Subscribe the App to **Issue comment**, **Check run**, **Check suite**, **Status**, and **Pull request** events. The App needs **Checks** write, **Commit statuses** read, **Administration** write, and **Merge queues** read in addition to its existing command permissions. Checks write lets Smyklot publish the merge authorization; Administration write lets it own the required-status-check ruleset; Merge queues read lets it fail closed on repositories whose queue commits Smyklot does not support. Existing installations must approve the new permissions before check mode can become ready.

### Service configuration

| Variable                          | Flag                        | Default                          | Description                                                                  |
| --------------------------------- | --------------------------- | -------------------------------- | ---------------------------------------------------------------------------- |
| `SMYKLOT_WEBHOOK_SECRET`          | -                           | required                         | Secret GitHub signs deliveries with; the process refuses to start without it |
| `SMYKLOT_LISTEN_ADDRESS`          | `--listen`                  | `:8080`                          | Address to listen on                                                         |
| `SMYKLOT_ADMIN_ADDRESS`           | `--admin-listen`            | `:9090`                          | Address for probes, metrics and recent failures                              |
| `SMYKLOT_WEBHOOK_PATH`            | `--webhook-path`            | `/webhook`                       | Path GitHub delivers to                                                      |
| `SMYKLOT_POLL_INTERVAL`           | `--poll-interval`           | `5m`                             | How often to sweep reactions and PRs waiting for CI; `0` disables            |
| `SMYKLOT_PENDING_CI_QUIET_PERIOD` | `--pending-ci-quiet-period` | `30s`                            | How long passing CI must remain unchanged before Smyklot merges              |
| `SMYKLOT_PATH_INDEX_INTERVAL`     | `--path-index-interval`     | `1h`                             | How often a repository's file list is checked; `0` checks every sweep        |
| `SMYKLOT_LOG_FORMAT`              | `--log-format`              | `json`                           | `json` or `text`                                                             |
| `SMYKLOT_LOG_LEVEL`               | `--log-level`               | `info`                           | `debug`, `info`, `warn` or `error`                                           |
| `GITHUB_APP_PRIVATE_KEY`          | -                           | required                         | PEM-encoded App private key                                                  |
| `GITHUB_APP_CLIENT_ID`            | -                           | recommended                      | App client ID, preferred for App JWTs                                        |
| `GITHUB_APP_ID`                   | -                           | optional                         | Numeric App JWT fallback when no client ID is set                            |
| `SMYKLOT_PANEL_PUBLIC_ORIGIN`     | `--panel-public-origin`     | disabled                         | Browser-visible scheme and host; setting it enables the panel                |
| `SMYKLOT_PANEL_BASE_PATH`         | `--panel-base-path`         | `/panel`                         | Public path subtree for the panel                                            |
| `SMYKLOT_DATABASE_URL`            | `--database-url`            | `/var/lib/smyklot/panel.sqlite3` | Where service state lives: a `postgres://` URL, or a path for SQLite         |
| `SMYKLOT_PANEL_SUPER_ROOT_ID`     | `--panel-super-root-id`     | required when panel is enabled   | Numeric GitHub user ID assigned as the singleton Super Root                  |
| `SMYKLOT_PANEL_SESSION_TTL`       | `--panel-session-ttl`       | `12h`                            | Signed-in panel session lifetime                                             |
| `SMYKLOT_PANEL_CLIENT_ID`         | -                           | required when panel is enabled   | Client ID of the OAuth App that signs panel users in                         |
| `SMYKLOT_PANEL_CLIENT_SECRET`     | -                           | required when panel is enabled   | Client secret of that OAuth App                                              |

The webhook secret, private key and OAuth client secret have no flag on purpose - a flag would put them in the process table. An explicit flag beats the environment for everything else.

`SMYKLOT_STATE_PATH` and `SMYKLOT_PANEL_STATE_PATH`, with their flags, still work and still mean a SQLite file. They are deprecated in favour of `SMYKLOT_DATABASE_URL`, which says the same thing and can also name a server. Setting more than one is an error rather than a guess.

### Administration panel

Set `SMYKLOT_PANEL_PUBLIC_ORIGIN` to enable the panel. The configured numeric `SMYKLOT_PANEL_SUPER_ROOT_ID` is matched against GitHub's immutable user ID on sign-in. Changing it promotes the new identity and demotes the former Super Root to Root when the new identity next signs in.

#### Sign-in registration

The panel signs users in through a **classic OAuth App**, registered separately from the GitHub App the bot acts as, and never through the GitHub App itself. Authorizing a GitHub App shows the permissions its registration asks for, so signing in through it asks someone who only wants to read a dashboard to grant write access to pull requests and issues. Nothing the client sends trims that screen: a GitHub App ignores the `scope` parameter and uses fine-grained permissions instead. An OAuth App does honour `scope`, and the panel asks for none, so the screen offers public profile read alone.

Register the OAuth App under the same account or organization that owns the GitHub App, with `<public origin><base path>/auth/github/callback` as its authorization callback URL, then set `SMYKLOT_PANEL_CLIENT_ID` and `SMYKLOT_PANEL_CLIENT_SECRET` from it. The service refuses to start if the panel is enabled without both. The panel calls `GET /user` once with the resulting token and discards it; nothing is stored but the profile it returns.

The panel synchronizes every workspace and repository visible to the App every five minutes. A workspace is one GitHub App installation as the panel presents it. Personal-account ownership follows the immutable GitHub user ID. Organization ownership follows organization members with the admin role and requires read-only **Members** organization permission on the GitHub App. Existing installations must approve that added permission before Owner synchronization succeeds. Regular access fails closed when an Owner snapshot is unavailable or more than 15 minutes old; Root diagnostics retain the workspace record. New workspaces default to **Off**, so the service only handles repositories an administrator enables deliberately. Account settings act as defaults, and the panel is one of the eight layers listed under [Configuration](#configuration) - which is the only place that order is written down. A repository may explicitly bypass an invalid file; that exception is visible and audited.

#### Merge after CI checks

Panel-managed workspaces default to check mode. The workspace settings choose the default mode, protected branch patterns, and stable-passing quiet period; each repository may inherit or override them. The GitHub Action and a service running without the panel retain label mode.

Check mode owns one repository ruleset named `Smyklot: merge after CI`. It requires the app-bound `Smyklot / merge after CI` context on the selected raw GitHub refs, whose default is `~DEFAULT_BRANCH`. Smyklot writes a successful baseline Check Run for every in-scope open pull request without an authorization, so the required context does not block ordinary work. A merge-after-CI command changes that exact head's check to in progress. After the other selected checks pass twice and remain unchanged for the configured quiet period, Smyklot marks its check successful and merges that exact head.

A head or base change never inherits the old authorization. Smyklot publishes an `action_required` check on the new head with a **Reauthorize** action. A user who still has the command permission can approve the new revision with one click; repeated changes replace the candidate. A quiet period of zero removes the delay but still requires two matching observations.

Readiness is fail-closed and visible on the repository settings page. Missing Checks or Administration write permission, missing Commit statuses or Merge queues read permission, a conflicting required context, two open pull requests sharing one head, or a merge queue prevents new check-mode commands. Saved settings remain in place while Smyklot retries reconciliation. Switching from checks to labels drains authorized check work before Smyklot removes the required ruleset. Switching from labels to checks preserves existing label authorizations while new check commands begin only after baselines and the required ruleset are ready. Smyklot removes only the ruleset whose ID it recorded as its own.

State lives in SQLite or PostgreSQL, chosen by `SMYKLOT_DATABASE_URL`: a `postgres://` or `postgresql://` URL picks PostgreSQL, and a bare path or `sqlite://` URL picks SQLite. Both drivers are pure Go, so the image still builds with `CGO_ENABLED=0`. Nothing above the storage package knows which one is running, and a linter enforces that.

The two engines are not a lowest common denominator. PostgreSQL stores timestamps as `timestamptz`, booleans as `boolean` and configuration patches as indexed `jsonb`; SQLite keeps the text and integer spellings it has always used. What they share is one set of queries, so a change lands in both at once, and one conformance suite, so parity is proven rather than assumed.

SQLite is still the smaller-deployment default and needs a writable volume. PostgreSQL needs none, which is what makes a read-only root filesystem workable. Either way the service runs one replica: deliveries are de-duplicated in memory and the reaction sweep has no leader election.

Moving between them keeps the data - `smyklot store migrate --from <old> --to <new>` copies every row and verifies the counts. Finished delivery history is retained for 30 days; audit history is not pruned.

The Root console reports whichever engine is live. Its overview carries a Database card beside service health - the engine and its release, the schema version applied, the size on disk, how long the database took to answer, and the connection pool with the number of callers that have ever queued for a free connection. Root settings repeats all of it as a reference list. The engine reports these itself through the storage port, so the panel prints the name and never branches on it.

A pool with waits behind it is the signal PostgreSQL introduced and a SQLite file never had: the queries still succeed, so nothing fails, but the service is waiting on the database rather than on GitHub. That count only ever grows, which is why it is shown next to the sampled counts that do not.

Run `mise run panel:dev:mock` to inspect every panel state with deterministic local data. The mock server uses the same HTTP response types and server-sent event shape as production.

### Organization sync

The service can keep every repository in a workspace carrying the same labels, set to the same repository settings, enforcing the same rulesets, and holding the same shared files. All four are configured in the panel, per workspace, and each is switched on separately.

Nothing is changed without being shown first. A reconcile works out what would differ, stores it as a plan, and waits: the panel lists every change against every repository, and applying is a second act. A plan is invalidated the moment the configuration behind it changes, so nobody can approve a screen that has gone stale.

A repository may opt out of one kind and keep the others. Removal is off unless it is switched on, and a label pattern can be excluded from it, so a label somebody added by hand is not deleted because it is not in the list.

Settings carry a rule labels do not. Every one of them has three states - on, off, and not configured - and the third means the repository keeps whatever it has. Some combinations GitHub refuses outright, with a 422 on the whole request, so those are found before they are sent: a commit wording whose merge strategy would be off, a change that would leave a repository with no way to merge at all, and a security feature the repository does not have. Each is left alone and named in the plan with the reason, rather than costing the repository every other setting in the same change.

Rulesets are how branch protection is expressed here, because a ruleset takes a ref pattern - `refs/heads/release/*` protects the release branch cut tomorrow, where the branch-protection endpoint takes one concrete branch and protects only what exists today. A ruleset the configuration names is owned whole: GitHub writes one by replacement, so what the configuration does not say stops being enforced, and the plan says what would go before it goes. A ruleset the configuration no longer names can be removed, which is the one thing here that destroys something somebody may have made by hand, so it needs removal switched on and it appears in a plan first. A ruleset the organization defines is read but never written: it is not the repository's to change, and the plan says when a repository-level one would be enforced beside it.

Files are the one kind that does not write to a repository. The templates are configuration rather than files kept somewhere else, and what a repository should end up with arrives as a pull request it can merge or close. Everything a repository needs goes into one commit behind one pull request, so a change lands whole or not at all. No branch is ever force-pushed and none is ever deleted: the commit is built on whatever the branch already points at, so a reviewer's fixup is a commit this one descends from rather than something that disappears.

Closing that pull request is how a repository refuses, and leaving it open is how it takes its time - neither is asked again. The branch is named after what the files should end up saying, so a proposal already in front of a repository answers for that change, and a configuration that changes is a different branch, which asks once more.

Deletion is a named list of retired paths and nothing else: a path named there is removed wherever a repository still has it, and there is no switch that removes what the configuration does not name. A repository can adjust a template rather than take it whole - merged by key for JSON and YAML, by heading for Markdown - and a merge that cannot be applied is an error rather than a fallback, so a broken adjustment leaves the file alone instead of overwriting it with the plain template.

git will put a file wherever a commit names one and say nothing about what it replaced, so a path a repository holds as a directory, a link or a submodule, or one whose parent is a file, refuses that repository whole rather than being written over - and it says which path and what git records there. Configuring a path and another path inside it is refused where it is typed, since no repository could hold both.

A repository refused that way receives none of the organization's files, and the panel says so on its Sync pane: what stopped it, in the words the planner used, and when it last looked. A refusal is asked again every sweep rather than held for the six hours a settled repository is, so a fix clears the notice on the next pass without anybody retrying anything.

Labels need the **Issues** write access the bot already holds. Repository settings and configured organization-sync rulesets need **Administration** write, and files need **Contents** write. Merge-after-CI check mode separately needs **Checks** write and **Administration** write for its owned required-context ruleset, **Commit statuses** read to observe legacy contexts, and **Merge queues** read to refuse unsupported queue-backed branches. An installation that has not approved a permission is not an error: that operation stands down, says which permission it wants, and unrelated work carries on.

Files under `.github/workflows/` need **Workflows** write on top of Contents. GitHub keeps them behind a permission of their own and enforces it where the branch moves, so a configuration naming one is checked before anything is planned rather than after somebody has approved it - and a retired workflow counts, since removing one is writing the tree that no longer holds it.

A repository that already matches is not read again for six hours. That is what keeps a steady sweep at almost no cost, and the horizon is what lets it still notice a label renamed by hand.

### Watching a running service

The webhook port carries `GET /healthz` for an ingress or tunnel that needs one reachable path. Everything else is on the admin port, which is not meant to be public: queue depth, failure reasons and Go runtime detail describe the service to anyone who can read them.

| Route       | Answers                                                                     |
| ----------- | --------------------------------------------------------------------------- |
| `/livez`    | 200 while the process is running                                            |
| `/readyz`   | 200 while GitHub and enabled panel storage are reachable                    |
| `/metrics`  | Prometheus exposition format                                                |
| `/failures` | The last 50 deliveries that were accepted and then failed, newest first     |

Liveness and readiness are separate because a restart cannot fix GitHub being down. The service checks GitHub every 30 seconds and starts unready, so it takes no traffic until its credentials have been proven.

Metrics are prefixed `smyklot_`: `webhook_requests_total{event,outcome}`, `deliveries_total{action,result}`, `delivery_duration_seconds`, `deliveries_in_flight`, `queue_depth`, `queue_capacity`, `sweeps_total{result}`, `sweep_duration_seconds` and `ready`. A rising `outcome="unsigned"` means a rotated secret or someone probing the port; a rising `outcome="refused"` means the queue is full.

Every log line about a delivery carries its `delivery_id`, along with the repository, pull request and comment action, so one webhook's whole trail can be found by that identifier. A delivery is answered before its command runs, so GitHub's own log shows a success either way; `/failures` and the `result="failure"` counter are how a failure afterwards stays visible.

The webhook secret and the App private key are replaced with `[REDACTED]` wherever they would reach a log line or the failures endpoint.

### How it differs from the Action

Each delivery names its own installation, so the process mints a token per installation as deliveries arrive. Installing the App on another repository needs no restart and no configuration.

Without the panel, behaviour per repository comes from [`.github/smyklot.yaml`](#option-2-repository-config-file), layered over the process's `SMYKLOT_CONFIG`. With the panel enabled, account settings sit below the file and repository panel settings sit above it. Actions repository variables are invisible to a process running outside Actions.

GitHub sends no webhook when someone adds or removes a reaction, so reaction commands are found by sweeping open pull requests on `--poll-interval`. Pending-CI requests use a durable scheduler and require passing checks to remain unchanged for `--pending-ci-quiet-period` before merge. This replaces the `poll-reactions.yaml` workflow.

A delivery is answered before its command runs, because GitHub allows ten seconds and does not retry a delivery that times out. A redelivery of an event that already took effect is recognised and skipped.

### Deploying to Kubernetes

The chart is published alongside the image, at the same version:

```bash
kubectl create secret generic smyklot-credentials \
  --from-literal=webhook-secret="$WEBHOOK_SECRET" \
  --from-file=private-key=key.pem

helm install smyklot oci://ghcr.io/smykla-skalski/charts/smyklot \
  --version 1.13.0 \
  --set github.clientId=Iv23liExample \
  --set github.existingSecret=smyklot-credentials \
  --set ingress.enabled=true \
  --set ingress.host=smyklot.example.com
```

The webhook secret and the private key are never chart values, only the name of a Secret holding them, so neither can end up in a values file or in `helm get values` output. Both settings are required and the chart refuses to render without them.

The ingress routes the webhook path and nothing else. Probes, metrics and the failures endpoint stay on the admin port, which the chart puts on the Service for probes and Prometheus but on no public route.

The chart runs one replica and updates with `Recreate`. Deliveries are de-duplicated in memory and the reaction sweep has no leader election, so a second process sweeps the same repositories and can act on the same reaction twice. A restart costs a few seconds of refused deliveries, which GitHub records and can redeliver; deliveries already running are given up to 45 seconds to finish, which is what `terminationGracePeriodSeconds` covers.

Readiness answers no while GitHub is unreachable, which takes the pod out of the Service. Deliveries then fail at the ingress instead of being accepted and dropped, and GitHub keeps them for redelivery.

Set `serviceMonitor.enabled` for a Prometheus Operator cluster. `helm show values oci://ghcr.io/smykla-skalski/charts/smyklot` lists everything else.

### Choosing which one handles a repository

The service handles every repository the App is installed on. A repository that should stay on the Action says so in its own [`.github/smyklot.yaml`](#option-2-repository-config-file):

```yaml
runner: action
```

Whichever entry point is not named stands down: no reaction, no comment, no approval. The Action leaves its reason in the job summary, since the pull request is where the service is already replying.

This is also the rollback. A repository moves back to the Action with one commit and no redeploy, and the workflow files can stay in place throughout - a workflow whose repository is on the service just exits without doing anything.

The Action reads the file on its very next run, since a workflow is a fresh process. The service caches it for 30 seconds, deliberately far shorter than the hour it caches CODEOWNERS for: that file only decides who is allowed to approve, while this one decides whether the service acts on the repository at all.

`runner` defaults to `service`, so **a repository upgrading past the release that added it stops responding to the Action unless it sets `runner: action` or has a service running.** Set it before you upgrade if that repository has no service behind it.

## Architecture

### How it works

1. User comments a command or adds a reaction on a PR
2. GitHub triggers `issue_comment` webhook
3. `pr-commands.yaml` workflow starts
4. Smyklot:
   - Parses the command (supports slash, mention, and bare formats)
   - Or processes reactions (👍 for approve, 🚀 for merge, ❤️ for cleanup)
   - Validates command combinations (cleanup cannot be combined with others)
   - Checks for duplicate approvals (prevents re-approving for both commands and reactions)
   - Fetches `.github/CODEOWNERS` via GitHub API
   - Checks user permissions
   - Calls GitHub API to approve/merge/cleanup
   - For merge: tries specified method or falls back (merge → squash → rebase)
   - Posts reactions and feedback
5. On comment edit/delete or reaction removal, updates accordingly

### Permission system

Supported today:

- Global owners (`* @username`), who can approve or merge any PR
- Team ownership (`@org/team-name`), resolved through the GitHub API
- Reaction-based approvals and merges, with removal tracking
- Self-approval prevention, configurable and off by default
- Fail-closed CODEOWNERS parsing - a corrupted file gives nobody permission

Not implemented: path-specific ownership patterns, approval scoped to the files
a PR changes, and a required-approvals count.

### Security

Smyklot implements defense-in-depth security practices.

#### Input validation

- Comment body length validation (max 10KB) - prevents DoS attacks
- Repository owner/name format validation - prevents path traversal
- All inputs passed via environment variables (no shell interpolation)

#### API security

- Parameterized GraphQL queries - prevents injection attacks
- HTTP client timeout (30s) - prevents hung requests
- Exponential backoff retry logic - handles rate limiting gracefully
- Connection pooling - optimizes resource usage

#### Access control

- CODEOWNERS-based authorization with fail-closed parsing
- Self-approval prevention (configurable, disabled by default)
- Minimal workflow permissions (contents: read, pull-requests: write)
- Token-based authentication via GitHub App

#### Data protection

- Sensitive data sanitization in logs (tokens, keys, secrets redacted)
- Maximum CODEOWNERS file size (1MB) - prevents memory exhaustion
- No repository checkout required (CODEOWNERS fetched via API)

#### Supply chain security

- Actions pinned by commit digest
- Go dependencies verified (`go mod verify`)
- Docker images use minimal base (`FROM scratch`)

## Development

### Requirements

- Go 1.25+
- [mise](https://mise.jdx.dev/) for tool management

### Setup

```bash
# Clone repository
git clone https://github.com/smykla-skalski/smyklot.git
cd smyklot

# Install tools
mise install

# Download dependencies
mise run deps

# Run tests
mise run test
```

### Available tasks

```bash
mise tasks ls            # Show available tasks
mise run test            # Run all tests with coverage
mise run test:unit       # Run unit tests only
mise run lint            # Run all linters
mise run build           # Build binaries
mise run clean           # Clean repository-local artifacts
```

### Testing

All tests use Ginkgo/Gomega BDD framework:

```bash
# Run all tests
mise run test

# Run specific package
ginkgo -r pkg/commands/

# Watch mode for TDD
ginkgo watch -r
```

Current test coverage: 130+ tests passing

- 52+ command parser tests
- 12 CODEOWNERS parser tests
- 30 permission checker tests
- 30 feedback system tests
- 18+ GitHub client tests

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feat/amazing-feature`)
3. Write tests first (TDD)
4. Implement the feature
5. Ensure all checks pass: `mise run lint && mise run test`
6. Commit with conventional commits (`feat:`, `fix:`, `docs:`, etc.)
7. Push to your fork
8. Open a pull request

## License

MIT License - see [LICENSE](LICENSE) for details

## Acknowledgments

Built with:

- [Ginkgo](https://github.com/onsi/ginkgo) - BDD testing framework
- [Gomega](https://github.com/onsi/gomega) - Matcher library
- [Viper](https://github.com/spf13/viper) - Configuration management
- [Cobra](https://github.com/spf13/cobra) - CLI framework
- [mise](https://mise.jdx.dev/) - Tool version and task runner
- [GoReleaser](https://goreleaser.com/) - Release automation

---

Made with ❤️ by [@bartsmykla](https://github.com/bartsmykla)
