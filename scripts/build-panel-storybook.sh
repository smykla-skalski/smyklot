#!/usr/bin/env bash
#
# Builds the panel's component catalogue as a static site, then proves it can
# actually run. `storybook build` exits 0 on a bundle whose first module throws:
# the catalogue it produced is a spinner and nothing else, and no other check in
# this repository loads it.
set -euo pipefail

ROOT="$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)"
frontend="$ROOT/internal/panel/frontend"
out="$frontend/storybook-static"

(
  CDPATH='' command cd -- "$frontend"
  SMYKLOT_PANEL_DEV_MOCK=1 command npm run storybook:build
)

# SvelteKit's client runtime reads a handful of `__SVELTEKIT_*` globals that its
# own Vite plugin replaces with literals. Storybook runs half of that plugin -
# `@storybook/sveltekit` drops the one that builds a Kit app - so a global whose
# only declaration is on the half that did not run survives into the bundle as a
# bare identifier. `client/payload.js` reads one at module scope, and a
# ReferenceError there takes down the whole preview before a story renders.
#
# The names are Kit's, so a version that adds one is caught by the shape rather
# than by a list this would have to be taught.
surviving="$(command grep -rho '__SVELTEKIT_[A-Z_]*__' "$out" | command sort -u || true)"
if [[ -n "$surviving" ]]; then
  printf 'the catalogue carries SvelteKit globals nothing declares:\n%s\n' "$surviving" >&2
  printf 'declare them in .storybook/main.ts, under viteFinal.\n' >&2
  exit 1
fi

# The entry the deploy publishes. A build that wrote nothing here is a bundle no
# server can serve, and `storybook build` is happy either way.
if [[ ! -f "$out/index.html" || ! -f "$out/iframe.html" ]]; then
  echo 'the catalogue is missing index.html or iframe.html' >&2
  exit 1
fi

printf 'catalogue built: %s\n' "$out"
