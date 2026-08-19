import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

import { syncPathIndex, type PathIndexAnswer, type PathScanRow } from '../dev/path-index.ts';

/**
 * The dev mock folds the finder's path list the way the service folds it.
 *
 * The mock is what a developer builds every panel change against, so where the
 * two disagree the panel is being written against a picture of the service
 * rather than the service. These two fields are the ones that hid it: neither
 * `repositories` nor `observed_at` is drawn as itself - one is the denominator
 * under "held by 4 of 6" and the other decides whether the notice above the
 * finder says the list is stale. The mock counted every repository the
 * installation has rather than the rows read, and stamped every answer with
 * `now()`, so the stale notice could not appear in development at all.
 *
 * One table, two implementations, the way `filemerge` holds the composer.
 * `internal/panel/pathindex_test.go` runs the same file through Go.
 */
interface IndexCase {
  name: string;
  rows: PathScanRow[];
  expected: PathIndexAnswer;
}

const table = JSON.parse(
  readFileSync(new URL('../../testdata/path-index.json', import.meta.url), 'utf8'),
) as { cases: IndexCase[] };

describe('path index [Unit]', () => {
  it('reads the table the service is held to', () => {
    expect(table.cases.length).toBeGreaterThanOrEqual(5);
  });

  for (const one of table.cases) {
    it(one.name, () => {
      expect(syncPathIndex(one.rows)).toEqual(one.expected);
    });
  }
});
