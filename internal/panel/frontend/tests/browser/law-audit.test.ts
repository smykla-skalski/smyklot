import type { Page } from 'playwright-core';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';

import { addressOf, inLanes, PANEL_ROUTES, startPanel, visit, type Panel } from './harness';

/**
 * The sheet's universal laws, asked of every route rather than of the page they were
 * written on.
 *
 * Every law here says "every" or "wherever" in its own words, and every one of them was
 * once written as a list of the classes that happened to need it that day. That is the
 * failure this measures: a family gains a member, the member is not on the list, and the
 * law silently does not reach it. Two shipped that way this week - a card laid out by a
 * page's own grid never got the automatic-minimum rule, and a heading row built by hand
 * never got the companion rule - and neither was visible in anything but a screenshot of
 * the one page nobody had opened at that width.
 */

interface Finding {
  route: string;
  law: string;
  where: string;
  detail: string;
}

let panel: Panel;
const findings: Finding[] = [];

beforeAll(async () => {
  panel = await startPanel();
  const swept = await inLanes(PANEL_ROUTES, async (route) => {
    const page = await panel.browser.newPage();
    try {
      await visit(page, addressOf(panel, route));
      return (await audit(page)).map((one) => ({ ...one, route }));
    } finally {
      await page.close();
    }
  });
  findings.push(...swept.flat());
}, 600_000);

afterAll(async () => {
  await panel?.close();
});

function audit(page: Page): Promise<Omit<Finding, 'route'>[]> {
  return page.evaluate(() => {
    const found: { law: string; where: string; detail: string }[] = [];
    const near = (a: number, b: number): boolean => Math.abs(a - b) < 0.6;

    const name = (element: Element): string => {
      const classes = element.className;
      const list = typeof classes === 'string' ? classes : '';
      const own = list
        .split(/\s+/u)
        .filter((one) => one !== '' && !one.startsWith('svelte-'))
        .slice(0, 2)
        .join('.');
      return element.tagName.toLowerCase() + (own === '' ? '' : `.${own}`);
    };

    const shown = (element: Element): boolean =>
      element.checkVisibility() && element.getBoundingClientRect().height > 0;

    /* A CARD IS NEVER WHAT DECIDES ITS COLUMN'S WIDTH. The sheet says so of the card
       itself now; before that it said it of two containers, and a third laid its cards
       out in a grid of its own. */
    for (const card of document.querySelectorAll<HTMLElement>('.card')) {
      if (!shown(card)) continue;
      const style = getComputedStyle(card);
      if (style.minWidth !== '0px') {
        found.push({
          law: 'a card gives way',
          where: name(card),
          detail: `min-width is ${style.minWidth}, not 0`,
        });
      }
      /* HOW FAR APART CARDS SIT, SAID ONCE, BY THE THING THAT STACKS THEM: a gap on the
         container, never a margin on the card, because a margin has to be cancelled
         wherever something else already spaces it. */
      for (const side of ['marginBlockStart', 'marginBlockEnd'] as const) {
        const margin = Number.parseFloat(style[side]);
        if (margin > 0) {
          found.push({
            law: 'cards are spaced by their container',
            where: name(card),
            detail: `${side} is ${style[side]}`,
          });
        }
      }
    }

    /* WHAT STANDS BESIDE A HEADING DOES NOT SET THE HEADING'S HEIGHT. A heading's box is
       its text's cap box, and the air above it is measured to the words - so a companion
       taller than the line sheds the difference at both ends rather than growing the row.
       Asked of every heading row in the product, however its class is spelled. */
    const heads = document.querySelectorAll<HTMLElement>(
      '[class*="-head"], [class*="-heading"], [class*="head-"]',
    );
    for (const head of heads) {
      if (!shown(head)) continue;
      if (getComputedStyle(head).display !== 'flex' && getComputedStyle(head).display !== 'grid') {
        continue;
      }
      const kids = [...head.children].filter((child): child is HTMLElement => shown(child));
      if (kids.length < 2) continue;
      const heading = kids.find((child) =>
        child.matches('h1, h2, h3, h4, h5, h6, .card-title, .group-name, .page-title'),
      );
      if (heading === undefined) continue;
      const line = heading.getBoundingClientRect().height;
      const tallest = kids.reduce((a, b) =>
        b.getBoundingClientRect().height > a.getBoundingClientRect().height ? b : a,
      );
      const tall = tallest.getBoundingClientRect().height;
      if (tallest === heading || near(tall, line)) continue;
      /* THE LAW IS ABOUT A ROW. A head laid out as a column - a verdict's is - and a row
         that has broken are both more than one line on purpose, and the question "did a
         companion grow the line" has no meaning there. Asked of the rendered lines rather
         than of a class, so a head that stacks only on a phone is judged as what it is at
         the width it is being measured at. */
      const lines = new Set(kids.map((child) => Math.round(child.getBoundingClientRect().top)));
      if (lines.size > 1) continue;
      const row = head.getBoundingClientRect().height;
      if (row > line + 0.6) {
        found.push({
          law: 'a companion does not set the heading line',
          where: `${name(head)} > ${name(tallest)}`,
          detail: `heading line ${line.toFixed(2)}px, companion ${tall.toFixed(2)}px, row ${row.toFixed(2)}px`,
        });
      }
    }

    /* THE SETTING ROW, and every clause is one term. Clause 1 is the half, and it is
       written as `50cqi` against the row's own containment - which only resolves if the
       row is a container. Clause 3 is `flex-wrap`, which IS the stacking. */
    for (const row of document.querySelectorAll<HTMLElement>('.setting-row, .policy-row')) {
      if (!shown(row)) continue;
      const style = getComputedStyle(row);
      if (!style.containerType.includes('inline-size')) {
        found.push({
          law: 'the setting row is its own container',
          where: name(row),
          detail: `container-type is ${style.containerType}`,
        });
      }
      if (style.flexWrap !== 'wrap') {
        found.push({
          law: 'the setting row breaks its line (clause 3)',
          where: name(row),
          detail: `flex-wrap is ${style.flexWrap}`,
        });
      }
      const sides = [...row.children].filter((child): child is HTMLElement => shown(child));
      /* THE HALF IS A HALF OF A ROW WITH TWO SIDES ON IT: once the row has stacked there
         is no other side, and the law's own container query hands the value the line. So
         the cap is only owed where the sides are still sharing one. */
      const lines = new Set(sides.map((side) => Math.round(side.getBoundingClientRect().top)));
      if (lines.size > 1) continue;
      for (const side of sides) {
        const cap = getComputedStyle(side).maxInlineSize;
        if (cap === 'none') continue;
        if (!cap.includes('cqi') && !cap.endsWith('px')) {
          found.push({
            law: 'the setting row caps each side at half (clause 1)',
            where: `${name(row)} > ${name(side)}`,
            detail: `max-inline-size is ${cap}`,
          });
        }
      }
    }

    /* NOTHING MOVES WHILE THE PAGE IS STILL ARRIVING - and the gate has to come OFF, or
       the app has no motion at all for the rest of its life. */
    if (document.documentElement.classList.contains('is-booting')) {
      found.push({
        law: 'the boot gate is released',
        where: 'html',
        detail: 'still wearing is-booting after the page settled',
      });
    }

    /* A CARD'S NOTE IS ITS TITLE'S SECOND LINE, not the card's body. One rule, one
       spacing, one voice - six components each carried their own and no two agreed. */
    for (const note of document.querySelectorAll<HTMLElement>('.group-note')) {
      if (!shown(note)) continue;
      const style = getComputedStyle(note);
      if (style.maxInlineSize !== 'none') {
        found.push({
          law: 'a card note takes the card as its measure',
          where: name(note),
          detail: `max-inline-size is ${style.maxInlineSize}`,
        });
      }
    }

    /* THE SHELL IS PRESSED THE WAY EVERYTHING ELSE IS. Every control in the rail and the
       sidebar takes the shared sink, the crease and the scale - and the crease is the
       CHROME's, because the page's is answered by the page's theme and this chrome is
       dark under a light page in the Root console. */
    const shellControls = document.querySelectorAll<HTMLElement>(
      '.rail-tile, .rail-ws, .side-fold, .side-search, .side-ws-mini, .tree-row, .tree-kid',
    );
    for (const control of shellControls) {
      if (!shown(control)) continue;
      const transition = getComputedStyle(control).transitionProperty;
      if (!transition.includes('translate')) {
        found.push({
          law: 'the shell presses like everything else',
          where: name(control),
          detail: `nothing eases its sink: transition-property is ${transition}`,
        });
      }
    }

    /* ONE PULSE FOR EVERY PLACEHOLDER IN THE PANEL. Eight components declared this
       animation under eight names, and the copies had drifted. */
    const pulses = new Set<string>();
    for (const element of document.querySelectorAll<HTMLElement>('*')) {
      if (!shown(element)) continue;
      const animation = getComputedStyle(element).animationName;
      if (animation === 'none' || animation === '') continue;
      if (/skeleton|placeholder|shimmer|pulse/iu.test(animation)) pulses.add(animation);
    }
    if (pulses.size > 1) {
      found.push({
        law: 'one pulse for every placeholder',
        where: 'the route',
        detail: `${pulses.size} placeholder animations: ${[...pulses].join(', ')}`,
      });
    }

    /* ONE COLUMN HEADING, FOR EVERY TABLE: a heading is a cell, a row inside it, and
       either a word or a button holding that word. Six tables drew this and six kept a
       copy. */
    for (const cell of document.querySelectorAll<HTMLElement>('thead th')) {
      if (!shown(cell)) continue;
      /* Of a heading that SHOWS a word. An actions column has none - its heading is a
         name for a screen reader and nothing else - and the wrapper this asks for exists
         to carry the padding under a word, so a column with no word needs neither. */
      const visible = [...cell.querySelectorAll('*')].every(
        (child) => !child.classList.contains('visually-hidden'),
      );
      if (cell.textContent?.trim() === '' || !visible) continue;
      if (cell.querySelector('.table-heading') === null) {
        found.push({
          law: 'one column heading for every table',
          where: `${name(cell.closest('table') ?? cell)} thead th`,
          detail: `"${cell.textContent?.trim().slice(0, 24)}" holds no .table-heading`,
        });
      }
    }

    /* THE COPY-RHYTHM LAW, WHICH IS 8px RENDERED. Both boxes are trimmed to their ink, so
       the declared margin IS the rendered gap - which is the only reason it can be
       measured from the outside like this. */
    for (const say of document.querySelectorAll<HTMLElement>('.setting-say')) {
      if (!shown(say)) continue;
      const first = say.querySelector<HTMLElement>('.setting-name');
      const second = say.querySelector<HTMLElement>('.setting-why');
      if (first === null || second === null || !shown(first) || !shown(second)) continue;
      const gap = second.getBoundingClientRect().top - first.getBoundingClientRect().bottom;
      const owed = Number.parseFloat(
        getComputedStyle(say).getPropertyValue('--row-copy-gap') || '8',
      );
      if (Math.abs(gap - owed) > 0.75) {
        found.push({
          law: 'the copy rhythm is 8px rendered',
          where: `${name(say)} name→why`,
          detail: `${gap.toFixed(2)}px, owed ${owed}px`,
        });
      }
    }

    /* ONE GUTTER FOR THE TREE: a row's glyph and a heading's first letter stand in one
       column, because one number puts them there. */
    const glyph = document.querySelector<HTMLElement>('.tree-row .gi');
    const heading = document.querySelector<HTMLElement>('.tree-group');
    if (glyph !== null && heading !== null && shown(glyph) && shown(heading)) {
      const left = glyph.getBoundingClientRect().left;
      const word = heading.getBoundingClientRect().left;
      if (!near(left, word)) {
        found.push({
          law: 'the tree has one gutter',
          where: '.tree-group beside .tree-row .gi',
          detail: `heading at ${word.toFixed(1)}px, glyph at ${left.toFixed(1)}px`,
        });
      }
    }

    return found;
  });
}

describe('the sheet’s laws, on every route [Integration]', () => {
  it('reach every card, heading row and setting row in the product', () => {
    const report = findings.map(
      (one) => `  ${one.route}  ${one.law}\n      ${one.where}: ${one.detail}`,
    );
    expect(findings, `laws that do not reach:\n${report.join('\n')}`).toEqual([]);
  });
});
