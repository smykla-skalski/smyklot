import { mkdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import type { Page } from 'playwright-core';

async function settleEditors(page: Page): Promise<void> {
  // CodeMirror measures visible editors. Bring each into view after a resize so
  // a long-page image does not retain the preceding viewport's wrapping/gutter.
  for (const editor of await page.locator('.cm-editor:visible').all()) {
    await editor.scrollIntoViewIfNeeded();
    await page.evaluate(
      () =>
        new Promise<void>((resolve) =>
          requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
        ),
    );
  }
}

/** Opt-in visual evidence from the same route inventory as the layout guards. */
export async function captureVisualAudit(page: Page, route: string): Promise<void> {
  const destination = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
  if (destination === undefined) return;
  const directory = join(destination, route.replaceAll(/[^a-z0-9-]/giu, '-'));
  await mkdir(directory, { recursive: true });
  const copy = await page.locator('body').innerText();
  await writeFile(join(directory, 'copy.txt'), copy);
  for (const colorScheme of ['light', 'dark'] as const) {
    await page.emulateMedia({ colorScheme });
    for (const width of [1440, 1024, 768, 375]) {
      await page.setViewportSize({ width, height: 1000 });
      await settleEditors(page);
      // Full-page images must start at the document's top. Otherwise fixed
      // mobile chrome is painted midway through the long image at the old scroll.
      await page.evaluate(() => {
        if (document.activeElement instanceof HTMLElement) document.activeElement.blur();
        window.scrollTo({ top: 0, left: 0, behavior: 'instant' });
      });
      await page.mouse.move(0, 0);
      await page.screenshot({
        path: join(directory, `${colorScheme}-${width}.png`),
        fullPage: true,
        animations: 'disabled',
      });
    }
  }
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.evaluate(() => window.scrollTo({ top: 0, left: 0, behavior: 'instant' }));
  const expanded = await page.locator('details').evaluateAll((elements) => {
    for (const element of elements) (element as HTMLDetailsElement).open = true;
    return elements.length;
  });
  if (expanded > 0) {
    await settleEditors(page);
    await page.evaluate(() => window.scrollTo({ top: 0, left: 0, behavior: 'instant' }));
    await writeFile(join(directory, 'expanded-copy.txt'), await page.locator('body').innerText());
    for (const colorScheme of ['light', 'dark'] as const) {
      await page.emulateMedia({ colorScheme });
      await page.screenshot({
        path: join(directory, `${colorScheme}-expanded.png`),
        fullPage: true,
        animations: 'disabled',
      });
    }
  }
}
