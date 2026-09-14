import { checkResponse } from './sync-check-fixture';
import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { SyncCell, SyncStatus } from '../../src/lib/types';
import { queueSeeds, syncPlanSeed } from '../../dev/fixtures';
import { addressOf, startPanel, visit, type Panel } from './harness';

let panel: Panel;
beforeAll(async () => {
  panel = await startPanel();
});
afterAll(async () => {
  await panel?.close();
});

const proposalURL = 'https://github.com/smykla-skalski/api-gateway/pull/42';
const observed = new Date(Date.now() - 5 * 60_000).toISOString();
const scenes: Array<{ name: string; cells: SyncCell[]; words: string[]; latest: string | null }> = [
  {
    name: 'unchecked',
    latest: null,
    cells: Array.from({ length: 4 }, () => ({ state: 'unknown' })),
    words: ['Not checked', 'No repository observations yet'],
  },
  {
    name: 'outcomes',
    latest: observed,
    cells: [
      { state: 'in_step', observed_at: observed, observed_outcome: 'matched' },
      { state: 'applied', observed_at: observed, observed_outcome: 'applied' },
      { state: 'off' },
      {
        state: 'proposed',
        proposal_url: proposalURL,
        observed_at: observed,
        observed_outcome: 'proposed',
        reason: 'A pull request was opened. Its changes still need to be merged.',
      },
    ],
    words: ['Matched at last check', 'Changes applied', 'Pull request proposed'],
  },
  {
    name: 'declined',
    latest: observed,
    cells: [
      { state: 'in_step', observed_at: observed, observed_outcome: 'matched' },
      { state: 'applied', observed_at: observed, observed_outcome: 'applied' },
      { state: 'off' },
      {
        state: 'declined',
        proposal_url: proposalURL,
        observed_at: observed,
        observed_outcome: 'declined',
        reason:
          'The pull request was closed without merging. This change will not be proposed again automatically.',
      },
    ],
    words: ['Pull request declined', 'closed without merging'],
  },
  {
    name: 'outdated',
    latest: observed,
    cells: Array.from({ length: 4 }, (_, index) => ({
      proposal_url: index === 3 ? proposalURL : undefined,
      state: 'outdated',
      observed_at: observed,
      observed_outcome: index === 3 ? 'proposed' : 'matched',
      reason: 'Settings changed since the last check.',
    })),
    words: ['Needs a fresh check', 'Settings changed since the last check.'],
  },
  {
    name: 'interrupted',
    latest: observed,
    cells: [
      {
        state: 'check_failed',
        observed_at: observed,
        observed_outcome: 'failed',
        reason: 'GitHub could not be reached.',
      },
      {
        state: 'refused',
        observed_at: observed,
        observed_outcome: 'blocked',
        reason: 'Administration permission is required.',
      },
      {
        state: 'needs_sync',
        observed_at: observed,
        observed_outcome: 'different',
        reason: 'The repository differs from the saved settings.',
      },
      { state: 'off' },
    ],
    words: [
      'Last attempt failed',
      'Administration permission is required.',
      'Changes needed',
      'Sync disabled',
    ],
  },
];

describe('desktop repository observation evidence', () => {
  it.each(['light', 'dark'] as const)(
    'separates each observed outcome in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(7000);
      try {
        await page
          .context()
          .route(proposalURL, (route) =>
            route.fulfill({ contentType: 'text/html', body: '<h1>Proposal destination</h1>' }),
          );
        await page.route('**/api/v1/targets/*/sync/plan', (route) =>
          route.fulfill({ json: { plan: null } }),
        );
        for (const scene of scenes) {
          const status: SyncStatus = {
            latest_observed_at: scene.latest,
            repositories: [
              {
                repository: 'api-gateway',
                cells: {
                  labels: scene.cells[0]!,
                  settings: scene.cells[1]!,
                  rulesets: scene.cells[2]!,
                  files: scene.cells[3]!,
                },
              },
            ],
          };
          await page.route('**/api/v1/targets/*/sync/status', (route) =>
            route.fulfill({ json: status }),
          );
          await visit(page, addressOf(panel, 'workspace/sync'));
          await page.getByRole('button', { name: /Account menu for/u }).click();
          await page
            .getByRole('radio', {
              name: `${colorScheme === 'light' ? 'Light' : 'Dark'} theme`,
              exact: true,
            })
            .locator('..')
            .click();
          await page.keyboard.press('Escape');
          await page.getByRole('button', { name: 'api-gateway sync details', exact: true }).click();
          for (const word of scene.words)
            expect(await page.getByText(word, { exact: false }).count()).toBeGreaterThan(0);
          expect(await page.locator('.sync-repo-detail').innerText()).not.toContain('undefined');
          expect(
            await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
          ).toBe(true);
          const directory = process.env.SMYKLOT_SYNC_OBSERVATION_SCREENSHOTS;
          if (scene.cells.some((cell) => cell.proposal_url)) {
            const link = page.locator('.sync-repo-detail').getByRole('link', {
              name: scene.name === 'outdated' ? 'View earlier pull request' : 'View pull request',
              exact: true,
            });
            expect(await link.getAttribute('href')).toBe(proposalURL);
            const [popup] = await Promise.all([page.waitForEvent('popup'), link.click()]);
            await popup.getByRole('heading', { name: 'Proposal destination' }).waitFor();
            expect(popup.url()).toBe(proposalURL);
            await popup.close();
          }
          if (directory && scene.cells.some((cell) => cell.proposal_url)) {
            await mkdir(directory, { recursive: true });
            await page.screenshot({
              path: join(directory, `F33-proposal-${scene.name}-${colorScheme}.png`),
              fullPage: true,
            });
          }
          if (scene.name === 'declined') {
            const detail = page.locator('.sync-repo-detail');
            expect(await detail.innerText()).toContain('reopen the pull request on GitHub');
            expect(await detail.innerText()).toContain('use Check now');
            const captureRecovery = async (state: string) => {
              await page.mouse.move(1900, 20);
              if (directory)
                await page.screenshot({
                  path: join(directory, `F33-recovery-${state}-${colorScheme}.png`),
                  fullPage: true,
                });
            };
            await captureRecovery('guidance');
            await detail.getByRole('link', { name: 'Review shared files', exact: true }).click();
            await page.getByRole('heading', { name: 'Shared files', exact: true }).waitFor();
            expect(new URL(page.url()).pathname).toMatch(/\/sync\/files$/u);
            await page.goBack();
            await page.getByRole('heading', { name: 'Sync status', exact: true }).waitFor();
            await page
              .getByRole('button', { name: 'api-gateway sync details', exact: true })
              .click();
            const check = {
              ...queueSeeds((offset) => new Date(Date.now() + offset).toISOString())[0]!,
              id: 'scan:desktop-check',
              kind: 'sync_scan' as const,
              title: 'Check repository settings',
              summary: 'Reading repository settings from GitHub',
              progress_current: 0,
              progress_total: 0,
            };
            await page.route('**/api/v1/targets/*/sync/checks/scan%3Adesktop-check', (route) =>
              route.fulfill({ json: checkResponse(check) }),
            );
            let requestKey = '';
            await page.route('**/api/v1/targets/*/sync/requests/check/*', (route) =>
              route.fulfill({
                json: {
                  target_id: '2001',
                  acceptance: {
                    action: 'check',
                    request_key: requestKey,
                    reason: 'Check sync from the status view',
                    check_id: check.id,
                    accepted_at: new Date().toISOString(),
                  },
                  observation_started_at: new Date().toISOString(),
                  observed_at: new Date().toISOString(),
                  comparison: null,
                  plan: null,
                  execution: checkResponse(check).execution,
                  check: { available: true },
                  dispatch: null,
                },
              }),
            );
            let requests = 0;
            await page.route('**/api/v1/targets/*/sync/run-now', (route) => {
              requests++;
              requestKey = route.request().postDataJSON().request_key;
              expect(route.request().method()).toBe('POST');
              expect(route.request().postDataJSON().reason).toBe('Check sync from the status view');
              return route.fulfill({ json: { status: 'check_accepted', check_id: check.id } });
            });
            await page.getByRole('button', { name: 'Check now', exact: true }).click();
            await page
              .getByText('Your request was accepted. Open it to see what happened.', {
                exact: true,
              })
              .waitFor();
            expect(requests).toBe(1);
            await captureRecovery('queued');
            await page.getByRole('link', { name: 'View request', exact: true }).click();
            await page.getByRole('dialog', { name: 'Sync request', exact: true }).waitFor();
            const inspector = page.getByRole('dialog');
            await inspector.getByRole('heading', { name: 'Sync request', exact: true }).waitFor();
            expect(new URL(page.url()).pathname).toContain(`/sync/request/check/${requestKey}`);
            await captureRecovery('check-running');
            await page.reload();
            await inspector.getByRole('heading', { name: 'Sync request', exact: true }).waitFor();
            check.summary = 'Checked 4 repository settings: 4 matched.';
            check.state = 'succeeded';
            await inspector
              .getByRole('region', { name: 'Check outcome', exact: true })
              .getByText(check.summary, { exact: true })
              .waitFor({ timeout: 20_000 });
            await captureRecovery('check-complete');
            await inspector.getByRole('button', { name: 'Close request', exact: true }).click();
            await page.waitForURL(/\/sync$/u);
            await page.unroute('**/api/v1/targets/*/sync/checks/scan%3Adesktop-check');
            status.repositories[0]!.cells.files = {
              state: 'in_step',
              observed_outcome: 'matched',
              observed_at: new Date().toISOString(),
            };
            status.latest_observed_at = status.repositories[0]!.cells.files.observed_at!;
            await page.reload();
            await page
              .getByRole('button', { name: 'api-gateway sync details', exact: true })
              .click();
            expect(await page.locator('.sync-repo-detail').innerText()).not.toContain(
              'reopen the pull request',
            );
            expect(await page.locator('.sync-repo-detail').innerText()).not.toContain(
              'Pull request declined',
            );
            await captureRecovery('refreshed');
            await page.unroute('**/api/v1/targets/*/sync/run-now');
          }
          await page.unroute('**/api/v1/targets/*/sync/status');
        }
      } finally {
        await page.close();
      }
    },
  );

  it.each(['light', 'dark'] as const)(
    'retains a selected result through completion and reload in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(7000);
      try {
        const plan = syncPlanSeed((offset) => new Date(Date.now() + offset).toISOString());
        const file = plan.actions.find((action) => action.kind === 'files')!;
        plan.state = 'applying';
        plan.actions = [
          { ...file, repository: 'api-gateway', state: 'applied', proposal_url: proposalURL },
          { ...file, repository: 'worker', state: 'pending' },
        ];
        plan.counts = { create: 2, update: 0, delete: 0 };
        let finished = false;
        await page.route('**/api/v1/targets/*/sync/plan', (route) =>
          route.fulfill({ json: { plan: finished ? null : plan } }),
        );
        await page.route('**/api/v1/targets/*/sync/plans/*', (route) =>
          route.fulfill({ json: { plan } }),
        );
        await page
          .context()
          .route(proposalURL, (route) =>
            route.fulfill({ contentType: 'text/html', body: '<h1>Proposal destination</h1>' }),
          );
        await visit(page, addressOf(panel, 'workspace/sync'));
        await page.getByRole('button', { name: /Account menu for/u }).click();
        await page
          .getByRole('radio', {
            name: `${colorScheme === 'light' ? 'Light' : 'Dark'} theme`,
            exact: true,
          })
          .locator('..')
          .click();
        await page.keyboard.press('Escape');
        await page.getByRole('button', { name: 'View changes', exact: true }).first().click();
        await page
          .getByRole('heading', { name: '1 of 2 changes processed', exact: true })
          .waitFor();
        expect(page.url()).toContain(`/sync/plan/${encodeURIComponent(plan.id)}`);
        const group = page.locator('.repo-row').filter({ hasText: 'api-gateway' });
        if ((await group.getAttribute('aria-expanded')) !== 'true') await group.click();
        const link = page.getByRole('link', { name: 'View pull request', exact: true });
        expect(await link.getAttribute('href')).toBe(proposalURL);
        const [popup] = await Promise.all([page.waitForEvent('popup'), link.click()]);
        await popup.getByRole('heading', { name: 'Proposal destination' }).waitFor();
        expect(popup.url()).toBe(proposalURL);
        await popup.close();
        const directory = process.env.SMYKLOT_SYNC_OBSERVATION_SCREENSHOTS;
        if (directory) {
          await mkdir(directory, { recursive: true });
          await page.screenshot({
            path: join(directory, `F33-proposal-action-${colorScheme}.png`),
            fullPage: false,
          });
        }
        finished = true;
        plan.state = 'applied';
        plan.actions = plan.actions.map((action) => ({ ...action, state: 'applied' }));
        await page.reload();
        await page.getByRole('heading', { name: '2 changes processed', exact: true }).waitFor();
        const retainedGroup = page.locator('.repo-row').filter({ hasText: 'api-gateway' });
        if ((await retainedGroup.getAttribute('aria-expanded')) !== 'true')
          await retainedGroup.click();
        expect(
          await page
            .getByRole('link', { name: 'View pull request', exact: true })
            .getAttribute('href'),
        ).toBe(proposalURL);
        if (directory)
          await page.screenshot({
            path: join(directory, `F33-history-completed-${colorScheme}.png`),
            fullPage: false,
          });
        await page.getByRole('button', { name: 'Close sync details', exact: true }).click();
        await page.waitForURL((url) => !url.pathname.includes('/sync/plan/'));
        await page.goBack();
        await page.getByRole('heading', { name: '2 changes processed', exact: true }).waitFor();
        await page.route('**/api/v1/targets/*/sync/plans/*', (route) =>
          route.fulfill({
            status: 404,
            json: { error: { code: 'not_found', message: 'Sync result not found' } },
          }),
        );
        await page.reload();
        await page.getByText('These changes are unavailable', { exact: true }).waitFor();
        expect(
          await page.getByRole('heading', { name: '2 changes processed', exact: true }).count(),
        ).toBe(0);
        if (directory)
          await page.screenshot({
            path: join(directory, `F33-history-missing-${colorScheme}.png`),
            fullPage: false,
          });
      } finally {
        await page.close();
      }
    },
  );

  it('invalidates an existing observation after a real settings save', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1920, height: 1200 } });
    try {
      await visit(page, addressOf(panel, 'workspace/sync/labels'));
      const before = await page.evaluate(
        async () => (await (await fetch('/api/v1/targets/2001/sync/status')).json()) as SyncStatus,
      );
      const matching = before.repositories.find((row) => row.cells.labels.state === 'in_step');
      expect(matching).toBeDefined();
      const checkbox = page.getByRole('checkbox', { name: 'Delete unlisted labels' });
      await page.locator('label.switch').filter({ has: checkbox }).click();
      await page.getByRole('button', { name: 'Save', exact: true }).click();
      await page.getByText('Settings saved', { exact: true }).waitFor();
      const after = await page.evaluate(
        async () => (await (await fetch('/api/v1/targets/2001/sync/status')).json()) as SyncStatus,
      );
      const changed = after.repositories.find((row) => row.repository === matching?.repository);
      expect(changed?.cells.labels.state).toBe('outdated');
      expect(changed?.cells.labels.observed_at).toBe(matching?.cells.labels.observed_at);
      expect(after.latest_observed_at).toBe(before.latest_observed_at);
    } finally {
      await page.close();
    }
  });
});

it.each(['light', 'dark'] as const)(
  'pages sync history and returns to the same page in %s',
  async (colorScheme) => {
    const page = await panel.browser.newPage({
      viewport: { width: 1920, height: 1200 },
      colorScheme,
      reducedMotion: 'reduce',
    });
    page.setDefaultTimeout(7000);
    let releaseHistory = () => {};
    const gate = new Promise<void>((resolve) => {
      releaseHistory = resolve;
    });
    const directory = process.env.SMYKLOT_SYNC_OBSERVATION_SCREENSHOTS;
    try {
      await visit(page, addressOf(panel, 'workspace/sync'));
      await page.getByRole('button', { name: /Account menu for/u }).click();
      await page
        .getByRole('radio', {
          name: `${colorScheme === 'light' ? 'Light' : 'Dark'} theme`,
          exact: true,
        })
        .locator('..')
        .click();
      await page.keyboard.press('Escape');
      await page.route(
        '**/api/v1/targets/*/sync/plans?*',
        async (route) => {
          await gate;
          await route.continue();
        },
        { times: 1 },
      );
      await page.getByRole('button', { name: 'Sync history', exact: true }).click();
      await page.getByText('Loading sync history…', { exact: true }).waitFor();
      if (directory)
        await page.screenshot({
          path: join(directory, `F33-history-list-loading-${colorScheme}.png`),
          fullPage: false,
        });
      releaseHistory();
      const table = page.getByRole('table', { name: 'Sync history', exact: true });
      await expect.poll(() => table.locator('tbody tr').count()).toBe(20);
      if (directory)
        await page.screenshot({
          path: join(directory, `F33-history-list-first-${colorScheme}.png`),
          fullPage: true,
        });
      await page.getByRole('button', { name: 'Next', exact: true }).click();
      await expect.poll(() => table.locator('tbody tr').count()).toBe(5);
      expect(await page.getByRole('button', { name: 'Next', exact: true }).isDisabled()).toBe(true);
      if (directory)
        await page.screenshot({
          path: join(directory, `F33-history-list-last-${colorScheme}.png`),
          fullPage: true,
        });
      const firstHref = await table
        .getByRole('link', { name: 'View result', exact: true })
        .first()
        .getAttribute('href');
      await table.getByRole('link', { name: 'View result', exact: true }).first().click();
      await page.getByRole('button', { name: 'Close sync details', exact: true }).waitFor();
      expect(page.url()).toContain(firstHref!);
      await page.getByRole('heading', { name: '14 changes processed', exact: true }).waitFor();
      if (directory)
        await page.screenshot({
          path: join(directory, `F33-history-list-inspector-${colorScheme}.png`),
          fullPage: false,
        });
      await page.getByRole('button', { name: 'Close sync details', exact: true }).click();
      await page.waitForURL((url) => url.pathname.endsWith('/sync/history'));
      await expect.poll(() => table.locator('tbody tr').count()).toBe(5);
      expect(
        await table
          .getByRole('link', { name: 'View result', exact: true })
          .first()
          .getAttribute('href'),
      ).toBe(firstHref);
      await page.getByRole('button', { name: 'Previous', exact: true }).click();
      await expect.poll(() => table.locator('tbody tr').count()).toBe(20);
      await page.getByRole('combobox', { name: 'Sync history per page', exact: true }).click();
      await page.getByRole('option', { name: '10', exact: true }).click();
      await expect.poll(() => table.locator('tbody tr').count()).toBe(10);
      expect(await page.getByRole('button', { name: 'Previous', exact: true }).isDisabled()).toBe(
        true,
      );
      if (directory)
        await page.screenshot({
          path: join(directory, `F33-history-list-size-${colorScheme}.png`),
          fullPage: true,
        });
      await page.route('**/api/v1/targets/*/sync/plans?*', (route) =>
        route.fulfill({
          status: 503,
          json: { error: { code: 'unavailable', message: 'History is temporarily unavailable' } },
        }),
      );
      await page.reload();
      await page.getByText('Sync history could not be loaded', { exact: true }).waitFor();
      if (directory)
        await page.screenshot({
          path: join(directory, `F33-history-list-error-${colorScheme}.png`),
          fullPage: false,
        });
      await page.unroute('**/api/v1/targets/*/sync/plans?*');
      await page.getByRole('button', { name: 'Try again', exact: true }).click();
      await expect.poll(() => table.locator('tbody tr').count()).toBe(20);
      await page.route('**/api/v1/targets/*/sync/plans?*', (route) =>
        route.fulfill({ json: { items: [], total: 0, next_cursor: null } }),
      );
      await page.reload();
      await page.getByText('No sync results here', { exact: true }).waitFor();
      if (directory)
        await page.screenshot({
          path: join(directory, `F33-history-list-empty-${colorScheme}.png`),
          fullPage: false,
        });
    } finally {
      releaseHistory();
      await page.close();
    }
  },
);
