# GitHub Actions Workflows

## `test.yaml`

- Triggers: push to `main`, `feat/**`, and PRs
- Steps: checkout → mise install → `ginkgo -r --race --cover` → golangci-lint → `go mod verify`

## `pr-commands.yaml`

- Triggers: `issue_comment` on PRs
- Uses local action reference (`uses: ./`)
- Environment variables from GitHub context: `GITHUB_TOKEN`, `COMMENT_BODY`, `COMMENT_ID`, `PR_NUMBER`, `REPO_OWNER`, `REPO_NAME`, `COMMENT_AUTHOR`

## Security Practices

- Actions pinned by **commit digest** (not tags)
- Untrusted input via **environment variables** (not CLI args, no shell interpolation)
- Minimal permissions: `contents: read`, `pull-requests: write`

## Running Locally

The binary reads environment variables — set `GITHUB_TOKEN`, `COMMENT_BODY`, `COMMENT_ID`, `PR_NUMBER`, `REPO_OWNER`, `REPO_NAME`, `COMMENT_AUTHOR` then run `./bin/smyklot-github-action`.

## Input Validation Limits

- Comment body: max 10KB
- Repository name: alphanumeric + hyphens only
- CODEOWNERS file: max 1MB
- HTTP client: 30s timeout with connection pooling
- Retry: exponential backoff for 429/5xx errors
