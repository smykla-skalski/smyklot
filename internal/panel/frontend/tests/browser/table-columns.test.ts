import type { Page } from 'playwright-core';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';

import { addressOf, inLanes, PANEL_ROUTES, startPanel, visit, type Panel } from './harness';

/**
 * Every column holds the widest value it could ever be given.
 *
 * A table laid out against the data that happens to be seeded is a table that
 * has never met the value that matters. GitHub allows a hundred characters in a
 * repository name; the demo's longest was seventeen, and with the name column
 * written as a bare `1fr` - which means `minmax(auto, 1fr)`, and `auto` on a
 * `nowrap` line is the whole line - one such name took 812px of a 977px row and
 * pushed the other four columns off the end of it.
 *
 * So the values are not read, they are replaced: every cell in the page is given
 * a run of 120 unbreakable characters, which is longer than any name, login or
 * path the service can produce and cannot wrap its way out of the problem.
 * Whatever survives that survives real data.
 *
 * Three things are measured, and they are the three ways a column goes wrong:
 *
 *  - A heading and the cells under it disagree about where the column starts.
 *    This is what content-sized tracks do in these tables, because every row is
 *    its own grid and a `max-content` track is therefore measured per row.
 *  - Something inside a cell reaches outside it. The Access table's action
 *    column was sized for its menu, and the rows that also carry a chevron drew
 *    the menu 18px to the left and 2px past the cell's own edge.
 *  - The cells of a row add up to more than the row. That is the overflow above,
 *    and it is the one a reader sees as columns walking off the screen.
 */

/** Longer than any value the service can put in a cell, and unbreakable. */
const FILLER = 'W'.repeat(120);

/** Whole device pixels at 1x; sub-pixel disagreement is the layout rounding. */
const TOLERANCE = 0.5;

interface Fault {
  route: string;
  table: string;
  detail: string;
}

interface Reading {
  route: string;
  tables: number;
  columns: number;
  faults: Fault[];
}

async function measure(page: Page, route: string): Promise<Reading> {
  const found = await page.evaluate(
    ({ filler, tolerance }) => {
      const faults: { table: string; detail: string }[] = [];
      let columns = 0;

      const name = (table: Element): string => {
        const classes = [...table.classList].filter((one) => !one.startsWith('svelte-'));
        return classes[0] ?? 'table';
      };

      const tables = [...document.querySelectorAll('table')].filter(
        (table) => table.querySelector('tbody tr td') !== null,
      );

      for (const table of tables) {
        const label = name(table);

        /* The stress. One text node per cell: a cell is a chip, an icon, a
           switch and a word, and replacing all of them measures markup this
           table will never render. Making the one run of real text as long as it
           can be is what widens the cell. */
        for (const cell of table.querySelectorAll('tbody :is(th, td)')) {
          const walker = document.createTreeWalker(cell, NodeFilter.SHOW_TEXT);
          let node = walker.nextNode();
          while (node !== null) {
            if ((node.nodeValue ?? '').trim() !== '') {
              node.nodeValue = filler;
              break;
            }
            node = walker.nextNode();
          }
        }

        const headings = [...table.querySelectorAll('thead th')];
        const rows = [...table.querySelectorAll('tbody tr')].filter(
          (row) =>
            !row.classList.contains('virtual-spacer') &&
            !row.classList.contains('empty-row') &&
            !row.classList.contains('visually-hidden') &&
            /* `:is(th, td)`, because a row's first cell is a `th scope="row"` wherever
               the row is about a person or a repository. Counting only `td` skipped every
               such row, which is how a whole table stayed outside this sweep. */
            row.querySelectorAll(':is(th, td)').length === headings.length,
        );
        if (rows.length === 0 || headings.length === 0) continue;
        columns += headings.length;

        /** The room a box keeps before whatever it holds. */
        const inset = (element: Element): number =>
          Number.parseFloat(getComputedStyle(element).paddingLeft) || 0;

        for (const row of rows) {
          const cells = [...row.querySelectorAll(':is(th, td)')];

          // 1. The heading and the cells under it start in the same place.
          for (const [index, cell] of cells.entries()) {
            const head = headings[index];
            if (head === undefined) continue;
            const drift = Math.abs(
              head.getBoundingClientRect().left - cell.getBoundingClientRect().left,
            );
            if (drift > tolerance) {
              faults.push({
                table: label,
                detail: `column ${index + 1} (${head.textContent?.trim().slice(0, 18)}) heading is ${drift.toFixed(1)}px from its cells`,
              });
            }

            /* And they keep the same room before what they hold. A heading gives its
               padding to the control inside it, so the boxes lining up no longer means
               the contents do - two columns can share an edge to the pixel and still
               read as a step. This is the measurement that caught a row header left on
               the browser's own 1px under a heading inset by 16. */
            const word = head.querySelector('.table-sort-button, .table-heading-label');
            if (word === null) continue;
            const step = Math.abs(inset(word) - inset(cell));
            if (step > tolerance) {
              faults.push({
                table: label,
                detail: `column ${index + 1} (${head.textContent?.trim().slice(0, 18)}) insets its cells by ${inset(cell).toFixed(1)}px under a heading inset by ${inset(word).toFixed(1)}px`,
              });
            }
          }

          /* 2. Nothing inside a cell is DRAWN outside it.
             A box wider than its cell is ordinary and harmless - a `nowrap` line
             is as wide as its text and always has been. What matters is whether
             anything cuts it: if nothing between the box and the cell clips the
             inline axis, that text is painted over the columns to its right. So
             the overflow is only a fault when the walk up finds nothing that
             clips, which is also what makes this measure the remedy rather than
             a particular way of writing it. */
          for (const [index, cell] of cells.entries()) {
            const box = cell.getBoundingClientRect();
            for (const child of cell.children) {
              const inner = child.getBoundingClientRect();
              if (inner.width === 0) continue;
              const out = Math.max(box.left - inner.left, inner.right - box.right);
              if (out <= tolerance) continue;

              let cut = false;
              for (let at: Element | null = child; at !== null; at = at.parentElement) {
                const overflow = getComputedStyle(at).overflowX;
                if (overflow !== 'visible') {
                  cut = true;
                  break;
                }
                if (at === cell) break;
              }
              if (cut) continue;

              faults.push({
                table: label,
                detail: `column ${index + 1}: ${child.tagName.toLowerCase()}.${[...child.classList].filter((c) => !c.startsWith('svelte-'))[0] ?? ''} is drawn ${out.toFixed(1)}px outside its cell, and nothing cuts it`,
              });
            }
          }

          // 3. The cells add up to the row.
          const rowBox = row.getBoundingClientRect();
          const total = cells.reduce((sum, cell) => sum + cell.getBoundingClientRect().width, 0);
          if (total - rowBox.width > tolerance) {
            faults.push({
              table: label,
              detail: `cells total ${total.toFixed(0)}px in a ${rowBox.width.toFixed(0)}px row`,
            });
          }
        }
      }

      /* One of each: a table with a fault has it in every row, and a hundred
         copies of one line is a failure nobody reads to the end of. */
      const once = new Map(faults.map((fault) => [`${fault.table}:${fault.detail}`, fault]));

      return { tables: tables.length, columns, faults: [...once.values()] };
    },
    { filler: FILLER, tolerance: TOLERANCE },
  );

  return { route, ...found, faults: found.faults.map((fault) => ({ ...fault, route })) };
}

let panel: Panel;
let readings: Reading[] = [];

beforeAll(async () => {
  panel = await startPanel();

  readings = await inLanes(PANEL_ROUTES, async (route) => {
    const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });
    try {
      await visit(page, addressOf(panel, route), { ready: 'tbody td' });

      return await measure(page, route);
    } finally {
      await page.close();
    }
  });
}, 300_000);

afterAll(async () => {
  await panel?.close();
});

describe('the columns of every table [Integration]', () => {
  it('found tables to stress', () => {
    // A route that failed to load reports no faults, which is what a route with
    // nothing wrong reports too. Counting what was looked at tells them apart.
    const columns = readings.reduce((sum, reading) => sum + reading.columns, 0);
    const perRoute = Object.fromEntries(readings.map((one) => [one.route, one.columns]));

    expect(columns, `columns measured per route: ${JSON.stringify(perRoute)}`).toBeGreaterThan(20);
  });

  it('holds its layout when every cell holds the longest value it can', () => {
    const faults = readings.flatMap((reading) => reading.faults);
    const summary = faults.map((one) => `  ${one.route}  ${one.table}: ${one.detail}`).join('\n');

    expect(
      faults.map((one) => `${one.route} ${one.table}`),
      `these columns did not hold:\n${summary}`,
    ).toEqual([]);
  });
});
