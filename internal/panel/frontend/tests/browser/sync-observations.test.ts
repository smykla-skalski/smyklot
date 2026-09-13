import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { SyncCell, SyncStatus } from '../../src/lib/types';
import { syncPlanSeed } from '../../dev/fixtures';
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
          await page.unroute('**/api/v1/targets/*/sync/status');
        }
      } finally {
        await page.close();
      }
    },
  );

  it.each(['light', 'dark'] as const)(
    'links a completed file action while sync continues in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
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
        await page.route('**/api/v1/targets/*/sync/plan', (route) =>
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
