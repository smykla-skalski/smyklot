import { readFileSync, readdirSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

import { appStylesheet, tokensStylesheet } from './theme';

/**
 * Selectors the browser throws away without saying so.
 *
 * A rule with an invalid selector is not an error anywhere in the pipeline - not in the compiler,
 * not in `svelte-check`, not in the console. The rule is simply dropped, and what you see is the
 * page without it, which usually looks like a styling mistake rather than a syntax one. It cost a
 * round of "the fix does not work" on the segmented control's hover, where `:has()` inside `:has()`
 * silently deleted the two rules that did the work.
 *
 * Checked as source, because the runtime here has no DOM and no cascade.
 */

const components = new URL('../src/lib/components/', import.meta.url);

const sources = [
  ...readdirSync(components)
    .filter((file) => file.endsWith('.svelte'))
    .map((file) => [file, readFileSync(new URL(file, components), 'utf8')] as const),
  ['app.css', appStylesheet] as const,
  ['tokens.css', tokensStylesheet] as const,
];

/** Every `:has(...)` in the source, with the text it encloses, matching parens by counting. */
function hasArguments(source: string): string[] {
  const found: string[] = [];
  for (
    let start = source.indexOf(':has(');
    start !== -1;
    start = source.indexOf(':has(', start + 1)
  ) {
    let depth = 0;
    for (let index = start + ':has'.length; index < source.length; index += 1) {
      if (source[index] === '(') depth += 1;
      else if (source[index] === ')') {
        depth -= 1;
        if (depth === 0) {
          found.push(source.slice(start + ':has('.length, index));
          break;
        }
      }
    }
  }
  return found;
}

describe('selectors', () => {
  it('never nest :has() inside :has()', () => {
    // Forbidden by the spec, so the whole rule is invalid and the browser drops it.
    const offenders = sources.flatMap(([file, source]) =>
      hasArguments(source)
        .filter((argument) => argument.includes(':has('))
        .map((argument) => `${file}: :has(${argument})`),
    );

    expect(offenders).toEqual([]);
  });

  it('finds the nesting when it is there', () => {
    // The check is worth nothing if it cannot see the shape it exists for. Every `:has(` is
    // reported, the inner one included, so it is the outer entry that carries the offence.
    expect(hasArguments('a:has(b:has(c)) {}')).toEqual(['b:has(c)', 'c']);
    expect(hasArguments('a:has(b + c) d:has(e) {}')).toEqual(['b + c', 'e']);
    expect(hasArguments('a:has(:not(b)) {}')).toEqual([':not(b)']);
    expect(hasArguments('a:has(b + c) {}').some((argument) => argument.includes(':has('))).toBe(
      false,
    );
    expect(hasArguments('a:has(b:has(c)) {}').some((argument) => argument.includes(':has('))).toBe(
      true,
    );
  });
});
