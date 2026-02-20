# Roadmap

## Phase 2: Enhanced Permissions (Planned)

- Path-specific ownership patterns in CODEOWNERS
- Scoped approval based on changed files
- Team support (`@org/team-name`)
- Required approvals count

## Phase 3: Kubernetes Deployment (Future)

### Remaining Prerequisites

- Structured logging with `slog`
- Request ID propagation through context
- Concurrency tests with `-race` flag
- Comprehensive audit logging

### Deployment Work

- HTTP webhook server
- Helm chart and Prometheus metrics
- Migration strategy from GitHub Action to persistent service

## Phase 4: Discord Integration (Future)

- Discord bot with unified command system
- Cross-platform notifications
