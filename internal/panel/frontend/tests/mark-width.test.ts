import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

import { stylesheets } from './theme';

/**
 * A mark is as wide as what it says, wherever it is put.
 *
 * `display: inline-flex` states that on its own everywhere EXCEPT inside a flex or
 * grid parent, which is most of this app: a flex item's display is blockified to
 * `flex`, and a cross size left `auto` is exactly what `align-items: stretch` acts on.
 * So a pill dropped into a column comes out the full width of the column with its two
 * words at the left end. An explicit inline size is not `auto`, so there is nothing
 * left to stretch.
 *
 * Measured across the catalogue rather than reasoned about: every primitive story was
 * being drawn at 1110px in a 1110px column, and the ones below wanted between 34px and
 * 237px. The app's own rows are horizontal, which is why this went unseen - a story
 * stands directly in the content column and a table cell does not.
 *
 * Named rather than swept, because "should this fill?" is a design decision and not a
 * property of the selector. A field fills its toolbar on purpose; `.select-wrap` is
 * `display: block` and `.identity-row` is `display: flex`, and both mean it. What is
 * listed here is only what must never stretch.
 */
const components = fileURLToPath(new URL('../src/lib/components/', import.meta.url));

/** Every mark, and the file its root rule is written in. */
const MARKS: ReadonlyArray<readonly [selector: string, file: string]> = [
  ['.btn', 'app.css'],
  ['.chip', 'app.css'],
  ['.status-pill', 'app.css'],
  ['.link', 'app.css'],
  ['.switch', 'Switch.svelte'],
  ['.linked-control', 'InheritControl.svelte'],
  ['.pattern-entries', 'PatternEntries.svelte'],
];

function rule(selector: string, file: string): string {
  const source = file === 'app.css' ? stylesheets : readFileSync(`${components}${file}`, 'utf8');
  /* The root rule, at either indentation: `app.css` writes it at the margin and a
     component's `<style>` writes it two in. */
  const found = new RegExp(
    String.raw`^[ ]*${selector.replace('.', String.raw`\.`)}\s*\{(?<body>[^}]*)\}`,
    'mu',
  ).exec(source);
  if (found?.groups?.body === undefined) {
    throw new Error(`${file} has no \`${selector}\` rule`);
  }
  return found.groups.body;
}

describe('a mark is as wide as what it says [Unit]', () => {
  it.each(MARKS)('%s holds its own width', (selector, file) => {
    expect(
      /inline-size:\s*fit-content/u.test(rule(selector, file)),
      `${selector} in ${file} can be stretched by a flex or grid parent; it needs ` +
        '`inline-size: fit-content` beside its display',
    ).toBe(true);
  });

  it('reads a rule it can actually find', () => {
    // The regex above returns '' for a rule it cannot parse, and '' passes nothing -
    // but a selector that stopped matching would fail every case with the same
    // message, which reads as a code change rather than a broken test. This says which.
    for (const [selector, file] of MARKS) {
      expect(rule(selector, file).length, `${selector} in ${file} parsed as empty`).toBeGreaterThan(
        10,
      );
    }
  });
});
