import type { Page } from 'playwright-core';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';

import { addressOf, inLanes, startPanel, visit, type Panel } from './harness';

/**
 * Every page against the mock it was drawn from.
 *
 * The mock at `.bart/mocks/shell-redesign.html` is the design, and it is one file holding
 * every view - so this walks both sides and compares what each page IS: its title, the
 * sentence under it, the sections it is made of and the controls in its toolbar. Not
 * pixels; shape. A page that grew a section the mock does not have, or lost one it does,
 * is the kind of drift no screenshot comparison catches because nobody opens both.
 *
 * Served by the mock server the design work already uses. Skipped, loudly, when that
 * server is not running - a parity check that quietly measures nothing is worse than one
 * that says it could not run.
 */

const MOCK = process.env.SMYKLOT_MOCK_ORIGIN ?? 'http://localhost:8123/shell-redesign.html';

/** Mock view, and the route in the app that has to match it. */
const PAGES: ReadonlyArray<readonly [view: string, route: string]> = [
  ['root-overview', 'root'],
  ['root-workspaces', 'root/workspaces'],
  ['root-queue', 'root/queue'],
  ['root-schedules', 'root/schedules'],
  ['root-audit', 'root/history/audit'],
  ['root-failures', 'root/history/failures'],
  ['root-users', 'root/access/users'],
  ['root-invitations', 'root/access/invitations'],
  ['root-service', 'root/runtime/service'],
  ['root-settings', 'root/runtime/settings'],
  ['repositories', 'workspace/repositories'],
  ['queue', 'workspace/queue'],
  ['labels', 'workspace/sync/labels'],
  ['settings', 'workspace/sync/settings'],
  ['rulesets', 'workspace/sync/rulesets'],
  ['files', 'workspace/sync/files'],
  // The separate plan page was replaced by automatic sync status and an inspector
  // (2026-09-05). Its legacy route is covered by plan-rows and sync-drafts.
  ['users', 'workspace/access/users'],
  ['invitations', 'workspace/access/invitations'],
  ['audit', 'workspace/history/audit'],
  ['failures', 'workspace/history/failures'],
  ['ws-settings', 'workspace/settings'],
  ['inbox', 'inbox'],
];

interface Shape {
  title: string;
  kicker: string;
  lead: string;
  sections: string[];
  toolbar: string[];
}

interface Gap {
  page: string;
  what: string;
  mock: string;
  app: string;
}

let panel: Panel;
const gaps: Gap[] = [];
let mockReachable = false;

beforeAll(async () => {
  panel = await startPanel();
  const probe = await panel.browser.newPage();
  try {
    const answer = await probe.goto(`${MOCK}?view=root-queue`, { timeout: 10_000 });
    mockReachable = answer !== null && answer.ok();
  } catch {
    mockReachable = false;
  } finally {
    await probe.close();
  }
  if (!mockReachable) return;

  const found = await inLanes(PAGES, async ([view, route]) => {
    const page = await panel.browser.newPage({ viewport: { width: 1440, height: 1000 } });
    try {
      await page.goto(`${MOCK}?view=${view}`);
      await page.waitForTimeout(700);
      const mock = await shapeOf(page);
      await visit(page, addressOf(panel, route));
      const app = await shapeOf(page);
      return compare(view, mock, app);
    } finally {
      await page.close();
    }
  });
  gaps.push(...found.flat());
}, 900_000);

afterAll(async () => {
  await panel?.close();
});

function shapeOf(page: Page): Promise<Shape> {
  return page.evaluate(() => {
    const words = (element: Element | null): string =>
      (element?.textContent ?? '').replaceAll(/\s+/gu, ' ').trim();
    const seen = (element: Element): boolean => element.checkVisibility();

    const heading = [...document.querySelectorAll('h1')].find((one) => seen(one)) ?? null;
    /* The head is one block, so the eyebrow and the sentence are found beside the
       heading rather than by a class each side happens to use - the mock calls its
       sentence `kind-head-sub` and the app `page-sub`, and reading only the app's name
       reported every page in the mock as having no sentence at all. */
    const head = heading?.closest('header, .page-head, .page-head-say') ?? null;
    const kicker = head === null ? '' : words(head.querySelector('.page-eyebrow, .eyebrow'));
    const lead =
      head === null
        ? ''
        : words(head.querySelector('.page-sub, .kind-head-sub, .page-lead, .lead'));

    /* EVERYTHING SCOPED TO THE VIEW ON SCREEN. The mock is one file holding every view
       with the rest hidden, so an unscoped `querySelector` answers from whichever view
       is written first - which is how the queue's toolbar came back as the repository
       matrix's. */
    const view = heading?.closest('main, .workspace, .app-shell') ?? document.body;

    const sections = [...view.querySelectorAll('.card-title, .group-name')]
      .filter((one) => seen(one))
      .map((one) => words(one))
      .filter((one) => one !== '');

    /* What a reader can drive the page with, by KIND rather than by label: a search, a
       segmented control, a menu, a switch. Labels differ between a mock's sample data
       and the app's; the set of instruments must not. */
    /* THE FILTER BAR AND THE HEAD'S ACTIONS ARE TWO DIFFERENT ROWS, and reading whichever
       came first in the document reported a page's Refresh button as its whole toolbar.
       Both are gathered, so a page that moved a control from one to the other says so. */
    const toolbar: string[] = [];
    const bars = [...view.querySelectorAll('.filter-bar, .toolbar, .page-actions')].filter((one) =>
      seen(one),
    );
    const has = (selector: string): boolean =>
      bars.some((bar) => bar.querySelector(selector) !== null);
    if (has('input[type="search"], .search-field')) toolbar.push('search');
    if (has('fieldset, .seg')) toolbar.push('segmented');
    if (has('.tools-trigger, .filter-trigger')) toolbar.push('tools');
    if (has('select, .value-select, [aria-haspopup]')) toolbar.push('menu');
    if (has('button.btn, a.btn')) toolbar.push('button');

    return { title: words(heading), kicker, lead, sections, toolbar };
  });
}

/**
 * WHAT A MOCK IS NOT, and so what this may not compare.
 *
 * The mock is drawn on sample data of its own, and it abbreviates: it shows two of the
 * eleven setting groups the service actually has, eleven rulesets where the fixture holds
 * two, and four items needing attention where the fixture holds two. Comparing section
 * NAMES on those pages asks the app to delete nine real groups because a sketch drew two.
 *
 * So the section list is compared only where the mock is drawing the whole page. Elsewhere
 * the page is named here with the reason, which is a claim somebody can check rather than
 * a silence.
 */
const ABBREVIATED = new Map<string, string>([
  ['root-overview', 'the attention count is the fixture, not the design'],
  ['root-settings', 'shows 2 of the 11 setting groups the service has'],
  ['root-service', 'omits the GitHub endpoint card'],
  ['root-schedules', 'omits the workspace-overrides card'],
  ['rulesets', 'draws 11 rulesets against a fixture holding 2'],
  ['repositories', 'names its own repository count in the sentence'],
  ['labels', 'draws its own label count'],
  ['files', 'draws its own file count'],
  ['root-failures', 'draws its own failure count'],
  ['failures', 'draws its own failure count'],
  ['audit', 'draws its own day headings'],
  ['root-audit', 'draws its own day headings'],
  ['users', 'draws its own member count'],
  ['root-users', 'draws its own member count'],
  ['ws-settings', 'shows 5 of the 12 setting groups a workspace has'],
  ['inbox', 'its cards are titled by the reason somebody typed, which is fixture text'],
]);

/**
 * WHERE THE PRODUCT HAS DECIDED SOMETHING THE MOCK DID NOT, with the decision.
 *
 * A mock is a drawing made on a day. Where a later decision overrules it, the right
 * record is the decision - not a silent exception and not a page nobody can make pass.
 */
const DECIDED = new Map<string, string>([
  [
    'root-queue',
    'three cards - needs a decision, running and waiting, done in the last day - rather ' +
      'than the mock’s one flat list (2026-09-01)',
  ],
  ['queue', 'automatic sync leaves no manual approvals in the fixture (2026-09-05)'],
  ['settings', 'the sentence says what managed means in full rather than the mock’s shorter line'],
]);

function compare(page: string, mock: Shape, app: Shape): Gap[] {
  const gaps: Gap[] = [];
  const say = (what: string, a: string, b: string): void => {
    if (a !== b) gaps.push({ page, what, mock: a, app: b });
  };
  say('title', mock.title, app.title);
  if (!ABBREVIATED.has(page) && !DECIDED.has(page)) {
    say('sentence under the title', mock.lead, app.lead);
    say('sections', mock.sections.join(' | '), app.sections.join(' | '));
  }
  /* THE MOCK'S INSTRUMENTS ARE THE FLOOR, not the whole set. A page may carry more than
     the design drew - the filter menu is the standing example, kept deliberately where
     the mock has none - and what would be a fault is a page missing something the design
     gave it. So this asks whether the app has everything the mock has, not whether the
     two lists are equal. */
  const missing = mock.toolbar.filter((one) => !app.toolbar.includes(one));
  if (missing.length > 0) {
    gaps.push({
      page,
      what: 'toolbar is missing what the design gives it',
      mock: mock.toolbar.join('+'),
      app: app.toolbar.join('+'),
    });
  }
  return gaps;
}

describe('every page against its mock [Integration]', () => {
  it('has the shape the design gives it', () => {
    expect(
      mockReachable,
      `the mock server is not answering at ${MOCK} — start it before this`,
    ).toBe(true);
    const byPage = new Map<string, Gap[]>();
    for (const gap of gaps) byPage.set(gap.page, [...(byPage.get(gap.page) ?? []), gap]);
    const report = [...byPage.entries()].map(([name, list]) =>
      [
        `  ${name}`,
        ...list.map(
          (one) => `      ${one.what}\n        mock: ${one.mock}\n        app:  ${one.app}`,
        ),
      ].join('\n'),
    );
    expect(gaps, `pages that differ from the mock:\n${report.join('\n')}`).toEqual([]);
  });
});
