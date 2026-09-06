import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import type { Page } from 'playwright-core';

import { startPanel, visit, type Panel } from './harness';
import { captureVisualAudit } from './visual-audit';

/**
 * The File sync card of a repository's own page, read for real.
 *
 * What it shows is server state, held by the shared query client so the stream
 * saying something changed refetches it. That wiring has no unit test that can
 * reach it: the card's own specs mount the presentational component with a
 * value handed to it, so they pass whether or not anything ever fetches one.
 * This opens the page and asks the browser.
 *
 * It used to be a PANE, behind a radio, and these specs pressed it. The page is
 * one scroll now and the card is simply there, so what was "open it and check"
 * is "check", and the two specs that were about the pane's own address are
 * `repository-page.test.ts`'s - which asserts those addresses answer 404.
 */
let panel: Panel;

beforeAll(async () => {
  panel = await startPanel();
});

afterAll(async () => {
  await panel?.close();
});

/**
 * Opens one repository's page and answers with it, once its heading says it is
 * the repository that was asked for.
 *
 * Through the harness's `visit` rather than a goto and a fixed sleep: the sleep
 * is a guess in both directions, and it is most of what this suite costs.
 */
async function repositoryPage(page: Page, name: string) {
  await visit(page, `${panel.origin}/workspace/${panel.account}/repositories/${name}`, {
    ready: '.repository-page',
  });
  await page
    .getByRole('heading', { name, exact: true })
    .waitFor({ state: 'visible', timeout: 30_000 });

  return page.locator('.repository-page');
}

describe('the repository file sync card in the development panel', () => {
  it('keeps actionable repository protection failures visible', async () => {
    const page = await panel.browser.newPage();
    try {
      const reason = 'GitHub denied permission to update branch protection';
      await page.route('**/api/v1/targets/*/repositories/*', async (route) => {
        const response = await route.fetch();
        const detail = await response.json();
        detail.pending_ci_gate = { ...detail.pending_ci_gate, readiness: 'blocked', reason };
        await route.fulfill({ response, json: detail });
      });
      const repository = await repositoryPage(page, 'api-gateway');
      expect(await repository.getByRole('alert').filter({ hasText: reason }).count()).toBe(1);
      await captureVisualAudit(page, 'repository-protection-blocked');
    } finally {
      await page.close();
    }
  });

  it.each(['light', 'dark'] as const)(
    'keeps merge rules out of the editor layout in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1440, height: 1000 },
        colorScheme,
      });
      try {
        const repository = await repositoryPage(page, 'api-gateway');
        const card = repository.getByRole('region', { name: 'File sync', exact: true });
        expect(await repository.locator('.gate-note').count()).toBe(0);
        const trigger = card.getByRole('button', { name: 'Merge rules', exact: true });
        const editor = card.locator('.cm-content');
        const mountedEditor = await editor.elementHandle();
        for (const width of [1440, 1024, 768, 375]) {
          await page.setViewportSize({ width, height: 1000 });
          await trigger.scrollIntoViewIfNeeded();
          const geometry = await card.evaluate((node) => {
            const title = node.querySelector('h2')!;
            const nested = node.querySelector('.file-editor h3')!;
            const ignored = [...node.querySelectorAll('.policy-row')].find((row) =>
              row.textContent?.includes('Ignored in this repository'),
            )!;
            const say = ignored.querySelector('.setting-say')!.getBoundingClientRect();
            const value = ignored.querySelector('.policy-value')!.getBoundingClientRect();
            const add = node.querySelector('.card-head > button')!.getBoundingClientRect();
            const heading = title.getBoundingClientRect();
            const path = node.querySelector('.file-heading input')!.getBoundingClientRect();
            const fileActions = node.querySelector('.file-actions')!.getBoundingClientRect();
            const code = node.querySelector('.code-editor')!.getBoundingClientRect();
            const help = node.querySelector('.editor-description')!.getBoundingClientRect();
            return {
              actionCenter: fileActions.y + fileActions.height / 2 - path.y - path.height / 2,
              pathWidth: path.width,
              pathAndActionsShareLine: Math.abs(fileActions.y - path.y) < 1,
              helperGap: help.top - code.bottom,
              bottomInset:
                node.getBoundingClientRect().bottom -
                Number.parseFloat(getComputedStyle(node).borderBottomWidth) -
                help.bottom,
              titleSize: Number.parseFloat(getComputedStyle(title).fontSize),
              nestedSize: getComputedStyle(nested).fontSize,
              labelSize: getComputedStyle(ignored.querySelector('.setting-name')!).fontSize,
              onRight: value.left >= say.right,
              addOnRight: add.left >= heading.right,
              height: node.getBoundingClientRect().height,
              scrollY,
              overflow: document.documentElement.scrollWidth - innerWidth,
            };
          });
          expect(Number.parseFloat(geometry.nestedSize)).toBeLessThan(geometry.titleSize);
          expect(geometry.nestedSize).toBe(geometry.labelSize);
          expect(geometry.overflow).toBeLessThanOrEqual(1);
          if (geometry.pathAndActionsShareLine)
            expect(Math.abs(geometry.actionCenter)).toBeLessThanOrEqual(1);
          expect(geometry.pathWidth).toBeLessThanOrEqual(400);
          expect(geometry.helperGap).toBeCloseTo(8, 0);
          expect(geometry.bottomInset).toBeCloseTo(width <= 576 ? 16 : 20, 0);
          if (width >= 1024) {
            expect(geometry.onRight).toBe(true);
            expect(geometry.addOnRight).toBe(true);
          }
          const editorBox = await editor.boundingBox();
          await trigger.click();
          const inspector = page.getByRole('dialog', { name: 'Merge rules', exact: true });
          await inspector.waitFor();
          await inspector.evaluate(async (node) => {
            await Promise.all(node.getAnimations().map((animation) => animation.finished));
          });
          expect(await inspector.getByText('renovate.json', { exact: true }).count()).toBe(1);
          expect(await inspector.locator('details').count()).toBe(0);
          const opened = await card.evaluate((node) => ({
            height: node.getBoundingClientRect().height,
            scrollY,
          }));
          expect(opened.height).toBeCloseTo(geometry.height, 0);
          expect(opened.scrollY).toBeCloseTo(geometry.scrollY, 0);
          expect(await editor.boundingBox()).toEqual(editorBox);
          expect(await editor.evaluate((node, mounted) => node === mounted, mountedEditor)).toBe(
            true,
          );
          const unframed = await inspector
            .locator('input, select, textarea, [role="checkbox"]')
            .evaluateAll(
              (controls) => controls.filter((control) => !control.closest('.card')).length,
            );
          expect(unframed).toBe(0);
          const bounds = await inspector.boundingBox();
          expect(bounds!.x).toBeGreaterThanOrEqual(0);
          expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(width + 1);
          expect(
            await inspector.evaluate((node) => node.scrollWidth - node.clientWidth),
          ).toBeLessThanOrEqual(1);
          const destination = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
          if (destination !== undefined) {
            const directory = join(destination, 'merge-inspector');
            await mkdir(directory, { recursive: true });
            await page.screenshot({
              path: join(directory, `${colorScheme}-${width}.png`),
              animations: 'disabled',
            });
          }
          await page.keyboard.press('Escape');
          await inspector.waitFor({ state: 'hidden' });
          expect(await trigger.evaluate((node) => node === document.activeElement)).toBe(true);
          expect(await editor.boundingBox()).toEqual(editorBox);
        }
        await captureVisualAudit(page, `repository-file-editor-${colorScheme}`);
      } finally {
        await page.close();
      }
    },
  );

  it('keeps code Undo and incomplete list rules when the inspector closes', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1440, height: 1000 } });
    try {
      const repository = await repositoryPage(page, 'api-gateway');
      const card = repository.getByRole('region', { name: 'File sync', exact: true });
      const editor = card.locator('.cm-content');
      const saved = await editor.innerText();
      await editor.fill('{"packageRules":[{"groupName":"local"}]}');
      await card.getByRole('button', { name: 'Merge rules', exact: true }).click();
      const inspector = page.getByRole('dialog', { name: 'Merge rules', exact: true });
      await inspector.getByRole('button', { name: 'Add a list rule', exact: true }).click();
      await inspector.getByRole('button', { name: 'Done', exact: true }).click();
      await inspector.waitFor({ state: 'hidden' });
      await card.getByRole('button', { name: 'Undo', exact: true }).click();
      expect(await editor.innerText()).toBe(saved);
      await card.getByRole('button', { name: 'Merge rules', exact: true }).click();
      expect(await inspector.getByRole('textbox', { name: 'List', exact: true }).count()).toBe(2);
      expect(
        await inspector.getByRole('textbox', { name: 'List', exact: true }).last().inputValue(),
      ).toBe('');
      await page.keyboard.press('Escape');
      await inspector.waitFor({ state: 'hidden' });
      expect(await page.getByRole('button', { name: 'Save', exact: true }).isDisabled()).toBe(true);
    } finally {
      await page.close();
    }
  });

  it('reads what a repository adjusts, and renders both grammars', async () => {
    const page: Page = await panel.browser.newPage({
      viewport: { width: 1280, height: 900 },
    });
    const crashes: string[] = [];
    page.on('pageerror', (error) => crashes.push(error.message));

    const reads: string[] = [];
    page.on('request', (request) => {
      const path = new URL(request.url()).pathname;
      if (request.method() === 'GET' && path.includes('/sync/files')) reads.push(path);
    });

    try {
      const repository = await repositoryPage(page, 'smyklot');

      // The adjustments the mock seeds for this repository, which nothing but
      // a real read produces: the card renders an entry per merge, and an empty
      // answer renders none. Both templates, so both grammars are on the page.
      const json = repository.locator('.sync-merge').first();
      await json.waitFor({ state: 'visible', timeout: 30_000 });

      expect(await json.getByRole('textbox', { name: 'File', exact: true }).inputValue()).toBe(
        'renovate.json',
      );
      const markdown = repository.locator('.sync-merge').nth(1);
      expect(await markdown.getByRole('textbox', { name: 'File', exact: true }).inputValue()).toBe(
        'CONTRIBUTING.md',
      );
      expect(
        await markdown.getByRole('textbox', { name: 'Heading', exact: true }).first().inputValue(),
      ).toBe('## Commits');
      expect(reads.length).toBeGreaterThan(0);

      expect(crashes).toEqual([]);
    } finally {
      await page.close();
    }
  });

  /**
   * A segment after the repository is somebody else's text, and `constructor`
   * is the shape that catches a guard written as `value in LABELS`: `in` walks
   * the prototype chain, so a key every object has reads as a section.
   *
   * Nothing follows a repository now - the page is one scroll - so the address
   * resolves to no route at either end and never reaches a guard at all.
   * Asserted here rather than trusted, because two copies of a list drifting is
   * exactly how the sync view came to 404.
   */
  it('refuses an address naming anything after the repository', async () => {
    // No crash assertion here, unlike its neighbours: the dev server serves the
    // bundle from the root and the worker registers relative to the address, so
    // an address the server refuses also has no worker under it. That 404 is
    // the harness, not the panel.
    const page: Page = await panel.browser.newPage({
      viewport: { width: 1280, height: 900 },
    });

    try {
      await page.goto(
        `${panel.origin}/workspace/${panel.account}/repositories/smyklot/constructor`,
        {
          waitUntil: 'domcontentloaded',
        },
      );
      await page.locator('.error-body').waitFor({ state: 'visible', timeout: 30_000 });

      expect(await page.locator('body').innerText()).toContain('Not found');
    } finally {
      await page.close();
    }
  });

  /**
   * Every box in the card is one of the panel's own control heights.
   *
   * `control-heights.test.ts` sweeps the routes and cannot reach this: the card
   * is on a repository's own page, which that sweep does not open, so its six
   * boxes kept the user agent's `2px inset` face and stood at 34px beside the
   * 23.8px chip they replace. The same rule, asked where the sweep cannot go.
   *
   * The seeded repository adjusts two templates, one JSON and one Markdown, so
   * every box the card can draw is on the page: a path, a list rule, a heading
   * and its ordinal, and a find/replace pair.
   */
  it('gives every box in the card a declared height', async () => {
    const page: Page = await panel.browser.newPage({
      viewport: { width: 1280, height: 900 },
    });

    try {
      const repository = await repositoryPage(page, 'smyklot');
      await repository
        .locator('.sync-merge')
        .first()
        .waitFor({ state: 'visible', timeout: 30_000 });

      const boxes = await page.evaluate(() =>
        [...document.querySelectorAll<HTMLInputElement>('.sync-merge input')]
          .filter((box) => !['checkbox', 'radio'].includes(box.type))
          .map((box) => ({
            where: box.getAttribute('placeholder') ?? box.type,
            height: box.getBoundingClientRect().height,
          })),
      );

      // Every shape the card draws, so a box added without a class fails here
      // rather than passing on an empty list.
      expect(boxes.length).toBeGreaterThanOrEqual(6);
      expect(
        boxes.filter((box) => box.height !== 34).map((box) => `${box.where} ${box.height}px`),
      ).toEqual([]);
    } finally {
      await page.close();
    }
  });

  /**
   * A repository the planner refuses receives none of the organization's files.
   * Everything about that lives on a second row the card reads beside the
   * adjustments, so a component spec handed a value proves none of it - this
   * asks the browser, through the real read.
   */
  it('says why a repository is getting none of the files', async () => {
    const page: Page = await panel.browser.newPage({
      viewport: { width: 1280, height: 900 },
    });
    const crashes: string[] = [];
    page.on('pageerror', (error) => crashes.push(error.message));

    try {
      // The blocker shown on Sync status must survive the recovery link to the
      // same repository, with the same reason beside its file configuration.
      const repository = await repositoryPage(page, 'legacy-service');

      /* Scoped to the stand-down line: the merge card's saved receipt is a
         status too, quietly present so a save can announce itself. */
      const notice = repository.locator('.sync-pane-standdown');
      await notice.waitFor({ state: 'visible', timeout: 30_000 });

      const said = await notice.textContent();
      expect(said).toContain('are not being synced here');
      expect(said).toContain('docs is not a directory in this repository');
      expect(said).toContain('Last checked');

      expect(crashes).toEqual([]);
    } finally {
      await page.close();
    }
  });
});
