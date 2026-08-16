#!/usr/bin/env bash
#
# Fails when a breaking change would not release a major version.
#
# semantic-release reads .releaserc.yml's releaseRules in order, takes the
# first match, and reads them before its own built-in rules. So a rule matching
# `type: feat` matches a breaking `feat!` as well, answers "minor", and the
# built-in `breaking -> major` is never reached.
#
# Nothing downstream notices. The release runs, the tag is plausible, and the
# only evidence is a version number nobody double-checks. It went unseen until
# the repository's first breaking change shipped as 1.31.0.
#
# The order is the whole of the fix, so the order is what this checks: the
# breaking rule exists, and nothing that could shadow it comes first.
set -euo pipefail

cd "$(dirname "$0")/.."
config=".releaserc.yml"

rules="$(yq -r \
  '.plugins[] | select(.[0] == "@semantic-release/commit-analyzer") | .[1].releaseRules' \
  "$config")"

if [ "$rules" = "null" ] || [ -z "$rules" ]; then
  # No custom rules means the built-in ones apply untouched, and those already
  # release a major. Nothing to hold.
  echo "$config sets no release rules; the built-in breaking rule applies"
  exit 0
fi

first="$(yq -r \
  '.plugins[] | select(.[0] == "@semantic-release/commit-analyzer")
   | .[1].releaseRules[0] | [.breaking, .release] | join(" ")' \
  "$config")"

if [ "$first" != "true major" ]; then
  echo "the first release rule in $config is not the breaking one" >&2
  echo "found: $first" >&2
  cat >&2 <<'WHY'
A custom rule is read before the built-in breaking rule and the first match
wins, so any rule ahead of it - `type: feat` above all - swallows a breaking
change and releases a minor. Put this first:

  releaseRules:
    - breaking: true
      release: major
WHY
  exit 1
fi

echo "a breaking change releases a major version"
