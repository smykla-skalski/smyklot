import { readFileSync, readdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

import { appStylesheet, tokensStylesheet } from './theme';

/**
 * Motion is read from the palette, never written down.
 *
 * The same rule `tone-ink` holds for colour, for the same reason: a duration spelled
 * at a call site is a duration nobody can change from one place, and this app had ten
 * of them where three are declared - `160ms` written out where `--duration-fast`
 * already means it, and a `cubic-bezier(0.22, 1, 0.36, 1)` shadowing
 * `--ease-standard`'s `(0.16, 1, 0.3, 1)` closely enough that no one would see the
 * difference and precisely enough that no one could say which was intended.
 *
 * Two families, and the distinction is what the ramp is for. A TRANSITION answers a
 * reader and belongs on press/fast/normal. A RHYTHM repeats while a condition holds -
 * a spinner, a countdown, a shimmer - and is named for the thing that beats at it,
 * because those are not steps on a scale and reading one as the next size up from
 * another would be wrong.
 *
 * Checked as source, because the runtime here has no DOM and no cascade.
 */
const src = fileURLToPath(new URL('../src/', import.meta.url));

/**
 * A literal that is not a design decision, and why each is allowed to stay.
 *
 * Every entry is a mechanism rather than a duration somebody chose from a scale: take
 * it out and the rule it serves stops working, which is not what a token would fix.
 */
const ALLOWED: Readonly<Record<string, string>> = {
  '0.01ms':
    'the blanket reduced-motion collapse in app.css, which is how every animation is ' +
    'shortened at once rather than a duration anything renders at',
  '1ms':
    'a reduced-motion re-assertion. An animation that drives a state change cannot be ' +
    'squashed to nothing or the state it ends never arrives - see animation-lifetime',
  '0s': 'an instant, which is the absence of a duration rather than a short one',
};

/**
 * Decor keeps its own timings, declared here with the reason.
 *
 * The night sky's stars are on four periods between 7 and 17 seconds with offset
 * delays, and that spread IS the effect: put them on one token and every star in the
 * sky blinks together. Nothing else in the panel beats at those rates and nothing
 * should, so a token would be a scale with one member used once.
 */
const DECOR = new Set(['NightSky.svelte']);

interface Site {
  readonly file: string;
  readonly declaration: string;
  readonly literals: readonly string[];
}

/** Every `transition` and `animation` in the app, with the literals it names. */
function sites(): Site[] {
  const files: [string, string][] = [
    ['app.css', appStylesheet],
    ['tokens.css', tokensStylesheet],
  ];
  for (const entry of readdirSync(src, { recursive: true, withFileTypes: true })) {
    if (!entry.isFile() || !entry.name.endsWith('.svelte')) continue;
    const source = readFileSync(`${entry.parentPath}/${entry.name}`, 'utf8');
    const style = /<style>(?<css>[\s\S]*)<\/style>/u.exec(source)?.groups?.css;
    if (style !== undefined) files.push([entry.name, style]);
  }

  const found: Site[] = [];
  for (const [file, css] of files) {
    if (DECOR.has(file)) continue;
    /* A `var()` is the answer, so its contents are not a literal - blanking them first
       is what stops `var(--ease-out)` reading as the keyword `ease-out`. */
    const spent = css.replaceAll(/var\(--[\w-]+(?:,[^)]*)?\)/gu, 'VAR');
    for (const rule of spent.matchAll(
      /(?:transition|animation)[a-z-]*\s*:\s*(?<value>[^;{}]+)/gu,
    )) {
      const value = (rule.groups?.value ?? '').replaceAll(/\s+/gu, ' ').trim();
      const literals = [
        ...[...value.matchAll(/(?<![\w-])\d*\.?\d+(?:ms|s)(?![\w-])/gu)].map((m) => m[0]),
        ...[
          ...value.matchAll(
            /cubic-bezier\([^)]*\)|(?<![\w-])(?:ease-in-out|ease-out|ease-in|ease|linear|steps\([^)]*\))(?![\w-])/gu,
          ),
        ].map((m) => m[0]),
      ].filter((literal) => ALLOWED[literal] === undefined);
      if (literals.length > 0) found.push({ declaration: value, file, literals });
    }
  }
  return found;
}

describe('motion [Unit]', () => {
  it('reads every duration and easing from a token', () => {
    const written = sites();
    expect(
      written.map((site) => `${site.file}: ${site.literals.join(', ')}  in  ${site.declaration}`),
      'these name a duration or an easing instead of reading one, so the ramp cannot ' +
        'be changed from one place',
    ).toEqual([]);
  });

  it('has motion to check', () => {
    // A sweep that stopped matching would report nothing wrong for the same reason it
    // reports nothing at all. This says which.
    const all = readdirSync(src, { recursive: true, withFileTypes: true }).filter(
      (entry) => entry.isFile() && entry.name.endsWith('.svelte'),
    );
    expect(all.length).toBeGreaterThan(50);
    expect(/--duration-press:\s*\d/u.test(tokensStylesheet)).toBe(true);
    expect(/--rhythm-spinner:\s*\d/u.test(tokensStylesheet)).toBe(true);
  });

  it('states a reason for every literal it still permits', () => {
    // Guards the exception list against becoming the place durations are quietly
    // parked: an entry without a reason is not an exception, it is an omission.
    for (const [literal, reason] of Object.entries(ALLOWED)) {
      expect(reason.length, `${literal} is permitted without saying why`).toBeGreaterThan(20);
    }
    expect(DECOR.size).toBeGreaterThan(0);
  });
});
