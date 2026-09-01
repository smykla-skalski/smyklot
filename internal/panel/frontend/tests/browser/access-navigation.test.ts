/**
 * Access is one sidebar heading over two addressable rows.
 *
 * These used to be tabs inside the page. Moving between them rebuilt the page
 * underneath, showed loading rows again, and could take seconds even after both
 * datasets had already been read. The sidebar now owns the route switch, while
 * one mounted management view keeps both query results and the reader's local
 * table state.
 *
 * The edge presses are deliberate. The sidebar draws the pressed ground across
 * the entire rounded row, so every point inside that ground has to activate the
 * link.
 */
import type { Locator, Page } from 'playwright-core';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';

import { startPanel, visit, type Panel } from './harness';

const VIEWPORT = { width: 1500, height: 950 };
const MAX_SWITCH_MS = 750;

let panel: Panel;
let page: Page;
let invitationMs = Infinity;
let usersMs = Infinity;
const apiCalls: string[] = [];
let keptManagementView = false;
let historyDefaultsNavigated = false;
let rootAuditNavigated = false;
let plainHoverVisible = false;
let plainPressVisible = false;
let selectedHoverVisible = false;
let selectedPressVisible = false;

beforeAll(async () => {
  panel = await startPanel();
  page = await panel.browser.newPage({ viewport: VIEWPORT });
  await visit(page, `${panel.origin}/i/${panel.account}/access/users`, {
    ready: '#user-management-heading',
  });
  await page.locator('.user-management').evaluate((element) => {
    element.setAttribute('data-navigation-probe', 'kept');
  });
  const plainPointer = await inspectPointerStyles(page, sidebarLink(page, 'Repositories'), false);
  const selectedPointer = await inspectPointerStyles(page, sidebarLink(page, 'Users'), true);
  plainHoverVisible = plainPointer.hover;
  plainPressVisible = plainPointer.press;
  selectedHoverVisible = selectedPointer.hover;
  selectedPressVisible = selectedPointer.press;

  const watch = (request: { url: () => string }): void => {
    const pathname = new URL(request.url()).pathname;
    if (pathname.startsWith('/api/')) apiCalls.push(pathname);
  };
  page.on('request', watch);
  try {
    invitationMs = await pressEdge(
      page,
      sidebarLink(page, 'Invitations'),
      `/i/${panel.account}/access/invitations`,
      'Invitations',
      'right',
    );
    usersMs = await pressEdge(
      page,
      sidebarLink(page, 'Users'),
      `/i/${panel.account}/access/users`,
      'Users',
      'left',
    );
  } finally {
    page.off('request', watch);
  }

  keptManagementView =
    (await page.locator('.user-management').getAttribute('data-navigation-probe')) === 'kept';

  await sidebarLink(page, 'Audit').click();
  await page.waitForURL((url) => url.pathname === `/i/${panel.account}/history/audit`);
  historyDefaultsNavigated = true;

  const rootInstallation = await panel.browser.newPage({ viewport: VIEWPORT });
  try {
    await visit(rootInstallation, `${panel.origin}/root/installations/${panel.account}/settings`, {
      ready: '#root-page-heading',
    });
    /* By address, not by word: the console's own Audit row and this installation's
       carry the same label, one above the other, and only the address tells them
       apart. */
    const installationAudit = `/root/installations/${panel.account}/history/audit`;
    await rootInstallation.locator(`.tree a.tree-row[href="${installationAudit}"]`).click();
    await rootInstallation.waitForURL((url) => url.pathname === installationAudit);
    rootAuditNavigated = true;
  } finally {
    await rootInstallation.close();
  }
}, 120_000);

afterAll(async () => {
  await page?.close();
  await panel?.close();
});

function sidebarLink(target: Page, name: string): Locator {
  return target
    .getByRole('navigation', { name: 'Pages' })
    .locator('a.tree-row')
    .filter({ hasText: name })
    .first();
}

async function pressEdge(
  target: Page,
  link: Locator,
  pathname: string,
  heading: string,
  edge: 'left' | 'right',
): Promise<number> {
  const box = await link.boundingBox();
  if (box === null) throw new Error(`${heading} has no pressable sidebar box`);
  const started = Date.now();
  await link.click({
    position: {
      x: edge === 'left' ? 2 : box.width - 2,
      y: box.height / 2,
    },
  });
  await target.waitForURL((url) => url.pathname === pathname);
  await target.getByRole('heading', { name: heading, exact: true }).waitFor({ state: 'visible' });
  /* The first row, not the table: both of these pages are object lists now, and the
     timing this file measures is how long the ROWS take to arrive - a heading is drawn
     from the route alone and would say nothing about the read behind it. */
  await target.locator('.object-list > li').first().waitFor({ state: 'visible' });
  return Date.now() - started;
}

async function inspectPointerStyles(
  target: Page,
  link: Locator,
  selected: boolean,
): Promise<{ hover: boolean; press: boolean }> {
  const box = await link.boundingBox();
  if (box === null) throw new Error('Sidebar link has no box');
  await target.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await target.waitForTimeout(100);
  const hover = await pointerStyleVisible(link, false, selected);

  await target.mouse.down();
  await target.waitForTimeout(100);
  const press = await pointerStyleVisible(link, true, selected);
  await target.mouse.move(box.x + box.width + 20, box.y + box.height + 20);
  await target.mouse.up();
  return { hover, press };
}

function pointerStyleVisible(link: Locator, pressed: boolean, selected: boolean): Promise<boolean> {
  return link.evaluate(
    (element, state) => {
      const label = element.querySelector<HTMLElement>('.t');
      const glyph = element.querySelector<HTMLElement>('.gi');
      const thumb = element.closest('.tree')?.querySelector<HTMLElement>('.nav-thumb');
      if (label === null || glyph === null || thumb === null || thumb === undefined) return false;
      const linkStyle = getComputedStyle(element);
      const labelStyle = getComputedStyle(label);
      const thumbStyle = getComputedStyle(thumb);
      /* THE INK DIPS, THE BOX STAYS. A row that moved its own box during
         pointerdown moved its top edge out from under the cursor and the
         mouseup landed elsewhere - the press painted and no click fired. So the
         anchor is pinned at every state and the pixel is travelled by its
         direct children, which is where this now reads it. */
      const boxStill = linkStyle.translate === 'none' && linkStyle.transform === 'none';
      const groundVisible = state.selected
        ? thumbStyle.display !== 'none' && thumbStyle.backgroundColor !== 'rgba(0, 0, 0, 0)'
        : linkStyle.backgroundColor !== 'rgba(0, 0, 0, 0)';
      return (
        (!state.pressed || element.matches(':active')) &&
        boxStill &&
        /* A selected row draws no ground of its own at any state: the thumb is
           its ground, and a second fill over it reads as a well. */
        (!state.selected || linkStyle.backgroundColor === 'rgba(0, 0, 0, 0)') &&
        (!state.selected || linkStyle.borderRadius !== '0px') &&
        /* The selected thumb is a RAISED surface, not a 1px hard edge: it lifts
           on hover, throws a shadow, and lands into a crease on the press. */
        (!state.selected ||
          (state.pressed
            ? thumbStyle.boxShadow.includes('inset')
            : thumbStyle.boxShadow.split(',').length >= 2)) &&
        (!state.pressed || labelStyle.translate === '0px 1px') &&
        (!state.pressed || getComputedStyle(glyph).translate === '0px 1px') &&
        (!state.pressed || state.selected || thumbStyle.translate === 'none') &&
        /* Read as a NUMBER, because the thumb eases its travel: the row's ink lands
           instantly and the ground under it takes the press duration to arrive, so a
           reading taken 100ms in caught 0.998757px and an equality check on `0px 1px`
           failed a thumb that was doing exactly what it should. */
        (!state.pressed ||
          !state.selected ||
          Math.abs(Number.parseFloat(thumbStyle.translate.split(' ')[1] ?? '') - 1) < 0.05) &&
        groundVisible
      );
    },
    { pressed, selected },
  );
}

describe('Access sidebar navigation [Integration]', () => {
  it('keeps one mounted management view between leaves', () => {
    expect(keptManagementView).toBe(true);
  });

  it('shows already-read data without a multi-second route wait', () => {
    expect(invitationMs, 'Invitations took too long to appear').toBeLessThan(MAX_SWITCH_MS);
    expect(usersMs, 'Users took too long to reappear').toBeLessThan(MAX_SWITCH_MS);
  });

  it('does not ask the API again while switching leaves', () => {
    expect(apiCalls).toEqual([]);
  });

  it('keeps the hit target still while its complete visual row responds', () => {
    expect(plainHoverVisible, 'ordinary hover').toBe(true);
    expect(plainPressVisible, 'ordinary press').toBe(true);
    expect(selectedHoverVisible, 'selected hover').toBe(true);
    expect(selectedPressVisible, 'selected press').toBe(true);
  });

  it('opens default History leaves from outside History', () => {
    expect(historyDefaultsNavigated, 'workspace History').toBe(true);
    expect(rootAuditNavigated, 'Root installation Audit').toBe(true);
  });
});
