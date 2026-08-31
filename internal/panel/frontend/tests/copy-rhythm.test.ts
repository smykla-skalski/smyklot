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
});
