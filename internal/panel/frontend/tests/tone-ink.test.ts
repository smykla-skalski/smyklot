import { readdirSync, readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

import { appStylesheet, tokensStylesheet } from './theme';

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

const sheets: [string, string][] = [
  ['app.css', appStylesheet],
  ['tokens.css', tokensStylesheet],
];
/* Every component, wherever it lives. Reading one directory left `src/routes/**` uncovered, and a
   route page carries a `<style>` block like any other component. */
const source = new URL('../src/', import.meta.url);

/** Every `.svelte` file under `src`, as `[label, css]` pairs. */
function componentStyles(): [string, string][] {
  return readdirSync(source, { recursive: true, withFileTypes: true })
    .filter((entry) => entry.isFile() && entry.name.endsWith('.svelte'))
    .map((entry) => {
      const path = `${entry.parentPath}/${entry.name}`;
      return [entry.name, styleOf(readFileSync(path, 'utf8'))] as [string, string];
    });
}

/**
 * Colours that are not the palette's to decide, each with the reason it is not.
 *
 * Keyed by the file and the literal, so moving a brand asset's own colour into another rule still
 * has to come past here.
 */
const NOT_THEME_COLOUR: Record<string, string> = {
  'BrandMark.svelte #09152b':
    'the logo artwork is drawn on its own ground, which the mark carries into any palette',
  'Rail.svelte #fff':
    'the selected workspace mark holds its ground at L 50 whatever the hue, so white letters ' +
    'clear it in every palette - the identity paint is not the theme to decide',
  'ApplyBar.svelte #000':
    "the apply bar's end-melt is a mask-image ramp: #000 there is the alpha channel's " +
    '"fully kept", not a colour any palette owns',
  /* The colour picker draws COLOUR SPACE, not theme: the saturation/value
     area's white-to-hue and transparent-to-black gradients, the hue rail's
     spectrum, and the knobs' white rings are physics in every palette. */
  'LabelColorPicker.svelte #000000': 'the value axis of the saturation/value area',
  'LabelColorPicker.svelte #ffffff': 'the saturation axis, and the knob rings that ride any hue',
  'LabelColorPicker.svelte #ff0000': 'the hue rail starts and ends on red',
  'LabelColorPicker.svelte #ffff00': "the hue rail's spectrum",
  'LabelColorPicker.svelte #00ff00': "the hue rail's spectrum",
  'LabelColorPicker.svelte #00ffff': "the hue rail's spectrum",
  'LabelColorPicker.svelte #0000ff': "the hue rail's spectrum",
  'LabelColorPicker.svelte #ff00ff': "the hue rail's spectrum",
  'LabelColorPicker.svelte #1b1f24':
    "the check's dark ink on a pale swatch - decided by the swatch's own luminance, not the theme",
};

/**
 * Every literal colour written inside a rule body, ignoring custom-property declarations.
 *
 * `--x: #fff` is the palette and is where a literal belongs; `var(--x)` is a use of one. Telling
 * them apart needs the position, not the characters: both start `--`, and skipping to the next
 * semicolon on sight of one swallowed the rest of whatever declaration it appeared in. Everything
 * after a token in the same value went unread, which is precisely where the next literal will sit -
 * `color-mix(in srgb, var(--danger) 45%, #ffffff)` is the house idiom.
 *
 * So a `--` counts as a declaration only where a declaration can begin: after `{`, `}` or `;`, with
 * only whitespace and comments in between.
 */
function literals(source: string, label: string): string[] {
  const found: string[] = [];
  let depth = 0;
  let index = 0;
  let atDeclarationStart = true;

  while (index < source.length) {
    const character = source[index];

    if (character === '/' && source[index + 1] === '*') {
      const close = source.indexOf('*/', index);
      // An unterminated comment is the rest of the file, and reading on from -1 would restart it.
      if (close === -1) break;
      index = close + 2;
      continue;
    }

    if (character === '{' || character === '}' || character === ';') {
      if (character === '{') depth += 1;
      if (character === '}') depth -= 1;
      atDeclarationStart = true;
      index += 1;
      continue;
    }

    if (/\s/u.test(character ?? '')) {
      index += 1;
      continue;
    }

    if (depth > 0 && atDeclarationStart && character === '-' && source[index + 1] === '-') {
      const semicolon = source.indexOf(';', index);
      const brace = source.indexOf('{', index);
      if (semicolon !== -1 && (brace === -1 || semicolon < brace)) {
        index = semicolon + 1;
        continue;
      }
    }

    if (depth > 0 && character === '#') {
      const hex = /^#(?:[\da-f]{3,4}|[\da-f]{6}|[\da-f]{8})\b/iu.exec(source.slice(index));
      if (hex !== null) {
        found.push(`${label} ${hex[0].toLowerCase()}`);
        atDeclarationStart = false;
        index += hex[0].length;
        continue;
      }
    }

    atDeclarationStart = false;
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

/**
 * The scanner, checked against the shapes it is asked to read.
 *
 * A guard that silently reads nothing passes for the same reason a clean codebase does, and this
 * one did: on sight of `--` it skipped to the next semicolon, so every literal sitting after a
 * token in the same declaration went unseen. The cases below are the ones that got past it.
 */
describe('the colour scan [Unit]', () => {
  it.each([
    ['a plain declaration', '.a { color: #ffffff; }', ['x #ffffff']],
    [
      'a literal after a token',
      '.a { border-color: color-mix(in srgb, var(--d) 45%, #ffffff); }',
      ['x #ffffff'],
    ],
    [
      'a literal in a list after a token',
      '.a { box-shadow: 0 0 0 1px var(--r), 0 1px 2px #00000033; }',
      ['x #00000033'],
    ],
    [
      'a literal across lines after a token',
      '.a {\n  background: linear-gradient(\n    var(--a),\n    #ff0000\n  );\n}',
      ['x #ff0000'],
    ],
    [
      'two literals in one declaration',
      '.a { background: linear-gradient(#ff0000, #00ff00); }',
      ['x #ff0000', 'x #00ff00'],
    ],
  ])('reads %s', (_case, css, expected) => {
    expect(literals(css, 'x')).toEqual(expected);
  });

  it.each([
    ['a custom property, which is the palette', ':root { --brand: #ff0000; }'],
    ['one declared after a comment', ':root { /* the ground */ --brand: #ff0000; }'],
    ['one declared after another declaration', ':root { --a: #ff0000; --b: #00ff00; }'],
    ['a selector outside any rule body', '#app { }'],
    ['a colour inside a comment', '.a { /* was #ff0000 */ color: var(--x); }'],
  ])('passes over %s', (_case, css) => {
    expect(literals(css, 'x')).toEqual([]);
  });
});

describe('the palette [Unit]', () => {
  it('is the only place a colour is written down', () => {
    const found = [
      ...sheets.flatMap(([file, css]) => literals(css, file)),
      ...componentStyles().flatMap(([file, css]) => literals(css, file)),
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
      ...sheets.flatMap(([file, css]) => literals(css, file)),
      ...componentStyles().flatMap(([file, css]) => literals(css, file)),
    ]);

    expect(Object.keys(NOT_THEME_COLOUR).filter((one) => !found.has(one))).toEqual([]);
  });
});
