import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { afterAll, beforeAll, expect, it } from 'vitest';
import { addressOf, startPanel, visit, type Panel } from './harness';
let panel: Panel;
beforeAll(async () => {
  panel = await startPanel();
});
afterAll(async () => {
  await panel?.close();
});
it.each(['light', 'dark'] as const)(
  'explains required reasons and retains rejected drafts in %s',
  async (colorScheme) => {
    const page = await panel.browser.newPage({
      viewport: { width: 1920, height: 1200 },
      colorScheme,
      reducedMotion: 'reduce',
    });
    page.setDefaultTimeout(5000);
    const capture = async (scene: string) => {
      const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
      if (!directory) return;
      await mkdir(directory, { recursive: true });
      await page.screenshot({
        path: join(directory, `F19-${scene}-${colorScheme}.png`),
        animations: 'disabled',
      });
    };
    try {
      let submissions = 0;
      await page.route('**/api/v1/targets/*/schedule-requests', async (route) => {
        if (route.request().method() !== 'POST') return route.continue();
        submissions++;
        await route.fulfill({
          status: 400,
          contentType: 'application/json',
          body: JSON.stringify({
            error: { code: 'invalid_request', message: 'Explain how the requested timing helps.' },
          }),
        });
      });
      await visit(page, addressOf(panel, 'workspace/settings'), { ready: '#ws-timing' });
      await page.locator('#ws-timing summary').click();
      await page.locator('#ws-timing').getByRole('button', { name: 'Request a change' }).click();
      const dialog = page.getByRole('dialog', {
        name: 'Request a change to when Smyklot acts',
        exact: true,
      });
      const reason = dialog.getByLabel('Reason', { exact: true });
      expect(await reason.getAttribute('required')).not.toBeNull();
      expect(await reason.getAttribute('aria-describedby')).toBe('schedule-request-reason-help');
      await dialog
        .getByText(
          'Required. Explain what the current timing prevents and how this change would help.',
          { exact: true },
        )
        .waitFor();
      const cadence = dialog.getByRole('textbox', { name: 'How often', exact: true });
      await cadence.fill('42');
      await reason.fill('   ');
      expect(
        await dialog.getByRole('button', { name: 'Send request', exact: true }).isDisabled(),
      ).toBe(true);
      expect(submissions).toBe(0);
      await capture('request-required');
      await reason.fill('Run during our staffed support hours.');
      await dialog.getByRole('button', { name: 'Send request', exact: true }).click();
      await dialog.getByRole('alert').waitFor();
      expect(submissions).toBe(1);
      expect(await reason.inputValue()).toBe('Run during our staffed support hours.');
      expect(await cadence.inputValue()).toBe('42');
      await capture('request-rejected');
      await page.keyboard.press('Escape');
      await page.goto(`${panel.origin}/root/queue`);
      await page.getByRole('button', { name: 'Retry now', exact: true }).click();
      const retry = page.getByRole('dialog', { name: 'Retry now', exact: true });
      const exception = retry.getByLabel('Reason', { exact: true });
      expect(await exception.getAttribute('required')).not.toBeNull();
      expect(await exception.getAttribute('aria-describedby')).toBe('queue-action-reason-help');
      await retry
        .getByText('Required. Explain why this occurrence should bypass its usual timing.', {
          exact: true,
        })
        .waitFor();
      expect(await retry.getByRole('button', { name: 'Retry now', exact: true }).isDisabled()).toBe(
        true,
      );
      await capture('queue-required');
      await page.keyboard.press('Escape');
      let decisions = 0;
      await page.route('**/api/v1/root/schedule-requests/*/decision', async (route) => {
        decisions++;
        await route.fulfill({
          status: 400,
          contentType: 'application/json',
          body: JSON.stringify({
            error: {
              code: 'invalid_request',
              message: 'Explain what the requester should do next.',
            },
          }),
        });
      });
      await page.goto(`${panel.origin}/root/schedules`);
      for (const action of ['Approve', 'Decline']) {
        await page.getByRole('button', { name: action, exact: true }).first().click();
        const decision = page.getByRole('dialog', {
          name: `${action} schedule request`,
          exact: true,
        });
        const field = decision.getByLabel('Decision reason', { exact: true });
        expect(await field.getAttribute('required')).not.toBeNull();
        expect(await field.getAttribute('aria-describedby')).toBe('schedule-decision-reason-help');
        expect(await decision.getByRole('button', { name: action, exact: true }).isDisabled()).toBe(
          true,
        );
        await capture(`decision-${action.toLowerCase()}-required`);
        await field.fill('Coordinate with the release team before changing the hours.');
        await decision.getByRole('button', { name: action, exact: true }).click();
        await decision.getByRole('alert').waitFor();
        expect(await field.inputValue()).toBe(
          'Coordinate with the release team before changing the hours.',
        );
        await capture(`decision-${action.toLowerCase()}-rejected`);
        await page.keyboard.press('Escape');
      }
      expect(decisions).toBe(2);
      await visit(page, addressOf(panel, 'workspace/sync'));
      await page.getByRole('button', { name: 'View changes', exact: true }).click();
      const inspector = page.getByRole('dialog', { name: 'Sync details', exact: true });
      await inspector.getByRole('button', { name: 'Run now', exact: true }).click();
      const run = page.getByRole('dialog', { name: 'Sync now?', exact: true });
      const runReason = run.getByLabel('Reason', { exact: true });
      expect(await runReason.getAttribute('required')).not.toBeNull();
      expect(await runReason.getAttribute('aria-describedby')).toBe('sync-run-reason-help');
      expect(await run.getByRole('button', { name: 'Run now', exact: true }).isDisabled()).toBe(
        true,
      );
      await capture('sync-required');
    } finally {
      await page.close();
    }
  },
);
