/**
 * The Queue's columns, against the rule the panel's other tables are held to.
 *
 * `table-columns.test.ts` stresses every table with an unbreakable run longer
 * than anything real and asks whether the layout holds. That catches a column
 * that is too NARROW. It cannot catch the other half of the rule, which is the
 * half these numbers exist for: a column wider than its own worst case is dead
 * space at the front of every row, and nothing complains about dead space.
 *
 * So this measures both ends, and it does it with the vocabulary rather than with
 * filler. Each column here draws from a closed set of words - the five CI states
 * plus the one a request wears before its first check, the nine reasons a next
 * look is scheduled, the three ways a request ends - and every one of them is
 * listed below, beside the function that produces it. A column is right when it
 * is the width of its own widest member and no wider.
 *
 * The two columns that cannot be sized that way say so and are checked their own
 * way: the pull request flexes, and the reason a request ended is service text
 * with no bound, capped at the two lines the row already stands at.
 */
import { beforeAll, afterAll, describe, expect, it } from 'vitest';

import { addressOf, startPanel, visit, type Panel } from './harness';

/** A quarter of a rem: the step the widths are rounded up to, and so the slack allowed. */
const STEP = 4;

/**
 * Every value each column can hold, beside where it comes from.
 *
 * `queueState` falls through to "Scheduled" for a state it does not know, and
 * that is not a defensive branch - `last_observed_state` is
 * `TEXT NOT NULL DEFAULT ''` and the insert in `sqlstore.Arm` does not set it, so
 * every request wears it between the command and its first reconciliation.
 */
const VOCABULARY = {
  waiting: {
    checks: ['Passing', 'Running', 'Failing', 'Unreadable', 'No checks', 'Scheduled'],
    /* `queueNext`: a merge landing, a countdown to one, and `formatUntil` at each
       of its own steps down to the date it falls back to. */
    lead: [
      'Merging now',
      'Merging in 0:30',
      'Checks again now',
      'Checks again in 59 minutes',
      'Checks again in 23 hours',
      'Checks again in 13 days',
      'Checks again 17 Aug 2026',
    ],
    /** `triggerReason`, all of it, plus the two the quiet period writes. */
    sub: [
      'Waiting for it to land',
      'Quiet period, then it lands',
      'A delivery moved it forward',
      'Asked for from this panel',
      'Tidying up after the merge',
      'First look since it was armed',
      'Nothing has moved for an hour',
      'Waiting for checks to appear',
      'The regular safety net',
    ],
    /** `shortAge` at each of its steps. */
    age: ['just now', '59 min', '23 hr', '6 d', '99 wk'],
  },
  recent: {
    /** `outcomeState`. A request reaches this table with one of these three. */
    outcome: ['Merged', 'Cancelled', 'Superseded'],
    /** `cleanupState`. */
    cleanup: ['Done', 'Pending', 'Failed'],
    age: ['just now', '59 min', '23 hr', '6 d', '99 wk'],
    /* `endReason`: the panel's own four, then every string the service writes into
       `reason` - `policy.go`, and the cancellation reasons in
       `pending_ci_github.go`. */
    reason: [
      'Checks passed and stayed quiet for 30 s',
      'Cancelled before it could merge',
      'Replaced by a later command',
      'Still waiting',
      'pull request merged outside pending CI reconciliation',
      'replaced by a newer authorized command',
      'base branch has no required status checks',
      'repository disabled in Smyklot',
      'source comment deleted',
      'source comment edited',
    ],
  },
} as const;

interface ColumnReading {
  column: string;
  /** What the column is given. */
  shipped: number;
  /** What its widest member needs, with the cell's own padding. */
  needs: number;
}

/** One state chip as it was actually drawn, before any value was written over it. */
interface ChipReading {
  label: string;
  height: number;
  /** How far the glyph's centre sits from the word's, which should be nothing. */
  markDrift: number;
  /** How far the chip's centre sits from its cell's content centre, same. */
  cellDrift: number;
}

interface Reading {
  columns: ColumnReading[];
  /** Rows that grew: a value that did not fit wrapped, and one row is now taller. */
  heights: number[];
  /** Reasons that needed more than the two lines the row stands at. */
  clipped: string[];
  /** The first column's chips, one per state the seed puts on the screen. */
  chips: ChipReading[];
}

let panel: Panel;
const readings = new Map<string, Reading>();

async function read(section: 'waiting' | 'recent'): Promise<Reading> {
  const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });
  try {
    const route = section === 'waiting' ? 'root/queue' : 'root/queue/recent';
    await visit(page, addressOf(panel, route), { ready: 'tbody td' });

    return await page.evaluate(
      ({ words, kind }) => {
        const table = document.querySelector<HTMLTableElement>(
          kind === 'waiting' ? '.waiting-table' : '.recent-table',
        );
        if (table === null) throw new Error(`no ${kind} table`);
        const body = table.tBodies[0];
        if (body === undefined) throw new Error('no body');
        const template = body.querySelector('.queue-row');
        if (template === null) throw new Error('no rows to fill');

        /* Read before anything is written over: these are the chips the SEED put
           on the screen, one per state, which is why `pendingCISeeds` carries one
           request in each. A chip's height is set by its glyph rather than by its
           word - the mark is 13px and the line is 12px - so a state whose glyph
           was drawn at another size would make one row's badge taller than the
           five beside it, and nothing else here would notice. */
        const middle = (box: DOMRect): number => box.top + box.height / 2;
        const seeded = [...body.querySelectorAll('.queue-row')] as HTMLTableRowElement[];
        const chips = seeded.flatMap((row) => {
          const chip = row.cells[0]?.querySelector('.chip');
          const cell = row.cells[0];
          if (chip == null || cell == null) return [];
          const mark = chip.querySelector('svg');
          const word = chip.querySelector('.chip-label');
          const style = getComputedStyle(cell);
          const cellBox = cell.getBoundingClientRect();
          /* The content centre, not the border box's: every row but the last
             carries a 1px bottom border, which would read as half a pixel of
             drift on five rows out of six. */
          const inner =
            cellBox.top +
            parseFloat(style.paddingTop) +
            (cellBox.height -
              parseFloat(style.paddingTop) -
              parseFloat(style.paddingBottom) -
              parseFloat(style.borderBottomWidth)) /
              2;
          const round = (value: number): number => Math.round(value * 1000) / 1000;

          return [
            {
              label: (chip.textContent ?? '').trim(),
              height: round(chip.getBoundingClientRect().height),
              markDrift:
                mark === null || word === null
                  ? 0
                  : round(
                      Math.abs(
                        middle(mark.getBoundingClientRect()) - middle(word.getBoundingClientRect()),
                      ),
                    ),
              cellDrift: round(Math.abs(middle(chip.getBoundingClientRect()) - inner)),
            },
          ];
        });

        const lists = Object.values(words) as string[][];
        const want = Math.max(...lists.map((one) => one.length));
        while (body.querySelectorAll('.queue-row').length < want) {
          body.append(template.cloneNode(true));
        }
        const rows = [...body.querySelectorAll('.queue-row')] as HTMLTableRowElement[];

        /* One text node per cell, the way the other sweep does it: a cell is a
           chip, a mark and a word, and replacing all of them would measure markup
           this table never renders. */
        const write = (host: Element | null, value: string): void => {
          if (host === null) throw new Error(`nowhere to write ${value}`);
          const walk = document.createTreeWalker(host, NodeFilter.SHOW_TEXT);
          let node = walk.nextNode();
          while (node !== null) {
            if ((node.nodeValue ?? '').trim() !== '') {
              node.nodeValue = value;
              return;
            }
            node = walk.nextNode();
          }
          throw new Error(`no text to replace for ${value}`);
        };

        const pick = (list: string[], index: number): string => list[index % list.length] as string;

        /* The ring is drawn only while a merge is landing, and then the lead is
           the countdown - so a ring beside "Checks again in 59 minutes" is a row
           this table cannot produce, and measuring it would buy the column 12px
           of width for nothing. Ring rows take the merging leads; the rest take
           the others. */
        const leads = words.lead ?? [];
        const merging = leads.filter((one) => one.startsWith('Merging'));
        const waiting = leads.filter((one) => !one.startsWith('Merging'));

        for (const [index, row] of rows.entries()) {
          if (kind === 'waiting') {
            write(row.cells[0] ?? null, pick(words.checks, index));
            const next = row.cells[2] ?? null;
            const hasRing = next?.querySelector('.next-lead .ring') != null;
            const lead =
              next?.querySelector('.next-lead .band-trim') ??
              next?.querySelector('.next-lead') ??
              null;
            write(lead, pick(hasRing ? merging : waiting, index));
            write(row.cells[2]?.querySelector('.next-sub') ?? null, pick(words.sub, index));
            write(row.cells[3] ?? null, pick(words.age, index));
          } else {
            write(row.cells[0] ?? null, pick(words.outcome, index));
            write(row.cells[2] ?? null, pick(words.cleanup, index));
            write(row.cells[3] ?? null, pick(words.reason, index));
            write(row.cells[4] ?? null, pick(words.age, index));
          }
        }

        /* Read one: the shipped layout, holding every value at once. A row that is
           taller than its neighbours is a value that wrapped where it should not
           have, and a reason whose full height is past its cap is one the reader
           cannot finish. */
        const heights = [
          ...new Set(rows.map((row) => Math.round(row.getBoundingClientRect().height))),
        ];
        /* Counted as line boxes rather than as height. `text-box: trim-both` ends
           the block at the baseline, so a one-line reason measures shorter than
           its own line and every height comparison reads it as overflowing. A
           range over the text reports one rect per line, whatever the box does. */
        const clipped: string[] = [];
        if (kind === 'recent') {
          for (const row of rows) {
            const reason = row.cells[3]?.querySelector('.reason') as HTMLElement | null;
            if (reason === null || reason === undefined) continue;
            const range = document.createRange();
            range.selectNodeContents(reason);
            const lines = range.getClientRects().length;
            if (lines > 2) clipped.push(`${(reason.textContent ?? '').trim()} (${lines} lines)`);
          }
        }

        const heads = [...(table.tHead?.rows[0]?.cells ?? [])];
        const shipped = heads.map((head) => head.getBoundingClientRect().width);

        /* Read two: what each column would take if it were free. Auto layout
           gives every column its own max-content, over the heading AND every cell
           - which is the researched rule, computed by the engine rather than
           restated here. */
        const loosen = document.createElement('style');
        loosen.textContent =
          '.queue-table{table-layout:auto!important;width:max-content!important}' +
          '.queue-table :is(th,td){width:auto!important}' +
          '.reason{display:block!important;overflow:visible!important;white-space:nowrap!important}';
        document.head.append(loosen);
        const needs = heads.map((head) => head.getBoundingClientRect().width);
        loosen.remove();

        return {
          columns: heads.map((head, index) => ({
            column: (head.textContent ?? '').trim() || 'Actions',
            shipped: Math.round((shipped[index] ?? 0) * 100) / 100,
            needs: Math.round((needs[index] ?? 0) * 100) / 100,
          })),
          heights,
          clipped,
          chips,
        };
      },
      { words: VOCABULARY[section] as unknown as Record<string, string[]>, kind: section },
    );
  } finally {
    await page.close();
  }
}

beforeAll(async () => {
  panel = await startPanel();
  readings.set('waiting', await read('waiting'));
  readings.set('recent', await read('recent'));
}, 300_000);

afterAll(async () => {
  await panel?.close();
});

/* The two that are not sized by their worst case, named so that a column added
   later has to say which it is rather than quietly joining them. */
const EXEMPT = new Set(['Pull request', 'Why it ended']);

describe('the Queue table columns [Integration]', () => {
  it.each(['waiting', 'recent'])('holds every value the %s section can show', (section) => {
    const reading = readings.get(section);
    const narrow = (reading?.columns ?? [])
      .filter((one) => !EXEMPT.has(one.column))
      .filter((one) => one.shipped + 0.5 < one.needs)
      .map((one) => `${one.column}: ${one.shipped}px given, ${one.needs}px needed`);

    expect(narrow, `these columns cut a value off:\n  ${narrow.join('\n  ')}`).toEqual([]);
  });

  it.each(['waiting', 'recent'])('reserves nothing it does not need in %s', (section) => {
    const reading = readings.get(section);
    const loose = (reading?.columns ?? [])
      .filter((one) => !EXEMPT.has(one.column))
      .filter((one) => one.shipped > one.needs + STEP)
      .map((one) => `${one.column}: ${one.shipped}px given, ${one.needs}px needed`);

    expect(loose, `these columns hold empty space in every row:\n  ${loose.join('\n  ')}`).toEqual(
      [],
    );
  });

  it.each(['waiting', 'recent'])('keeps every row in %s the same height', (section) => {
    expect(readings.get(section)?.heights ?? []).toHaveLength(1);
  });

  it('shows every reason a request ended in full', () => {
    expect(readings.get('recent')?.clipped ?? []).toEqual([]);
  });

  /* The seed carries one request in each of the six states, so this is measured
     rather than argued: 6.4px of padding, a 13px glyph and 6.4px of padding is
     25.8px, and the glyph is what sets it - the word's line box is 12px and never
     reaches the edges. Every state's glyph is drawn at the same size, so every
     badge is the same height, and a redraw that changed one would show here. */
  it('draws every state of the Checks column at one height', () => {
    const chips = readings.get('waiting')?.chips ?? [];
    const shown = chips.map((one) => one.label).sort();

    expect(shown).toEqual(
      ['Failing', 'No checks', 'Passing', 'Running', 'Scheduled', 'Unreadable'].sort(),
    );
    expect(new Set(chips.map((one) => one.height)).size, JSON.stringify(chips)).toBe(1);
  });

  it('centres every state chip on its mark and on its cell', () => {
    const off = (readings.get('waiting')?.chips ?? [])
      .filter((one) => one.markDrift > 0.01 || one.cellDrift > 0.01)
      .map(
        (one) => `${one.label}: ${one.markDrift}px from its mark, ${one.cellDrift}px from centre`,
      );

    expect(off, `these chips are not centred:\n  ${off.join('\n  ')}`).toEqual([]);
  });
});
