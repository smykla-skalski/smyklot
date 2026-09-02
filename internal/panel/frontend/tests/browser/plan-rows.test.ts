import type { Page } from 'playwright-core';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';

import { addressOf, startPanel, visit, type Panel } from './harness';

/**
 * The sync plan's rows and its operation counts, held to the two rules a reader
 * scans them by.
 *
 * A COUNT KEEPS ITS COLUMN. The three counts on a repository card were laid out
 * as a right-aligned flex run, so the rank a count fell in depended on how many
 * of the three that repository happened to have: a card with no removals put
 * its `+3` where the card above it put `~2`, and reading down the rail a green
 * number and a blue one shared a column with only the sign to tell them apart.
 * Each card is its own grid, so this cannot be fixed by content sizing - the
 * slots have to be declared, and this measures the declaration from outside.
 *
 * A ROW IS A ROW. Rows that open a diff were a second component - a 24px
 * button - beside the 40px div every other row used, holding the same three
 * spans. One list therefore kept two rhythms, and the only row a reader could
 * press was the one that looked least like the others. Both faults are geometry
 * and neither is visible to a unit test.
 */

let panel: Panel;

beforeAll(async () => {
  panel = await startPanel();
});

afterAll(async () => {
  await panel?.close();
});

/** Whole device pixels at 1x; below this is the layout's own rounding. */
const TOLERANCE = 0.5;

/**
 * What counts as one line for the cap bands inside a row.
 *
 * Tighter than the tolerance above, because the fault this replaced was a whole
 * pixel: the sans kind cell stood 1px above its two mono neighbours and the row
 * grew to cover both. What is left is the fractional width of a subgrid track,
 * which reads as 0.00px at 1512 and 0.01px at 1280 - a number that moves with
 * the viewport is the rasteriser, not a disagreement about where the line is.
 */
const BAND = 0.05;

interface CountCell {
  repo: string;
  operation: string;
  x: number;
}

interface RowCell {
  /** `button` where the row opens a diff, `div` where it does not. */
  tag: string;
  class: string;
  height: number;
  /** Cap-band centres of the cells that carry words. */
  centres: number[];
  /** How many lines the subject stacks into. One for every kind but settings. */
  lines: number;
}

interface Reading {
  counts: CountCell[];
  rows: RowCell[];
  /** Every class used as a row's inner line, so a second one cannot hide. */
  lineClasses: string[];
}

async function read(page: Page): Promise<Reading> {
  return page.evaluate(() => {
    const counts: { repo: string; operation: string; x: number }[] = [];
    for (const row of document.querySelectorAll('.repo-row')) {
      const name = row.querySelector('.object-name')?.textContent?.trim() ?? '';
      for (const cell of row.querySelectorAll('.repo-group-counts > *')) {
        const operation = cell.className.replace(/svelte-\w+/gu, '').trim();
        counts.push({ repo: name, operation, x: cell.getBoundingClientRect().x });
      }
    }

    const rows: {
      tag: string;
      class: string;
      height: number;
      centres: number[];
      lines: number;
    }[] = [];
    const lineClasses: string[] = [];
    for (const row of document.querySelectorAll('.action-row')) {
      const line = row.querySelector('.action-row-line');
      if (line === null) continue;
      lineClasses.push(line.className.replace(/svelte-\w+/gu, '').trim());
      /* A settings action is one action and several facts - one line per field -
         so it is legitimately taller. Every OTHER row is one line, and those are
         the ones that have to agree. */
      const stacked = line.querySelectorAll('.setting-line').length;
      const startAligned = getComputedStyle(line).alignItems === 'start';
      const centres: number[] = [];
      for (const cell of line.children) {
        if (cell.classList.contains('action-more')) continue;
        const box = cell.getBoundingClientRect();
        // An unwritten kind cell has no band to place; it is not a fault.
        if (box.height === 0) continue;
        /* A STACKED SUBJECT IS MEASURED BY ITS FIRST LINE. The verb leads a
           settings row rather than floating against the middle of it - centred,
           it sat beside the gap between two fields and labelled nothing - so
           the line they all have to share is the first one. */
        const band = cell.classList.contains('action-settings')
          ? (cell.querySelector('.setting-line')?.getBoundingClientRect() ?? box)
          : box;
        /* MEASURE WHAT THE ROW DECLARES. A centred row promises its cells share
           a middle; a start-aligned one promises they share a top, and the two
           are not the same promise where the bands differ in height - the kind
           is sans at a 8.94px cap against mono's 8.76, so aligning the tops puts
           the centres 0.09 apart and aligning the centres does the reverse.
           Reading centres on a start-aligned row reported the alignment
           working as a fault. */
        centres.push(startAligned ? band.y : band.y + band.height / 2);
      }
      rows.push({
        tag: line.tagName.toLowerCase(),
        class: row.className.replace(/svelte-\w+/gu, '').trim(),
        height: row.getBoundingClientRect().height,
        centres,
        lines: stacked === 0 ? 1 : stacked,
      });
    }

    return { counts, rows, lineClasses };
  });
}

describe('workspace sync plan rows', () => {
  it('gives every operation its own column and every row one height', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });

    try {
      await visit(page, addressOf(panel, 'workspace/sync/plan'), { ready: 'h1' });
      await page.waitForSelector('.repo-row .repo-group-counts');
      const reading = await read(page);

      /* The fixture has to actually exercise the fault: cards that all carry the
         same three operations would agree however the group is laid out. */
      const byRepo = new Map<string, number>();
      for (const cell of reading.counts) byRepo.set(cell.repo, (byRepo.get(cell.repo) ?? 0) + 1);
      expect(byRepo.size).toBeGreaterThan(1);
      expect(new Set(byRepo.values()).size).toBeGreaterThan(1);

      const columns = new Map<string, number[]>();
      for (const cell of reading.counts) {
        columns.set(cell.operation, [...(columns.get(cell.operation) ?? []), cell.x]);
      }

      // Each operation lands on one x, whichever card it is on.
      for (const [operation, xs] of columns) {
        const spread = Math.max(...xs) - Math.min(...xs);
        expect(`${operation} spread ${spread.toFixed(2)}px`).toBe(`${operation} spread 0.00px`);
      }

      // And the three operations are three different columns, not one.
      const lefts = [...columns.values()].map((xs) => xs[0] ?? 0);
      expect(new Set(lefts).size).toBe(columns.size);

      /* One row component. The tags differ, because a row that opens something
         is a button and a row that does not is not - but the class, and so the
         geometry, is the same one. */
      expect(new Set(reading.lineClasses)).toEqual(new Set(['action-row-line']));
      expect(new Set(reading.rows.map((row) => row.tag))).toEqual(new Set(['div', 'button']));

      /* One line, one height. A settings row carries a line per field and is
         taller for a reason, so it is measured against the others of its own
         depth rather than excused. */
      const byDepth = new Map<number, number[]>();
      for (const row of reading.rows) {
        byDepth.set(row.lines, [...(byDepth.get(row.lines) ?? []), row.height]);
      }
      expect(byDepth.get(1)?.length ?? 0).toBeGreaterThan(3);
      for (const [lines, heights] of byDepth) {
        const spread = Math.max(...heights) - Math.min(...heights);
        expect(
          `${lines}-line rows spread ${spread <= TOLERANCE ? 'within' : spread.toFixed(2)}`,
        ).toBe(`${lines}-line rows spread within`);
      }

      /* Every cell of a row sits on one line, natively: the cells are trimmed to
         their own cap bands, so a sans kind beside two mono neighbours has no
         half-leading left to disagree with. */
      for (const row of reading.rows) {
        const spread = Math.max(...row.centres) - Math.min(...row.centres);
        expect(`${row.class} band spread ${spread <= BAND ? 'within' : spread.toFixed(2)}`).toBe(
          `${row.class} band spread within`,
        );
      }
    } finally {
      await page.close();
    }
  });
});
