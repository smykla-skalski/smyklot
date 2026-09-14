import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { addressOf, startPanel, type Panel } from './harness';

let panel: Panel;
beforeAll(async () => {
  panel = await startPanel();
});
afterAll(async () => {
  await panel?.close();
});

describe('desktop invitation recovery', () => {
  it.each(['light', 'dark'] as const)(
    'keeps a usable recovery route in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(10_000);
      const capture = async (scene: string) => {
        const directory = process.env.SMYKLOT_INVITATION_RECOVERY_SCREENSHOTS;
        if (!directory) return;
        await mkdir(directory, { recursive: true });
        await page.mouse.move(1900, 20);
        await page.screenshot({ path: join(directory, `F41-${scene}-${colorScheme}.png`) });
      };
      try {
        for (const [status, code] of [
          [401, 'invalid_invitation'],
          [403, 'wrong_identity'],
        ] as const) {
          await page.goto(addressOf(panel, `__error/${status}/${code}`));
          const back = page.getByRole('link', { name: 'Go to the panel', exact: true });
          await back.waitFor();
          expect(await page.getByRole('link', { name: 'Review invitation' }).count()).toBe(0);
          await capture(code);
          if (colorScheme === 'light') await back.click();
          else await back.press('Enter');
          await expect.poll(() => page.locator('.error-body').count()).toBe(0);
        }
        await page.goto(addressOf(panel, '__error/403/wrong_identity?recover=1'));
        const review = page.getByRole('link', { name: 'Review invitation', exact: true });
        await review.waitFor();
        const href = await review.getAttribute('href');
        expect(href).toMatch(/^\/invite\/[A-Za-z0-9_-]{43}$/);
        await capture('verified-account');
        if (colorScheme === 'light') await review.click();
        else await review.press('Enter');
        await page.getByRole('link', { name: 'Accept with GitHub', exact: true }).waitFor();
        expect(new URL(page.url()).pathname).toBe(href);
        await capture('review');
        const accept = new URL(
          (await page
            .getByRole('link', { name: 'Accept with GitHub', exact: true })
            .getAttribute('href'))!,
          page.url(),
        );
        expect(accept.searchParams.get('invite')).toBe(href!.split('/').at(-1));
        expect(accept.searchParams.get('action')).toBe('accept');
        expect(await page.getByText('Pending', { exact: true }).count()).toBe(1);
      } finally {
        await page.close();
      }
    },
  );
});
