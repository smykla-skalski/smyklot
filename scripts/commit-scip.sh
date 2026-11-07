#!/usr/bin/env bash
# Commits the regenerated scip/*.scip onto main, as one child of the commit they
# were built from.
#
# Through the Git data API rather than `git push`: main requires signed commits,
# and GitHub signs a commit an App creates through the API. The ref update is not
# forced, so when main has moved on since BASE this fails rather than overwriting
# it, and the run for the newer commit carries fresher indexes anyway.
set -euo pipefail

: "${GH_TOKEN:?}" "${REPO:?}" "${BASE:?}"

work="$(mktemp -d)"
trap 'rm -rf -- "$work"' EXIT

echo '[]' >"$work/tree.json"
for file in scip/*.scip; do
  base64 -w0 "$file" | jq -Rs '{encoding: "base64", content: .}' >"$work/blob.json"
  sha="$(gh api "repos/$REPO/git/blobs" --input "$work/blob.json" --jq .sha)"
  jq --arg path "$file" --arg sha "$sha" \
    '. + [{path: $path, mode: "100644", type: "blob", sha: $sha}]' \
    "$work/tree.json" >"$work/tree.next.json"
  mv "$work/tree.next.json" "$work/tree.json"
done

base_tree="$(gh api "repos/$REPO/git/commits/$BASE" --jq .tree.sha)"
jq -n --arg base "$base_tree" --slurpfile tree "$work/tree.json" \
  '{base_tree: $base, tree: $tree[0]}' >"$work/tree.request.json"
tree="$(gh api "repos/$REPO/git/trees" --input "$work/tree.request.json" --jq .sha)"

if [ "$tree" = "$base_tree" ]; then
  echo "The indexes already match $BASE"
  exit 0
fi

commit="$(gh api "repos/$REPO/git/commits" \
  -f message="chore(scip): refresh code intelligence indexes" \
  -f tree="$tree" -f "parents[]=$BASE" --jq .sha)"
gh api --method PATCH "repos/$REPO/git/refs/heads/main" -f sha="$commit" -F force=false >/dev/null
echo "Committed $commit onto main"
