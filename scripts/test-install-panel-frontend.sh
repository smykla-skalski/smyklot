#!/usr/bin/env bash
#
# Proves a restored node_modules cache still rebuilds SvelteKit's generated
# config. The cache contains the install stamp but deliberately excludes
# .svelte-kit, matching what actions/cache restores in the frontend jobs.
set -euo pipefail

cd "$(dirname "$0")/.."
work="$(mktemp -d)"

cleanup() {
  local temp_root="${TMPDIR:-/tmp}"
  temp_root="${temp_root%/}"
  if [[ -z "${work:-}" || ! -d "$work" ]]; then
    return
  fi
  case "$work" in
  "$temp_root"/* | /tmp/* | /private/tmp/*)
    command rm -rf -- "$work"
    ;;
  *)
    printf 'refusing to remove unexpected temporary directory: %s\n' "$work" >&2
    return 1
    ;;
  esac
}
trap cleanup EXIT

frontend="$work/frontend"
fake_npm="$work/npm"
npm_log="$work/npm.log"
# SvelteKit 3 writes the tsconfig every other one extends to `node_modules/$app`;
# 2 wrote it to `.svelte-kit`. The directory really is called `$app`, so this is
# single quoted and every shell below that names it quotes it too - unquoted it
# expands to nothing and the path becomes `node_modules//tsconfig.json`, which is
# a file nothing writes and every check then reports as missing.
# shellcheck disable=SC2016 # `$app` is the directory's name, not an expansion.
generated_relative='node_modules/$app/tsconfig.json'
generated_output="internal/panel/frontend/$generated_relative"

if ! mise tasks info panel:frontend:install --json |
  yq -e ".outputs | contains([\"$generated_output\"])" >/dev/null; then
  echo "panel:frontend:install does not declare $generated_relative as an output" >&2
  exit 1
fi

toolchain='.github/actions/toolchain/action.yaml'
cache_paths="$(
  yq -r '.runs.steps[] | select(.name == "Restore the panel bundle") | .with.path' "$toolchain"
)"
if ! command grep -Fqx -- "$generated_output" <<<"$cache_paths"; then
  echo "panel bundle cache does not preserve $generated_relative" >&2
  exit 1
fi
mark_script="$(
  yq -r '.runs.steps[] | select(.name == "Mark the panel bundle as current") | .run' "$toolchain"
)"

# The mark step is run against a fake restored workspace rather than read for the
# path it names, because what has to hold is behavioural: the output it exists to
# out-date is newer afterwards. That covers every way of getting it wrong - a
# dropped touch, and the quoting `$app` needs, where the step's own `set -u` turns
# a bare one into an unbound variable and takes down every job that restores this
# cache. Matching the spelling by text would also refuse a correct one:
# `"...\$app/..."` is as right as `'...$app/...'`.
#
# All four outputs, not just the generated one: any of them left behind the
# checkout puts the whole chain back, which is the half-minute per job this cache
# exists to skip.
marked="$work/marked"
reference="$work/mark-reference"
mkdir -p \
  "$marked/$(command dirname "$generated_output")" \
  "$marked/internal/panel/frontend/dist" \
  "$marked/internal/panelassets/generated"
: >"$marked/$generated_output"
: >"$marked/internal/panel/frontend/node_modules/.package-lock.json"
: >"$marked/internal/panel/frontend/dist/index.html"
: >"$marked/internal/panelassets/generated/bundle.zip"
# Older than the checkout the step exists to out-date, which the reference stands in
# for: it is written after them, so anything the step misses stays behind it.
command find "$marked" -type f -exec touch -t 200001010000 {} +
: >"$reference"

printf '%s\n' "$mark_script" >"$work/mark.sh"
(
  CDPATH='' command cd -- "$marked"
  command bash "$work/mark.sh"
)

for output in \
  "$generated_output" \
  internal/panel/frontend/node_modules/.package-lock.json \
  internal/panel/frontend/dist/index.html \
  internal/panelassets/generated/bundle.zip; do
  if [[ ! "$marked/$output" -nt "$reference" ]]; then
    echo "panel bundle cache does not mark $output as current" >&2
    exit 1
  fi
done

mkdir -p "$frontend/node_modules"
printf '{"name":"cache-hit"}\n' >"$frontend/package.json"
printf '{"lockfileVersion":3}\n' >"$frontend/package-lock.json"

{
  printf 'package.json\0'
  command cat "$frontend/package.json"
  printf 'package-lock.json\0'
  command cat "$frontend/package-lock.json"
} >"$frontend/node_modules/.smyklot-panel-stamp"

cat >"$fake_npm" <<'FAKE'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"${SMYKLOT_PANEL_NPM_LOG:?}"
if [[ "$*" != 'run prepare' ]]; then
  printf 'unexpected npm command: %s\n' "$*" >&2
  exit 64
fi
generated="${SMYKLOT_PANEL_GENERATED:?}"
mkdir -p "$(dirname "$generated")"
printf '{}\n' >"$generated"
FAKE
chmod +x "$fake_npm"

SMYKLOT_PANEL_FRONTEND_DIR="$frontend" \
  SMYKLOT_PANEL_NPM="$fake_npm" \
  SMYKLOT_PANEL_NPM_LOG="$npm_log" \
  SMYKLOT_PANEL_GENERATED="$generated_relative" \
  ./scripts/install-panel-frontend.sh

if [[ ! -f "$frontend/$generated_relative" ]]; then
  echo "cache hit did not regenerate $generated_relative" >&2
  exit 1
fi
if [[ "$(command cat "$npm_log")" != 'run prepare' ]]; then
  printf 'cache hit ran unexpected npm commands:\n%s\n' "$(command cat "$npm_log")" >&2
  exit 1
fi

echo "cache hit regenerated $generated_relative"
