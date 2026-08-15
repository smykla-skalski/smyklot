#!/usr/bin/env bash
#
# Pull a backup of the service's PostgreSQL database to this machine.
#
# Fly's volume snapshots are the second copy. This is the first: a file on a
# machine that is not Fly, which is what survives losing the volume, the app or
# the organization.
#
#   mise run db:backup
#   scripts/backup-db.sh --output-dir ~/backups/smyklot --retain 30
#
# pg_dump runs inside the same image the server runs, reached through a local
# `fly proxy`. That is deliberate: pg_dump refuses a server newer than itself,
# so a Homebrew client one major behind would fail every night, and this way
# the versions cannot drift apart.
#
# The dump is verified before anything old is deleted, so a night where the
# dump came back truncated leaves yesterday's copy where it is rather than
# pruning it away in favour of a broken one.

set -euo pipefail

app="smyklot-db"
database="smyklot"
user="smyklot"
image="postgres:18-alpine"
output_dir="${SMYKLOT_BACKUP_DIR:-$HOME/backups/smyklot}"
retain=14
port=15432
host=""

usage() {
    cat <<'USAGE'
Usage: backup-db.sh [options]

  --app NAME           Fly app running PostgreSQL (default: smyklot-db)
  --database NAME      Database to dump (default: smyklot)
  --user NAME          Role to connect as (default: smyklot)
  --output-dir PATH    Where to write dumps (default: $SMYKLOT_BACKUP_DIR,
                       or ~/backups/smyklot)
  --retain N           Dumps to keep (default: 14, 0 keeps everything)
  --port N             Port to connect on (default: 15432)
  --host HOST          Connect here instead of opening a Fly proxy, for a
                       database already reachable from this machine
  --image REF          Image supplying pg_dump (default: postgres:18-alpine)
  -h, --help           This text

The database password is read from POSTGRES_PASSWORD. It cannot be read back
out of Fly, which never returns a secret once it is set, so keep a copy where
you keep your other credentials and export it before running this:

  export POSTGRES_PASSWORD='...'
USAGE
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --app) app="$2"; shift 2 ;;
        --database) database="$2"; shift 2 ;;
        --user) user="$2"; shift 2 ;;
        --output-dir) output_dir="$2"; shift 2 ;;
        --retain) retain="$2"; shift 2 ;;
        --port) port="$2"; shift 2 ;;
        --host) host="$2"; shift 2 ;;
        --image) image="$2"; shift 2 ;;
        -h|--help) usage; exit 0 ;;
        *) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
    esac
done

fail() {
    echo "backup-db: $*" >&2
    exit 1
}

required_tools=(docker)
# fly is only needed when this has to open the tunnel itself.
[[ -n "$host" ]] || required_tools+=(fly)
for tool in "${required_tools[@]}"; do
    command -v "$tool" >/dev/null 2>&1 || fail "$tool is not installed"
done

# Checked here rather than left to pg_dump, whose failure on a missing password
# is a connection error that reads like the database is down.
[[ -n "${POSTGRES_PASSWORD:-}" ]] || fail "POSTGRES_PASSWORD is not set (see --help)"

mkdir -p "$output_dir"

# Named for the instant it covers, sorted the same way whether by name or by
# date, and unambiguous across time zones.
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
dump="$output_dir/$database-$stamp.dump"
partial="$dump.partial"

cleanup() {
    local status=$?
    [[ -n "${proxy_pid:-}" ]] && kill "$proxy_pid" 2>/dev/null || true
    rm -f "$partial"
    exit "$status"
}
trap cleanup EXIT

if [[ -n "$host" ]]; then
    target="$host"
else
    target="host.docker.internal"

    echo "backup-db: opening a proxy to $app"
    fly proxy "$port:5432" --app "$app" >/dev/null 2>&1 &
    proxy_pid=$!

    # The proxy takes a moment to bind, and connecting before it does looks
    # exactly like a database that is refusing connections.
    for _ in $(seq 1 30); do
        if nc -z 127.0.0.1 "$port" 2>/dev/null; then
            break
        fi
        kill -0 "$proxy_pid" 2>/dev/null || fail "the proxy to $app exited"
        sleep 1
    done
    nc -z 127.0.0.1 "$port" 2>/dev/null \
        || fail "the proxy to $app never accepted connections"
fi

# host.docker.internal is how a container reaches the proxy running out here.
# Docker Desktop, OrbStack and Colima all provide it.
run_pg() {
    docker run --rm --interactive \
        --env PGPASSWORD="$POSTGRES_PASSWORD" \
        --add-host host.docker.internal:host-gateway \
        "$image" "$@"
}

echo "backup-db: dumping $database"
# The custom format is compressed and lets pg_restore pick out one table, which
# matters when the reason for restoring is one bad migration rather than a lost
# volume.
run_pg pg_dump \
    --host "$target" \
    --port "$port" \
    --username "$user" \
    --dbname "$database" \
    --format custom \
    --compress 9 \
    --no-owner \
    --no-privileges \
    >"$partial"

[[ -s "$partial" ]] || fail "the dump is empty"

# A dump that cannot be listed cannot be restored, and finding that out on the
# night it is needed is the whole failure this guards against.
echo "backup-db: verifying"
tables="$(run_pg pg_restore --list <"$partial" | grep -c '^[0-9]' || true)"
[[ "$tables" -gt 0 ]] || fail "the dump lists no objects, so it would restore nothing"

mv "$partial" "$dump"
echo "backup-db: wrote $dump ($(du -h "$dump" | cut -f1), $tables objects)"

# Pruned only after a good dump landed, so a run that failed never costs an
# older copy.
if [[ "$retain" -gt 0 ]]; then
    # Read with a loop rather than mapfile, which needs bash 4 and so is
    # missing on the macOS this is most likely to run on.
    while IFS= read -r old; do
        [[ -n "$old" ]] || continue
        echo "backup-db: pruning $(basename "$old")"
        rm -f "$old"
    done < <(
        find "$output_dir" -maxdepth 1 -name "$database-*.dump" -type f \
            | sort -r \
            | tail -n "+$((retain + 1))"
    )
fi
