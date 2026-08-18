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

**Note**: Only global owners (`* @username`) are read. Path-specific patterns are not implemented, so a line naming a path is ignored.

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
- **Subscribe to events**: Issue comment, Check run, Check suite, Status, Pull request

The App needs **Checks** write, **Commit statuses** read, **Administration** write, and **Merge queues** read in addition to its command permissions. Checks write lets Smyklot publish the merge authorization; Administration write lets it own the required-status-check ruleset; Merge queues read lets it fail closed on repositories whose queue commits Smyklot does not support. Existing installations must approve the new permissions after the App registration changes.

Organization sync asks for more. Label sync needs only the **Issues** write access the bot already holds, so it works the day it is switched on. Settings and ruleset sync need **Administration** write. File sync needs **Contents** write, and **Workflows** write on top of it wherever a synchronized path sits under `.github/workflows/` - GitHub keeps those behind a permission of their own and refuses the push without it. Until an installation approves one, that kind stands down and names the permission in the panel; the rest of the sync runs. Nothing is written to a repository on a permissions listing the service could not read.

### 2. Create the Secret

The chart never takes either credential as a value, only the name of a Secret holding both, so neither can end up in a values file, in git, or in `helm get values` output:

```bash
kubectl create secret generic smyklot-credentials \
  --from-literal=webhook-secret="$WEBHOOK_SECRET" \
  --from-literal=database-url="postgres://smyklot:PASSWORD@postgres:5432/smyklot" \
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
  --set database.existingSecret=smyklot-credentials \
  --set ingress.enabled=true \
  --set ingress.host=smyklot.example.com
```

The chart and the image carry the same version, so `--version` picks both.

The chart needs a database and refuses to render without one. `database.existingSecret` is the form to use: `database.url` puts a connection string, and usually a password, into the values file and into `helm get values`. Point either at a PostgreSQL the cluster already runs - the chart deploys no database of its own, and the operators that do this well are better at it than a subchart here would be.

There is no SQLite option in the chart. The pod runs with a read-only root filesystem and mounts no volume, and giving one replica of one process a PersistentVolumeClaim to hold a file is the arrangement PostgreSQL exists to replace.

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

**A PostgreSQL it does not manage.** The chart takes a connection string and nothing else. Backups, failover and upgrades belong to whoever runs that database.

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
  SMYKLOT_PANEL_CLIENT_ID="Ov23liExample" \
  SMYKLOT_PANEL_CLIENT_SECRET="your-oauth-app-client-secret" \
  --app smyklot
```

Secrets live encrypted in Fly and are injected as environment variables. Nothing sensitive belongs in `fly.toml`, which is committed.

The two `SMYKLOT_PANEL_*` values come from a classic OAuth App, not from the GitHub App above. Registering a second application is what keeps the sign-in consent screen down to public profile read instead of listing every permission the bot approves and merges with - see [Sign-in registration](README.md#sign-in-registration). Its authorization callback URL is `https://smyklot.com/auth/github/callback`.

The private key goes in exactly as GitHub hands it out. Both PEM encodings are read: PKCS#1, which starts `-----BEGIN RSA PRIVATE KEY-----` and is what the App download gives you, and PKCS#8, which starts `-----BEGIN PRIVATE KEY-----`. There is nothing to convert.

Check which key you have before setting it, because the wrong one fails the same way a revoked one does - `Bad credentials`, with nothing to say the key simply belongs to something else:

```bash
JWT=$(...)   # minted from the key you are about to use
curl -s -o /dev/null -w '%{http_code}\n' -H "Authorization: Bearer $JWT" https://api.github.com/app
```

200 means that key belongs to this App. 401 means it does not.

The committed Fly configuration serves the panel at `https://smyklot.com/`,
binds ownership to the immutable GitHub identity first verified as
`bartsmykla`, and stores SQLite at `/data/panel.sqlite3`. Its `smyklot_data`
mount is one GB with 14 days of snapshots. Confirm the volume exists before
deploying the panel build:

```bash
fly volumes create smyklot_data --app smyklot --region fra --size 1 \
  --scheduled-snapshots --snapshot-retention 14 --yes
fly volumes list --app smyklot
```

Run `volumes create` exactly once, only when `volumes list` shows no
`smyklot_data` volume. Fly encrypts volume contents by default.

In the **OAuth App** settings - not the GitHub App's - add the exact callback URL
`https://smyklot.com/auth/github/callback`. It is separate from the webhook URL
and must match the redirect URI byte for byte.

### 2. Deploy a Released Version

SQLite has one canonical volume and cannot tolerate competing writers. Before a
manual deployment, require at most one app machine and exactly one
`smyklot_data` volume. Zero machines is valid for the first deployment because
`--ha=false` creates one. If there are multiple machines or volumes, stop and
reconcile them by identity; never auto-scale an unknown replica away.

```bash
machines="$(fly machines list --app smyklot --json)"
test "$(jq '[.[] | select(.config.env.FLY_PROCESS_GROUP == "app")] | length' \
  <<<"$machines")" -le 1
test "$(fly volumes list --app smyklot --json | \
  jq '[.[] | select(.name == "smyklot_data")] | length')" -eq 1

fly deploy --app smyklot --ha=false \
  --image ghcr.io/smykla-skalski/smyklot:1.13.0
```

Always name a version. `latest` makes it impossible to tell what is running from the outside.

After deployment, verify that the sole `smyklot_data` volume is attached to the
sole app machine before accepting traffic or changing settings:

```bash
fly machines list --app smyklot
fly volumes list --app smyklot
```

After a release, `.github/workflows/deploy.yaml` does this automatically. It needs `FLY_API_TOKEN` as a repository secret, from `fly tokens create deploy -a smyklot`. A PostgreSQL-backed deployment needs a second secret as well, `FLY_DB_READ_TOKEN`, minted under "Running on PostgreSQL" below. The same workflow can be dispatched by hand with a version, which is how you roll back.

### 3. Point the Domain at It

```bash
fly certs add smyklot.com --app smyklot
```

That prints the DNS records to create. A subdomain can be a single `CNAME` to the app, but an apex cannot, so `smyklot.com` needs both:

```text
A     smyklot.com  ->  <shared IPv4 from fly ips list>
AAAA  smyklot.com  ->  <dedicated IPv6 from fly ips list>
```

Fly will not verify the certificate from the `A` record alone. Add the two records `fly certs setup` also prints, which prove the domain is yours without waiting on traffic:

```text
CNAME  _acme-challenge.smyklot.com  ->  smyklot.com.<app>.flydns.net.
TXT    _fly-ownership.smyklot.com   ->  app-<app>
```

Watch it go from pending to ready with `fly certs check`, not `fly certs list` - the list column says `Issued` as soon as the certificate exists, minutes before the edge will serve it. Until then TLS fails outright rather than serving a wrong certificate:

```bash
fly certs check smyklot.com --app smyklot
curl -sS -o /dev/null -w '%{http_code}\n' https://smyklot.com/healthz
curl -sS -o /dev/null -w '%{http_code}\n' https://smyklot.com/
curl -sS -o /dev/null -w '%{http_code}\n' https://smyklot.com/auth/github/start
```

The expected results are 200 for health and the panel, then 302 to GitHub for
the sign-in start. Complete one sign-in as `bartsmykla` and verify that a reload
preserves the selected installation and settings before enabling repositories.

Set the App's webhook URL to `https://smyklot.com/webhook` once that returns 200, not before - GitHub validates the endpoint when you save it. Confirm the endpoint end to end by redelivering a past delivery rather than waiting for a real comment:

```bash
curl -X POST -H "Authorization: Bearer $JWT" \
  https://api.github.com/app/hook/deliveries/<id>/attempts
```

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

## Running on PostgreSQL

SQLite is the default and needs nothing but a volume. PostgreSQL is the other supported engine, and what `smyklot.com` is built to move to. Choose it with one variable:

```bash
SMYKLOT_DATABASE_URL='postgres://smyklot:PASSWORD@smyklot-db.internal:5432/smyklot'
```

A `postgres://` or `postgresql://` URL picks PostgreSQL. A bare path or a `sqlite://` URL picks SQLite. `SMYKLOT_STATE_PATH` and `SMYKLOT_PANEL_STATE_PATH` still work and still mean a SQLite file; setting more than one of the three is an error rather than a guess.

Nothing else in the service changes. The migrations, the schema and the conformance suite are the same on both, so a repository, a command or a panel screen behaves identically whichever is underneath.

### Why Not Fly Managed Postgres

`deploy/postgres/fly.toml` is a second Fly app running the stock `postgres:18-alpine` image on one volume. It costs about $3.65 a month and belongs to us. Managed Postgres starts higher and adds a control plane this does not need for a database that holds a few thousand rows.

The trade is real and worth stating: one machine, one volume, no replica, no automatic failover. Losing the volume loses the database back to the last snapshot or the last dump. That is acceptable here and would not be for something with users.

### 1. Create the Database App

```bash
fly apps create smyklot-db --org personal
fly secrets set POSTGRES_PASSWORD="$(openssl rand -hex 32)" --app smyklot-db
fly deploy --config deploy/postgres/fly.toml
```

Keep that password. Fly never hands a secret back, and both the service's connection string and the backup script need it.

Do **not** allocate an IP address for this app. With no `[http_service]` and no IP, it is reachable only at `smyklot-db.internal:5432` over the organization's private network, which is the whole of its access control.

Check it came up:

```bash
fly status --app smyklot-db
fly logs --app smyklot-db          # look for "database system is ready to accept connections"
```

### 2. Copy the Data Across

Take the SQLite file off the running service first, and stop the service so nothing writes after the copy has read a table:

```bash
fly ssh sftp get /data/panel.sqlite3 ./panel.sqlite3 --app smyklot
fly scale count 0 --app smyklot
fly ssh sftp get /data/panel.sqlite3 ./panel.sqlite3 --app smyklot   # again, now quiesced
```

Then copy it in, through a proxy to the database:

```bash
fly proxy 15432:5432 --app smyklot-db &

./bin/smyklot-github-action store migrate \
  --from ./panel.sqlite3 \
  --to "postgres://smyklot:$POSTGRES_PASSWORD@localhost:15432/smyklot?sslmode=disable"
```

It prints a row count per table. The destination is migrated to the current schema on the way in and must be empty; a second run refuses rather than merging two histories, and `--force` empties it first if a first attempt needs redoing.

Both databases are migrated, the source included - the copy reads its columns, and a source a few releases behind would be missing some. That is why the steps above pull `panel.sqlite3` down first: the command writes to whatever it is pointed at, and the untouched original on the volume is the thing a rollback needs.

Nothing is copied outside one transaction, so a failure leaves the database empty rather than half-populated. `schema_migrations` is deliberately not carried: the destination wrote its own while migrating.

### 3. Point the Service at It

```bash
fly secrets set \
  SMYKLOT_DATABASE_URL="postgres://smyklot:$POSTGRES_PASSWORD@smyklot-db.internal:5432/smyklot" \
  --app smyklot

cp deploy/postgres/smyklot.fly.toml fly.toml
git commit -sS -am 'feat(deploy): move service state to PostgreSQL'

VERSION=1.23.2          # or whichever release you are deploying
fly deploy --app smyklot --ha=false \
  --image "ghcr.io/smykla-skalski/smyklot:$VERSION"
fly scale count 1 --app smyklot
```

The configuration swap is what removes `[[mounts]]`, and it is committed rather than done by hand so that the next automated deploy agrees with the last manual one. `deploy.yaml` reads which backend the app is on from whether `SMYKLOT_DATABASE_URL` is set, and checks the topology that matches, so a release after this point verifies the database is up instead of verifying a volume that is no longer used.

That check reads a second app, so it needs a second token. `FLY_API_TOKEN` is scoped to `smyklot` and Fly answers `unauthorized` for anything else, which is how release 1.26.0 failed. Add `FLY_DB_READ_TOKEN` as a repository secret, read-only, so the pipeline that watches the database has no way to change it:

```bash
parent="$(fly tokens create deploy --app smyklot-db --expiry 8760h \
  --name 'GitHub Actions db read (parent of the CI read-only child)' --json | jq -r .token)"

child="$(FLY_API_TOKEN="$parent" fly tokens create readonly --from-existing \
  --expiry 8760h --name 'GitHub Actions db read' --json | jq -r .token)"

printf %s "$child" | gh secret set FLY_DB_READ_TOKEN --repo smykla-skalski/smyklot
```

A read-only token is an attenuation of an existing one rather than an entry of its own, so `fly tokens list --app smyklot-db` lists only the parent, and revoking the parent revokes the child with it. Keep that entry and throw its string away: it is the half of the pair that can write. Confirm what shipped before trusting it — the child has to read the database app, refuse the service app, and refuse to write anything:

```bash
FLY_API_TOKEN="$child" fly machines list --app smyklot-db --json   # one started machine
FLY_API_TOKEN="$child" fly machines list --app smyklot --json      # unauthorized
FLY_API_TOKEN="$child" fly tokens create deploy --app smyklot-db \
  --expiry 1h --name probe                                        # not authorized

unset parent child
```

Both tokens expire after a year. `fly tokens list --scope org --org personal` is where you find that out before a release does.

Verify before deleting anything:

```bash
fly proxy 9090 --app smyklot &
curl localhost:9090/readyz
curl -sS -o /dev/null -w '%{http_code}\n' https://smyklot.com/
```

Sign in to the panel and confirm the installation, its settings and the audit trail are the ones you had. Only then:

```bash
fly volumes list --app smyklot
fly volumes destroy <id> --app smyklot
```

Keep `panel.sqlite3` somewhere off both machines until you are sure.

### 4. Roll Back

Rolling back is the same four steps in reverse, and the SQLite file is what makes it possible:

```bash
fly scale count 0 --app smyklot
fly secrets unset SMYKLOT_DATABASE_URL --app smyklot
git revert <the cutover commit>          # restores [[mounts]] and the state path
fly volumes create smyklot_data --app smyklot --region fra --size 1 \
  --scheduled-snapshots --snapshot-retention 14 --yes
fly deploy --app smyklot --ha=false --image "ghcr.io/smykla-skalski/smyklot:$VERSION"
fly ssh sftp shell --app smyklot         # put panel.sqlite3 back at /data/panel.sqlite3
fly scale count 1 --app smyklot
```

Anything written while PostgreSQL was live is not in that file. `store migrate` runs the other way too, so a rollback that has to keep those writes copies them back first:

```bash
smyklot store migrate --from "postgres://..." --to ./panel.sqlite3
```

### Backups

Fly snapshots the volume daily and keeps 14. That covers losing the disk. It does not cover losing the organization, the account, or a `fly volumes destroy` typed at the wrong app, so the first copy is a dump on a machine that is not Fly:

```bash
export POSTGRES_PASSWORD='...'
mise run db:backup
```

That opens a proxy, runs `pg_dump` from the server's own image, verifies the result with `pg_restore --list`, writes it to `~/backups/smyklot`, and only then prunes anything older than the retention. A night where the dump comes back truncated leaves yesterday's copy alone.

`pg_dump` runs in a container rather than from Homebrew on purpose: it refuses a server newer than itself, so a client one major version behind would fail every night for a reason that reads like the database is down.

Run it daily on a Mac with the launchd agent:

```bash
mkdir -p ~/.config/smyklot
install -m 600 /dev/null ~/.config/smyklot/postgres-password
printf '%s' "$POSTGRES_PASSWORD" > ~/.config/smyklot/postgres-password

sed -e "s|__REPO__|$PWD|g" \
    -e "s|__HOME__|$HOME|g" \
    -e "s|__PASSWORD_FILE__|$HOME/.config/smyklot/postgres-password|g" \
    deploy/launchd/com.smykla.smyklot-backup.plist \
    > ~/Library/LaunchAgents/com.smykla.smyklot-backup.plist
launchctl load ~/Library/LaunchAgents/com.smykla.smyklot-backup.plist
```

It runs at 04:30 and logs to `~/backups/smyklot/backup.log`. Read that occasionally: a backup that has been failing quietly for a month is the same as no backup.

Restoring is `pg_restore` into an empty database:

```bash
fly proxy 15432:5432 --app smyklot-db &
docker run --rm -i --env PGPASSWORD="$POSTGRES_PASSWORD" \
  --add-host host.docker.internal:host-gateway postgres:18-alpine \
  pg_restore --host host.docker.internal --port 15432 \
    --username smyklot --dbname smyklot --no-owner \
  < ~/backups/smyklot/smyklot-20260314T043000Z.dump
```

Practise it once against a scratch database. A backup nobody has restored is a file, not a backup.

### Operating the Database

```bash
fly proxy 15432:5432 --app smyklot-db          # then connect on localhost:15432
fly logs --app smyklot-db
fly status --app smyklot-db
fly ssh console --app smyklot-db --command "psql -U smyklot -d smyklot -c '\\dt'"
```

The service opens at most 16 connections against a limit of 50, so there is room for a session at the prompt while it runs.

**The service will not start.** Its migrations run on startup, so an unreachable or unmigratable database means the machine never becomes ready. `fly logs --app smyklot` names which; check `fly status --app smyklot-db` first.

**Sizing.** 512MB, `shared_buffers=96MB`, `max_connections=50`. 256MB is the cheaper size and is not enough - shared buffers plus the backends plus the postmaster sit close enough to it that a checkpoint under load ends in the OOM killer.

**The major version is pinned** to `postgres:18-alpine`. PostgreSQL does not read a data directory written by a newer major, so an unpinned tag would eventually start a machine that refuses to come up and needs `pg_upgrade` to recover.

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
