import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

import {
  composeFile,
  composesNothing,
  formatJson,
  parseJson,
  type JsonValue,
  type MergeSpec,
} from '#lib/merge.js';

/**
 * The panel composes the same bytes the service composes.
 *
 * Not "a merge that looks right": the panel draws the file a repository is
 * about to hold and lets somebody EDIT it, so a composer that disagrees with
 * the service by a byte turns an edit into a save of something else. The
 * panel's copy used to be RFC 7396 and nothing else while the service also
 * honoured `strategy`, `arrays` and `deduplicate` - so a repository with an
 * append rule was shown its own list replacing the template's, under a picker
 * that changed nothing on screen.
 *
 * One table, two implementations. `internal/orgsync/filemerge/panel_parity_test.go`
 * runs the same file through the Go engine, so a case added in one place is run
 * by both and neither can be quietly taught a different rule.
 */
interface ParityCase {
  name: string;
  path: string;
  template: string;
  spec: MergeSpec;
  expected?: string;
  /** A merge neither side will compose. */
  refused?: boolean;
  /** A spec that composes nothing, so the template is what the repository holds. */
  verbatim?: boolean;
}

const table = JSON.parse(
  readFileSync(
    new URL('../../../orgsync/filemerge/testdata/panel-parity.json', import.meta.url),
    'utf8',
  ),
) as { cases: ParityCase[] };

describe('merge parity [Unit]', () => {
  it('reads the table the Go engine is held to', () => {
    expect(table.cases.length).toBeGreaterThan(15);
  });

  for (const one of table.cases) {
    it(one.name, () => {
      const template = parseJson(one.template);
      expect(template, 'the case template is not JSON').not.toBeUndefined();

      if (one.verbatim === true) {
        expect(composesNothing(one.spec)).toBe(true);

        return;
      }

      const composed = composeFile(template as JsonValue, one.spec);

      if (one.refused === true) {
        expect(composed.ok, 'this merge should have been refused').toBe(false);

        return;
      }

      expect(composed.ok ? '' : composed.reason).toBe('');
      expect(composed.ok && formatJson(composed.value)).toBe(one.expected);
    });
  }
});
