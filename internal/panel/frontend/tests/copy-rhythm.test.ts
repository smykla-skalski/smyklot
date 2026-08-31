import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

/**
 * THE COPY-RHYTHM LAW: inside a card row, a title line's box stands 8px from its
 * description line's box - one number for every family.
 *
 * Both families had derived it instead of reading it, as `calc(var(--leading-meta)
 * - 1cap)`, which resolves 10.31px once both boxes are trimmed to their ink. The
 * number was invisible in the source and identical in both places, so nothing
 * looked wrong: every row in the panel simply sat 2.31px looser than the law, and
 * a reader saw two lines that belonged together drift apart.
 *
 * A derivation is how a family whose geometry adds structural slack reaches 8 -
 * `--row-copy-gap-kind` is one, and it is declared at the token where the working
 * can be read. A derivation written at the rule is a second law nobody wrote down.
 */
const SHEET = readFileSync(new URL('../src/app.css', import.meta.url), 'utf8');
const TOKENS = readFileSync(new URL('../src/tokens.css', import.meta.url), 'utf8');

function rule(selector: string): string {
  const start = SHEET.indexOf(selector);
  expect(start, `${selector} is not in app.css`).toBeGreaterThan(-1);
  const open = SHEET.indexOf('{', start);
  const close = SHEET.indexOf('}', open);

  return SHEET.slice(open, close);
}

describe('the copy-rhythm law [Unit]', () => {
  it('is one number, and it is 8px', () => {
    expect(TOKENS).toMatch(/--row-copy-gap:\s*8px/u);
  });

  it.each([['.object-main > :is(.object-sum, .sum-swap)'], ['.setting-say > .setting-why']])(
    '%s reads the law rather than deriving one',
    (selector) => {
      expect(rule(selector)).toMatch(/margin-block-start:\s*var\(--row-copy-gap\)/u);
    },
  );

  /*
   * A badge on a name row bleeds past the name's cap, and the copy gap is where it
   * bleeds to. At the shared control height it came within 2.84px of the sentence
   * below it; sized from the gap, its bleed is exactly half of one - 8px under the
   * row's edge, 4px above the sentence, and the row still the height it declares.
   */
  it('sizes a badge in a copy pair so it bleeds half the gap, no more', () => {
    const bleeding = rule(
      '.object-main > .object-name-row > :not(.object-name, .file-path, .mono-title)',
    );

    expect(bleeding).toMatch(
      /--badge-in-a-pair:\s*calc\(\s*var\(--object-name-line\) - \(var\(--object-name-slack\) \* 2\) \+\s*var\(--row-copy-gap\)\s*\)/u,
    );
    expect(bleeding).toMatch(/min-block-size:\s*var\(--badge-in-a-pair\)/u);
    // A mark states its height outright, so a floor alone would never reach it.
    expect(rule('.object-main > .object-name-row > .mx-mark')).toMatch(
      /block-size:\s*var\(--badge-in-a-pair\)/u,
    );
  });
});
