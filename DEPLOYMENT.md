# Deployment Guide

<!-- markdownlint-disable MD013 -->

This guide covers deploying Smyklot to your GitHub repository.

## Prerequisites

- GitHub repository with pull requests
- Ability to create `.github/CODEOWNERS` file
- Ability to create/modify GitHub Actions workflows

## Deployment Steps

### 1. Create CODEOWNERS File

Create `.github/CODEOWNERS` in your repository root:

```text
# Global owners (can approve/merge any PR)
* @yourusername @teammate1 @teammate2
```

**Note**: Only global owners (`* @username`) are supported in Phase 1. Path-specific patterns will be supported in Phase 2.

### 2. Create Workflow File

Create `.github/workflows/pr-commands.yaml`:

Smyklot supports two ways to pass parameters:

- **Inputs** (recommended): Cleaner syntax using action inputs
- **Environment variables**: Alternative approach, useful for compatibility

Both approaches support automatic fallback to environment variables when inputs are not provided.

#### Option A: Using GITHUB_TOKEN (simpler, comments from workflow user)

Using inputs:

```yaml
name: PR Commands

on:
  issue_comment:
    types: [created]

permissions:
  contents: read
  pull-requests: write
  issues: write

jobs:
  handle-command:
    name: Handle PR Command
    if: |
      github.event.issue.pull_request &&
      github.event.comment.user.type != 'Bot' &&
      (
        startsWith(github.event.comment.body, '/approve') ||
        startsWith(github.event.comment.body, '/merge') ||
        contains(github.event.comment.body, '@smyklot')
      )
    runs-on: ubuntu-24.04
    steps:
      - name: Checkout repository
        uses: actions/checkout@08c6903cd8c0fde910a37f88322edcfb5dd907a8 # v5.0.0

      - name: Run Smyklot
        uses: smykla-skalski/smyklot@v0.1.0
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          comment-body: ${{ github.event.comment.body }}
          comment-id: ${{ github.event.comment.id }}
          pr-number: ${{ github.event.issue.number }}
          repo-owner: ${{ github.repository_owner }}
          repo-name: ${{ github.event.repository.name }}
          comment-author: ${{ github.event.comment.user.login }}
```

Using environment variables:

```yaml
      - name: Run Smyklot
        uses: smykla-skalski/smyklot@v0.1.0
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          COMMENT_BODY: ${{ github.event.comment.body }}
          COMMENT_ID: ${{ github.event.comment.id }}
          PR_NUMBER: ${{ github.event.issue.number }}
          REPO_OWNER: ${{ github.repository_owner }}
          REPO_NAME: ${{ github.event.repository.name }}
          COMMENT_AUTHOR: ${{ github.event.comment.user.login }}
```

#### Option B: Using GitHub App (recommended, comments from app)

Using inputs:

```yaml
name: PR Commands

on:
  issue_comment:
    types: [created]

permissions:
  contents: read
  pull-requests: write
  issues: write

jobs:
  handle-command:
    name: Handle PR Command
    if: |
      github.event.issue.pull_request &&
      github.event.comment.user.type != 'Bot' &&
      (
        startsWith(github.event.comment.body, '/approve') ||
        startsWith(github.event.comment.body, '/merge') ||
        contains(github.event.comment.body, '@smyklot')
      )
    runs-on: ubuntu-24.04
    steps:
      - name: Checkout repository
        uses: actions/checkout@08c6903cd8c0fde910a37f88322edcfb5dd907a8 # v5.0.0

      - name: Generate GitHub App token
        id: generate-token
        uses: actions/create-github-app-token@67018539274d69449ef7c02e8e71183d1719ab42 # v2.1.4
        with:
          app-id: ${{ vars.APP_ID }}
          private-key: ${{ secrets.APP_PRIVATE_KEY }}

      - name: Run Smyklot
        uses: smykla-skalski/smyklot@v0.1.0
        with:
          token: ${{ steps.generate-token.outputs.token }}
          comment-body: ${{ github.event.comment.body }}
          comment-id: ${{ github.event.comment.id }}
          pr-number: ${{ github.event.issue.number }}
          repo-owner: ${{ github.repository_owner }}
          repo-name: ${{ github.event.repository.name }}
          comment-author: ${{ github.event.comment.user.login }}
```

Using environment variables:

```yaml
      - name: Run Smyklot
        uses: smykla-skalski/smyklot@v0.1.0
        env:
          GITHUB_TOKEN: ${{ steps.generate-token.outputs.token }}
          COMMENT_BODY: ${{ github.event.comment.body }}
          COMMENT_ID: ${{ github.event.comment.id }}
          PR_NUMBER: ${{ github.event.issue.number }}
          REPO_OWNER: ${{ github.repository_owner }}
          REPO_NAME: ${{ github.event.repository.name }}
          COMMENT_AUTHOR: ${{ github.event.comment.user.login }}
```

### 3. (Optional) Configure GitHub App Authentication

**Only needed if using Option B workflow above.**

To have comments appear from the GitHub App instead of the default `GITHUB_TOKEN` user:

1. **Create or use existing GitHub App**:
   - Go to Settings → Developer settings → GitHub Apps
   - Note the App ID
   - Generate and download a private key (.pem file)
   - Install the app on your repository

2. **Add App ID as variable and private key as secret**:

   ```bash
   gh variable set APP_ID --body "1197525"
   # Private key must be in PKCS#8 format
   # (starts with "-----BEGIN PRIVATE KEY-----")
   # If your key is in OpenSSH format, convert it first:
   # ssh-keygen -p -N "" -m pem -f openssh-key.pem
   # openssl pkcs8 -topk8 -inform PEM -outform PEM -nocrypt \
   #   -in openssh-key.pem -out pkcs8-key.pem
   gh secret set APP_PRIVATE_KEY < pkcs8-key.pem
   ```

**Note**: The `actions/create-github-app-token` action automatically detects the installation ID, so you don't need to configure it separately.

### 4. Commit and Push

```bash
git add .github/CODEOWNERS .github/workflows/pr-commands.yaml
git commit -sS -m "feat(ci): add Smyklot PR command automation"
git push
```

### 5. Test the Deployment

Create a test pull request and try the commands:

1. **Test approval**:
   - Comment: `/approve` or `@smyklot approve`
   - Expected: ✅ reaction + PR approved

2. **Test merge**:
   - Comment: `/merge` or `@smyklot merge`
   - Expected: ✅ reaction + PR merged (if mergeable)

3. **Test unauthorized user** (optional):
   - Have a non-CODEOWNER comment `/approve`
   - Expected: ❌ reaction + explanation comment

## Deploying as a Service

Everything above deploys the Action, which runs once per comment. The alternative is one long-running process that serves every repository the App is installed on, with no workflow file in any of them.

There are two supported ways to run it. [Fly.io](#deploying-to-flyio) is what this project actually runs on and is the shorter path. The Helm chart below is for anyone who already has a cluster.

Whichever you pick, the service must stay running. Reaction commands (👍) are found by sweeping open pull requests on a timer, because GitHub sends no webhook when someone adds a reaction, and a stopped process runs no timers. That rules out scale-to-zero.

### 1. Configure the App's Webhook

In the App's settings:

- **Webhook URL**: `https://your-host/webhook`
- **Webhook secret**: generate one and keep it
- **Subscribe to events**: Issue comment

The App needs no new permissions. It already has the ones the Action uses.

### 2. Create the Secret

The chart never takes either credential as a value, only the name of a Secret holding both, so neither can end up in a values file, in git, or in `helm get values` output:

```bash
kubectl create secret generic smyklot-credentials \
  --from-literal=webhook-secret="$WEBHOOK_SECRET" \
  --from-file=private-key=key.pem
```

Any secret manager that produces a Kubernetes Secret works the same way - External Secrets, SOPS, Vault. The chart only reads it.

### 3. Install the Chart

```bash
helm install smyklot oci://ghcr.io/smykla-skalski/charts/smyklot \
  --version 1.13.0 \
  --namespace smyklot --create-namespace \
  --set github.clientId=Iv23liExample \
  --set github.existingSecret=smyklot-credentials \
  --set ingress.enabled=true \
  --set ingress.host=smyklot.example.com
```

The chart and the image carry the same version, so `--version` picks both.

Check it came up:

```bash
kubectl -n smyklot port-forward svc/smyklot 9090
curl localhost:9090/readyz
```

`/readyz` answers 200 once the App credentials have been proven against GitHub. Until then the pod takes no traffic.

### 4. Move a Repository Across

The service handles every repository the App is installed on, so there is nothing to do per repository. The workflow files can stay: a workflow whose repository is on the service exits without acting, and says so in the job summary.

A repository that should **stay** on the Action says so in its own `.github/smyklot.yaml`, on the default branch:

```yaml
runner: action
```

Whichever entry point is not named stands down completely - no reaction, no comment, no approval. This is what stops both of them acting on the same comment.

### 5. Roll Back

Rolling back needs no code change and no redeploy.

**One repository**: commit `runner: action` to its `.github/smyklot.yaml`. The Action picks it up on its very next run, because a workflow starts a fresh process every time. The service caches that file for 30 seconds, so it stops within about half a minute of the merge.

**Every repository**: set the organization variable `SMYKLOT_CONFIG` to include `"runner": "action"`, which every Action run reads, then scale the service to zero:

```bash
kubectl -n smyklot scale deployment/smyklot --replicas=0
```

Order matters. Put the repositories back on the Action first, then stop the service, or commands go unanswered in between.

### Operating It

| Route       | Answers                                                     |
|-------------|-------------------------------------------------------------|
| `/livez`    | 200 while the process is running                            |
| `/readyz`   | 200 while GitHub is reachable, 503 with a reason otherwise  |
| `/metrics`  | Prometheus exposition format                                |
| `/failures` | The last 50 deliveries that were accepted and then failed   |

These are on the admin port, which the chart routes no ingress to. Queue depth, failure reasons and Go runtime detail describe the service to anyone who can read them.

Set `serviceMonitor.enabled=true` on a cluster running the Prometheus Operator.

### What the Chart Assumes

**One replica.** Deliveries are de-duplicated in memory and the reaction sweep has no leader election, so a second process sweeps the same repositories and can act on the same reaction twice.

**`Recreate` updates**, for the same reason. A rolling update would run two processes for as long as the old one takes to drain. The cost is a few seconds of refused deliveries, which GitHub records and an operator can redeliver from the App's Advanced tab.

**A 60 second grace period**, which covers the worst case the service allows itself: 15 seconds to stop accepting and 30 to finish deliveries already running.

### Service Troubleshooting

**`/readyz` answers 503**: read the `reason` it returns. Bad credentials and an unreachable GitHub look different there.

**Deliveries never arrive**: check the App's Advanced tab. A 401 there means the webhook secret in the Secret and the one in the App settings have drifted apart.

**A command did nothing**: check `/failures` first, then the log line carrying that delivery's `delivery_id`. If neither shows it, the repository is probably on `runner: action`.

**Two of everything**: a repository is being handled twice. Check its `.github/smyklot.yaml` on the **default branch** - that is the copy both entry points read.

## Deploying to Fly.io

`fly.toml` in the repository root is the live deployment. It runs the released image, so what is deployed and what the release notes describe are the same build.

### 1. Create the App and Set the Secrets

```bash
fly apps create smyklot --org personal

fly secrets set \
  SMYKLOT_WEBHOOK_SECRET="$(openssl rand -hex 32)" \
  GITHUB_APP_PRIVATE_KEY="$(cat key.pem)" \
  GITHUB_APP_CLIENT_ID="Iv23liExample" \
  --app smyklot
```

Secrets live encrypted in Fly and are injected as environment variables. Nothing sensitive belongs in `fly.toml`, which is committed.

The private key goes in exactly as GitHub hands it out. Both PEM encodings are read: PKCS#1, which starts `-----BEGIN RSA PRIVATE KEY-----` and is what the App download gives you, and PKCS#8, which starts `-----BEGIN PRIVATE KEY-----`. There is nothing to convert.

Check which key you have before setting it, because the wrong one fails the same way a revoked one does - `Bad credentials`, with nothing to say the key simply belongs to something else:

```bash
JWT=$(...)   # minted from the key you are about to use
curl -s -o /dev/null -w '%{http_code}\n' -H "Authorization: Bearer $JWT" https://api.github.com/app
```

200 means that key belongs to this App. 401 means it does not.

### 2. Deploy a Released Version

```bash
fly deploy --image ghcr.io/smykla-skalski/smyklot:1.13.0
```

Always name a version. `latest` makes it impossible to tell what is running from the outside.

After a release, `.github/workflows/deploy.yaml` does this automatically. It needs `FLY_API_TOKEN` as a repository secret, from `fly tokens create deploy -a smyklot`. The same workflow can be dispatched by hand with a version, which is how you roll back.

### 3. Point the Domain at It

```bash
fly certs add hook.smyklot.com --app smyklot
```

That prints the DNS records to create. Add them at the registrar, then watch the certificate go from pending to ready:

```bash
fly certs show hook.smyklot.com --app smyklot
```

Set the App's webhook URL to `https://hook.smyklot.com/webhook` once the certificate is issued, not before - GitHub validates the endpoint when you save it.

### 4. Watch It

The admin port is not published. `fly.toml` exposes 8080 and nothing else, so the probes, metrics and recent failures on 9090 are reachable only over Fly's private network:

```bash
fly proxy 9090 --app smyklot
curl localhost:9090/readyz
curl localhost:9090/failures
curl localhost:9090/metrics
```

Logs, which carry a `delivery_id` on every line about a delivery:

```bash
fly logs --app smyklot
```

### What This Costs

One `shared-cpu-1x` machine with 256MB, running continuously, is roughly $2 a month. There is no plan fee. The measured service uses about 17MB idle and 23MB under a burst, so 256MB is the smallest size Fly offers rather than a size this needs.

### Why One Machine

`min_machines_running = 1` with auto-stop disabled, and `strategy = "rolling"` rather than bluegreen or canary. Deliveries are de-duplicated in memory and the reaction sweep has no leader election, so two machines answer the same comment twice. Rolling replaces the one machine in place; the others start a second one first.

The cost is a few seconds of refused deliveries per deploy. GitHub records those and they can be redelivered from the App's Advanced tab.

## Command Reference

### Available Commands

| Command    | Alias              | Action         | Requirements           |
|------------|--------------------|----------------|------------------------|
| `/approve` | `@smyklot approve` | Approve the PR | Listed in CODEOWNERS   |
| `/merge`   | `@smyklot merge`   | Merge the PR   | CODEOWNERS + mergeable |

### Feedback System

**Success** (emoji only):

- ✅ - Command executed successfully

**Errors** (emoji + comment):

- ❌ - Unauthorized or error
- ⚠️ - Warning (e.g., merge conflict)
- 👀 - Processing (added immediately)

## Troubleshooting

### Command Not Working

1. **Check CODEOWNERS file exists**:

   ```bash
   cat .github/CODEOWNERS
   ```

2. **Check workflow file exists**:

   ```bash
   cat .github/workflows/pr-commands.yaml
   ```

3. **Check Actions tab**:
   - Go to repository → Actions tab
   - Look for "PR Commands" workflow
   - Check for errors in workflow runs

4. **Check permissions**:
   - Verify user is listed in CODEOWNERS
   - Verify workflow has correct permissions

### No Reaction on Comment

1. Check if comment is on a pull request (not an issue)
2. Check Actions tab for workflow execution
3. Check workflow logs for errors

### Approval Not Working

1. Verify GITHUB_TOKEN has write permissions
2. Check if user is in CODEOWNERS
3. Check workflow logs for API errors

### Merge Not Working

1. Verify PR is mergeable (no conflicts)
2. Verify required checks have passed
3. Check branch protection rules
4. Check workflow logs for API errors

## Security Considerations

### Permissions

The workflow requires minimal permissions:

- `contents: read` - Read repository files
- `pull-requests: write` - Approve and merge PRs
- `issues: write` - Add reactions and comments

### GITHUB_TOKEN

The workflow uses the built-in `GITHUB_TOKEN` which:

- Is automatically created for each workflow run
- Has repository-scoped permissions
- Expires after the workflow completes
- Cannot be used outside the repository

### Input Validation

All user inputs are passed via environment variables (not shell interpolation) to prevent injection attacks:

```yaml
env:
  COMMENT_BODY: ${{ github.event.comment.body }}
  # Not: run: ./bot "${{ github.event.comment.body }}"
```

## Updating Smyklot

To update to a new version:

1. Change the version reference in workflow:

   ```yaml
   uses: smykla-skalski/smyklot@v1.0.0  # Update this line
   ```

2. Commit and push:

   ```bash
   git add .github/workflows/pr-commands.yaml
   git commit -sS -m "chore(ci): update Smyklot to v1.0.0"
   git push
   ```

## Support

- **Issues**: <https://github.com/smykla-skalski/smyklot/issues>
- **Discussions**: <https://github.com/smykla-skalski/smyklot/discussions>
- **Documentation**: <https://github.com/smykla-skalski/smyklot/blob/main/README.md>
