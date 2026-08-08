#!/usr/bin/env bash
set -euo pipefail

ROOT="$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)"
frontend="${SMYKLOT_PANEL_FRONTEND_DIR:-$ROOT/internal/panel/frontend}"
manifest="$frontend/package.json"
lockfile="$frontend/package-lock.json"
stamp="$frontend/node_modules/.smyklot-panel-stamp"
npm="${SMYKLOT_PANEL_NPM:-npm}"

if [[ ! -f "$manifest" ]]; then
  printf 'panel frontend manifest does not exist: %s\n' "$manifest" >&2
  exit 1
fi

if [[ ! -f "$lockfile" ]]; then
  printf 'panel frontend lockfile does not exist: %s\n' "$lockfile" >&2
  exit 1
fi

stamp_tmp="$frontend/.smyklot-panel-stamp.$$"
trap 'command rm -f -- "$stamp_tmp"' EXIT

{
  printf 'package.json\0'
  command cat "$manifest"
  printf 'package-lock.json\0'
  command cat "$lockfile"
} >"$stamp_tmp"

if [[ -f "$stamp" ]] && command cmp -s "$stamp_tmp" "$stamp"; then
  exit 0
fi

(
  CDPATH='' command cd -- "$frontend"
  command "$npm" ci --include=dev --no-audit --no-fund
)

command mv "$stamp_tmp" "$stamp"
