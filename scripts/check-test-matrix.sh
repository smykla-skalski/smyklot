#!/usr/bin/env bash
#
# Fails when a suite exists that the Test workflow's matrices do not run.
#
# The workflow gives each area its own runner, so what runs is a list of
# directories rather than the repository. A list is a thing you can forget to
# add to, and nothing downstream notices: a new package arrives with its own
# specs, they pass locally, and CI reports green having never compiled them.
# That used not to matter, because the release ran `mise run test` over the
# whole tree and would have caught it. The release runs this workflow now.
#
# So the workflow is asked which directories it names, the repository is asked
# which ones hold specs, and the second must be inside the first.
set -euo pipefail

cd "$(dirname "$0")/.."
workflow=".github/workflows/test.yaml"

# Only the SQLite pass. The PostgreSQL matrix runs a deliberate subset - the
# layers that open an engine - so a suite it does not name is not a gap.
#
# ginkgo -r recurses, so a named directory stands for everything beneath it.
named="$(
  yq -r '.jobs.go.strategy.matrix.include[].dirs' "$workflow" |
    tr ' ' '\n' | sed 's|^\./||' | grep -v '^$' | sort -u
)"

if [ -z "$named" ]; then
  echo "read no directories out of the go matrix in $workflow" >&2
  exit 1
fi

missing=""
while read -r suite; do
  covered=""
  while read -r dir; do
    case "$suite/" in "$dir"/*)
      covered="yes"
      break
      ;;
    esac
  done <<<"$named"
  [ -n "$covered" ] || missing="${missing}  ${suite}"$'\n'
done < <(git ls-files '*_test.go' | sed 's|/[^/]*$||' | sort -u)

if [ -n "$missing" ]; then
  echo "the go matrix in $workflow runs nothing in:" >&2
  printf '%s' "$missing" >&2
  echo "add each to an existing area, or give it one of its own" >&2
  exit 1
fi

echo "every Go suite is named by the go matrix in $workflow"

# The browser matrix names files rather than directories, and it has a second
# way to go wrong the Go one does not: a file named twice runs twice, on two
# runners, and the only symptom is a job that got slower for no reason anybody
# can see. So both directions are checked.
browser="internal/panel/frontend"
listed="$(yq -r '.jobs.browser.strategy.matrix.include[].files' "$workflow" | tr ' ' '\n' | grep -v '^$')"

if [ -z "$listed" ]; then
  echo "read no files out of the browser matrix in $workflow" >&2
  exit 1
fi

twice="$(printf '%s\n' "$listed" | sort | uniq -d)"
if [ -n "$twice" ]; then
  echo "the browser matrix in $workflow runs these on more than one runner:" >&2
  printf '%s\n' "$twice" | sed 's|^|  |' >&2
  exit 1
fi

present="$(git ls-files "$browser/tests/browser/*.test.ts" | sed "s|^$browser/||" | sort)"
missing="$(comm -23 <(printf '%s\n' "$present") <(printf '%s\n' "$listed" | sort))"
gone="$(comm -13 <(printf '%s\n' "$present") <(printf '%s\n' "$listed" | sort))"

if [ -n "$missing" ]; then
  echo "the browser matrix in $workflow runs nothing in:" >&2
  printf '%s\n' "$missing" | sed 's|^|  |' >&2
  echo "add each to an existing area, or give it one of its own" >&2
  exit 1
fi

if [ -n "$gone" ]; then
  echo "the browser matrix in $workflow names files that do not exist:" >&2
  printf '%s\n' "$gone" | sed 's|^|  |' >&2
  exit 1
fi

echo "every browser suite is named exactly once by the browser matrix in $workflow"
