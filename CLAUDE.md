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

- `cmd/github-action/` — entrypoint; parses env vars, routes to command/reaction handlers, calls `pkg/` packages
- `pkg/commands/` — parses PR comments into `Command` structs; called by entrypoint handlers
- `pkg/permissions/` — parses `.github/CODEOWNERS` (global `*` pattern only), checks if user is owner; called before approve/merge
- `pkg/config/` — loads config via Viper (CLI flags > env vars > JSON > defaults); consumed by all handlers
- `pkg/feedback/` — builds reaction/comment responses; called after each command execution
- `pkg/github/` — GitHub API client (REST + GraphQL); used by all handlers for approvals, merges, reactions, comments
- Data flow: webhook event → `cmd/github-action/main.go` → parse command/reaction → check permissions → execute via `pkg/github/` → send feedback

## Gotchas

- CODEOWNERS parser is **fail-closed** — if parsing fails, no one has permissions (see `pkg/permissions/errors.go`)
- Cleanup command **cannot** be combined with other commands — parser rejects the entire comment (`pkg/commands/parser.go`)
- Success feedback is **reaction-only** (no comment); errors/warnings post both reaction AND comment
- Only global owners (`*` pattern) supported in Phase 1 — path-specific patterns are not implemented
- Self-approval is disabled by default; enable with `allow_self_approval` config option
- All GitHub Action inputs come via **environment variables**, not CLI args (security: no shell interpolation)
- Workflow files use `.yaml` extension (not `.yml`) for consistency

## Code Style

- Use `github.com/pkg/errors` for error wrapping (not `fmt.Errorf`)
- Sentinel errors: `var ErrOpName = errors.New("msg")` — see `pkg/permissions/errors.go` for pattern
- Test tags: `[Unit]` or `[Integration]` in Describe block — e.g., `Describe("Parser [Unit]", ...)`
- Ginkgo BDD structure: `Describe/Context/It` with table-driven `Entry` where appropriate
- TDD workflow: write failing test first, implement minimum code, refactor
- Use `httptest` for mocking GitHub API in tests (see `pkg/github/client_test.go`)

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

- Phase 1 (GitHub Action): complete — 181 tests, 59.5% coverage
- Phase 2 (path-specific CODEOWNERS, teams): planned
- Phase 3 (Kubernetes deployment): future — see `.claude/rules/roadmap.md`
