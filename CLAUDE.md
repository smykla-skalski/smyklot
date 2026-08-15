# CLAUDE.md

Smyklot: GitHub App for automated PR approvals/merges based on CODEOWNERS.
Go + Ginkgo/Gomega, deployed as Docker-based GitHub Action.

## Commands

- Build: `task build`
- Test (all): `task test`
- Test (unit only): `task test:unit`
- Test (single package): `ginkgo -r pkg/commands`
- Test (focused): `ginkgo -r --focus "parses slash commands" pkg/commands`
- Test (watch): `ginkgo watch -r`
- Lint (all): `task lint`
- Lint (Go only): `task lint:go`
- Lint (markdown): `task lint:markdown`
- Pre-commit: `task lint && task test`

## Architecture

- `cmd/github-action/` — three entrypoints in one binary: the Action (`main.go`), the cron sweep (`poll.go`), and the webhook service (`serve.go`, `server.go`, `sweep.go`)
- `pkg/commands/` — parses PR comments into `Command` structs; called by entrypoint handlers
- `pkg/permissions/` — parses `.github/CODEOWNERS` (global `*` pattern only), checks if user is owner; called before approve/merge
- `pkg/config/` — loads config via Viper (CLI flags > env vars > JSON > defaults); consumed by all handlers
- `pkg/feedback/` — builds reaction/comment responses; called after each command execution
- `pkg/github/` — GitHub API client (REST + GraphQL); used by all handlers for approvals, merges, reactions, comments
- `pkg/githubapp/` — mints and caches App JWTs and per-installation tokens; the service needs one token per installation
- `pkg/webhook/` — parses `issue_comment` deliveries and de-duplicates them; re-exports signature verification from `go-githubauth/webhook`
- `pkg/logging/` — builds the `slog` logger, carries it on the context, and redacts known secrets from every line
- `pkg/metrics/` — the Prometheus collectors the service reports, on a registry it owns rather than the default one
- Data flow (Action): env vars → `run()` → client → repo config → `executeComment`
- Data flow (service): signed delivery → `handleDelivery` → dedupe → worker → installation token → client → repo config → `executeComment`

## Gotchas

- CODEOWNERS parser is **fail-closed** — if parsing fails, no one has permissions (`pkg/permissions/errors.go:18`)
- Cleanup command **cannot** be combined with other commands — parser rejects the entire comment (`pkg/commands/parser.go:49`)
- Success feedback is **reaction-only** (no comment); errors/warnings post both reaction AND comment (`pkg/feedback/feedback.go:15`)
- Only global owners (`*` pattern) supported in Phase 1 — path-specific patterns are not implemented
- Self-approval is disabled by default; enable with `allow_self_approval` config option (`pkg/config/config.go:72`)
- All GitHub Action inputs come via **environment variables**, not CLI args (security: no shell interpolation)
- Workflow files use `.yaml` extension (not `.yml`) for consistency
- `serve` **refuses to start** without `SMYKLOT_WEBHOOK_SECRET` — fail closed, or anyone reaching the port could drive the bot
- Webhook signatures cover the **body only**; header values like `X-GitHub-Delivery` are unverified (`cmd/github-action/server.go:safeDeliveryID`)
- Delivery dedupe keys on comment id + `updated_at`, **not** the delivery GUID — GitHub does not document whether the GUID survives a redelivery
- The `runner` key in `.github/smyklot.yaml` decides who acts, and it defaults to **`service`** — the Action stands down unless a repo sets `runner: action`. Both entry points check it, at all four places work starts: `run`, `runPoll`, `handleIssueComment`, `sweepRepo` (`cmd/github-action/runner.go`)
- Standing down is **silent on the PR** — the other entry point has already reacted. The Action's reason goes to the job summary instead
- `repoConfigTTL` (30s) is deliberately far shorter than `codeownersTTL` (1h) and shorter than the sweep interval. CODEOWNERS decides who may approve; `.github/smyklot.yaml` decides whether the service acts at all, so a stale copy means a rolled-back repo gets answered by both (`cmd/github-action/server.go`)
- An unparseable `.github/smyklot.yaml` is **fail-closed with feedback** — no command runs, and the bot says why. Never fall back to defaults: the file is where `allowed_commands` is narrowed
- `dispatch` must never send on `s.jobs` directly — use `enqueue`, which holds `queueMu` for read. `Shutdown` abandons a running handler once its deadline passes, and a bare send on the closed queue panics rather than taking `default` (`cmd/github-action/server.go`)
- Metrics live on the **admin listener**, never the webhook one — the webhook port faces the internet, and queue depth and failure reasons should not
- Never use a request header as a metric label — the signature does not cover headers, so an unbounded value mints a time series per request (`eventLabel` in `cmd/github-action/server.go`)
- Log through `logging.From(ctx)` where the work carries per-item attributes (a delivery, a repo, a PR); use `s.logger` in background loops that carry none (`probe`, `pollLoop`, `drain`). Never `log` or `fmt.Print`
- Whoever **starts** an attribute chain seeds it — `sweep` does `logging.Into(ctx, s.logger)` itself rather than trusting its caller, so a direct call still logs where it should
- Attach an attribute in **one** place. `pollAllPRs` owns `repo` and `processPR` owns `pr`; adding either again downstream prints it twice
- The chart runs **one replica** with `Recreate` — dedupe is in-memory and the sweep has no leader election, so a second process double-acts on reactions (`charts/smyklot/values.yaml`)
- Panel sign-in uses a **classic OAuth App**, never the GitHub App — `SMYKLOT_PANEL_CLIENT_ID` / `SMYKLOT_PANEL_CLIENT_SECRET`, with no fallback to `GITHUB_APP_CLIENT_ID`. Authorizing a GitHub App shows whatever its registration asks for, so signing in through it listed the bot's write permissions on the consent screen. Setting `Scopes` cannot fix that: a GitHub App ignores `scope`, and `x/oauth2` omits the parameter entirely when the slice is empty (`internal/panel/github.go`)
- The webhook secret and private key are **never chart values**, only `github.existingSecret` — a value would land in a values file and in `helm get values`
- Chart version and `appVersion` are rewritten by semantic-release, so `helm install --version X` and `smyklot:X` are always one release (`.releaserc.yml`)

## Code Style

- Wrap errors with `fmt.Errorf` and `%w`, or with the typed constructors in `cmd/github-action/errors.go` (`NewGitHubError`, `NewInputError`, `NewConfigError`)
- Sentinel errors: `var ( ErrX = errors.New("...") )` block pattern — see `pkg/permissions/errors.go:10`
- Test tags: `[Unit]` or `[Integration]` in Describe block — e.g., `Describe("Parser [Unit]", ...)`
- Ginkgo BDD structure: `Describe/Context/It` with table-driven `Entry` where appropriate
- Use `httptest` for mocking GitHub API in tests (`pkg/github/client_test.go:16`)

## Git Workflow

- Conventional commits: `feat:`, `fix:`, `docs:`, `refactor:`, `test:`
- Commit flags: always use `-sS` (sign-off + GPG sign)
- `feat:` → minor bump, `fix:` → patch bump, `feat!:` → major bump
- Releases: fully automated via `auto-release.yaml` (see `RELEASING.md`)

## Common Tasks

### Adding a New Command

1. Add test in `pkg/commands/parser_test.go`
2. Implement in `pkg/commands/parser.go`
3. Add command type to `pkg/commands/types.go`
4. Add handler in `cmd/github-action/main.go`
5. Update README command table

### Adding a New Feedback Type

1. Add test in `pkg/feedback/feedback_test.go`
2. Implement `New*` function in `pkg/feedback/feedback.go`
3. Use in command handlers

### Modifying GitHub API Client

1. Add/update test in `pkg/github/client_test.go`
2. Implement in `pkg/github/client.go`
3. Use `httptest` for mocking

## Configuration

Config precedence: CLI flags > env vars (`SMYKLOT_*` prefix) > JSON (`SMYKLOT_CONFIG`) > defaults.
See `pkg/config/` for all options and `README.md` for full configuration reference.

## Phase Status

- Phase 1 (GitHub Action): complete
- Phase 2 (path-specific CODEOWNERS, teams): planned
- Phase 3 (Kubernetes deployment): future — see `.claude/rules/roadmap.md`
