/**
 * A filter menu says what its column says.
 *
 * The menu draws each value as the chip the column draws, which is only worth
 * doing if the two agree. They stopped agreeing quietly: `FilterTone` carried a
 * `default` member that named no colour, `FilterMenu` read it as "not a tone" and
 * drew a bare word, and every value whose column is the neutral chip lost its
 * chip in the menu while the four values around it kept theirs. Nothing failed -
 * the menu still filtered, and the odd row out looked like a deliberate choice.
 *
 * So this opens the menus that name what a column shows and compares the two
 * drawings: same shape, same tone, same glyph.
 */
import type { Page } from 'playwright-core';
import { beforeAll, afterAll, describe, expect, it } from 'vitest';

import { addressOf, startPanel, visit, type Panel } from './harness';

interface Pairing {
  route: string;
  /** The heading whose filter was opened. */
  column: string;
  label: string;
  /** How the menu drew it, and how the column drew it. `null` where there was no chip. */
  menu: string | null;
  table: string | null;
}

/**
 * Where a column's own values are filtered.
 *
 * The schedule filter in the queue is deliberately absent: there is no schedule
 * column, so its options are words rather than chips and there is nothing to
 * agree with. That is the case `tone` is omitted for.
 */
const PLACES = [
  { route: 'root/queue', column: 'Checks', heading: 1 },
  { route: 'root/queue/recent', column: 'Outcome', heading: 1 },
  { route: 'root/access/users', column: 'Status', heading: null },
  { route: 'root/access/invitations', column: 'Status', heading: null },
] as const;

let panel: Panel;
const found: Pairing[] = [];

async function collect(page: Page, place: (typeof PLACES)[number]): Promise<Pairing[]> {
  await visit(page, addressOf(panel, place.route), { ready: 'tbody td' });

  const opened = await page.evaluate((column: string) => {
    const heads = [...document.querySelectorAll('thead th')];
    const head = heads.find((one) => (one.textContent ?? '').trim().startsWith(column));
    const trigger = head?.querySelector('.filter-trigger') as HTMLElement | undefined;
    if (trigger === undefined) return false;
    trigger.click();
    return true;
  }, place.column);
  if (!opened) return [];
  await page.waitForSelector('[role="option"]', { timeout: 2000 });

  return page.evaluate(
    ({ route, column }) => {
      /** A chip's tone and glyph, which is the whole of how it is drawn. */
      const drawing = (chip: Element | null | undefined): string | null => {
        if (chip === null || chip === undefined) return null;
        const tone = [...chip.classList].find(
          (one) => one.startsWith('chip-') && one !== 'chip-small',
        );
        const glyph = chip.querySelector('svg use')?.getAttribute('href') ?? '';
        const shape = chip.querySelector('svg path')?.getAttribute('d')?.slice(0, 24) ?? '';

        return `${tone ?? 'no-tone'} ${glyph}${shape}`;
      };

      /* The table's own chips, by the word they carry - which is what the menu
         option carries too, and so what pairs the two. */
      const shown = new Map<string, string | null>();
      for (const chip of document.querySelectorAll('tbody .chip')) {
        const label = (chip.textContent ?? '').trim();
        if (label !== '' && !shown.has(label)) shown.set(label, drawing(chip));
      }

      const pairs: {
        route: string;
        column: string;
        label: string;
        menu: string | null;
        table: string | null;
      }[] = [];
      for (const option of document.querySelectorAll('[role="option"]')) {
        const chip = option.querySelector('.chip');
        const label = (
          chip?.textContent ??
          option.querySelector('strong')?.textContent ??
          ''
        ).trim();
        if (label === '' || !shown.has(label)) continue;
        pairs.push({
          route,
          column,
          label,
          menu: drawing(chip),
          table: shown.get(label) ?? null,
        });
      }

      return pairs;
    },
    { route: place.route, column: place.column },
  );
}

beforeAll(async () => {
  panel = await startPanel();
  for (const place of PLACES) {
    const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });
    try {
      found.push(...(await collect(page, place)));
    } finally {
      await page.close();
    }
  }
}, 300_000);

afterAll(async () => {
  await panel?.close();
});

describe('a filter menu and its column [Integration]', () => {
  it('found values drawn in both places', () => {
    // Nothing to compare reports no disagreement, which is what agreement reports
    // too. Counting the pairs tells them apart.
    expect(found.map((one) => `${one.route} ${one.label}`).length).toBeGreaterThan(4);
  });

  it('draws every filtered value the way its column draws it', () => {
    const apart = found
      .filter((one) => one.menu !== one.table)
      .map(
        (one) =>
          `${one.route} ${one.column} "${one.label}": menu ${one.menu ?? 'plain word'}, column ${one.table ?? 'plain word'}`,
      );

    expect(apart, `these values are drawn two ways:\n  ${apart.join('\n  ')}`).toEqual([]);
  });
});
