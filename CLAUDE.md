# CLAUDE.md

Smyklot: GitHub App for automated PR approvals/merges based on CODEOWNERS.
Go + Ginkgo/Gomega, deployed as Docker-based GitHub Action.

## Commands

- Discover workflows: `mise tasks ls`
- Build: `mise run build`
- Test (all): `mise run test`
- Test (unit only): `mise run test:unit`
- Test (single package): `ginkgo -r pkg/commands`
- Test (focused): `ginkgo -r --focus "parses slash commands" pkg/commands`
- Test (watch): `ginkgo watch -r`
- Lint (all): `mise run lint`
- Lint (Go only): `mise run lint:go`
- Lint (markdown): `mise run lint:markdown`
- Pre-commit: `mise run ci`

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
- `internal/storage/` — the port: `Store`, the models, the sentinel errors. No `database/sql`, no driver, no engine name
- `internal/storage/sqlstore/` — every query, written once and parameterized by a `Dialect`. Both engines run this
- `internal/storage/sqlite/`, `internal/storage/postgres/` — a driver, a DSN, a dialect and a migration series each, embedding `*sqlstore.Store`
- `internal/storage/open/` — picks the engine from a connection string; the only thing above the port that names one
- `internal/storage/storagetest/` — the conformance suite both engines run, and `Seed`, which fills every table through the port
- `internal/storage/transfer/` — copies a database between engines; behind `smyklot store migrate`
- Data flow (Action): env vars → `run()` → client → repo config → `executeComment`
- Data flow (service): signed delivery → `handleDelivery` → dedupe → worker → installation token → client → repo config → `executeComment`

## Gotchas

- CODEOWNERS parser is **fail-closed** — if parsing fails, no one has permissions (`pkg/permissions/errors.go:18`)
- Cleanup command **cannot** be combined with other commands — parser rejects the entire comment (`pkg/commands/parser.go:49`)
- Success feedback is **reaction-only** (no comment); errors/warnings post both reaction AND comment (`pkg/feedback/feedback.go:15`)
- The CODEOWNERS parser reads the global `*` pattern only — path-specific patterns are not implemented, and `Checker.CanApprove` takes a path and ignores it
- Self-approval is disabled by default; enable with `allow_self_approval` config option (`pkg/config/config.go:72`)
- All GitHub Action inputs come via **environment variables**, not CLI args (security: no shell interpolation)
- Workflow files use `.yaml` extension (not `.yml`) for consistency
- `serve` **refuses to start** without `SMYKLOT_WEBHOOK_SECRET` — fail closed, or anyone reaching the port could drive the bot
- Webhook signatures cover the **body only**; header values like `X-GitHub-Delivery` are unverified (`cmd/github-action/server.go:safeDeliveryID`)
- Delivery dedupe currently keys on comment id + `updated_at`; service deliveries should use the durable webhook inbox instead of assuming an accepted in-memory job survives restart
- Nothing above `internal/storage/**` may import `database/sql`, `modernc.org/sqlite` or `github.com/jackc/pgx`, and nothing outside `internal/storage/**` or `internal/storage/open/**` may import an engine package. `depguard` in `.golangci.yml` enforces both — decoupling that is not enforced decays
- A query goes in `sqlstore` and must run on both engines. What they spell differently goes through the `Dialect`; what one does *better* goes in an override on that engine's `Store`, which embeds the shared one. The shared core is a floor, not a ceiling
- SQLite stores timestamps as fixed-width text (`2006-01-02T15:04:05.000000000Z`) so string order equals time order. `RFC3339Nano` trims trailing zeros and silently breaks `ORDER BY` and expiry comparisons — never format a stored timestamp with it (`internal/storage/sqlstore/time.go`)
- PostgreSQL's `?` is a jsonb operator, so `JSONHasKey` renders the **function** form `jsonb_exists(col, ?)`. The operator form would collide with placeholder rebinding
- The PostgreSQL adapter pins every connection to UTC. A `timestamptz` comes back in the session's zone, which is the server's locale, so without it the same instant formats differently depending on where the database runs
- `Store.Status` names its engine, and that name is **data to be printed**, never a value anything branches on. It returns no error: a database that will not answer is a status worth reading, not a failure to produce one, so the unreachable case is carried in the struct (`Reachable`, `Error`) and every caller renders one shape. The panel turns that into `healthy`/`degraded`/`unavailable` in `databaseState`, which is the only place that decision lives
- Pool pressure is deliberately **not** part of the database's health state. `InUse == Max` is a busy instant, not a fault, and a light that flapped on one would teach an operator to ignore it. `ConnectionStats.WaitCount` is the durable evidence instead - it only grows, so it records a stall the sampled counts have already forgotten
- A read-then-write outside a transaction is only safe on SQLite, which runs one connection. Under PostgreSQL's pool each caller reads its own snapshot — this is how the session cap silently stopped holding. Lock the row (`Dialect.RowLock`) or wrap the pair
- The PostgreSQL specs skip themselves without `SMYKLOT_TEST_POSTGRES_DSN`. `mise run db:dev` prints one; `mise run test:storage:postgres` fails if any spec skipped, because a skip is not proof
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
- Storage specs go in `internal/storage/storagetest`, never beside one adapter — an engine supplies a `Harness` and runs them all, which is what makes parity provable
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

### Adding a Storage Method

1. Add the spec to `internal/storage/storagetest/specs.go` — it runs against both engines
2. Add the method to the port in `internal/storage/store.go`
3. Implement it once in `internal/storage/sqlstore/`, using `?` placeholders and the helpers in `fragments.go`
4. Only if the engines genuinely disagree, add a `Dialect` method rather than branching on the engine
5. `mise run test:storage` runs it on both

### Adding a Table

1. Add the migration to **both** `internal/storage/sqlite/migrations/` and `internal/storage/postgres/migrations/001_baseline.sql` — `TestSchemaParity` fails if they drift
2. Add it to `transfer.tables` in dependency order — `TestTableListCoversSchema` fails if you forget
3. Add it to `storagetest.SeededTables` and fill it in `Seed`, so the copy is proven on real rows

## Configuration

Config precedence: CLI flags > env vars (`SMYKLOT_*` prefix) > JSON (`SMYKLOT_CONFIG`) > defaults.
See `pkg/config/` for all options and `README.md` for full configuration reference.

Storage is one knob: `SMYKLOT_DATABASE_URL` / `--database-url`. A `postgres://` URL picks PostgreSQL; a bare path or `sqlite://` picks SQLite. `SMYKLOT_STATE_PATH` and `SMYKLOT_PANEL_STATE_PATH` are deprecated aliases that still mean a SQLite file. Setting more than one is an error rather than a guess.
