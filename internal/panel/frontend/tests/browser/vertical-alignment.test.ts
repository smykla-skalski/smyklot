import { writeFileSync } from 'node:fs';

import type { Page } from 'playwright-core';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';

import { addressOf, inLanes, PANEL_ROUTES, startPanel, visit, type Panel } from './harness';

/**
 * Where a row says it centres its contents, the contents are centred by eye.
 *
 * `align-items: center` centres BOXES. What a reader sees centred is the typographic band - the
 * cap line down to the baseline - and a text box is not that band: it carries the leading above
 * the capitals and the descender space below, and the two are not equal. So a row holding a 34px
 * control beside a 28px title can be perfectly centred by the engine and visibly off by a pixel
 * and a half, which is what the page headers were correcting by hand with a measured
 * `translateY(round(0.0382em, 1px))`.
 *
 * This finds those rows rather than listing them. Every element that *declares* a centre is a
 * promise, and each one is checked: the band of each child, text measured cap-to-baseline and
 * anything painted measured as its box, against the band of its siblings.
 *
 * Both edges come from the engine. The cap height is read from a probe carrying
 * `text-box: trim-both cap alphabetic`, and the baseline from a zero-size inline-block, which sits
 * its bottom edge on it - no font-metric arithmetic anywhere. The band and not the glyph outline,
 * because an outline moves the moment a repository name contains a `g`, and rows that shift with
 * their data are not aligned, they are coincident.
 */

interface Row {
  route: string;
  where: string;
  spread: number;
  parts: { label: string; centre: number; kind: string }[];
}

/* Every route. The sync page was held out of this list while the question its rows asked was open
   - a row with a hairline under it reported itself off-centre against its own box - and that is
   answered where `separator` is defined below: the rule is the seam between two rows and not the
   underside of either, which is the same argument this sweep already made for a table cell. */
const ROUTES = PANEL_ROUTES;

/**
 * A quarter of a pixel: under one device row at 2x and at 3x alike, so nothing this permits can be
 * drawn as a difference.
 *
 * Set from what the product actually measures rather than from what would be comfortable. With
 * everything below fixed the worst row left in eleven routes is 0.16px, the bulk are 0.09 and
 * 0.06, and those are the trim's own rounding rather than anything laid out wrongly. Half a pixel
 * would have been the obvious number and would have hidden a whole tier: every button's label sat
 * 0.47px above its icon, which is a device row at 2x and visible in an overlay.
 */
const TOLERANCE = 0.25;

interface Field {
  route: string;
  where: string;
  top: number;
  bottom: number;
}

async function rowsOn(page: Page): Promise<{
  examined: number;
  found: Omit<Row, 'route'>[];
  fields: Omit<Field, 'route'>[];
}> {
  return page.evaluate(() => {
    const capCache = new Map<string, number>();

    /** Band height, straight from the engine's own trim - never from nominal font metrics. */
    function capHeight(style: CSSStyleDeclaration): number {
      const key = `${style.fontStyle} ${style.fontWeight} ${style.fontSize} ${style.fontFamily}`;
      const known = capCache.get(key);
      if (known !== undefined) return known;
      const probe = document.createElement('span');
      probe.textContent = 'H';
      probe.style.cssText =
        'position:absolute;left:-9999px;top:0;display:inline-block;line-height:1;' +
        `text-box:trim-both cap alphabetic;font-style:${style.fontStyle};` +
        `font-weight:${style.fontWeight};font-size:${style.fontSize};font-family:${style.fontFamily}`;
      document.body.append(probe);
      const height = probe.getBoundingClientRect().height;
      probe.remove();
      capCache.set(key, height);
      return height;
    }

    /** The y of a baseline: a zero-size inline-block sits its bottom edge on it. */
    function baselineOf(element: Element, where: 'first' | 'last'): number {
      const strut = document.createElement('span');
      strut.style.cssText = 'display:inline-block;width:0;height:0;vertical-align:baseline';
      if (where === 'first') element.prepend(strut);
      else element.append(strut);
      const y = strut.getBoundingClientRect().top;
      strut.remove();
      return y;
    }

    /** Whether anything between this run and the row clips it out of sight. */
    function hidden(from: Element, root: Element): boolean {
      for (let at: Element | null = from; at !== null && at !== root; at = at.parentElement) {
        const style = getComputedStyle(at);
        // `inset(50%)` clips to nothing: the `.visually-hidden` idiom, and the only clip in the
        // product. Named exactly rather than "any clip-path", so a decorative one later is still
        // measured instead of being silently dropped.
        if (style.clipPath === 'inset(50%)' || style.opacity === '0') return true;
        const rect = at.getBoundingClientRect();
        if (rect.width <= 1 || rect.height <= 1) return true;
      }

      return false;
    }

    /**
     * The typographic band of everything written inside an element: the cap line of its highest
     * line down to the baseline of its lowest.
     *
     * Each run of text is wrapped for the measurement rather than measured through its container,
     * because the strut only sits on a baseline inside an inline formatting context. Dropped into
     * a flex or grid container it becomes an item of that container instead, and gets centred - so
     * a monogram disc reported its own middle as its baseline and read four pixels out.
     */
    function bandOf(root: Element): { top: number; bottom: number } | null {
      const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
      const runs: Text[] = [];
      while (walker.nextNode() !== null) {
        const node = walker.currentNode as Text;
        if ((node.nodeValue ?? '').trim() !== '') runs.push(node);
      }

      let top = Number.POSITIVE_INFINITY;
      let bottom = Number.NEGATIVE_INFINITY;
      for (const run of runs) {
        const parent = run.parentNode;
        if (parent === null) continue;
        const wrapper = document.createElement('span');
        parent.insertBefore(wrapper, run);
        wrapper.append(run);
        /* Words nobody sees are not part of the band. `.visually-hidden` keeps its label in
           layout - full width, `white-space: nowrap` - and hides it by clipping to a single pixel,
           so the run's own rect says nothing. The clip is on an ancestor, which is where to look.
           Every search field in the product carries one, which is how the toolbar rows read eleven
           pixels out when they were exactly right. */
        if (!hidden(wrapper, root)) {
          const cap = capHeight(getComputedStyle(wrapper));
          top = Math.min(top, baselineOf(wrapper, 'first') - cap);
          bottom = Math.max(bottom, baselineOf(wrapper, 'last'));
        }
        parent.insertBefore(run, wrapper);
        wrapper.remove();
      }

      return top === Number.POSITIVE_INFINITY ? null : { top, bottom };
    }

    /** The alpha a colour paints at, so `rgba(0, 0, 0, 0)` is read as the nothing it is. */
    const alphaOf = (color: string): number => {
      const inside = /^rgba?\((?<channels>[^)]+)\)$/u.exec(color)?.groups?.channels;
      if (inside === undefined) return color === 'transparent' ? 0 : 1;
      const parts = inside.split(/[\s,/]+/u).filter(Boolean);
      return parts.length > 3 ? Number(parts[3]) : 1;
    };

    /**
     * Whether this is a row with a rule against it rather than a surface with a word on it.
     *
     * A surface is enclosed - a ground, a shadow, or a border the whole way round. A separator is
     * a line on ONE edge and nothing else, which makes the box it draws asymmetric about its own
     * content by construction: the rule is the seam between this row and the next, and it is not
     * the underside of anything.
     */
    const separator = (element: Element, style: CSSStyleDeclaration): boolean => {
      if (alphaOf(style.backgroundColor) > 0) return false;
      if (style.backgroundImage !== 'none' || style.boxShadow !== 'none') return false;

      const edges = (['Top', 'Right', 'Bottom', 'Left'] as const).filter(
        (edge) =>
          style[`border${edge}Style`] !== 'none' &&
          Number.parseFloat(style[`border${edge}Width`]) > 0,
      );

      return edges.length === 1 && (edges[0] === 'Top' || edges[0] === 'Bottom');
    };

    /** Whether what the eye sees of this child is a surface rather than the words in it. */
    const painted = (element: Element, style: CSSStyleDeclaration): boolean =>
      element instanceof SVGElement ||
      element.tagName === 'IMG' ||
      element.tagName === 'INPUT' ||
      alphaOf(style.backgroundColor) > 0 ||
      style.backgroundImage !== 'none' ||
      style.boxShadow !== 'none' ||
      (style.borderTopStyle !== 'none' && Number.parseFloat(style.borderTopWidth) > 0);

    /**
     * Whether every painted thing inside this element sits within the band its
     * own words draw.
     *
     * Half a pixel of tolerance, because a keyline drawn on the band's own edge
     * is the band rather than something standing outside it.
     */
    const encloses = (element: Element, band: { top: number; bottom: number }): boolean => {
      for (const inner of element.querySelectorAll('*')) {
        const style = getComputedStyle(inner);
        if (!painted(inner, style)) continue;
        if (style.display === 'none' || style.visibility === 'hidden') continue;
        const rect = inner.getBoundingClientRect();
        if (rect.height === 0 || rect.width === 0) continue;
        if (rect.top < band.top - 0.5 || rect.bottom > band.bottom + 0.5) return false;
      }

      return true;
    };

    const label = (element: Element): string => {
      const classes = [...element.classList]
        .filter((one) => !one.startsWith('svelte-'))
        .slice(0, 2)
        .join('.');
      return `${element.tagName.toLowerCase()}${classes === '' ? '' : `.${classes}`}`;
    };

    const path = (element: Element): string => {
      const trail: string[] = [];
      for (
        let at: Element | null = element;
        at !== null && trail.length < 4;
        at = at.parentElement
      ) {
        trail.unshift(label(at));
        if (at.id !== '') break;
      }
      return trail.join(' > ');
    };

    const found: {
      where: string;
      spread: number;
      parts: { label: string; centre: number; kind: string }[];
    }[] = [];
    /* Counted separately from `found`, because a route where everything lines up contributes
       nothing to it - and a route that silently failed to load looks exactly the same. */
    let examined = 0;

    for (const holder of document.querySelectorAll<HTMLElement>('*')) {
      const style = getComputedStyle(holder);
      if (!/^(?:inline-)?(?:flex|grid)$/u.test(style.display)) continue;
      if (style.alignItems !== 'center') continue;
      if (style.flexWrap === 'wrap') continue;

      const box = holder.getBoundingClientRect();
      if (box.height === 0 || box.width === 0) continue;

      const parts: { label: string; centre: number; kind: string; top: number; bottom: number }[] =
        [];
      for (const child of holder.children) {
        const childStyle = getComputedStyle(child);
        if (childStyle.position === 'absolute' || childStyle.position === 'fixed') continue;
        if (childStyle.display === 'none' || childStyle.visibility === 'hidden') continue;
        if (childStyle.alignSelf === 'stretch' || childStyle.alignSelf === 'baseline') continue;
        const rect = child.getBoundingClientRect();
        if (rect.height === 0 || rect.width === 0) continue;

        /* A child is measured by its words unless what the eye sees of it is a
           surface. `painted` asks that of the child itself, and there is a
           third case it does not reach: a child that paints nothing but HOLDS
           something painted which sizes one of its lines. A file row's name
           sits on a line with chips, so the line is the chip's 20px and the
           name's cap sits 5px below the top of it - the block's text band is
           then not the block, and comparing it against the mark beside it
           reported 2.80px on a row whose boxes are centred to a hundredth of a
           pixel. Same shape in a plan group's counts. What a reader lines up
           there is the box, because the painted thing is what they see. */
        const written = painted(child, childStyle) ? null : bandOf(child);
        const band = written !== null && encloses(child, written) ? written : null;
        if (band === null) {
          parts.push({
            label: label(child),
            kind: 'box',
            centre: rect.y + rect.height / 2,
            top: rect.y,
            bottom: rect.y + rect.height,
          });
          continue;
        }
        parts.push({
          label: label(child),
          kind: 'band',
          centre: (band.top + band.bottom) / 2,
          top: band.top,
          bottom: band.bottom,
        });
      }

      if (parts.length < 2) continue;
      // Wrapped onto more than one visual line: the row is not one row, and its children were
      // never asked to share a centre.
      const overlapping = parts.every(
        (part) => part.top < Math.min(...parts.map((other) => other.bottom)) + 0.5,
      );
      if (!overlapping) continue;

      examined += 1;
      const centres = parts.map((part) => part.centre);
      const spread = Math.max(...centres) - Math.min(...centres);
      if (spread > 0.01) {
        found.push({
          where: path(holder),
          spread,
          parts: parts.map(({ label: name, centre, kind }) => ({ label: name, centre, kind })),
        });
      }
    }

    /**
     * Table rows, which the sweep above cannot see.
     *
     * A cell centres with `vertical-align: middle`, not with `align-items`, so no `<td>` in the
     * product declares a centre in a way that sweep recognises - and every table body was
     * therefore going unmeasured. `vertical-align: middle` centres the cell's content BOX, so it
     * carries the same fault as everything else here, and more visibly: a cell holding one line
     * sits against a cell holding two, and the two-line block's leading and descender space put
     * its words off the centre its neighbour is on.
     *
     * What is compared is what a reader compares - each cell's ink against the ink beside it: the
     * band of the words in it, and the box of anything painted, taken together.
     */
    for (const row of document.querySelectorAll<HTMLTableRowElement>('tr')) {
      const cells: { label: string; centre: number; kind: string; top: number; bottom: number }[] =
        [];
      for (const cell of row.cells) {
        const cellStyle = getComputedStyle(cell);
        if (cellStyle.display === 'none' || cell.getBoundingClientRect().height === 0) continue;

        let top = Number.POSITIVE_INFINITY;
        let bottom = Number.NEGATIVE_INFINITY;
        /* Painted descendants are taken at their outermost: a chip's own box is what the eye
           sees, and the words inside it are already centred in it by the chip's own padding. */
        for (const mark of cell.querySelectorAll('*')) {
          const markStyle = getComputedStyle(mark);
          if (markStyle.position === 'absolute' || markStyle.position === 'fixed') continue;
          if (!painted(mark, markStyle)) continue;
          if (mark.parentElement?.closest('[data-painted]') != null) continue;
          const rect = mark.getBoundingClientRect();
          if (rect.height === 0 || rect.width === 0) continue;
          if (hidden(mark, cell)) continue;
          mark.setAttribute('data-painted', '');
          top = Math.min(top, rect.y);
          bottom = Math.max(bottom, rect.y + rect.height);
        }
        for (const mark of cell.querySelectorAll('[data-painted]'))
          mark.removeAttribute('data-painted');

        const band = bandOf(cell);
        if (band !== null) {
          top = Math.min(top, band.top);
          bottom = Math.max(bottom, band.bottom);
        }
        if (top === Number.POSITIVE_INFINITY) continue;
        cells.push({
          label: label(cell),
          kind: 'cell',
          centre: (top + bottom) / 2,
          top,
          bottom,
        });
      }

      if (cells.length < 2) continue;
      examined += 1;
      const centres = cells.map((cell) => cell.centre);
      const spread = Math.max(...centres) - Math.min(...centres);
      if (spread > 0.01) {
        found.push({
          where: path(row),
          spread,
          parts: cells.map(({ label: name, centre, kind }) => ({ label: name, centre, kind })),
        });
      }
    }

    /**
     * A control centres its own words in its own box.
     *
     * The sweeps above ask whether things sit level with each OTHER. This asks the question a
     * control has to answer by itself: a button, a chip, a pill, a menu row is a surface with a
     * word on it, and the word belongs on the middle of that surface. Nothing else in the row can
     * make up for it, because the surface is what the reader sees the edges of.
     *
     * It is the same fault in a smaller place. A button centres its label BOX, and the box carries
     * the leading above the capitals and the descender space below - so the words in every button
     * with an icon sat 0.47px above the icon beside them.
     *
     * Measured to the border box, since the border is part of the surface, and only for the
     * innermost painted thing around a run of words: a card holding a button holds the button's
     * words too, and it is the button that has to centre them.
     */
    for (const control of document.querySelectorAll('*')) {
      const style = getComputedStyle(control);
      if (!painted(control, style)) continue;
      if (style.position === 'absolute' || style.position === 'fixed') continue;
      // Only where the padding is the whole story. A control that is taller than its content by
      // design - a text area, a card - is a surface with a layout inside it, not a label on a
      // ground, and the sweeps above already ask about that layout.
      if (control.tagName === 'TEXTAREA' || control.tagName === 'INPUT') continue;
      /* A row separated by a rule is not a control.
         ----------------------------------------
         A surface is a thing with edges the reader can see all the way round, and a word belongs
         on the middle of it. A rule under a row is one edge and it belongs to the SEAM between two
         rows, not to either of them - so the box it makes is a half-pixel taller below the words
         by design, and asking that box to centre them is asking the wrong box.

         Table cells were named here for exactly this, and naming them was the narrow version of
         the rule: the sync page's plan rows are the same anatomy drawn without a table, and they
         reported 0.53px on every row and 4.93px on the last one for having a hairline under them.
         So the test is what the element IS rather than what it is called - a border on one edge
         and nothing else painted is a separator, whoever drew it.

         The sweeps above still ask the question that matters about such a row, which is where its
         ink sits against the ink beside it.

         Table structure stays named as well as tested. A heading cell carries a ground of its
         own, and an empty-state row may carry one too, so the shape test alone reads either as a
         surface. Neither is a label control: rows belong to the table sweep above and cells are
         layout within them, whether or not somebody tinted either one. */
      if (control.tagName === 'TR' || control.tagName === 'TH' || control.tagName === 'TD')
        continue;
      if (separator(control, style)) continue;
      if (
        [...control.querySelectorAll('*')].some((inner) => painted(inner, getComputedStyle(inner)))
      )
        continue;
      /* A grid surface with several children is a card with a layout, not a label painted on a
         control. Its rows were already measured by the centre sweep above. Buttons and pills use
         inline-flex, so this keeps testing every actual control while avoiding a second, invalid
         question about the card's outer padding. */
      if (style.display === 'grid' && control.children.length > 1) continue;

      const rect = control.getBoundingClientRect();
      if (rect.height === 0 || rect.width === 0) continue;
      if (hidden(control, document.body)) continue;
      const band = bandOf(control);
      if (band === null) continue;
      // Wrapped to more lines than the surface was drawn for: its own layout, not its padding.
      if (band.bottom - band.top > rect.height) continue;

      examined += 1;
      const spread = Math.abs((band.top + band.bottom) / 2 - (rect.y + rect.height / 2));
      if (spread > 0.01) {
        found.push({
          where: path(control),
          spread,
          parts: [
            { label: 'words', centre: (band.top + band.bottom) / 2, kind: 'band' },
            { label: 'surface', centre: rect.y + rect.height / 2, kind: 'box' },
          ],
        });
      }
    }

    /**
     * A field is symmetric about the line it holds.
     *
     * An `<input>` keeps its value outside the DOM, so there is no text node to measure and the
     * band trick above cannot reach it. What CAN be said is the thing that decides where the line
     * lands: the engine centres a single-line field's inner editor in the content box, so the text
     * is centred exactly when the room above it equals the room below. Padding and border, both -
     * a border is drawn on the outside of the content and moves the content box with it.
     */
    const fields: { where: string; top: number; bottom: number }[] = [];
    for (const field of document.querySelectorAll('input, select, textarea')) {
      const style = getComputedStyle(field);
      if (style.display === 'none' || field.getBoundingClientRect().height === 0) continue;
      if (field instanceof HTMLInputElement && ['checkbox', 'radio'].includes(field.type)) continue;
      const top = Number.parseFloat(style.paddingTop) + Number.parseFloat(style.borderTopWidth);
      const bottom =
        Number.parseFloat(style.paddingBottom) + Number.parseFloat(style.borderBottomWidth);
      examined += 1;
      if (Math.abs(top - bottom) > 0.01) fields.push({ where: path(field), top, bottom });
    }

    return { examined, found, fields };
  });
}

let panel: Panel;
let rows: Row[];
let fields: Field[];
let examined: Record<string, number>;

beforeAll(async () => {
  panel = await startPanel();
  rows = [];
  fields = [];
  examined = {};

  /* A page each, and several at once. One page walked in sequence was the arrangement, and it made
     this file wait fourteen times for a route to load. Nothing read below is a timing: a cap height
     comes from the engine's own trim and a padding from the computed style, and both answer the
     same on a busy machine as on an idle one. */
  const readings = await inLanes(ROUTES, async (route) => {
    const address = addressOf(panel, route);
    const page = await panel.browser.newPage();
    try {
      await visit(page, address);

      return { route, ...(await rowsOn(page)) };
    } finally {
      await page.close();
    }
  });

  for (const reading of readings) {
    examined[reading.route] = reading.examined;
    rows.push(...reading.found.map((row) => ({ ...row, route: reading.route })));
    fields.push(...reading.fields.map((field) => ({ ...field, route: reading.route })));
  }
}, 300_000);

afterAll(async () => {
  await panel?.close();
});

describe('the rows that declare a centre [Integration]', () => {
  it('found rows to measure on every route', () => {
    // A route that failed to load is otherwise indistinguishable from one where everything is
    // aligned, and this whole file would pass by measuring nothing.
    expect(
      Object.fromEntries(ROUTES.map((route) => [route, (examined[route] ?? 0) >= 5])),
      `rows examined per route: ${JSON.stringify(examined)}`,
    ).toEqual(Object.fromEntries(ROUTES.map((route) => [route, true])));
  });

  it('centres what they hold, by eye', () => {
    const off = rows
      .filter((row) => row.spread > TOLERANCE)
      .sort((first, second) => second.spread - first.spread);

    writeFileSync(
      '/tmp/vertical-alignment.txt',
      off
        .map(
          (row) =>
            `${row.spread.toFixed(2)}px  ${row.route}  ${row.where}\n` +
            row.parts
              .map((part) => `        ${part.centre.toFixed(2)}  ${part.kind}  ${part.label}`)
              .join('\n'),
        )
        .join('\n'),
    );

    const summary = off
      .slice(0, 20)
      .map((row) => `  ${row.spread.toFixed(2)}px  ${row.route}  ${row.where}`)
      .join('\n');

    expect(
      off.map((row) => `${row.route} ${row.where}`),
      `${off.length} rows are off:\n${summary}`,
    ).toEqual([]);
  });

  it('keeps every field symmetric about the line it holds', () => {
    const uneven = [
      ...new Set(fields.map((field) => `${field.where} ${field.top}/${field.bottom}`)),
    ];
    expect(
      uneven,
      `fields with unequal room above and below the text (padding + border, in px):\n  ${uneven.join('\n  ')}`,
    ).toEqual([]);
  });
});
