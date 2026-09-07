import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Page, Request } from 'playwright-core';

import { startPanel, visit, type Panel } from './harness';

let panel: Panel;

beforeAll(async () => {
  panel = await startPanel();
});

afterAll(async () => {
  await panel?.close();
});

function runtimeUpdate(page: Page): Promise<Request> {
  return page.waitForRequest(
    (request) =>
      request.method() === 'PUT' &&
      new URL(request.url()).pathname === '/api/v1/root/runtime/settings',
  );
}

describe('Root runtime settings drafts', () => {
  it.each(
    [375, 1440].flatMap((width) =>
      (['light', 'dark'] as const).map((colorScheme) => ({ width, colorScheme })),
    ),
  )(
    'uses shared behavior choices and restores inheritance in $colorScheme at $width',
    async ({ width, colorScheme }) => {
      const page = await panel.browser.newPage({
        colorScheme,
        viewport: { width, height: 1100 },
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(8000);
      const writes: Request[] = [];
      page.on('request', (request) => {
        if (request.method() === 'PUT') writes.push(request);
      });
      try {
        await page.goto(`${panel.origin}/root/runtime/settings`, { waitUntil: 'domcontentloaded' });
        const card = page.getByRole('region', { name: 'Behavior', exact: true });
        await card.waitFor({ timeout: 30000 });
        expect(await page.locator('html').getAttribute('data-theme')).toBe(colorScheme);
        const label = 'Merge draft pull requests';
        expect(await card.getByRole('checkbox', { name: label }).count()).toBe(0);
        await card.getByRole('button', { name: 'Override another', exact: true }).click();
        const choice = card.getByRole('button', { name: label, exact: true });
        await choice.waitFor();
        await card.evaluate((node) => node.scrollIntoView({ block: 'center' }));
        await page.mouse.move(0, 0);
        await page.evaluate(() => document.fonts.ready);
        const geometry = await card.evaluate((node) => ({
          overflow: node.scrollWidth - node.clientWidth,
          controls: [...node.querySelectorAll('.setting-value-wrap button')].map((button) => {
            const bounds = button.getBoundingClientRect();
            const cardBounds = node.getBoundingClientRect();
            return {
              shared: button.classList.contains('btn'),
              label: Boolean(button.querySelector(':scope > .button-label')),
              height: bounds.height,
              contained: bounds.left >= cardBounds.left && bounds.right <= cardBounds.right,
            };
          }),
        }));
        expect(geometry.overflow).toBeLessThanOrEqual(1);
        expect(geometry.controls.length).toBeGreaterThan(2);
        for (const control of geometry.controls) {
          expect(control).toEqual({ shared: true, label: true, height: 34, contained: true });
        }
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (directory) {
          await mkdir(directory, { recursive: true });
          await card.screenshot({
            path: join(directory, `behavior-choices-${colorScheme}-${width}.png`),
          });
        }
        const resting = await choice.evaluate((node) => getComputedStyle(node).backgroundImage);
        await choice.hover();
        expect(await choice.evaluate((node) => getComputedStyle(node).backgroundImage)).not.toBe(
          resting,
        );
        await page.mouse.down();
        const pressed = await choice.evaluate((node) => ({
          active: node.matches(':active'),
          shadow: getComputedStyle(node).boxShadow,
          translate: getComputedStyle(node).translate,
        }));
        expect(pressed.active).toBe(true);
        expect(pressed.shadow).toContain('inset');
        expect(pressed.translate).toBe('0px 1px');
        await page.mouse.up();
        const input = card.getByRole('checkbox', { name: label, exact: true });
        await input.waitFor();
        expect(await input.isChecked()).toBe(false);
        expect(await choice.count()).toBe(0);
        await input.locator('..').click();
        await expect.poll(() => input.isChecked()).toBe(true);
        const row = card
          .locator('.policy-row')
          .filter({ has: page.getByRole('checkbox', { name: label, exact: true }) });
        if (directory) {
          await page.mouse.move(0, 0);
          await card.screenshot({
            path: join(directory, `behavior-managed-${colorScheme}-${width}.png`),
          });
        }
        await row.getByRole('button', { name: 'Reset', exact: true }).click();
        await expect.poll(() => input.count()).toBe(0);
        await expect
          .poll(() => page.getByRole('button', { name: 'Save', exact: true }).count())
          .toBe(0);
        await card.getByRole('button', { name: 'Override another', exact: true }).click();
        await choice.waitFor();
        await card.getByRole('button', { name: 'Cancel', exact: true }).click();
        if (colorScheme === 'dark' && width === 1440) {
          await page.goto(`${panel.origin}/workspace/${panel.account}/settings`, {
            waitUntil: 'domcontentloaded',
          });
          await card.waitFor({ timeout: 30000 });
          expect(await page.locator('html').getAttribute('data-theme')).toBe(colorScheme);
          await card.getByRole('button', { name: 'Override another', exact: true }).click();
          await card.evaluate((node) => node.scrollIntoView({ block: 'center' }));
          await page.mouse.move(0, 0);
          expect(await card.locator('.add-chip').count()).toBe(0);
          const options = card.locator('.setting-value-wrap button');
          expect(await options.count()).toBeGreaterThan(2);
          expect(
            await options.evaluateAll((nodes) =>
              nodes.every(
                (node) =>
                  node.classList.contains('btn') && node.querySelector(':scope > .button-label'),
              ),
            ),
          ).toBe(true);
          if (directory)
            await card.screenshot({
              path: join(directory, 'behavior-workspace-choices-dark-1440.png'),
            });
          await card.getByRole('button', { name: 'Cancel', exact: true }).click();
        }
        expect(writes).toHaveLength(0);
      } finally {
        await page.close();
      }
    },
  );

  it('places the automatic-work action below its copy when the card is narrow', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 375, height: 900 } });
    try {
      await visit(page, `${panel.origin}/root/runtime/settings`);
      const geometry = await page.locator('.emergency-card').evaluate((card) => {
        const copy = card.querySelector('.emergency-copy')!.getBoundingClientRect();
        const action = card.querySelector('button')!.getBoundingClientRect();
        const heading = card.querySelector('.group-name')!.getBoundingClientRect();
        const status = card.querySelector('.status-pill')!.getBoundingClientRect();
        const cardBox = card.getBoundingClientRect();
        return {
          actionTop: action.top,
          copyBottom: copy.bottom,
          actionHeight: action.height,
          actionRight: action.right,
          headingRight: heading.right,
          statusRight: status.right,
          cardRight: cardBox.right,
          scrollWidth: card.scrollWidth,
          clientWidth: card.clientWidth,
        };
      });
      expect(geometry.actionTop - geometry.copyBottom).toBeGreaterThanOrEqual(15);
      expect(geometry.actionHeight).toBe(34);
      expect(geometry.actionRight).toBeLessThan(geometry.cardRight);
      expect(geometry.headingRight).toBeLessThan(geometry.cardRight);
      expect(geometry.statusRight).toBeLessThan(geometry.cardRight);
      expect(geometry.scrollWidth).toBeLessThanOrEqual(geometry.clientWidth);
    } finally {
      await page.close();
    }
  });

  it('keeps behavior and formatting edits together and clears both when restored', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });
    try {
      await visit(page, `${panel.origin}/root/runtime/settings`);
      const prefix = page.getByLabel('Prefix', { exact: true });
      const originalPrefix = await prefix.inputValue();
      const width = page.getByLabel('Indent Width', { exact: true });
      const originalWidth = await width.inputValue();
      await prefix.fill('/bot');
      await width.fill(originalWidth === '4' ? '2' : '4');
      expect(await prefix.inputValue()).toBe('/bot');
      await page.reload({ waitUntil: 'domcontentloaded' });
      await expect.poll(() => prefix.inputValue()).toBe('/bot');
      expect(await width.inputValue()).not.toBe(originalWidth);
      await prefix.fill(originalPrefix);
      await width.fill(originalWidth);
      await expect.poll(() => page.locator('[data-unsaved="true"]').count()).toBe(0);
      await expect
        .poll(() => page.getByRole('button', { name: 'Save', exact: true }).count())
        .toBe(0);
    } finally {
      await page.close();
    }
  });

  it('persists raw input across routes and reloads, then saves the full document once', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });
    const crashes: string[] = [];
    const writes: Request[] = [];
    page.on('pageerror', (error) => crashes.push(error.message));
    page.on('request', (request) => {
      if (
        request.method() === 'PUT' &&
        new URL(request.url()).pathname === '/api/v1/root/runtime/settings'
      ) {
        writes.push(request);
      }
    });

    try {
      await page.goto(`${panel.origin}/root/runtime/settings`, { waitUntil: 'domcontentloaded' });

      const overrideButton = page.getByRole('button', {
        name: 'Override the deployment session lifetime',
      });
      await overrideButton.waitFor({ state: 'visible', timeout: 30_000 });
      await overrideButton.click();

      const amount = page.getByRole('textbox', { name: 'Session lifetime amount' });
      await amount.waitFor({ state: 'visible' });
      await page.getByRole('combobox', { name: 'Session lifetime unit' }).click();
      await page.getByRole('option', { name: 'hours', exact: true }).click();
      await amount.fill('2');
      expect(writes).toHaveLength(0);

      await page.getByRole('link', { name: 'Service health' }).click();
      await page.waitForURL((url) => url.pathname === '/root/runtime/service');
      await page.getByRole('link', { name: 'Service settings' }).click();
      await page.waitForURL((url) => url.pathname === '/root/runtime/settings');
      await expect.poll(() => amount.inputValue()).toBe('2');

      await page.reload({ waitUntil: 'domcontentloaded' });
      const amountBack = page.getByRole('textbox', {
        name: 'Session lifetime amount',
      });
      await amountBack.waitFor({ state: 'visible', timeout: 30_000 });
      expect(await amountBack.inputValue()).toBe('2');
      expect(writes).toHaveLength(0);

      const saved = runtimeUpdate(page);
      await page.getByRole('button', { name: 'Save', exact: true }).click();
      const savedRequest = await saved;
      expect(savedRequest.postDataJSON()).toEqual({
        bot_config: null,
        log_level: null,
        reaction_poll_interval_seconds: null,
        merge_after_ci_quiet_period_seconds: null,
        path_index_interval_seconds: null,
        session_ttl_seconds: 7_200,
        expected_revision: 0,
      });
      await page.getByText('Settings saved').waitFor({ state: 'visible' });
      expect(writes).toHaveLength(1);

      await page.reload({ waitUntil: 'domcontentloaded' });
      const savedAmount = page.getByRole('textbox', {
        name: 'Session lifetime amount',
      });
      await savedAmount.waitFor({ state: 'visible', timeout: 30_000 });
      expect(await savedAmount.inputValue()).toBe('2');

      const sessionRow = page
        .locator('.policy-row')
        .filter({ has: page.getByRole('textbox', { name: 'Session lifetime amount' }) });
      await sessionRow
        .getByRole('button', { name: 'Stop overriding - take the value from the deployment' })
        .click();
      expect(writes).toHaveLength(1);
      const restored = runtimeUpdate(page);
      await page.getByRole('button', { name: 'Save', exact: true }).click();
      const restoreRequest = await restored;
      expect(restoreRequest.postDataJSON()).toMatchObject({
        session_ttl_seconds: null,
        expected_revision: 1,
      });
      await page.getByText('From the deployment: 1 day').waitFor({ state: 'visible' });
      expect(writes).toHaveLength(2);
      expect(crashes).toEqual([]);
    } finally {
      await page.close();
    }
  });

  it('pauses and resumes automatic work without staging a settings draft', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });

    try {
      await page.goto(`${panel.origin}/root/runtime/settings`, { waitUntil: 'domcontentloaded' });
      const pause = page.getByRole('button', { name: 'Pause automatic work', exact: true });
      await pause.waitFor({ state: 'visible', timeout: 30_000 });

      const paused = runtimeUpdate(page);
      await pause.click();
      await page.getByRole('button', { name: 'Pause background work', exact: true }).click();
      const pausedBody = (await paused).postDataJSON();
      expect(pausedBody).toMatchObject({ background_work_paused: true });
      await page.getByText('Paused', { exact: true }).waitFor({ state: 'visible' });
      await page.getByText('Queued work is kept', { exact: false }).waitFor({ state: 'visible' });

      const resumed = runtimeUpdate(page);
      await page.getByRole('button', { name: 'Resume automatic work', exact: true }).click();
      const resumedBody = (await resumed).postDataJSON();
      expect(resumedBody).toMatchObject({
        background_work_paused: false,
        expected_revision: pausedBody.expected_revision + 1,
      });
      await page.getByText('Running', { exact: true }).waitFor({ state: 'visible' });
    } finally {
      await page.close();
    }
  });

  it('keeps invalid raw duration text and blocks Save before the wire', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });
    const writes: Request[] = [];
    page.on('request', (request) => {
      if (
        request.method() === 'PUT' &&
        new URL(request.url()).pathname === '/api/v1/root/runtime/settings'
      ) {
        writes.push(request);
      }
    });

    try {
      await page.goto(`${panel.origin}/root/runtime/settings`, { waitUntil: 'domcontentloaded' });
      await page.getByRole('button', { name: 'Override the deployment session lifetime' }).click();
      const amount = page.getByRole('textbox', { name: 'Session lifetime amount' });
      await amount.fill('1e');
      await page.getByRole('button', { name: 'Save', exact: true }).click();

      await page
        .getByText('Session lifetime must be between 1 minute and 30 days')
        .first()
        .waitFor();
      expect(writes).toHaveLength(0);
      expect(await amount.getAttribute('aria-invalid')).toBe('true');

      await page.getByRole('link', { name: 'Service health' }).click();
      await page.waitForURL((url) => url.pathname === '/root/runtime/service');
      await page.reload({ waitUntil: 'domcontentloaded' });
      await page.getByRole('link', { name: 'Service settings' }).click();
      await page.waitForURL((url) => url.pathname === '/root/runtime/settings');
      expect(
        await page.getByRole('textbox', { name: 'Session lifetime amount' }).inputValue(),
      ).toBe('1e');
      expect(writes).toHaveLength(0);
      await page.getByRole('button', { name: 'Discard' }).click();
    } finally {
      await page.close();
    }
  });
});

describe('Root Runtime routes', () => {
  it('holds the service and its store on one page, apart from the editable settings', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });

    try {
      await visit(page, `${panel.origin}/root/runtime/settings`, { ready: '#root-page-heading' });
      const runtimeKids = page.locator('.tree a.tree-row[href^="/root/runtime/"]');
      expect(await runtimeKids.locator('> .t').allTextContents()).toEqual([
        'Service health',
        'Service settings',
      ]);
      expect(await page.locator('#root-page-heading').innerText()).toBe('Service settings');
      expect(await page.getByRole('heading', { name: 'Credentials' }).count()).toBe(0);
      expect(await page.getByRole('heading', { name: 'Database', exact: true }).count()).toBe(0);

      await runtimeKids.filter({ hasText: 'Service health' }).click();
      await page.waitForURL((url) => url.pathname === '/root/runtime/service');
      await page.getByRole('heading', { name: 'Credentials' }).waitFor({ state: 'visible' });

      expect(await page.locator('#root-page-heading').innerText()).toBe('Service health');
      // The database is a card here, and the page does not repeat its own name.
      expect(await page.getByRole('heading', { name: 'Database', exact: true }).count()).toBe(1);
      expect(await page.getByRole('heading', { name: 'Service health', exact: true }).count()).toBe(
        1,
      );

      // The database's old address is kept, and lands on the page that absorbed it.
      await page.goto(`${panel.origin}/root/runtime/database`, { waitUntil: 'domcontentloaded' });
      await page.waitForURL((url) => url.pathname === '/root/runtime/service');
      expect(await page.locator('#root-page-heading').innerText()).toBe('Service health');
    } finally {
      await page.close();
    }
  });

  it('redirects the bare Runtime address to Service with its query', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });

    try {
      await page.goto(`${panel.origin}/root/runtime?from=section`, {
        waitUntil: 'domcontentloaded',
      });
      await page.waitForURL((url) => url.pathname === '/root/runtime/service');
      expect(new URL(page.url()).search).toBe('?from=section');
    } finally {
      await page.close();
    }
  });
});

/**
 * Addresses the panel has no route for, which come in two shapes.
 *
 * One the server refuses, answering with its own error document. One it serves - because
 * it decides from the decoded path, while the router matches on the raw one, so a
 * percent-encoded separator means the console to the server and nothing to the router.
 * The panel reads its route from the router now, so the second shape is where reading the
 * route and reading the address disagree, and it is the reason the getters fall back.
 */
describe('an address that resolves to nothing', () => {
  it('shows what happened when the server refuses it', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });

    try {
      await visit(page, `${panel.origin}/root/definitely-not-a-page`, {
        ready: '.error-body',
        mount: 5_000,
      });

      expect(await page.locator('body').innerText()).toContain('Not found');
      expect(new URL(page.url()).pathname).toBe('/root/definitely-not-a-page');
    } finally {
      await page.close();
    }
  });

  it('stays put when the server serves it and no route matches', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });

    try {
      await visit(page, `${panel.origin}/root%2Fworkspaces`, { mount: 5_000 });

      // Without the fallback the console does not know it is the console, and the
      // workspace resolver replaces this address with a workspace.
      expect(new URL(page.url()).pathname, 'the panel navigated away').toBe('/root%2Fworkspaces');
    } finally {
      await page.close();
    }
  });
});
