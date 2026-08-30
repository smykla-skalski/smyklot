import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join } from 'node:path';

import { describe, expect, it } from 'vitest';

import { stylesheets } from './theme';

/**
 * AN ICON NAMES ITS TIER.
 *
 * The size used to be a bare number, on the reasoning that a chip, a row, a heading and
 * a rail tile do not agree on a step - so every call site picked its own. What that
 * produced was eight sizes across 204 sites, with 13 and 14 as one tier spelled two
 * ways: a button glyph at 13 beside a tile glyph at 14, which is a weight disagreement
 * nobody chose. The scale was documentation rather than a rule.
 *
 * A NUMBER IS STILL ALLOWED, and only for what is not an icon - an avatar, an empty
 * state's illustration, a mark. Those are drawings, not glyphs; they do not belong to
 * this scale, and adding a tier per drawing is how a scale stops being one. The rule
 * this holds is the boundary: anything at or under the top tier must name it, and only
 * something bigger may be a number.
 */
const TIERS = ['nano', 'micro', 'xs', 'sm', 'base', 'md'] as const;

function sources(dir: string, found: { file: string; text: string }[] = []) {
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) sources(path, found);
    else if (path.endsWith('.svelte')) found.push({ file: path, text: readFileSync(path, 'utf8') });
  }

  return found;
}

const files = [
  ...sources(new URL('../src', import.meta.url).pathname),
  ...sources(new URL('../stories', import.meta.url).pathname),
].filter(({ file }) => !file.endsWith('Icon.svelte'));

/** Every `<Icon …>` in the app, with whatever it asked for. */
const uses = files.flatMap(({ file, text }) =>
  [...text.matchAll(/<Icon\b(?<attrs>[^>]*)>/gu)].map((match) => ({
    attrs: match.groups?.attrs ?? '',
    file: file.replace(/^.*\/(?:src|stories)\//u, ''),
  })),
);

describe('the icon scale [Unit]', () => {
  it('has icons to check', () => {
    // A guard that stands down when its pattern stops matching is not a guard.
    expect(uses.length).toBeGreaterThan(100);
  });

  it('declares six tiers and no more', () => {
    const declared = [...stylesheets.matchAll(/--icon-(?<tier>[\w-]+):\s*(?<value>[^;]+);/gu)].map(
      (match) => ({ tier: match.groups?.tier ?? '', value: (match.groups?.value ?? '').trim() }),
    );
    expect(declared.map(({ tier }) => tier).sort()).toEqual([...TIERS].sort());
    // In px: an icon does not grow with the reader's font size, so it is not on the
    // rem side of the units law - and two of these were rem, which made them
    // fractional at any root but 16.
    expect(declared.filter(({ value }) => !/^\d+px$/u.test(value))).toEqual([]);
  });

  it('names a tier at every call site', () => {
    const literal = uses.filter(({ attrs }) => /size=\{\d+\}/u.test(attrs));
    const offScale = literal.filter(({ attrs }) => {
      const px = Number(/size=\{(?<px>\d+)\}/u.exec(attrs)?.groups?.px);

      /* The top tier is 18. Anything at or under it has a tier to name and must name
         one; anything above it is a drawing rather than a glyph. Read from the sheet
         so raising the scale raises this with it. */
      const top = Math.max(
        ...[...stylesheets.matchAll(/--icon-[\w-]+:\s*(?<px>\d+)px/gu)].map((match) =>
          Number(match.groups?.px),
        ),
      );

      return px <= top;
    });
    expect(
      offScale.map(({ file, attrs }) => `${file}: ${attrs.trim().slice(0, 60)}`),
      'a size the scale has a tier for has to name it',
    ).toEqual([]);
  });

  it('spells every named size as a tier that exists', () => {
    const named = uses
      .map(({ file, attrs }) => ({
        file,
        size: /size="(?<name>[\w-]+)"/u.exec(attrs)?.groups?.name,
      }))
      .filter((use): use is { file: string; size: string } => use.size !== undefined);
    const unknown = named.filter(({ size }) => !TIERS.includes(size as (typeof TIERS)[number]));
    expect(unknown.map(({ file, size }) => `${file}: size="${size}"`)).toEqual([]);
  });
});
