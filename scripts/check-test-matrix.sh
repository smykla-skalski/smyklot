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

# Twice is as wrong as never, and quieter. Because `-r` recurses, an area that
# names a parent has already named everything under it: naming a child as well
# runs that suite twice in the one job, and naming it from a second area runs it
# again on a second runner. Both cost what the suite costs and neither reports
# itself as anything but a job that got slower - which is how the storage area
# came to run the SQLite conformance suite, its slowest, twice over.
missing=""
twice=""
while read -r suite; do
  covered=""
  for dir in $named; do
    case "$suite/" in "$dir"/*)
      covered="${covered}${dir} "
      ;;
    esac
  done
  if [ -z "$covered" ]; then
    missing="${missing}  ${suite}"$'\n'
  elif [ "$(echo "$covered" | wc -w)" -gt 1 ]; then
    twice="${twice}  ${suite} is run by: ${covered}"$'\n'
  fi
done < <(git ls-files '*_test.go' | sed 's|/[^/]*$||' | sort -u)

if [ -n "$missing" ]; then
  echo "the go matrix in $workflow runs nothing in:" >&2
  printf '%s' "$missing" >&2
  echo "add each to an existing area, or give it one of its own" >&2
  exit 1
fi

if [ -n "$twice" ]; then
  echo "the go matrix in $workflow runs these more than once:" >&2
  printf '%s' "$twice" >&2
  echo "ginkgo -r recurses, so drop whichever entry the other already covers" >&2
  exit 1
fi

echo "every Go suite is named exactly once by the go matrix in $workflow"

# Being named by the matrix is not the same as being run. Ginkgo specs are
# registered by Describe at init and executed by one RunSpecs bootstrap; a
# package that has the first and not the second compiles, reports ok, and runs
# nothing. It looks identical to a package whose specs all pass. This is how
# seven specs rode along unexecuted after their file moved out of a package
# that had a bootstrap into one that did not.
orphaned=""
for directory in $(git ls-files '*_test.go' | xargs -n1 dirname | sort -u); do
  if ! grep -lq 'onsi/ginkgo' "$directory"/*_test.go 2>/dev/null; then
    continue
  fi
  if ! grep -q 'RunSpecs(' "$directory"/*_test.go 2>/dev/null; then
    orphaned="$orphaned  $directory"$'\n'
  fi
done

if [ -n "$orphaned" ]; then
  echo "these packages register Ginkgo specs that nothing executes:" >&2
  printf '%s' "$orphaned" >&2
  echo "add a suite bootstrap calling RunSpecs, or the specs never run" >&2
  exit 1
fi

echo "every package holding Ginkgo specs has a bootstrap that runs them"

# The browser matrix names files rather than directories, and it has a second
# way to go wrong the Go one does not: a file named twice runs twice, on two
# runners, and the only symptom is a job that got slower for no reason anybody
# can see. So both directions are checked.
#
# CI deliberately does not run every browser file - the sweeps and the budgets
# earn a runner, one screen's own behaviour does not, and the rest are gathered
# in the `panel:frontend:test:browser:local` task with the reasoning beside them.
# That makes "not in CI" an answer rather than an omission, so both lists are
# read here and a file has to be in exactly one of them. A file in neither is the
# case this check was written for: it runs nowhere and nothing says so.
browser="internal/panel/frontend"
tasks=".mise.toml"
onCI="$(yq -r '.jobs.browser.strategy.matrix.include[].files' "$workflow" | tr ' ' '\n' | grep -v '^$')"
# `-oy` only to silence yq's warning that it is guessing an output format from
# the extension; the value read is a plain string either way.
local_only="$(
  yq -p toml -oy -r '.tasks."panel:frontend:test:browser:local".run' "$tasks" |
    tr ' ' '\n' | grep '^tests/browser/.*\.test\.ts$' || true
)"

if [ -z "$onCI" ]; then
  echo "read no files out of the browser matrix in $workflow" >&2
  exit 1
fi

if [ -z "$local_only" ]; then
  echo "read no files out of panel:frontend:test:browser:local in $tasks" >&2
  exit 1
fi

listed="$(printf '%s\n%s\n' "$onCI" "$local_only")"
twice="$(printf '%s\n' "$listed" | sort | uniq -d)"
if [ -n "$twice" ]; then
  echo "these browser suites are named more than once, by $workflow and/or $tasks:" >&2
  printf '%s\n' "$twice" | sed 's|^|  |' >&2
  echo "a file runs on one runner, or locally, and never both" >&2
  exit 1
fi

present="$(git ls-files "$browser/tests/browser/*.test.ts" | sed "s|^$browser/||" | sort)"
missing="$(comm -23 <(printf '%s\n' "$present") <(printf '%s\n' "$listed" | sort))"
gone="$(comm -13 <(printf '%s\n' "$present") <(printf '%s\n' "$listed" | sort))"

if [ -n "$missing" ]; then
  echo "nothing runs these browser suites:" >&2
  printf '%s\n' "$missing" | sed 's|^|  |' >&2
  echo "add each to an area in $workflow, or to the local task in $tasks" >&2
  exit 1
fi

if [ -n "$gone" ]; then
  echo "$workflow or $tasks names browser suites that do not exist:" >&2
  printf '%s\n' "$gone" | sed 's|^|  |' >&2
  exit 1
fi

echo "every browser suite is named exactly once, by $workflow or by $tasks"
