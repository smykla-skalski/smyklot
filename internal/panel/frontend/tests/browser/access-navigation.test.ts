/**
 * Access is one sidebar section with two addressable leaves.
 *
 * These used to be tabs inside the page. Moving between them rebuilt the page
 * underneath, showed loading rows again, and could take seconds even after both
 * datasets had already been read. The sidebar now owns the route switch, while
 * one mounted management view keeps both query results and the reader's local
 * table state.
 *
 * The edge presses are deliberate. The sidebar draws the pressed ground across
 * the entire rounded row, so every point inside that ground has to activate the
 * link. A parent row also has a destination: Access returns to Users rather than
 * looking pressable and doing nothing while Invitations is selected.
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
let legacyRedirected = false;
let hoverStyleVisible = false;
let pressStyleVisible = false;

beforeAll(async () => {
  panel = await startPanel();
  page = await panel.browser.newPage({ viewport: VIEWPORT });
  await visit(page, `${panel.origin}/i/${panel.account}/access/users`, {
    ready: '#user-management-heading',
  });
  await page.locator('.user-management').evaluate((element) => {
    element.setAttribute('data-navigation-probe', 'kept');
  });
  const plainPointer = await inspectPointerStyles(
    page,
    sidebarLink(page, 'Settings', 'tree-row'),
    false,
  );
  const selectedPointer = await inspectPointerStyles(
    page,
    sidebarLink(page, 'Users', 'tree-kid'),
    true,
  );
  hoverStyleVisible = plainPointer.hover && selectedPointer.hover;
  pressStyleVisible = plainPointer.press && selectedPointer.press;

  const watch = (request: { url: () => string }): void => {
    const pathname = new URL(request.url()).pathname;
    if (pathname.startsWith('/api/')) apiCalls.push(pathname);
  };
  page.on('request', watch);
  try {
    invitationMs = await pressEdge(
      page,
      sidebarLink(page, 'Invitations', 'tree-kid'),
      `/i/${panel.account}/access/invitations`,
      'Invitations',
      'right',
    );
    usersMs = await pressEdge(
      page,
      sidebarLink(page, 'Access', 'tree-row'),
      `/i/${panel.account}/access/users`,
      'Users',
      'left',
    );
  } finally {
    page.off('request', watch);
  }

  keptManagementView =
    (await page.locator('.user-management').getAttribute('data-navigation-probe')) === 'kept';

  const legacy = await panel.browser.newPage({ viewport: VIEWPORT });
  try {
    await legacy.goto(`${panel.origin}/i/${panel.account}/users`, {
      waitUntil: 'domcontentloaded',
    });
    await legacy.waitForURL((url) => url.pathname === `/i/${panel.account}/access/users`);
    await legacy.goto(`${panel.origin}/root/installations/${panel.account}/invitations`, {
      waitUntil: 'domcontentloaded',
    });
    await legacy.waitForURL(
      (url) => url.pathname === `/root/installations/${panel.account}/access/invitations`,
    );
    legacyRedirected = true;
  } finally {
    await legacy.close();
  }
}, 120_000);

afterAll(async () => {
  await page?.close();
  await panel?.close();
});

function sidebarLink(target: Page, name: string, className: string): Locator {
  return target
    .getByRole('navigation', { name: 'Pages' })
    .locator(`a.${className}`)
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
  await target
    .locator('.table-region tbody tr:not(.virtual-spacer)')
    .first()
    .waitFor({ state: 'visible' });
  return Date.now() - started;
}

async function inspectPointerStyles(
  target: Page,
  link: Locator,
  selected: boolean,
): Promise<{ hover: boolean; press: boolean }> {
  const box = await link.boundingBox();
  if (box === null) throw new Error('Settings has no sidebar box');
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
      const surface = element.querySelector<HTMLElement>('.row-surface');
      const label = element.querySelector<HTMLElement>('.t');
      const thumb = element.closest('.tree')?.querySelector<HTMLElement>('.nav-thumb');
      if (surface === null || label === null || thumb === null || thumb === undefined) return false;
      const surfaceStyle = getComputedStyle(surface);
      const labelStyle = getComputedStyle(label);
      const thumbStyle = getComputedStyle(thumb);
      const groundVisible = state.selected
        ? thumbStyle.translate !== 'none'
        : surfaceStyle.backgroundColor !== 'rgba(0, 0, 0, 0)';
      return (
        (!state.pressed || element.matches(':active')) &&
        (!state.pressed || state.selected || surfaceStyle.translate === 'none') &&
        (!state.pressed || state.selected || surfaceStyle.boxShadow !== 'none') &&
        (!(state.selected || state.pressed) || labelStyle.translate !== 'none') &&
        groundVisible &&
        Number(labelStyle.zIndex) > Number(surfaceStyle.zIndex)
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

  it('keeps the label above the animated hover and press surfaces', () => {
    expect(hoverStyleVisible).toBe(true);
    expect(pressStyleVisible).toBe(true);
  });

  it('redirects old flat Access links to the canonical hierarchy', () => {
    expect(legacyRedirected).toBe(true);
  });
});
