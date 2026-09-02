import { readFileSync } from 'node:fs';

import type { Browser } from 'playwright-core';
import { chromium } from 'playwright-core';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';

import { HTML_COMMENT, stripAll } from '../support/markup';

/**
 * Every glyph's ink is centred in its own 24-unit box.
 *
 * A symbol that sits half a unit high of its box rides half a unit high of every word beside it, in
 * every button, chip, menu row and table cell that draws it - and nothing corrects for it, because
 * `INK_BEARING` is horizontal only. `repositories` and `refresh` already carry comments saying they
 * were redrawn for exactly this reason; this is that rule, asked of the whole set.
 *
 * The shapes are read out of the component and handed to a real SVG engine, which answers with
 * `getBBox()` - geometry, not stroke, since the stroke is symmetric about the outline and is what
 * `INK_BEARING` subtracts separately.
 *
 * The reading has to follow the file's one nested branch: the `file-*` family is a shared document
 * outline with a mark drawn inside it. A first attempt split on every `if name ===` and measured
 * those four marks without the outline they sit in, reporting all four two units low. So the parse
 * is checked against the component's own `IconName` union, which is the list of glyphs that exist -
 * a name that goes missing fails here rather than going unmeasured.
 */

const source = readFileSync(
  new URL('../../src/lib/components/Icon.svelte', import.meta.url),
  'utf8',
);

/** The glyphs the component says it has. */
function declared(): string[] {
  const union = source.slice(source.indexOf('export type IconName'), source.indexOf(';', 20));
  const names = [...union.matchAll(/'(?<name>[a-z-]+)'/gu)].map(
    (match) => match.groups?.name ?? '',
  );
  if (names.length < 40) throw new Error(`IconName parsed to only ${names.length} names`);

  return names;
}

/** Each glyph's shapes, with a nested branch read as a refinement of the one that holds it. */
function glyphs(): Map<string, string> {
  const open = source.indexOf('\n<svg');
  const close = source.indexOf('\n</svg>');
  if (open === -1 || close === -1) throw new Error('Icon.svelte no longer has one <svg> block');
  const markup = source.slice(source.indexOf('>', source.indexOf('aria-hidden')) + 1, close);

  const clean = (text: string): string => stripAll(text, HTML_COMMENT).trim();
  const naming = (condition: string): string[] =>
    [...condition.matchAll(/name === '(?<name>[a-z-]+)'/gu)].map(
      (match) => match.groups?.name ?? '',
    );

  /* Top-level branches only: a `{#if}` opened inside one is skipped over, then read again below as
     that branch's own refinements. */
  const token = /\{(?<kind>[#:/])(?:else )?if?(?<condition>[^}]*)\}/gu;
  const found = new Map<string, string>();
  const outer: { condition: string; from: number; to: number }[] = [];
  let depth = 0;
  let pending: { condition: string; from: number } | undefined;

  for (const mark of markup.matchAll(token)) {
    const at = mark.index ?? 0;
    const kind = mark.groups?.kind ?? '';
    if (kind === '/') {
      depth -= 1;
      if (depth === 0 && pending !== undefined) {
        outer.push({ ...pending, to: at });
        pending = undefined;
      }
      continue;
    }
    if (kind === '#') depth += 1;
    if (depth !== 1) continue;
    if (pending !== undefined) outer.push({ ...pending, to: at });
    pending = { condition: mark.groups?.condition ?? '', from: at + mark[0].length };
  }
  if (outer.length < 30) throw new Error(`only ${outer.length} top-level branches matched`);

  for (const branch of outer) {
    const body = markup.slice(branch.from, branch.to);
    const inner: { name: string; shapes: string }[] = [];
    const nested = /\{[#:](?:else )?if(?<condition>[^}]*)\}/gu;
    const marks = [...body.matchAll(nested)];
    const end = body.indexOf('{/if}');
    for (const [index, mark] of marks.entries()) {
      inner.push({
        name: naming(mark.groups?.condition ?? '')[0] ?? '',
        shapes: clean(
          body.slice((mark.index ?? 0) + mark[0].length, marks[index + 1]?.index ?? end),
        ),
      });
    }
    // What every refinement shares: the branch with its nested block cut out.
    const common = clean(marks.length === 0 ? body : body.slice(0, marks[0]?.index));
    for (const name of naming(branch.condition)) found.set(name, common);
    for (const refinement of inner) found.set(refinement.name, `${common}${refinement.shapes}`);
  }

  return found;
}

const names = declared();
const icons = glyphs();

interface Box {
  x: number;
  y: number;
  width: number;
  height: number;
}

let browser: Browser;
let boxes: Map<string, Box>;

beforeAll(async () => {
  browser = await chromium.launch({ channel: 'chrome' });
  const page = await browser.newPage();
  try {
    await page.setContent(
      `<body>${[...icons]
        .map(
          ([name, shapes]) =>
            `<svg data-icon="${name}" viewBox="0 0 24 24" width="240" height="240" fill="none"` +
            ` stroke="#000" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">` +
            `${shapes}</svg>`,
        )
        .join('')}</body>`,
    );
    boxes = new Map(
      await page.evaluate(() =>
        [...document.querySelectorAll('svg[data-icon]')].map((svg) => {
          const box = (svg as SVGSVGElement).getBBox();
          return [
            svg.getAttribute('data-icon') ?? '',
            { x: box.x, y: box.y, width: box.width, height: box.height },
          ] as [string, Box];
        }),
      ),
    );
  } finally {
    await page.close();
  }
}, 120_000);

afterAll(async () => {
  await browser?.close();
});

describe('the icon set [Integration]', () => {
  it('measures every glyph the component declares', () => {
    expect([...names].filter((name) => !boxes.has(name))).toEqual([]);
    expect([...boxes].filter(([, box]) => box.width === 0 || box.height === 0)).toEqual([]);
  });

  /* A quarter of a unit is 0.15px at the 14px icons in a chip and 0.19px at the 18px ones in a
     button - under the half-pixel a display can show, and comfortably inside the rounding of any
     overlay comparison. Past it is a glyph riding visibly high or low of its label. */
  const TOLERANCE = 0.25;

  it('centres every glyph vertically in its own box', () => {
    const off = [...boxes]
      .map(([name, box]) => [name, box.y + box.height / 2 - 12] as const)
      .filter(([, offset]) => Math.abs(offset) > TOLERANCE)
      .map(
        ([name, offset]) =>
          `${name} sits ${Math.abs(offset).toFixed(3)} units ${offset < 0 ? 'high' : 'low'}`,
      );

    expect(off, `glyphs off their box's vertical centre:\n  ${off.join('\n  ')}`).toEqual([]);
  });

  /* The horizontal half of the same measurement. `INK_BEARING` is what a button subtracts so its
     symbol starts where its word would, and its comment says it was read off the geometry - but it
     is a hand-kept list beside the paths it describes, which is the arrangement that goes stale.
     Redrawing a glyph's width without touching the list is silent otherwise. */
  it('states each glyph horizontal bearing as drawn', () => {
    const declaredBearing = new Map(
      [
        ...source
          .slice(
            source.indexOf('INK_BEARING'),
            source.indexOf('\n  };', source.indexOf('INK_BEARING')),
          )
          .matchAll(/'?(?<name>[a-z-]+)'?:\s*(?:\[(?<pair>[^\]]+)\]|(?<one>[\d.]+))/gu),
      ].map((match) => {
        const raw = match.groups?.pair ?? match.groups?.one ?? '';
        const [start, end] = raw.split(',').map((part) => Number(part.trim()));
        return [match.groups?.name ?? '', [start ?? 0, end ?? start ?? 0]] as const;
      }),
    );

    const wrong = [...boxes]
      .map(([name, box]) => {
        const stated = declaredBearing.get(name);
        if (stated === undefined) return `${name} has no INK_BEARING entry`;
        const measured = [box.x, 24 - (box.x + box.width)];
        return measured.every((edge, side) => Math.abs(edge - (stated[side] ?? 0)) <= 0.02)
          ? ''
          : `${name} states [${stated.join(', ')}] but is drawn at ` +
              `[${measured.map((edge) => edge.toFixed(2)).join(', ')}]`;
      })
      .filter(Boolean);

    expect(wrong, `INK_BEARING no longer matches the paths:\n  ${wrong.join('\n  ')}`).toEqual([]);
  });
});
