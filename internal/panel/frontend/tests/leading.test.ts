import { readFileSync, readdirSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

import { BLOCK_COMMENT, HTML_COMMENT, stripAll } from './support/markup';
import { stylesheets } from './theme';

/**
 * LEADING COMES FROM THE SCALE, and lands on a whole pixel.
 *
 * A ratio is the trap. It inherits AS A RATIO, so every tier under the one that set it
 * multiplies by its own size and lands wherever that falls: `body` carried `font:
 * 0.9375rem/1.5`, which put 15px copy on a 22.5px line and every odd-px tier beneath it
 * on a fraction of its own - 2485 elements at 22.5, 1446 at 19.5. Half a pixel is not a
 * rounding error on a screen at 2x; it is a soft edge on every glyph above it and a row
 * that starts off the device grid.
 *
 * A `font` shorthand is the second trap, and the reason this reads them: leaving the
 * leading out of one does not inherit the leading, it RESETS it to `normal` - the font's
 * own metric, which is fractional. So a shorthand has to carry it explicitly.
 *
 * The scale itself is rem, not px, because leading is on the rem side of the units law:
 * it has to grow when the reader enlarges the text, and a px line under a rem size
 * crowds and then clips at 200%.
 */
/** A stylesheet with its comments taken out - a comment is where a replaced rule is explained. */
const uncommented = (text: string): string => stripAll(text, BLOCK_COMMENT);

/**
 * A component's `<style>` block, and NOTHING else in the file.
 *
 * A component with no block has no rules, so it contributes nothing - not its whole
 * file, which is how `Button.svelte` reported its own contract as five declarations. It
 * is the one component that deliberately has no block at all, and the prose explaining
 * why mentions the very property this reads.
 */
function styleBlock(source: string): string {
  /* The markup comments go FIRST. `Button.svelte`'s contract says "No `<style>` block
     here, and there must never be one" - and searching the raw file finds that phrase,
     opens a block that does not exist, and reports the prose after it as CSS. */
  const text = stripAll(source, HTML_COMMENT);
  const opened = text.indexOf('<style>');
  if (opened === -1) return '';

  return uncommented(text.slice(opened + '<style>'.length, text.lastIndexOf('</style>')));
}

const source = readdirSync(new URL('../src/lib/components', import.meta.url))
  .filter((file) => file.endsWith('.svelte'))
  .map((file) => ({
    name: file,
    text: styleBlock(
      readFileSync(new URL(`../src/lib/components/${file}`, import.meta.url), 'utf8'),
    ),
  }))
  .concat({ name: 'app.css', text: uncommented(stylesheets) });

/** Every `line-height:` declaration, with the file it is in. */
const declarations = source.flatMap(({ name, text }) =>
  [...text.matchAll(/line-height:\s*(?<value>[^;]+);/gu)].map((match) => ({
    file: name,
    value: (match.groups?.value ?? '').trim(),
  })),
);

/** Every `font:` shorthand, which carries its leading after the slash or not at all. */
const shorthands = source.flatMap(({ name, text }) =>
  [...text.matchAll(/font:\s*(?<value>[^;]+);/gu)].map((match) => ({
    file: name,
    value: (match.groups?.value ?? '').trim(),
  })),
);

describe('the leading scale [Unit]', () => {
  it('has declarations to check, and shorthands', () => {
    // A guard that stands down when its pattern stops matching is not a guard.
    expect(declarations.length).toBeGreaterThan(60);
    expect(shorthands.length).toBeGreaterThan(40);
  });

  it('names a tier rather than a number', () => {
    const literal = declarations.filter(({ value }) => !value.includes('var(--leading-'));
    expect(
      literal.map(({ file, value }) => `${file}: line-height: ${value}`),
      'a leading is a decision the scale has already taken',
    ).toEqual([]);
  });

  it('carries the leading in every font shorthand', () => {
    /* `font: <size> <family>` is not "leave the leading alone" - it is `line-height:
       normal`, which is whatever the face happens to say and is fractional in all four
       of ours.

       `font: inherit` is the one form that means what it says. It is how a form control
       is told to stop using the browser's own face, and it takes the leading with it,
       which is the whole point of writing it. */
    const bare = shorthands.filter(
      ({ value }) => !value.includes('/') && value.trim() !== 'inherit',
    );
    expect(
      bare.map(({ file, value }) => `${file}: font: ${value}`),
      'a shorthand without a leading resets it to the font’s own fractional metric',
    ).toEqual([]);
  });

  it('names a tier in every font shorthand too', () => {
    const literal = shorthands.filter(
      ({ value }) => value.includes('/') && !value.includes('var(--leading-'),
    );
    expect(literal.map(({ file, value }) => `${file}: font: ${value}`)).toEqual([]);
  });

  it('lands every tier on a whole pixel', () => {
    /* At the 16px root the browser gives us. `--leading-flat` is the exception and is
       not a length at all: it is the glyph box a count, a mark or an icon sits in, and
       a ratio of exactly 1 is whole against any size. */
    const scale = [...stylesheets.matchAll(/--leading-(?<tier>[\w-]+):\s*(?<value>[^;/]+)/gu)].map(
      (match) => ({
        tier: match.groups?.tier ?? '',
        value: (match.groups?.value ?? '').trim(),
      }),
    );
    expect(scale.length).toBeGreaterThan(6);

    const fractional = scale.filter(({ tier, value }) => {
      if (tier === 'flat') return value !== '1';
      const rem = /^(?<size>[\d.]+)rem$/u.exec(value)?.groups?.size;
      if (rem === undefined) return true;

      return !Number.isInteger(Number(rem) * 16);
    });
    expect(
      fractional.map(({ tier, value }) => `--leading-${tier}: ${value}`),
      'every tier is a rem multiple that is whole at a 16px root',
    ).toEqual([]);
  });
});
