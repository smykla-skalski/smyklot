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
generated_output='internal/panel/frontend/.svelte-kit/tsconfig.json'

if ! mise tasks info panel:frontend:install --json |
  yq -e ".outputs | contains([\"$generated_output\"])" >/dev/null; then
  echo 'panel:frontend:install does not declare .svelte-kit/tsconfig.json as an output' >&2
  exit 1
fi

toolchain='.github/actions/toolchain/action.yaml'
cache_paths="$(
  yq -r '.runs.steps[] | select(.name == "Restore the panel bundle") | .with.path' "$toolchain"
)"
if ! command grep -Fqx -- "$generated_output" <<<"$cache_paths"; then
  echo 'panel bundle cache does not preserve .svelte-kit/tsconfig.json' >&2
  exit 1
fi
mark_script="$(
  yq -r '.runs.steps[] | select(.name == "Mark the panel bundle as current") | .run' "$toolchain"
)"
if ! command grep -Fqx -- "touch $generated_output" <<<"$mark_script"; then
  echo 'panel bundle cache does not mark .svelte-kit/tsconfig.json as current' >&2
  exit 1
fi

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
mkdir -p .svelte-kit
printf '{}\n' >.svelte-kit/tsconfig.json
FAKE
chmod +x "$fake_npm"

SMYKLOT_PANEL_FRONTEND_DIR="$frontend" \
  SMYKLOT_PANEL_NPM="$fake_npm" \
  SMYKLOT_PANEL_NPM_LOG="$npm_log" \
  ./scripts/install-panel-frontend.sh

if [[ ! -f "$frontend/.svelte-kit/tsconfig.json" ]]; then
  echo 'cache hit did not regenerate .svelte-kit/tsconfig.json' >&2
  exit 1
fi
if [[ "$(command cat "$npm_log")" != 'run prepare' ]]; then
  printf 'cache hit ran unexpected npm commands:\n%s\n' "$(command cat "$npm_log")" >&2
  exit 1
fi

echo 'cache hit regenerated .svelte-kit/tsconfig.json'
