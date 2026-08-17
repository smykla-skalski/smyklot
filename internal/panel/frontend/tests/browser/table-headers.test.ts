import { afterAll, beforeAll, describe, expect, it } from 'vitest';

import { inLanes, startPanel, visit, type Panel } from './harness';

/**
 * Every table in the product draws one column heading.
 *
 * Six components render a table and each used to carry its own copy of the heading's type and its
 * own rule under the band. The copies drifted: two inks, three line-heights, a weight of 650 that
 * nothing else uses, and the queue's header with no rule under it at all.
 *
 * `app.css` states it once now, and this is what keeps it stated once. The trap it exists for is
 * specificity rather than carelessness: a component writing `th, td { font-size: … }` inside a
 * Svelte `<style>` gets a scoping class, which outranks the two element selectors of `thead th` -
 * so three tables went on rendering their headings at 13px while the shared rule said 11, and
 * nothing anywhere was obviously wrong.
 *
 * The ground and the rule are read per shell rather than as one value: the Root console re-skins
 * the palette, so `--table-header-bg` and `--rule` are legitimately different there. What has to
 * match is that every table in the SAME shell resolves them the same way.
 */

/** Every route in the panel that renders a table, and which shell it renders in. */
const TABLES = [
  { route: 'i/repositories', shell: 'panel' },
  { route: 'i/users', shell: 'panel' },
  { route: 'i/invitations', shell: 'panel' },
  { route: 'i/history', shell: 'panel' },
  { route: 'root/queue', shell: 'root' },
  { route: 'root/queue/recent', shell: 'root' },
  { route: 'root/installations', shell: 'root' },
  { route: 'root/access/users', shell: 'root' },
  { route: 'root/access/invitations', shell: 'root' },
  { route: 'root/history/audit', shell: 'root' },
] as const;

interface Heading {
  font: string;
  letterSpacing: string;
  textTransform: string;
  color: string;
  background: string;
  borderBottom: string;
}

/** One sortable heading: what it presses over, against the cell it stands in. */
interface Target {
  label: string;
  /** Cell width minus the button's, in CSS pixels. Zero is the whole cell. */
  narrowerBy: number;
  /** Cell height minus the button's, less the rule under the band. */
  shorterBy: number;
  /** Whether a filter trigger shares the cell. */
  filtered: boolean;
}

let panel: Panel;
let headings: Record<string, Heading | string>;
let targets: Record<string, Target[]>;
/** One heading with a filter, read with the pointer on its words and then on its funnel. */
interface HeadingHover {
  label: string;
  /** Whether the heading lit when the pointer was on the words. */
  litFromWords: boolean;
  /** Whether it lit when the pointer was on the funnel. It should not. */
  litFromFunnel: boolean;
  /** Whether the funnel itself lit when the pointer was on it. */
  triggerLit: boolean;
  /** Whether the sort arrow answered a pointer on the funnel. It should not. */
  arrowMoved: boolean;
}

let behindTheFilter: Record<string, HeadingHover[]>;

beforeAll(async () => {
  panel = await startPanel();
  headings = {};
  targets = {};
  behindTheFilter = {};

  /* A page each, and several at once: ten routes read one after another is ten waits for a route
     to load, and a computed style is the same style whatever else the machine is doing. */
  const read = await inLanes(TABLES, async ({ route }) => {
    const address = route.startsWith('i/')
      ? `${panel.origin}/i/${panel.account}/${route.slice(2)}`
      : `${panel.origin}/${route}`;
    const page = await panel.browser.newPage();
    try {
      // The heading itself, because that is what is read below. Not an assertion in disguise: a
      // route that never draws one is measured anyway and reported by name.
      await visit(page, address, { ready: 'thead th' });

      const heading = await page.evaluate((): Heading | string => {
        const th = document.querySelector('thead th');
        if (th === null) return 'this route rendered no table header';
        const style = getComputedStyle(th);
        return {
          font: style.font,
          letterSpacing: style.letterSpacing,
          textTransform: style.textTransform,
          color: style.color,
          background: style.backgroundColor,
          borderBottom: style.borderBottom,
        };
      });

      const measured = await page.evaluate((): Target[] =>
        [...document.querySelectorAll('thead th')]
          .map((th) => {
            const button = th.querySelector('.table-sort-button');
            if (button === null) return null;
            const cell = th.getBoundingClientRect();
            const target = button.getBoundingClientRect();
            // The rule under the band is the cell's border, not room the button declined.
            const rule = Number.parseFloat(getComputedStyle(th).borderBottomWidth) || 0;
            return {
              label: (th.textContent ?? '').trim().slice(0, 24),
              narrowerBy: Number((cell.width - target.width).toFixed(2)),
              shorterBy: Number((cell.height - rule - target.height).toFixed(2)),
              filtered: th.querySelector('.filter-trigger') !== null,
            };
          })
          .filter((one): one is Target => one !== null),
      );

      /* One heading, two pointer positions. On the words the whole cell has to light, including
         the ground behind the funnel - that is the target being the cell. On the funnel only the
         funnel lights, because the tint names what a press would act on and a press there opens a
         menu rather than sorting. The arrow answers the second question the same way, and must not
         move at all while the pointer is on the trigger.

         Read as paint rather than as a rule, so the answer holds however the tint is drawn. */
      const painted: HeadingHover[] = [];
      const funnels = await page.$$('thead th:has(.table-sort-button) .filter-trigger');
      const state = (trigger: Element) => {
        const th = trigger.closest('th');
        const button = th?.querySelector('.table-sort-button');
        const indicator = th?.querySelector('.sort-indicator');
        const style =
          indicator === null || indicator === undefined ? null : getComputedStyle(indicator);
        return {
          label: (th?.textContent ?? '').trim().slice(0, 24),
          headingLit:
            button !== null &&
            button !== undefined &&
            getComputedStyle(button).backgroundImage !== 'none',
          triggerLit: getComputedStyle(trigger).backgroundImage !== 'none',
          arrow: style === null ? '' : `${style.opacity} ${style.transform}`,
        };
      };

      for (const funnel of funnels) {
        // Away first, so what the arrow does untouched is the thing it is compared against.
        await page.mouse.move(0, 0);
        await page.waitForTimeout(250);
        const resting = await funnel.evaluate(state);

        // The words: the far end of the heading, well clear of the trigger.
        const cell = await funnel.evaluate((trigger) => {
          const box = trigger.closest('th')!.getBoundingClientRect();
          return { x: box.left + 8, y: box.top + box.height / 2 };
        });
        await page.mouse.move(cell.x, cell.y);
        await page.waitForTimeout(250);
        const onWords = await funnel.evaluate(state);

        await funnel.hover();
        await page.waitForTimeout(250);
        const onFunnel = await funnel.evaluate(state);

        painted.push({
          label: resting.label,
          litFromWords: onWords.headingLit,
          litFromFunnel: onFunnel.headingLit,
          triggerLit: onFunnel.triggerLit,
          arrowMoved: onFunnel.arrow !== resting.arrow,
        });
      }

      return { route, heading, measured, painted };
    } finally {
      await page.close();
    }
  });

  for (const { route, heading, measured, painted } of read) {
    headings[route] = heading;
    targets[route] = measured;
    behindTheFilter[route] = painted;
  }
}, 300_000);

afterAll(async () => {
  await panel?.close();
});

describe('the column heading [Integration]', () => {
  it('is rendered on every route that has a table', () => {
    const missing = Object.entries(headings)
      .filter(([, heading]) => typeof heading === 'string')
      .map(([route, heading]) => `${route}: ${String(heading)}`);

    expect(missing, `a table was expected here:\n  ${missing.join('\n  ')}`).toEqual([]);
  });

  /* Type is the shell's business to keep identical: it comes from one rule and no palette changes
     it, so all ten routes must agree exactly. */
  it.each(['font', 'letterSpacing', 'textTransform', 'color'] as const)(
    'draws the same %s in every table',
    (property) => {
      const values = new Map<string, string[]>();
      for (const { route } of TABLES) {
        const heading = headings[route];
        if (typeof heading === 'string') continue;
        const value = heading[property];
        values.set(value, [...(values.get(value) ?? []), route]);
      }

      const report = [...values]
        .map(([value, routes]) => `  ${value}\n    ${routes.join(', ')}`)
        .join('\n');
      expect([...values.keys()], `tables disagree about ${property}:\n${report}`).toHaveLength(1);
    },
  );

  /* The ground and the rule are palette-bound, so they agree within a shell rather than across
     both - the Root console re-skins them on purpose. */
  it.each(['background', 'borderBottom'] as const)(
    'draws the same %s within a shell',
    (property) => {
      for (const shell of ['panel', 'root'] as const) {
        const values = new Set(
          TABLES.filter((table) => table.shell === shell)
            .map((table) => headings[table.route])
            .filter((heading): heading is Heading => typeof heading !== 'string')
            .map((heading) => heading[property]),
        );
        expect([...values], `${shell} tables disagree about ${property}`).toHaveLength(1);
      }
    },
  );

  it('draws a rule under the heading band', () => {
    const without = Object.entries(headings)
      .filter(
        ([, heading]) =>
          typeof heading !== 'string' &&
          (heading.borderBottom === '' || heading.borderBottom.startsWith('0px')),
      )
      .map(([route]) => route);

    expect(without, `no rule under the heading:\n  ${without.join('\n  ')}`).toEqual([]);
  });
});

/**
 * What a sortable heading answers the pointer over.
 *
 * The fault this exists for is the oldest one in the pattern: a heading that lights up over the
 * whole cell and only presses over the words. Polaris shipped it, Carbon shipped it, and the queue
 * shipped it - measured at 54% of its cell's width and 35% of its height against the audit table's
 * 100%. The remedy is the same everywhere: the cell gives its padding to the button, so the target
 * IS the cell.
 *
 * It is a test rather than a rule because the way it comes back is by accident. A component that
 * positions its own filter trigger gets a Svelte scoping class, which beats a shared rule of the
 * same shape - and the funnel goes back into the flow, taking 28px of a 136px cell out of the
 * target with nothing anywhere looking wrong.
 */
describe('a sortable heading [Integration]', () => {
  it('is drawn on the routes that sort', () => {
    const sortable = Object.entries(targets).filter(([, found]) => found.length > 0);
    expect(sortable.length, 'no route drew a sortable heading at all').toBeGreaterThan(4);
  });

  it('presses over the whole cell', () => {
    const short = Object.entries(targets).flatMap(([route, found]) =>
      found
        .filter((one) => one.narrowerBy > 0.5 || one.shorterBy > 0.5)
        .map(
          (one) =>
            `${route} "${one.label}": ${one.narrowerBy}px of width and ${one.shorterBy}px of height are not pressable`,
        ),
    );

    expect(
      short,
      `a heading lights up over more than it answers:\n  ${short.join('\n  ')}`,
    ).toEqual([]);
  });

  it('lights from anywhere in the cell', () => {
    const dark = Object.entries(behindTheFilter).flatMap(([route, found]) =>
      found.filter((one) => !one.litFromWords).map((one) => `${route} "${one.label}"`),
    );

    expect(dark, `a heading did not answer its own words:\n  ${dark.join('\n  ')}`).toEqual([]);
  });

  /* The three things a pointer on the funnel must do, which are all one decision: the tint names
     what a press would act on, and a press there opens a menu rather than sorting. */
  it('hands the whole hover to the filter trigger, and nothing else', () => {
    const wrong = Object.entries(behindTheFilter).flatMap(([route, found]) =>
      found
        .filter((one) => one.litFromFunnel || !one.triggerLit || one.arrowMoved)
        .map((one) => {
          const faults = [
            one.litFromFunnel && 'the heading lit too',
            !one.triggerLit && 'the trigger did not light',
            one.arrowMoved && 'the sort arrow moved',
          ].filter(Boolean);
          return `${route} "${one.label}": ${faults.join(', ')}`;
        }),
    );

    expect(wrong, `with the pointer on the funnel:\n  ${wrong.join('\n  ')}`).toEqual([]);
  });
});
