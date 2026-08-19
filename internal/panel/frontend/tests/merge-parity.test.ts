import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

import {
  composable,
  composeFile,
  composesNothing,
  formatJson,
  parseJson,
  validateSpec,
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
  /**
   * A merge the service composes and the panel declines to, by format.
   *
   * The panel reads JSON; `filemerge` also reads YAML and edits Markdown by its
   * headings. That gap is real and the table is where it is written down - a
   * case with no verb would read as agreement, and one marked `refused` would
   * claim the service refuses a merge it runs every sweep.
   */
  unsupported?: boolean;
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
      if (one.unsupported === true) {
        // Declined by format, and only by format: the reason names the file
        // rather than anything in the spec, and `composable` is what the
        // component asks before it draws a merge at all.
        expect(composable(one.path), `${one.path} is one this composes`).toBe(false);
        expect(validateSpec(one.path, one.spec)).toContain(one.path);

        return;
      }

      const template = parseJson(one.template);
      expect(template, 'the case template is not JSON').not.toBeUndefined();

      if (one.verbatim === true) {
        expect(composesNothing(one.spec)).toBe(true);

        return;
      }

      const asked = JSON.stringify(one.spec);
      const composed = composeFile(one.path, template as JsonValue, one.spec);

      /* Composing is a question, so asking it twice answers the same thing.
         It did not: a shallow merge stores the adjustment's value by reference
         rather than a copy, so writing a rule's joined list back reached into
         the adjustment it came from - the file grew the template's own entries
         every time it was drawn, and the check `deriveOverrides` makes by
         composing its candidate again passed because both sides of the
         comparison were one object. Asserted for every case rather than for the
         one that found it. */
      const again = composeFile(one.path, template as JsonValue, one.spec);
      expect(JSON.stringify(one.spec), 'composing changed the spec it was given').toBe(asked);
      expect(again.ok && formatJson(again.value)).toBe(composed.ok && formatJson(composed.value));

      if (one.refused === true) {
        expect(composed.ok, 'this merge should have been refused').toBe(false);

        return;
      }

      expect(composed.ok ? '' : composed.reason).toBe('');
      expect(composed.ok && formatJson(composed.value)).toBe(one.expected);
    });
  }
});
