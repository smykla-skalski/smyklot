#!/usr/bin/env bash
#
# Runs the deploy workflow's preflight checks against canned flyctl answers.
#
# These checks stand between a release and the running service, and nothing
# else exercises them: they only run during a deploy, against a healthy
# production, where every failure case is by definition absent. Release 1.26.0
# is what that costs - a step read the database app with a token scoped to the
# service app, and the first thing that noticed was the deploy.
#
# So the step scripts are lifted out of the workflow and run here against a
# flyctl that answers from a fixture. The scripts are read from the workflow
# rather than copied into this file, because a copy is a second thing to edit
# and the one that stops being edited is this one.
set -euo pipefail

cd "$(dirname "$0")/.."
workflow=".github/workflows/deploy.yaml"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
PATH="$work:$PATH"
pass=0
fail=0

# Stands in for flyctl. The fixture file is the command joined by dashes, and
# a ".exit" file beside it makes the call fail the way flyctl would - which is
# how the unauthorized case gets tested without an unauthorized token.
cat >"$work/flyctl" <<'FAKE'
#!/usr/bin/env bash
set -uo pipefail
body="$FIXTURE/$(printf '%s' "$*" | tr -c 'a-zA-Z0-9' '-' | tr -s '-')"
if [ -f "$body.exit" ]; then
  [ -f "$body" ] && cat "$body" >&2
  exit "$(cat "$body.exit")"
fi
if [ ! -f "$body" ]; then
  echo "no fixture for 'flyctl $*'" >&2
  exit 127
fi
cat "$body"
FAKE
chmod +x "$work/flyctl"

# A step that has been renamed extracts to nothing, and an empty script passes
# every assertion below without running a line of the thing it claims to test.
# Each one therefore has to still contain the check it is here for.
lift() {
  local step="$1" marker="$2" into="$work/$3"
  yq -r ".jobs.deploy.steps[] | select(.name == \"$step\") | .run" "$workflow" >"$into"
  if ! grep -qF "$marker" "$into"; then
    echo "no step named '$step' in $workflow still contains: $marker" >&2
    exit 1
  fi
}

lift 'Identify the storage backend' 'SMYKLOT_DATABASE_URL' backend.sh
lift 'Verify the single writer topology' 'smyklot_data' writer.sh
lift 'Verify the database is up' 'smyklot-db' database.sh

# Asserting on the message as well as the status, because every one of these
# scripts runs under `set -e` and exits non-zero for any reason at all. A test
# that only reads the status passes just as happily when the check it names has
# been deleted and something unrelated is what failed.
check() {
  local name="$1" script="$2" want="$3" says="$4" got out
  set +e
  out="$(bash "$work/$script" 2>&1)"
  got=$?
  set -e
  if [ "$got" -ne "$want" ]; then
    printf 'FAIL  %-44s exit %s, wanted %s\n%s\n' "$name" "$got" "$want" "$out" >&2
    fail=$((fail + 1))
    return
  fi
  if [ -n "$says" ] && [ "${out#*"$says"}" = "$out" ]; then
    printf 'FAIL  %-44s never said: %s\n%s\n' "$name" "$says" "$out" >&2
    fail=$((fail + 1))
    return
  fi
  printf 'ok    %-44s exit %s\n' "$name" "$got"
  pass=$((pass + 1))
}

# Which token a step is handed is the one thing about it that cannot be tested
# by running it, and it is exactly what release 1.26.0 got wrong. So it is read
# off the workflow instead.
wired() {
  local step="$1" want="$2" got
  # shellcheck disable=SC2016 # a set of characters to delete, not an expansion
  got="$(yq -r ".jobs.deploy.steps[] | select(.name == \"$step\") | .env.FLY_API_TOKEN" "$workflow" |
    tr -d ' ${}')"
  if [ "$got" = "$want" ]; then
    printf 'ok    %-44s %s\n' "$step" "$want"
    pass=$((pass + 1))
  else
    printf 'FAIL  %-44s reads %s, wanted %s\n' "$step" "$got" "$want" >&2
    fail=$((fail + 1))
  fi
}

wrote() {
  local name="$1" needle="$2"
  if grep -q "$needle" "$GITHUB_OUTPUT"; then
    printf 'ok    %-44s\n' "$name"
    pass=$((pass + 1))
  else
    printf 'FAIL  %-44s wrote: %s\n' "$name" "$(cat "$GITHUB_OUTPUT")" >&2
    fail=$((fail + 1))
  fi
}

fixture() {
  FIXTURE="$work/fixture"
  export FIXTURE
  rm -rf "$FIXTURE"
  mkdir -p "$FIXTURE"
  GITHUB_OUTPUT="$work/step-output"
  export GITHUB_OUTPUT
  : >"$GITHUB_OUTPUT"
}

answers() { printf '%s' "$2" >"$FIXTURE/$1"; }
db() { answers machines-list-app-smyklot-db-json "$(jq "$1" <<<"$db_machines")"; }

# The shape flyctl really reports, trimmed to the fields the checks read.
db_machines='[{"id":"784ed76dfd6e38","state":"started","checks":[{"name":"ready","status":"passing"}]}]'
one_app='[{"id":"aaa","config":{"env":{"FLY_PROCESS_GROUP":"app"}}}]'
two_apps='[{"id":"aaa","config":{"env":{"FLY_PROCESS_GROUP":"app"}}},
           {"id":"bbb","config":{"env":{"FLY_PROCESS_GROUP":"app"}}}]'

echo '--- Which token each step is handed'

wired 'Identify the storage backend' 'secrets.FLY_API_TOKEN'
wired 'Verify the single writer topology' 'secrets.FLY_API_TOKEN'
wired 'Verify the database is up' 'secrets.FLY_DB_READ_TOKEN'

echo '--- Identify the storage backend'

fixture
answers secrets-list-app-smyklot-json \
  '[{"name":"SMYKLOT_WEBHOOK_SECRET"},{"name":"SMYKLOT_DATABASE_URL"}]'
check 'the database URL means PostgreSQL' backend.sh 0 'PostgreSQL'
wrote 'and says so to the later steps' 'backend=postgres'

fixture
answers secrets-list-app-smyklot-json '[{"name":"SMYKLOT_WEBHOOK_SECRET"}]'
check 'no database URL means SQLite' backend.sh 0 'SQLite'
wrote 'and says that to the later steps' 'backend=sqlite'

# The dangerous direction the step's own comment names: reading nothing has to
# stop the deploy rather than quietly select SQLite.
fixture
answers secrets-list-app-smyklot-json '[]'
check 'unreadable secrets stop the deploy' backend.sh 1 'cannot read'

echo '--- Verify the single writer topology'

BACKEND=postgres
export BACKEND

# No volumes fixture is written, so the fake flyctl exits 127 if the check ever
# asks about one. Passing is what proves PostgreSQL skips the volume half.
fixture
answers machines-list-app-smyklot-json "$one_app"
check 'postgres: one machine, no volume asked' writer.sh 0 ''

fixture
answers machines-list-app-smyklot-json "$two_apps"
check 'postgres: refuses a second writer' writer.sh 1 'found 2'

BACKEND=sqlite
export BACKEND

fixture
answers machines-list-app-smyklot-json "$one_app"
answers volumes-list-app-smyklot-json '[{"name":"smyklot_data","attached_machine_id":"aaa"}]'
check 'sqlite: volume on the sole machine' writer.sh 0 ''

fixture
answers machines-list-app-smyklot-json "$one_app"
answers volumes-list-app-smyklot-json '[{"name":"smyklot_data","attached_machine_id":"zzz"}]'
check 'sqlite: refuses a volume elsewhere' writer.sh 1 'unexpected machine zzz'

fixture
answers machines-list-app-smyklot-json "$one_app"
answers volumes-list-app-smyklot-json '[{"name":"smyklot_data"},{"name":"smyklot_data"}]'
check 'sqlite: refuses two volumes' writer.sh 1 'found 2'

fixture
answers machines-list-app-smyklot-json "$one_app"
answers volumes-list-app-smyklot-json '[]'
check 'sqlite: refuses a missing volume' writer.sh 1 'found 0'

fixture
answers machines-list-app-smyklot-json "$two_apps"
answers volumes-list-app-smyklot-json '[{"name":"smyklot_data","attached_machine_id":"aaa"}]'
check 'sqlite: refuses a second writer' writer.sh 1 'found 2'

unset BACKEND

echo '--- Verify the database is up'

fixture
db '.'
check 'a started machine passing its check' database.sh 0 ''

# What 1.26.0 hit. The message has to name the token, because flyctl's own
# "unauthorized" reads like the database being gone.
fixture
answers machines-list-app-smyklot-db-json 'Error: failed to list VMs: unauthorized'
answers machines-list-app-smyklot-db-json.exit '1'
check 'a token that cannot see the app' database.sh 1 'FLY_DB_READ_TOKEN must be scoped to smyklot-db'

fixture
db 'map(.state = "stopped")'
answers status-app-smyklot-db 'canned diagnostic'
check 'nothing started' database.sh 1 'found 0'

fixture
db '. + .'
answers status-app-smyklot-db 'canned diagnostic'
check 'two machines started' database.sh 1 'found 2'

# The diagnostic is best-effort, so losing it must not turn a refusal into a
# pass - the fixture for it is deliberately absent here.
fixture
db 'map(.state = "stopped")'
check 'still refuses without a diagnostic' database.sh 1 'found 0'

# A machine reporting no checks passes "nothing is failing" while saying
# nothing about whether Postgres is answering, so they have to be counted.
fixture
db 'map(del(.checks))'
answers checks-list-app-smyklot-db 'canned diagnostic'
check 'no checks reported at all' database.sh 1 'found 0 of 0'

fixture
db 'map(.checks[0].status = "critical")'
answers checks-list-app-smyklot-db 'canned diagnostic'
check 'a check that is not passing' database.sh 1 'found 0 of 1'

fixture
db 'map(.checks += [{"name":"extra","status":"passing"}])'
check 'every check passing' database.sh 0 ''

printf '\n%s passed, %s failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
