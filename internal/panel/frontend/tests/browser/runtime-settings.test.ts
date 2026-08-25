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
      await amount.fill('2');
      await page.getByRole('button', { name: 'Session lifetime unit' }).click();
      await page.getByRole('option', { name: 'hours' }).click();
      expect(writes).toHaveLength(0);

      await page.getByRole('link', { name: 'Service' }).click();
      await page.waitForURL((url) => url.pathname === '/root/runtime/service');
      await page.getByRole('link', { name: 'Settings' }).click();
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
        .getByRole('button', { name: 'Stop overriding - follow the deployment configuration' })
        .click();
      expect(writes).toHaveLength(1);
      const restored = runtimeUpdate(page);
      await page.getByRole('button', { name: 'Save', exact: true }).click();
      const restoreRequest = await restored;
      expect(restoreRequest.postDataJSON()).toMatchObject({
        session_ttl_seconds: null,
        expected_revision: 1,
      });
      await page.getByText('Follows the deployment - 1 day').waitFor({ state: 'visible' });
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
      await page
        .getByText('Queue items remain durable', { exact: false })
        .waitFor({ state: 'visible' });

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

      await page.getByRole('link', { name: 'Database' }).click();
      await page.waitForURL((url) => url.pathname === '/root/runtime/database');
      await page.reload({ waitUntil: 'domcontentloaded' });
      await page.getByRole('link', { name: 'Settings' }).click();
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
  it('orders and isolates service, database, and editable settings leaves', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });

    try {
      await visit(page, `${panel.origin}/root/runtime/settings`, { ready: '#root-page-heading' });
      const runtimeKids = page.locator('.tree-kids[data-label="Runtime"] .tree-kid');
      expect(await runtimeKids.locator('.row-visual > .t').allTextContents()).toEqual([
        'Service',
        'Database',
        'Settings',
      ]);
      expect(await page.locator('#root-page-heading').innerText()).toBe('Runtime settings');
      expect(await page.getByRole('heading', { name: 'Service and deployment' }).count()).toBe(0);
      expect(await page.getByRole('heading', { name: 'Database', exact: true }).count()).toBe(0);
      expect(await page.locator('.service-grid').count()).toBe(0);

      await runtimeKids.filter({ hasText: 'Service' }).click();
      await page.waitForURL((url) => url.pathname === '/root/runtime/service');
      await page.locator('#root-service').waitFor({ state: 'visible' });

      expect(await page.locator('#root-page-heading').innerText()).toBe('Service and deployment');
      expect(await page.getByRole('heading', { name: 'Service and deployment' }).count()).toBe(2);
      expect(await page.getByRole('heading', { name: 'Runtime', exact: true }).count()).toBe(0);
      expect(await page.getByRole('heading', { name: 'Database', exact: true }).count()).toBe(0);
      expect(await page.locator('.service-grid').count()).toBe(1);

      await runtimeKids.filter({ hasText: 'Database' }).click();
      await page.waitForURL((url) => url.pathname === '/root/runtime/database');
      await page.locator('#root-database').waitFor({ state: 'visible' });

      expect(await page.locator('#root-page-heading').innerText()).toBe('Database');
      expect(await page.getByRole('heading', { name: 'Database', exact: true }).count()).toBe(2);
      expect(await page.getByRole('heading', { name: 'Runtime', exact: true }).count()).toBe(0);
      expect(await page.getByRole('heading', { name: 'Service and deployment' }).count()).toBe(0);
      expect(await page.locator('.service-grid').count()).toBe(1);
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
      await visit(page, `${panel.origin}/root%2Finstallations`, { mount: 5_000 });

      // Without the fallback the console does not know it is the console, and the
      // workspace resolver replaces this address with an installation.
      expect(new URL(page.url()).pathname, 'the panel navigated away').toBe(
        '/root%2Finstallations',
      );
    } finally {
      await page.close();
    }
  });
});
