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

let panel: Panel;
let headings: Record<string, Heading | string>;

beforeAll(async () => {
  panel = await startPanel();
  headings = {};

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

      return {
        route,
        heading: await page.evaluate((): Heading | string => {
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
        }),
      };
    } finally {
      await page.close();
    }
  });

  for (const { route, heading } of read) headings[route] = heading;
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
