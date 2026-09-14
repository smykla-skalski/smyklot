import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { addressOf, startPanel, visit, type Panel } from './harness';

let panel: Panel;
beforeAll(async () => {
  panel = await startPanel();
});
afterAll(async () => {
  await panel?.close();
});

describe('desktop read-only confirmation', () => {
  it.each(['light', 'dark'] as const)(
    'confirms after command permission loss in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(10_000);
      let release = () => {};
      try {
        const pending = {
          action: 'check',
          request_key: 'viewer-original-check',
          reason: 'Verify labels',
        };
        await page.addInitScript((input) => {
          const get = Storage.prototype.getItem;
          let seeded = false;
          Storage.prototype.getItem = function (key) {
            if (!seeded && key.startsWith('smyklot.sync-check.v1:')) {
              this.setItem(key, JSON.stringify(input));
              seeded = true;
            }
            return get.call(this, key);
          };
        }, pending);
        await page.route('**/api/v1/targets', async (route) => {
          const response = await route.fetch();
          const body = await response.json();
          for (const target of body.targets) target.effective_role = 'viewer';
          await route.fulfill({ response, json: body });
        });
        let mutations = 0;
        page.on('request', (request) => {
          if (new URL(request.url()).pathname.includes('/sync/') && request.method() !== 'GET')
            mutations++;
        });
        let mode = 'missing';
        let reads = 0;
        const held = new Promise<void>((resolve) => {
          release = resolve;
        });
        await page.route(
          '**/api/v1/targets/2001/sync/requests/check/viewer-original-check',
          async (route) => {
            reads++;
            if (mode === 'missing') {
              await route.fulfill({
                status: 404,
                json: { error: { code: 'not_found', message: 'Accepted request not found' } },
              });
              return;
            }
            await held;
            await route.fulfill({
              json: {
                target_id: '2001',
                acceptance: {
                  ...pending,
                  check_id: 'original-check',
                  accepted_at: '2026-09-14T10:00:00Z',
                },
                observation_started_at: '2026-09-14T10:01:00Z',
                observed_at: '2026-09-14T10:01:00Z',
                comparison: null,
                plan: null,
                execution: null,
                check: { available: false },
                dispatch: null,
              },
            });
          },
        );
        await visit(page, addressOf(panel, 'workspace/sync'));
        const capture = async (scene: string) => {
          const directory = process.env.SMYKLOT_SYNC_OBSERVATION_SCREENSHOTS;
          if (!directory) return;
          await mkdir(directory, { recursive: true });
          await page.mouse.move(1900, 20);
          await page.screenshot({
            path: join(directory, `F33-confirm-viewer-${scene}-${colorScheme}.png`),
          });
        };
        const confirm = page.getByRole('button', { name: 'Confirm request', exact: true });
        await confirm.click();
        await page
          .getByText(
            'Your request is not recorded yet. It may still be arriving. Confirm again to check its status.',
            { exact: true },
          )
          .waitFor();
        expect(
          await page.getByRole('button', { name: 'Retry original request', exact: true }).count(),
        ).toBe(0);
        expect(mutations).toBe(0);
        await capture('missing');
        mode = 'accepted';
        await confirm.click();
        const confirming = page.getByRole('button', { name: 'Confirming…', exact: true });
        await confirming.waitFor();
        expect(await confirming.getAttribute('aria-disabled')).toBe('true');
        expect(await confirming.evaluate((element) => element === document.activeElement)).toBe(
          true,
        );
        await confirming.press('Enter');
        expect(reads).toBe(2);
        expect(mutations).toBe(0);
        await capture('loading');
        release();
        const link = page.getByRole('link', { name: 'View request', exact: true });
        await link.waitFor();
        expect(await link.evaluate((element) => element === document.activeElement)).toBe(true);
        expect(await link.getAttribute('href')).toContain(pending.request_key);
        expect(
          await page.evaluate(
            () =>
              Object.keys(sessionStorage).filter((key) => key.startsWith('smyklot.sync-check.v1:'))
                .length,
          ),
        ).toBe(0);
        expect(mutations).toBe(0);
        await capture('accepted');
      } finally {
        release();
        await page.close();
      }
    },
  );
});
