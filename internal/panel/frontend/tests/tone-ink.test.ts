import { readdirSync, readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

/**
 * A rule paints from the palette, never from a colour it names itself.
 *
 * This is the shape of the bug it exists for. `.btn-stop` filled with `--danger` and then wrote
 * `color: #ffffff`, which is right on the light palettes and 2.04:1 on both dark ones - a token
 * that is a deep red on one side of the theme and a pale pink on the other cannot have its ink
 * decided where it is used. Nothing pointed at it: the declaration was correct in the palette the
 * author was looking at, and the four palettes are why it was not correct in the product.
 *
 * The scan is deliberately blunt - every hex literal in every rule body, in `app.css` and in each
 * component's `<style>`. A theme-aware colour has a token; a colour that genuinely has nothing to
 * do with the theme is an exception worth stating out loud rather than a pattern worth allowing.
 *
 * Hex only, and not `rgb()`. The `rgb(0 0 0 / x%)` a mask or a shadow is written with is an alpha
 * ramp rather than a colour - the night sky alone draws forty of them - and a rule that flagged
 * those would be answered by an exception list longer than the thing it guards.
 */

const stylesheet = new URL('../src/app.css', import.meta.url);
const components = new URL('../src/lib/components/', import.meta.url);

/**
 * Colours that are not the palette's to decide, each with the reason it is not.
 *
 * Keyed by the file and the literal, so moving a brand asset's own colour into another rule still
 * has to come past here.
 */
const NOT_THEME_COLOUR: Record<string, string> = {
  'BrandMark.svelte #09152b':
    'the logo artwork is drawn on its own ground, which the mark carries into any palette',
};

/** Every literal colour written inside a rule body, ignoring custom-property declarations. */
function literals(source: string, label: string): string[] {
  const found: string[] = [];
  let depth = 0;
  let index = 0;

  while (index < source.length) {
    const character = source[index];
    if (character === '{') depth += 1;
    else if (character === '}') depth -= 1;
    else if (character === '/' && source[index + 1] === '*') {
      index = source.indexOf('*/', index) + 2;
      continue;
    } else if (depth > 0 && character === '-' && source[index + 1] === '-') {
      // A custom-property declaration: the palette itself, which is where literals belong.
      const semicolon = source.indexOf(';', index);
      const brace = source.indexOf('{', index);
      if (semicolon !== -1 && (brace === -1 || semicolon < brace)) {
        index = semicolon + 1;
        continue;
      }
    } else if (depth > 0 && character === '#') {
      const hex = /^#(?:[\da-f]{3,4}|[\da-f]{6}|[\da-f]{8})\b/iu.exec(source.slice(index));
      if (hex !== null) {
        found.push(`${label} ${hex[0].toLowerCase()}`);
        index += hex[0].length;
        continue;
      }
    }
    index += 1;
  }

  return found;
}

/** A component's `<style>` blocks, which is the only part of it CSS may live in. */
function styleOf(source: string): string {
  return [...source.matchAll(/<style[^>]*>(?<body>[\s\S]*?)<\/style>/gu)]
    .map((match) => match.groups?.body ?? '')
    .join('\n');
}

describe('the palette [Unit]', () => {
  it('is the only place a colour is written down', () => {
    const found = [
      ...literals(readFileSync(stylesheet, 'utf8'), 'app.css'),
      ...readdirSync(components)
        .filter((file) => file.endsWith('.svelte'))
        .flatMap((file) =>
          literals(styleOf(readFileSync(new URL(file, components), 'utf8')), file),
        ),
    ];

    const unexplained = found.filter((one) => NOT_THEME_COLOUR[one] === undefined);
    expect(
      unexplained,
      'these rules name a colour instead of reading one, so they answer for one palette ' +
        `of four:\n  ${unexplained.join('\n  ')}`,
    ).toEqual([]);
  });

  it('states a reason for each colour that is not the theme to decide', () => {
    // Guards the exception list against becoming the place colours are quietly parked: an entry
    // that no longer matches anything is removed rather than left standing.
    const found = new Set([
      ...literals(readFileSync(stylesheet, 'utf8'), 'app.css'),
      ...readdirSync(components)
        .filter((file) => file.endsWith('.svelte'))
        .flatMap((file) =>
          literals(styleOf(readFileSync(new URL(file, components), 'utf8')), file),
        ),
    ]);

    expect(Object.keys(NOT_THEME_COLOUR).filter((one) => !found.has(one))).toEqual([]);
  });
});
