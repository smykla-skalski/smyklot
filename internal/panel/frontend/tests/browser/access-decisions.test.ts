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
  'explains access scope and retains refused choices in %s',
  async (theme) => {
    const page = await panel.browser.newPage({
      viewport: { width: 1920, height: 1200 },
      colorScheme: theme,
      reducedMotion: 'reduce',
    });
    let release: (() => void) | undefined;
    const pending = new Promise<void>((resolve) => {
      release = resolve;
    });
    let writes = 0;
    async function capture(scene: string) {
      const directory = process.env.SMYKLOT_SYNC_OBSERVATION_SCREENSHOTS;
      if (!directory) return;
      await mkdir(directory, { recursive: true });
      await page.mouse.move(1900, 20);
      await page.screenshot({ path: join(directory, `F10-${scene}-${theme}.png`) });
    }
    try {
      await page.route('**/api/v1/targets/2001/users/*', async (route) => {
        if (route.request().method() !== 'PUT') return route.continue();
        writes++;
        const input = route.request().postDataJSON();
        expect(input.suspended).toBe(true);
        expect(input.suspension_reason).toBe('Review workspace access');
        expect(typeof input.expected_revision).toBe('number');
        await pending;
        await route.fulfill({
          status: 409,
          json: {
            error: 'conflict',
            message: 'Access changed. Review the current access before trying again.',
          },
        });
      });
      await visit(page, addressOf(panel, 'workspace/access/users?dialog=access-decision&user=ada'));
      const decision = page.getByRole('dialog');
      expect(await decision.textContent()).toContain(
        'Only access to Smykla Skalski changes. Account sign-in and other workspaces are unaffected.',
      );
      await page.locator('label.choice-card').filter({ hasText: 'Suspend access' }).click();
      await decision.getByRole('textbox').fill('Review workspace access');
      await capture('suspend');
      await decision.getByRole('button', { name: 'Suspend workspace access', exact: true }).click();
      await decision.getByRole('button', { name: 'Saving…' }).waitFor();
      await page.keyboard.press('Escape');
      expect(await decision.isVisible()).toBe(true);
      expect(await decision.getByRole('button', { name: 'Cancel', exact: true }).isDisabled()).toBe(
        true,
      );
      expect(await decision.getByRole('textbox').isDisabled()).toBe(true);
      await capture('suspend-pending');
      release?.();
      await decision.getByRole('alert').waitFor();
      expect(await decision.getByRole('textbox').inputValue()).toBe('Review workspace access');
      expect(
        await decision
          .getByRole('button', { name: 'Suspend workspace access', exact: true })
          .isEnabled(),
      ).toBe(true);
      expect(writes).toBe(1);
      await capture('suspend-refused');
      await decision.getByRole('button', { name: 'Cancel', exact: true }).click();
      await visit(
        page,
        addressOf(panel, 'workspace/access/users?dialog=access-decision&user=margaret'),
      );
      expect(
        await decision.getByRole('button', { name: 'Keep suspended', exact: true }).isDisabled(),
      ).toBe(true);
      await capture('review-suspension');
      await page
        .locator('label.choice-card')
        .filter({ hasText: 'Restore workspace access' })
        .click();
      expect(
        await decision
          .getByRole('button', { name: 'Restore workspace access', exact: true })
          .isEnabled(),
      ).toBe(true);
      await capture('restore');
      await visit(
        page,
        addressOf(panel, 'workspace/access/users?dialog=decision-history&user=margaret'),
      );
      expect(await decision.textContent()).toContain(
        'Account sign-in and other workspaces are unaffected.',
      );
      expect(await decision.locator('dd').filter({ hasText: 'Smykla Skalski' }).count()).toBe(1);
      await capture('history');
      await visit(page, addressOf(panel, 'root/access/users/ada/ban'));
      await decision.getByRole('button', { name: 'Ban account', exact: true }).waitFor();
      expect(await decision.textContent()).toContain('Sign-in is blocked for this account');
      await capture('root-ban');
    } finally {
      release?.();
      await page.close();
    }
  },
);
