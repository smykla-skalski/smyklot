import { mkdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import axe from 'axe-core';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { WRITTEN_QUEUE_SECTIONS } from '../../src/lib/routes';
import { addressOf, PANEL_ROUTES, startPanel, visit, type Panel } from './harness';

let panel: Panel;
beforeAll(async () => {
  panel = await startPanel();
});
afterAll(async () => {
  await panel?.close();
});

const captures: Record<string, string> = {
  'root/queue': 'queue',
  'root/runtime/service': 'service-health',
  'root/access/users': 'users',
  'root/history/audit': 'audit',
  'root/runtime/settings': 'settings',
  'root/workspaces/{account}/settings': 'workspace-settings',
};

describe('console landmark ownership', () => {
  it.each(['light', 'dark'] as const)(
    'has one main and distinct regions across console routes in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(10_000);
      try {
        const routes = [
          ...PANEL_ROUTES.filter((path) => path.startsWith('root')),
          ...WRITTEN_QUEUE_SECTIONS.map((section) => `root/queue/${section}`),
        ];
        for (const route of routes) {
          await visit(page, addressOf(panel, route));
          await page.locator('.root-workspace').waitFor();
          expect(await page.getByRole('main').count(), route).toBe(1);
          await page.evaluate(axe.source);
          const result = await page.evaluate(async () =>
            (window as unknown as { axe: typeof axe }).axe.run(document, {
              runOnly: {
                type: 'rule',
                values: [
                  'landmark-unique',
                  'landmark-one-main',
                  'landmark-main-is-top-level',
                  'aria-valid-attr-value',
                ],
              },
            }),
          );
          expect(result.violations, route).toEqual([]);
          const evidence = process.env.SMYKLOT_LANDMARK_EVIDENCE;
          if (evidence) {
            await mkdir(evidence, { recursive: true });
            await writeFile(
              join(evidence, `F07-${route.replaceAll('/', '_')}-${colorScheme}-axe.json`),
              JSON.stringify(result, null, 2),
            );
          }
          const scene = captures[route];
          const directory = process.env.SMYKLOT_LANDMARK_SCREENSHOTS;
          if (scene && directory) {
            await mkdir(directory, { recursive: true });
            await page.mouse.move(1900, 20);
            await page.screenshot({ path: join(directory, `F07-${scene}-${colorScheme}.png`) });
          }
        }
      } finally {
        await page.close();
      }
    },
    90_000,
  );
});
