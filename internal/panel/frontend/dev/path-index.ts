/**
 * The finder's path list, folded the way the service folds it.
 *
 * Its own module rather than a few lines inside the handler, because this is
 * the half of the mock a test can hold to the service. The two had drifted on
 * exactly the fields nothing on screen states plainly: `repositories` is the
 * denominator under "held by 4 of 6", and `observed_at` decides whether the
 * notice above the finder says the list is stale. The mock counted every
 * repository the workspace has and stamped every answer with `now()`, so a
 * developer could not see that notice at all - the one state hardest to reach
 * in production was the one made unreachable in development.
 *
 * `internal/panel/testdata/path-index.json` is the table both sides run,
 * `internal/panel/pathindex_test.go` on the Go side and
 * `tests/path-index.test.ts` here.
 */

/** One repository's stored list, as the sweep last read it. */
export interface PathScanRow {
  repository_id: string;
  paths: readonly string[];
  /** When that reading was taken. */
  observed_at: string;
  /** Whether GitHub finished listing the tree. */
  partial?: boolean;
}

export interface PathIndexAnswer {
  paths: { path: string; repositories: number }[];
  repositories: number;
  partial: boolean;
  observed_at?: string;
}

export function syncPathIndex(rows: readonly PathScanRow[]): PathIndexAnswer {
  const counts = new Map<string, number>();
  let observed: string | undefined;
  let partial = false;

  for (const row of rows) {
    /* The OLDEST, which is the same reading `partial` takes: this answer is the
       union of every repository's list, so how far it can be trusted is decided
       by its weakest row rather than by its freshest. The newest said "checked
       a minute ago" for a list holding a repository nothing had looked at in a
       week. Compared as text, which is what an RFC 3339 instant in UTC allows -
       and both writers here are the same one. */
    if (observed === undefined || row.observed_at < observed) observed = row.observed_at;
    partial = partial || row.partial === true;
    for (const path of row.paths) counts.set(path, (counts.get(path) ?? 0) + 1);
  }

  const paths = [...counts]
    .map(([path, repositories]) => ({ path, repositories }))
    /* Held by most first, then by path - and by path the way the service breaks
       that tie, which is `strings.Compare`, a byte comparison. `localeCompare`
       puts `README.md` and `api.md` the other way round, so the mock and the
       service disagreed about the order of the very first rows a reader sees. */
    .sort((left, right) => {
      if (left.repositories !== right.repositories) {
        return right.repositories - left.repositories;
      }

      return left.path < right.path ? -1 : left.path > right.path ? 1 : 0;
    });

  /* `repositories` counts the rows this was built FROM, not the workspace's
     repositories: counting ones nothing has ever looked at would put a ceiling
     under "held by 4 of 6" that no path can reach. */
  const answer: PathIndexAnswer = { paths, repositories: rows.length, partial };
  if (observed !== undefined) answer.observed_at = observed;

  return answer;
}
