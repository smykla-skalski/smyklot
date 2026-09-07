import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { mkdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import type { Page } from 'playwright-core';

import { startPanel, type Panel } from './harness';
import { expectAddPill, expectAdditionPicker, expectStableExpansion } from './add-control-geometry';
import { formatJson, parseJson, type JsonValue } from '../../src/lib/merge';

let panel: Panel;

beforeAll(async () => {
  panel = await startPanel();
});

afterAll(async () => {
  await panel?.close();
});

async function openRepositories(page: Page): Promise<void> {
  await page.goto(`${panel.origin}/workspace/${panel.account}/repositories`, {
    waitUntil: 'domcontentloaded',
  });
  await page.locator('.repository-row').first().waitFor({ state: 'visible', timeout: 30_000 });
}

describe('settings draft destinations [Integration]', () => {
  it.each(['light', 'dark'] as const)(
    'keeps long check names clear of rule actions in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        colorScheme,
        viewport: { width: 375, height: 1000 },
        reducedMotion: 'reduce',
      });
      try {
        await page.goto(
          `${panel.origin}/workspace/${panel.account}/sync/rulesets/main-protection`,
          { waitUntil: 'domcontentloaded' },
        );
        const row = page.locator('.policy-row').filter({
          has: page.getByText('Require status checks', { exact: true }),
        });
        await row.getByRole('button', { name: 'Edit', exact: true }).click();
        const editor = page.getByRole('dialog', { name: 'Require status checks', exact: true });
        await editor.getByRole('button', { name: 'Add a check', exact: true }).click();
        const check = page.getByRole('textbox', { name: 'Check to add', exact: true });
        const name = 'W'.repeat(24);
        await check.fill(name);
        await check.press('Enter');
        await editor.getByRole('button', { name: 'Done', exact: true }).click();
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (directory) {
          await mkdir(directory, { recursive: true });
          await row.screenshot({ path: join(directory, `rules-long-${colorScheme}-375.png`) });
        }
        const chip = row.locator('.param-chip').filter({ hasText: name });
        const geometry = await chip.evaluate((node) => {
          const rect = node.getBoundingClientRect();
          const actions = node
            .closest('.rule-value')!
            .querySelector('.rule-actions')!
            .getBoundingClientRect();
          return {
            right: rect.right,
            left: actions.left,
            overflow: node.scrollWidth - node.clientWidth,
          };
        });
        expect(geometry.overflow).toBeLessThanOrEqual(1);
        expect(geometry.right).toBeLessThanOrEqual(geometry.left - 4);
        await page.getByRole('button', { name: 'Discard', exact: true }).click();
      } finally {
        await page.close();
      }
    },
  );

  it.each(
    [375, 768, 1024, 1440].flatMap((width) =>
      (['light', 'dark'] as const).map((colorScheme) => ({ width, colorScheme })),
    ),
  )(
    'keeps each rule action group together at $colorScheme $width',
    async ({ width, colorScheme }) => {
      const page = await panel.browser.newPage({
        colorScheme,
        viewport: { width, height: 1000 },
        reducedMotion: 'reduce',
      });
      try {
        await page.goto(
          `${panel.origin}/workspace/${panel.account}/sync/rulesets/main-protection`,
          { waitUntil: 'domcontentloaded' },
        );
        const card = page
          .locator('.card')
          .filter({ has: page.getByRole('heading', { name: 'What it enforces', exact: true }) });
        await card.waitFor({ state: 'visible', timeout: 30_000 });
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (directory) {
          await mkdir(directory, { recursive: true });
          await card.screenshot({
            path: join(directory, `rules-${colorScheme}-${width}.png`),
            animations: 'disabled',
          });
        }
        for (const name of ['Require a pull request', 'Require status checks']) {
          const row = card
            .locator('.policy-row')
            .filter({ has: page.getByText(name, { exact: true }) });
          const edit = await row.getByRole('button', { name: 'Edit', exact: true }).boundingBox();
          const remove = await row
            .getByRole('button', { name: 'Switch the rule off', exact: true })
            .boundingBox();
          expect.soft(remove!.height).toBe(34);
          expect.soft(remove!.width).toBe(34);
          expect
            .soft(Math.abs(edit!.y + edit!.height / 2 - (remove!.y + remove!.height / 2)))
            .toBeLessThanOrEqual(1);
        }
        expect(
          await card.evaluate((node) => node.scrollWidth - node.clientWidth),
        ).toBeLessThanOrEqual(1);
        const statusRule = card.locator('.policy-row').filter({
          has: page.getByText('Require status checks', { exact: true }),
        });
        await statusRule.getByRole('button', { name: 'Switch the rule off', exact: true }).click();
        expect(await statusRule.count()).toBe(0);
        expect(await page.locator('.card.is-unsaved').count()).toBe(1);
        await page.getByRole('button', { name: 'Discard', exact: true }).click();
        await expect.poll(() => statusRule.count()).toBe(1);
        expect(await page.locator('.card.is-unsaved').count()).toBe(0);
      } finally {
        await page.close();
      }
    },
  );

  it.each(
    [375, 768, 1024, 1440].flatMap((width) =>
      (['light', 'dark'] as const).map((colorScheme) => ({ width, colorScheme })),
    ),
  )(
    'keeps ruleset changes local and separated at $colorScheme $width',
    async ({ colorScheme, width }) => {
      const page = await panel.browser.newPage({
        colorScheme,
        viewport: { width, height: 1000 },
        reducedMotion: 'reduce',
      });
      try {
        await page.goto(
          `${panel.origin}/workspace/${panel.account}/sync/rulesets/main-protection`,
          { waitUntil: 'domcontentloaded' },
        );
        const bypass = page
          .locator('.card')
          .filter({ has: page.getByRole('heading', { name: 'Bypass list', exact: true }) });
        await bypass.waitFor({ state: 'visible', timeout: 30_000 });
        const appRow = bypass.locator('.actor-row').filter({ hasText: 'Smyklot' });
        const mode = appRow.getByRole('combobox');
        await mode.click();
        await page.getByRole('option', { name: 'Always allow', exact: true }).click();
        await expect.poll(() => appRow.getAttribute('data-unsaved')).toBe('true');
        expect(await page.locator('.card.is-unsaved').count()).toBe(1);
        expect(await page.locator('.policy-row.is-unsaved').count()).toBe(0);
        expect(await bypass.locator('.actor-row.is-unsaved').count()).toBe(1);
        const addActor = bypass.getByRole('button', { name: 'Add an actor', exact: true });
        await expectAddPill(addActor);
        expect(
          await bypass.locator('.card-head').getByRole('button', { name: 'Add an actor' }).count(),
        ).toBe(1);
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (directory) {
          await mkdir(directory, { recursive: true });
          await bypass.evaluate((node) => node.scrollIntoView({ block: 'center' }));
          await page.mouse.move(0, 0);
          await bypass.screenshot({
            path: join(directory, `bypass-header-${colorScheme}-${width}.png`),
          });
        }
        await mode.click();
        await page.getByRole('option', { name: 'Pull requests only', exact: true }).click();
        await expect.poll(() => page.locator('.is-unsaved[data-unsaved]').count()).toBe(0);
        await addActor.click();
        await bypass.getByRole('combobox', { name: 'Who' }).waitFor();
        expect(await addActor.getAttribute('aria-expanded')).toBe('true');
        const cancelStyle = await bypass
          .getByRole('button', { name: 'Cancel', exact: true })
          .evaluate((node) => ({
            border: getComputedStyle(node).borderTopStyle,
            radius: getComputedStyle(node).borderTopLeftRadius,
            height: node.getBoundingClientRect().height,
          }));
        expect(cancelStyle).toEqual({ border: 'solid', radius: '8px', height: 34 });
        if (directory) {
          await bypass
            .getByText('Looking for actors', { exact: false })
            .waitFor({ state: 'hidden' });
          await bypass.evaluate((node) => node.scrollIntoView({ block: 'center' }));
          await page.mouse.move(0, 0);
          await bypass.screenshot({
            path: join(directory, `bypass-expanded-${colorScheme}-${width}.png`),
          });
        }
        await addActor.click();
        await bypass.getByRole('combobox', { name: 'Who' }).waitFor({ state: 'hidden' });
        expect(await addActor.getAttribute('aria-expanded')).toBe('false');
        await addActor.click();
        await bypass.getByRole('button', { name: 'Cancel', exact: true }).click();
        expect(await addActor.evaluate((node) => document.activeElement === node)).toBe(true);

        const conditions = page
          .locator('.card')
          .filter({ has: page.getByRole('heading', { name: 'Where it applies', exact: true }) });
        const included = conditions.locator('.policy-row').filter({ hasText: 'Included branches' });
        const excluded = conditions.locator('.policy-row').filter({ hasText: 'Excluded branches' });
        await expectAddPill(included.getByRole('button', { name: 'Add a pattern' }));
        await expectAddPill(excluded.getByRole('button', { name: 'Add a pattern' }));
        await included.getByRole('button', { name: 'Add a pattern' }).click();
        await page.getByRole('textbox', { name: 'Pattern to add' }).fill('release/*');
        await page.getByRole('textbox', { name: 'Pattern to add' }).press('Enter');
        await page
          .getByRole('textbox', { name: 'Pattern to add' })
          .waitFor({ state: 'hidden', timeout: 3000 });
        await expect.poll(() => included.getAttribute('data-unsaved')).toBe('true');
        expect(await excluded.getAttribute('data-unsaved')).toBeNull();
        await excluded.getByRole('button', { name: 'Add a pattern' }).click();
        await page.getByRole('textbox', { name: 'Pattern to add' }).fill('archive/*');
        await page.getByRole('textbox', { name: 'Pattern to add' }).press('Enter');
        await page
          .getByRole('textbox', { name: 'Pattern to add' })
          .waitFor({ state: 'hidden', timeout: 3000 });
        await expect.poll(() => excluded.getAttribute('data-unsaved')).toBe('true');
        const geometry = await included.evaluate((node) => {
          const style = getComputedStyle(node);
          const next = node.nextElementSibling!;
          return {
            gap: next.getBoundingClientRect().top - node.getBoundingClientRect().bottom,
            clearPixel: style.borderBottomWidth,
            border: style.borderBottomColor,
            clip: style.backgroundClip,
            separator: getComputedStyle(node, '::after').backgroundColor,
            animations: node.getAnimations().length,
          };
        });
        if (directory) {
          await writeFile(
            join(directory, `adjacent-edits-${colorScheme}-${width}.json`),
            JSON.stringify(geometry, null, 2),
          );
          await conditions.evaluate((node) => node.scrollIntoView({ block: 'center' }));
          await page.mouse.move(0, 0);
          await conditions.screenshot({
            path: join(directory, `adjacent-edits-${colorScheme}-${width}.png`),
          });
        }
        expect(geometry).toEqual({
          gap: 0,
          clearPixel: '1px',
          border: 'rgba(0, 0, 0, 0)',
          clip: 'padding-box',
          separator: 'rgba(0, 0, 0, 0)',
          animations: 0,
        });
        await included.getByRole('button', { name: 'Remove release/*', exact: true }).click();
        await excluded.getByRole('button', { name: 'Remove archive/*', exact: true }).click();
        await expect.poll(() => page.locator('.is-unsaved[data-unsaved]').count()).toBe(0);
        expect(await page.locator('.card.is-unsaved').count()).toBe(0);
        const addRule = page.getByRole('button', { name: 'Add a rule', exact: true });
        await expectAddPill(addRule);
        const rules = page.getByRole('dialog', { name: 'Rule choices', exact: true });
        await expectStableExpansion(page, addRule, rules);
        const choices = rules.locator('.addition-choices .btn-add');
        expect(await choices.count()).toBeGreaterThan(1);
        for (const choice of await choices.all()) await expectAddPill(choice);
        await expectAdditionPicker(rules.locator('.addition-picker'));
        if (directory) {
          await page.mouse.move(0, 0);
          await page.screenshot({
            path: join(directory, `rule-choices-${colorScheme}-${width}.png`),
          });
        }
        await rules.getByRole('button', { name: 'Cancel', exact: true }).click();
        await rules.waitFor({ state: 'hidden' });
        await expect
          .poll(() => addRule.evaluate((node) => document.activeElement === node))
          .toBe(true);
        await addRule.click();
        await rules.getByRole('button', { name: 'Require code scanning', exact: true }).click();
        const inspector = page.getByRole('dialog', { name: 'Require code scanning', exact: true });
        await inspector.waitFor();
        await expect
          .poll(() => inspector.evaluate((node) => node.contains(document.activeElement)))
          .toBe(true);
        await inspector.getByRole('button', { name: 'Cancel', exact: true }).click();
        await inspector.waitFor({ state: 'hidden' });
        await expect
          .poll(() => addRule.evaluate((node) => document.activeElement === node))
          .toBe(true);
        expect(await page.locator('.card.is-unsaved').count()).toBe(0);
      } finally {
        await page.close();
      }
    },
  );

  it('identifies the dirty workspace, section, repository, and exact setting', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });

    try {
      await openRepositories(page);
      const repositoryRow = page.locator('.repository-row').first();
      const repositoryName =
        (await repositoryRow.locator('.object-name').textContent())?.trim() ?? '';
      expect(repositoryName).not.toBe('');
      await repositoryRow.locator('a.row-hit').click();

      const quietPeriod = page.getByRole('textbox', { name: 'Quiet period after checks pass' });
      await quietPeriod.waitFor({ state: 'visible', timeout: 15_000 });
      const savedValue = await quietPeriod.inputValue();
      await quietPeriod.fill(savedValue === '45' ? '46' : '45');
      await quietPeriod.blur();
      await page.getByText('1 changed setting').waitFor({ state: 'visible' });
      const settingRow = page.locator('.policy-row').filter({ has: quietPeriod });
      expect(await settingRow.getAttribute('data-unsaved')).toBe('true');

      /* The draft survives the reload, and what says so is the page it is on plus the
         tree's mark on every scope holding one. Nothing announces it: a notice would
         report a thing the reader had not just done, and cover the page saying it. */
      await page.reload({ waitUntil: 'domcontentloaded' });
      await page.getByText('1 changed setting').waitFor({ state: 'visible', timeout: 15_000 });
      expect(await page.locator('.tree-row.has-dirty').count()).toBeGreaterThan(0);

      await page.goto(`${panel.origin}/root/runtime/settings`, { waitUntil: 'domcontentloaded' });
      await page.getByRole('button', { name: 'Override the deployment session lifetime' }).click();
      await page.getByText('1 changed setting').waitFor({ state: 'visible' });

      await page.goto(`${panel.origin}/root/workspaces`, { waitUntil: 'domcontentloaded' });
      const rootRepositoryHref = `/root/workspaces/${panel.account}/repositories`;
      /* The console's catalog is a list of sentences now, and a workspace is opened by
         name rather than by pressing its row - so the link is the row's one act, and it
         still carries where the unsaved work is. */
      const workspaceLink = page.locator(`a[href="${rootRepositoryHref}"]`);
      await workspaceLink.waitFor({ state: 'visible', timeout: 30_000 });
      const workspaceRow = page.locator('.object-row', { has: workspaceLink });
      expect(await workspaceRow.getAttribute('data-unsaved')).toBe('true');
      expect(await workspaceRow.innerText()).toContain('1 unsaved setting');

      await workspaceLink.click();
      await page.waitForURL((url) => url.pathname === rootRepositoryHref);
      const repositoryLeaf = page.locator(`a.tree-row[href="${rootRepositoryHref}"]`);
      expect(await repositoryLeaf.innerText()).toContain('Unsaved changes');

      const markedRepository = page
        .locator('.repository-row')
        .filter({ has: page.locator('.object-name', { hasText: repositoryName }) })
        .first();
      await markedRepository.waitFor({ state: 'visible', timeout: 15_000 });
      expect(await markedRepository.getAttribute('data-unsaved')).toBe('true');
      expect(await markedRepository.innerText()).toContain('Unsaved changes');
      expect(await page.locator('.settings-composer').count()).toBe(1);

      await page.getByRole('button', { name: 'Discard', exact: true }).click();
      await markedRepository.waitFor({ state: 'visible' });
      expect(await markedRepository.getAttribute('data-unsaved')).toBeNull();
      // Discard clears the draft immediately; the composer then finishes its exit animation.
      await expect
        .poll(() => page.locator('.settings-composer').count(), { timeout: 2_000 })
        .toBe(0);

      await page.goto(`${panel.origin}/root/runtime/settings`, { waitUntil: 'domcontentloaded' });
      await page.getByText('1 changed setting').waitFor({ state: 'visible' });
      await page.getByRole('button', { name: 'Discard', exact: true }).click();
      await expect
        .poll(() => page.locator('.settings-composer').count(), { timeout: 2_000 })
        .toBe(0);
    } finally {
      await page.close();
    }
  });
});

describe('rule inspectors [Browser]', () => {
  it.each(
    [375, 1440].flatMap((width) =>
      (['light', 'dark'] as const).flatMap((colorScheme) =>
        [false, true].map((delayed) => ({ width, colorScheme, delayed })),
      ),
    ),
  )(
    'retains a private rule when another tab saves it at $colorScheme $width delayed=$delayed',
    async ({ width, colorScheme, delayed }) => {
      const context = await panel.browser.newContext({
        colorScheme,
        viewport: { width, height: 1000 },
        reducedMotion: 'reduce',
      });
      const [page, other] = await Promise.all([context.newPage(), context.newPage()]);
      for (const current of [page, other]) current.setDefaultTimeout(8000);
      let releaseMessages: (() => void) | undefined;
      const pendingMessages: Array<string | Buffer> = [];
      if (delayed) {
        // Keep the saved record in localStorage, but hold both notification paths.
        // Done must compare against the refreshed registry before staging.
        await page.addInitScript(() => {
          window.addEventListener(
            'storage',
            (event) => {
              const root = document.documentElement;
              root.dataset.heldStorageEvents = String(
                Number(root.dataset.heldStorageEvents ?? 0) + 1,
              );
              event.stopImmediatePropagation();
            },
            true,
          );
          window.addEventListener('storage', () => {
            const root = document.documentElement;
            root.dataset.deliveredStorageEvents = String(
              Number(root.dataset.deliveredStorageEvents ?? 0) + 1,
            );
          });
        });
        await page.routeWebSocket(
          (url) => url.pathname.endsWith('/api/v1/events'),
          (socket) => {
            const server = socket.connectToServer();
            let holding = true;
            server.onMessage((message) => {
              const event = JSON.parse(
                typeof message === 'string' ? message : message.toString(),
              ) as { type?: string };
              if (holding && event.type === 'target.changed') pendingMessages.push(message);
              else socket.send(message);
            });
            releaseMessages = () => {
              holding = false;
              for (const message of pendingMessages.splice(0)) socket.send(message);
            };
          },
        );
      }
      const title = 'Require a pull request';
      const message = delayed
        ? 'This edit could not be staged · close and reopen the editor to review the current settings'
        : 'This rule changed elsewhere · close and reopen the editor to review its current values';
      const url = `${panel.origin}/workspace/${panel.account}/sync/rulesets/main-protection`;
      const editor = page.getByRole('dialog', { name: title, exact: true });
      const second = other.getByRole('dialog', { name: title, exact: true });
      const input = editor.getByRole('spinbutton', { name: 'Approvals required' });
      const open = async (current: Page) => {
        const trigger = current
          .locator('.rule-row')
          .filter({ has: current.getByText(title, { exact: true }) })
          .getByRole('button', { name: 'Edit', exact: true });
        await trigger.waitFor({ timeout: 30000 });
        await trigger.click();
      };
      try {
        await Promise.all([page.goto(url), other.goto(url)]);
        await open(page);
        const initial = await input.inputValue();
        const remoteValue = initial === '2' ? '3' : '2';
        await input.fill('4');
        const originalInput = await input.elementHandle();
        await open(other);
        await second.getByRole('spinbutton', { name: 'Approvals required' }).fill(remoteValue);
        await second.getByRole('button', { name: 'Done', exact: true }).click();
        const [request] = await Promise.all([
          other.waitForRequest(
            (request) =>
              request.method() === 'PUT' &&
              /\/api\/v1\/targets\/[^/]+\/settings$/u.test(new URL(request.url()).pathname),
          ),
          other.getByRole('button', { name: 'Save', exact: true }).click(),
        ]);
        expect((await request.response())?.status()).toBe(200);
        await other.getByText('Settings saved', { exact: true }).waitFor();
        if (delayed) {
          await expect
            .poll(() =>
              page.evaluate(() => Number(document.documentElement.dataset.heldStorageEvents ?? 0)),
            )
            .toBeGreaterThan(0);
          expect(
            await page.evaluate(() =>
              Number(document.documentElement.dataset.deliveredStorageEvents ?? 0),
            ),
          ).toBe(0);
          expect(await editor.getByRole('button', { name: 'Done', exact: true }).isEnabled()).toBe(
            true,
          );
          expect(await editor.getByText(message, { exact: true }).count()).toBe(0);
          await editor.getByRole('button', { name: 'Done', exact: true }).click();
        }
        await editor.getByText(message, { exact: true }).waitFor();
        releaseMessages?.();
        expect(await input.inputValue()).toBe('4');
        expect(await originalInput!.evaluate((node) => node.isConnected)).toBe(true);
        expect(await editor.getByRole('button', { name: 'Done', exact: true }).isDisabled()).toBe(
          true,
        );
        await expect
          .poll(() => page.getByRole('button', { name: 'Save', exact: true }).count())
          .toBe(0);
        const stored = await page.evaluate(async () =>
          (await fetch('/api/v1/targets/2001/sync/config/rulesets')).json(),
        );
        expect(
          stored.document.rulesets.find((rule: { name: string }) => rule.name === 'main-protection')
            .rules.pull_request.required_approving_review_count,
        ).toBe(Number(remoteValue));
        await page.evaluate(() => document.fonts.ready);
        const rhythm = await editor.getByText(message, { exact: true }).evaluate((node) => {
          const style = getComputedStyle(node);
          const probe = document.createElement('span');
          probe.textContent = 'H';
          probe.style.cssText = `position:absolute;display:inline-block;text-box:trim-both cap alphabetic;line-height:1;font-family:${style.fontFamily};font-size:${style.fontSize};font-weight:${style.fontWeight}`;
          document.body.append(probe);
          const cap = probe.getBoundingClientRect().height;
          probe.remove();
          const range = document.createRange();
          range.selectNodeContents(node);
          const lines = Array.from(range.getClientRects()).filter(
            (rect) => rect.width > 0.5 && rect.height > 0.5,
          );
          return {
            cap,
            lineHeight: parseFloat(style.lineHeight),
            height: node.getBoundingClientRect().height,
            lines: lines.length,
            gaps: lines.slice(1).map((line, index) => line.top - lines[index]!.top - cap),
          };
        });
        expect(Math.abs(rhythm.lineHeight - rhythm.cap - 8)).toBeLessThanOrEqual(0.25);
        for (const gap of rhythm.gaps) expect(Math.abs(gap - 8)).toBeLessThanOrEqual(0.25);
        expect(
          Math.abs(rhythm.height - rhythm.cap - (rhythm.lines - 1) * rhythm.lineHeight),
        ).toBeLessThanOrEqual(0.25);
        if (width === 375) expect(rhythm.lines).toBeGreaterThan(1);
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (directory) {
          await mkdir(directory, { recursive: true });
          await page.mouse.move(0, 0);
          await page.screenshot({
            path: join(
              directory,
              `rule-${delayed ? 'stage-rejected' : 'changed'}-${colorScheme}-${width}.png`,
            ),
          });
        }
        await editor.getByRole('button', { name: 'Cancel', exact: true }).click();
        await open(page);
        expect(await input.inputValue()).toBe(remoteValue);
        expect(await editor.getByRole('button', { name: 'Done', exact: true }).isEnabled()).toBe(
          true,
        );
        await editor.getByRole('button', { name: 'Cancel', exact: true }).click();

        await open(other);
        await second.getByRole('spinbutton', { name: 'Approvals required' }).fill(initial);
        await second.getByRole('button', { name: 'Done', exact: true }).click();
        await other.getByRole('button', { name: 'Save', exact: true }).click();
        await other.getByText('Settings saved', { exact: true }).waitFor();
      } finally {
        await context.close();
      }
    },
  );

  it.each(
    [375, 768, 1024, 1440].flatMap((width) =>
      (['light', 'dark'] as const).map((colorScheme) => ({ width, colorScheme })),
    ),
  )(
    'guards private rule dismissal without losing focus or values at $colorScheme $width',
    async ({ width, colorScheme }) => {
      const page = await panel.browser.newPage({
        colorScheme,
        viewport: { width, height: 700 },
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(8000);
      try {
        await page.goto(
          `${panel.origin}/workspace/${panel.account}/sync/rulesets/main-protection`,
          { waitUntil: 'domcontentloaded' },
        );
        const row = page
          .locator('.rule-row')
          .filter({ has: page.getByText('Require a pull request', { exact: true }) });
        const trigger = row.getByRole('button', { name: 'Edit', exact: true });
        await trigger.waitFor({ timeout: 30000 });
        await trigger.click();
        const editor = page.getByRole('dialog', { name: 'Require a pull request', exact: true });
        const approvals = editor.getByRole('spinbutton', { name: 'Approvals required' });
        await approvals.fill('11');
        const focused = editor.getByRole('checkbox', { name: 'Rebase', exact: true });
        await focused.focus();
        const element = await approvals.elementHandle();
        const body = editor.locator('.modal-body').first();
        const scrollBefore = await body.evaluate((node) => node.scrollTop);
        await focused.press('Escape');
        const confirmation = page.getByRole('dialog', {
          name: 'Discard rule changes?',
          exact: true,
        });
        await confirmation.waitFor();
        await expect
          .poll(() =>
            confirmation
              .getByRole('button', { name: 'Keep editing', exact: true })
              .evaluate((node) => node === document.activeElement),
          )
          .toBe(true);
        expect(await element!.evaluate((node) => node.isConnected)).toBe(true);
        for (let i = 0; i < 5; i++) {
          await page.keyboard.press('Tab');
          expect(await confirmation.evaluate((node) => node.contains(document.activeElement))).toBe(
            true,
          );
        }
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (directory) {
          await mkdir(directory, { recursive: true });
          await page.mouse.move(0, 0);
          await page.screenshot({
            path: join(directory, `discard-rule-${colorScheme}-${width}.png`),
          });
        }
        await confirmation.getByRole('button', { name: 'Keep editing', exact: true }).click();
        await confirmation.waitFor({ state: 'hidden' });
        expect(await approvals.inputValue()).toBe('11');
        expect(await focused.evaluate((node) => node === document.activeElement)).toBe(true);
        expect(await body.evaluate((node) => node.scrollTop)).toBe(scrollBefore);
        expect(await editor.getByRole('button', { name: 'Done', exact: true }).isDisabled()).toBe(
          true,
        );
        await editor.getByRole('button', { name: 'Close rule editor', exact: true }).click();
        await confirmation.getByRole('button', { name: 'Discard changes', exact: true }).click();
        await editor.waitFor({ state: 'hidden' });
        expect(await trigger.evaluate((node) => node === document.activeElement)).toBe(true);
        expect(await page.getByRole('button', { name: 'Save', exact: true }).count()).toBe(0);

        await trigger.click();
        await approvals.fill('4');
        const bounds = await editor.boundingBox();
        if (bounds!.x > 0) {
          await page
            .locator('[data-dialog-overlay][data-state="open"]')
            .click({ position: { x: bounds!.x / 2, y: 350 } });
        } else {
          await editor.getByRole('button', { name: 'Close rule editor', exact: true }).click();
        }
        await confirmation.waitFor();
        await page.keyboard.press('Escape');
        await confirmation.waitFor({ state: 'hidden' });
        expect(await approvals.inputValue()).toBe('4');
        await expect
          .poll(() => editor.evaluate((node) => node.contains(document.activeElement)))
          .toBe(true);
        await editor.getByRole('button', { name: 'Cancel', exact: true }).click();
        await editor.waitFor({ state: 'hidden' });
        expect(await confirmation.count()).toBe(0);
        expect(await page.getByRole('button', { name: 'Save', exact: true }).count()).toBe(0);

        const add = page.getByRole('button', { name: 'Add a rule', exact: true });
        await add.click();
        await page.getByRole('button', { name: 'Restrict updates', exact: true }).click();
        const newRule = page.getByRole('dialog', { name: 'Restrict updates', exact: true });
        const upstream = newRule.getByRole('checkbox', {
          name: 'Allow syncing from upstream',
          exact: true,
        });
        await upstream.locator('..').click();
        await expect.poll(() => upstream.isChecked()).toBe(true);
        await page.keyboard.press('Escape');
        await confirmation.getByRole('button', { name: 'Discard changes', exact: true }).click();
        await newRule.waitFor({ state: 'hidden' });
        expect(await add.evaluate((node) => node === document.activeElement)).toBe(true);
        expect(
          await page
            .locator('.rule-row')
            .filter({ has: page.getByText('Restrict updates', { exact: true }) })
            .count(),
        ).toBe(0);
        expect(await page.getByRole('button', { name: 'Save', exact: true }).count()).toBe(0);
      } finally {
        await page.close();
      }
    },
  );

  it('saves edited rules with exact app pins and unknown fields through reload', async () => {
    const page = await panel.browser.newPage({
      viewport: { width: 1024, height: 1000 },
      reducedMotion: 'reduce',
    });
    page.setDefaultTimeout(8000);
    const endpoint = '**/api/v1/targets/*/sync/config/rulesets';
    const rawRule =
      '{"required_status_checks":[{"context":"test","integration_id":9007199254740993,"future_child":1e400},{"context":"lint","integration_id":77}],"strict_required_status_checks_policy":true,"future_parameter":1e-400}';
    await page.route(endpoint, async (route) => {
      const response = await route.fetch();
      const config = parseJson(await response.text()) as {
        document: { rulesets: { name: string; rules: Record<string, unknown> }[] };
      };
      config.document.rulesets.find(
        (rule) => rule.name === 'main-protection',
      )!.rules.required_status_checks = parseJson(rawRule);
      await route.fulfill({ response, body: formatJson(config as unknown as JsonValue) });
    });
    try {
      const href = `${panel.origin}/workspace/${panel.account}/sync/rulesets/main-protection`;
      await page.goto(href, { waitUntil: 'domcontentloaded' });
      const row = page
        .locator('.rule-row')
        .filter({ has: page.getByText('Require status checks', { exact: true }) });
      await row.getByRole('button', { name: 'Edit', exact: true }).click();
      const editor = page.getByRole('dialog', { name: 'Require status checks', exact: true });
      await editor.getByText('App ID 9007199254740993', { exact: true }).waitFor();
      await editor.getByRole('button', { name: 'Remove test', exact: true }).click();
      await editor.getByRole('button', { name: 'Add a check', exact: true }).click();
      const input = page.getByRole('textbox', { name: 'Check to add', exact: true });
      await input.fill('test');
      await input.press('Enter');
      await input.waitFor({ state: 'hidden' });
      await editor
        .locator('label.switch')
        .filter({
          has: page.getByRole('checkbox', {
            name: 'Require branches to be up to date',
            exact: true,
          }),
        })
        .click();
      await editor.getByRole('button', { name: 'Done', exact: true }).click();
      await editor.waitFor({ state: 'hidden' });
      await page.reload({ waitUntil: 'domcontentloaded' });
      await row.getByRole('button', { name: 'Edit', exact: true }).click();
      await editor.getByText('App ID 9007199254740993', { exact: true }).waitFor();
      expect(
        await editor
          .getByRole('checkbox', { name: 'Require branches to be up to date', exact: true })
          .isChecked(),
      ).toBe(false);
      await editor.getByRole('button', { name: 'Cancel', exact: true }).click();
      await editor.waitFor({ state: 'hidden' });
      const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
      if (directory) {
        await mkdir(directory, { recursive: true });
        await page.screenshot({ path: join(directory, 'restored-rule-before-save.png') });
        await writeFile(
          join(directory, 'restored-rule-before-save.txt'),
          await page.locator('body').innerText(),
        );
      }
      await page.unroute(endpoint);
      const [request] = await Promise.all([
        page.waitForRequest(
          (request) =>
            request.method() === 'PUT' &&
            /\/api\/v1\/targets\/[^/]+\/settings$/u.test(new URL(request.url()).pathname),
        ),
        page.getByRole('button', { name: 'Save', exact: true }).click(),
      ]);
      const body = parseJson(request.postData()!) as {
        sync_configs: {
          kind: string;
          document: { rulesets: { name: string; rules: Record<string, JsonValue> }[] };
        }[];
      };
      const saved = body.sync_configs
        .find((config) => config.kind === 'rulesets')!
        .document.rulesets.find((rule) => rule.name === 'main-protection')!.rules
        .required_status_checks!;
      const expected = {
        ...(parseJson(rawRule) as Record<string, JsonValue>),
        strict_required_status_checks_policy: false,
      };
      expect(formatJson(saved)).toBe(formatJson(expected));
      expect((await request.response())?.status()).toBe(200);
      await page.getByText('Settings saved', { exact: true }).waitFor();
      const stored = await page.evaluate(async () =>
        (await fetch('/api/v1/targets/2001/sync/config/rulesets')).text(),
      );
      const readback = parseJson(stored) as {
        document: { rulesets: { name: string; rules: Record<string, JsonValue> }[] };
      };
      expect(
        formatJson(
          readback.document.rulesets.find((rule) => rule.name === 'main-protection')!.rules
            .required_status_checks!,
        ),
      ).toBe(formatJson(expected));
    } finally {
      await page.close();
    }
  });

  it.each(
    [375, 768, 1024, 1440].flatMap((width) =>
      (['light', 'dark'] as const).map((colorScheme) => ({ width, colorScheme })),
    ),
  )(
    'keeps rule inspectors compact and Cancel unstaged at $colorScheme $width',
    async ({ width, colorScheme }) => {
      const page = await panel.browser.newPage({
        colorScheme,
        viewport: { width, height: 1000 },
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(8000);
      const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
      const observations: unknown[] = [];
      await page.route('**/api/v1/targets/*/sync/config/rulesets', async (route) => {
        const response = await route.fetch();
        const config = parseJson(await response.text()) as {
          document: { rulesets: { name: string; rules: Record<string, unknown> }[] };
        };
        config.document.rulesets.find(
          (rule) => rule.name === 'main-protection',
        )!.rules.required_status_checks = parseJson(
          '{"required_status_checks":[{"context":"build / integration tests on supported runtimes","integration_id":9007199254740993},{"context":"lint","integration_id":1197525}],"strict_required_status_checks_policy":true}',
        );
        await route.fulfill({ response, body: formatJson(config as unknown as JsonValue) });
      });
      try {
        await page.goto(
          `${panel.origin}/workspace/${panel.account}/sync/rulesets/main-protection`,
          { waitUntil: 'domcontentloaded' },
        );
        const card = page.locator('.card').filter({
          has: page.getByRole('heading', { name: 'What it enforces', exact: true }),
        });
        await card.waitFor({ timeout: 30_000 });
        expect(await page.locator('html').getAttribute('data-theme')).toBe(colorScheme);
        for (const [key, title, fresh] of [
          ['pull-request', 'Require a pull request', false],
          ['status-checks', 'Require status checks', false],
          ['update', 'Restrict updates', true],
          ['code-scanning', 'Require code scanning', true],
        ] as const) {
          const row = card
            .locator('.rule-row')
            .filter({ has: page.getByText(title, { exact: true }) });
          const trigger = fresh
            ? card.getByRole('button', { name: 'Add a rule', exact: true })
            : row.getByRole('button', { name: 'Edit', exact: true });
          await trigger.evaluate((node) => node.scrollIntoView({ block: 'center' }));
          const before = await card.evaluate((node) => ({
            height: node.getBoundingClientRect().height,
            scroll: scrollY,
          }));
          await trigger.click();
          if (fresh)
            await page
              .getByRole('dialog', { name: 'Rule choices', exact: true })
              .getByRole('button', { name: title, exact: true })
              .click();
          const editor = page.getByRole('dialog', { name: title, exact: true });
          await editor.waitFor();
          if (key === 'pull-request') {
            await editor.getByRole('spinbutton', { name: 'Approvals required' }).fill('3');
          }
          if (key === 'code-scanning') {
            expect(
              await editor.getByRole('button', { name: 'Done', exact: true }).isDisabled(),
            ).toBe(true);
            for (const tool of ['CodeQL', 'Infrastructure security analysis']) {
              await editor.getByRole('button', { name: 'Add a tool', exact: true }).click();
              const input = page.getByRole('textbox', { name: 'Tool to add', exact: true });
              await input.fill(tool);
              await input.press('Enter');
              await input.waitFor({ state: 'hidden' });
            }
            const picker = editor.getByRole('combobox', { name: 'CodeQL alerts', exact: true });
            await picker.click();
            await page.getByRole('option', { name: 'Errors and warnings', exact: true }).click();
            expect(await picker.innerText()).toContain('Errors and warnings');
          }
          if (key === 'status-checks') {
            await editor.getByText('App name unavailable', { exact: true }).waitFor();
            await editor.getByText('smyklot', { exact: true }).waitFor();
            expect(await editor.locator('.pin-source .avatar').count()).toBe(1);
          }
          await page.evaluate(() => document.fonts.ready);
          await page.mouse.move(0, 0);
          const geometry = await editor.evaluate((node) => {
            const bounds = node.getBoundingClientRect();
            const body = node.querySelector('.modal-body')!;
            const rect = (element: Element) => {
              const box = element.getBoundingClientRect();
              return {
                left: box.left,
                right: box.right,
                top: box.top,
                bottom: box.bottom,
                height: box.height,
                width: box.width,
              };
            };
            return {
              viewport: innerWidth,
              bounds: rect(node),
              overflow: body.scrollWidth - body.clientWidth,
              controls: [...node.querySelectorAll('button, input.text-input')]
                .filter((element) => element.getBoundingClientRect().height > 0)
                .map((element) => ({
                  ...rect(element),
                  label: element.getAttribute('aria-label') ?? element.textContent,
                  square: element.classList.contains('icon-button'),
                })),
              rows: [...body.querySelectorAll('.form-row')]
                .filter((element) => element.children.length === 2)
                .map((element) => {
                  const [copy, action] = [...element.children].map(rect);
                  return { copy, action, bounds: rect(element) };
                }),
              switchCopy: [...body.querySelectorAll('.form-row')].flatMap((element) => {
                const control = element.querySelector(':scope > .switch');
                const label = element.querySelector('.form-label');
                const help =
                  element.querySelector('.form-help') ??
                  (element.parentElement?.matches('.form-field')
                    ? element.parentElement.querySelector(':scope > .form-help')
                    : null);
                if (!control || !label || !help) return [];
                const title = rect(label);
                const description = rect(help);
                const action = rect(control);
                return [
                  {
                    label: label.textContent,
                    gap: description.top - title.bottom,
                    leftDifference: description.left - title.left,
                    centerDifference:
                      (action.top + action.bottom - title.top - description.bottom) / 2,
                  },
                ];
              }),
              fields: [...body.querySelectorAll('label.form-field')].map((element) => {
                const [label, input, help] = [...element.children].map(rect);
                return {
                  labelGap: input!.top - label!.bottom,
                  helpGap: help ? help.top - input!.bottom : null,
                };
              }),
              right: bounds.right,
            };
          });
          observations.push({ key, ...geometry });
          if (directory) {
            await mkdir(directory, { recursive: true });
            await page.screenshot({ path: join(directory, `${key}-${colorScheme}-${width}.png`) });
          }
          expect(geometry.overflow).toBeLessThanOrEqual(1);
          expect(geometry.bounds.left).toBeGreaterThanOrEqual(0);
          expect(geometry.right).toBeLessThanOrEqual(width);
          for (const control of geometry.controls) {
            expect.soft(control.height, `${key}: ${control.label}`).toBe(34);
            expect.soft(control.left).toBeGreaterThanOrEqual(geometry.bounds.left);
            expect.soft(control.right).toBeLessThanOrEqual(geometry.bounds.right);
            if (control.square) expect.soft(control.width).toBe(control.height);
          }
          for (const { copy, action, bounds } of geometry.rows) {
            expect
              .soft(Math.abs((copy!.top + copy!.bottom) / 2 - (action!.top + action!.bottom) / 2))
              .toBeLessThanOrEqual(1);
            expect.soft(copy!.right).toBeLessThanOrEqual(action!.left - 8);
            expect.soft(action!.right).toBeLessThanOrEqual(bounds.right);
          }
          for (const copy of geometry.switchCopy) {
            expect
              .soft(Math.abs(copy.gap - 8), `${key}: ${copy.label} ink gap`)
              .toBeLessThanOrEqual(0.25);
            expect
              .soft(Math.abs(copy.leftDifference), `${key}: ${copy.label} left edge`)
              .toBeLessThanOrEqual(0.25);
            expect
              .soft(Math.abs(copy.centerDifference), `${key}: ${copy.label} switch center`)
              .toBeLessThanOrEqual(0.5);
          }
          expect(geometry.switchCopy.length).toBe(
            key === 'pull-request' ? 2 : key === 'code-scanning' ? 0 : 1,
          );
          for (const field of geometry.fields) {
            expect.soft(field.labelGap).toBeCloseTo(8, 0);
            if (field.helpGap !== null) expect.soft(field.helpGap).toBeCloseTo(8, 0);
          }
          await editor.getByRole('button', { name: 'Cancel', exact: true }).click();
          await editor.waitFor({ state: 'hidden' });
          await expect
            .poll(() => trigger.evaluate((node) => node === document.activeElement))
            .toBe(true);
          const after = await card.evaluate((node) => ({
            height: node.getBoundingClientRect().height,
            scroll: scrollY,
          }));
          expect(after.height).toBeCloseTo(before.height, 0);
          expect(Math.abs(after.scroll - before.scroll)).toBeLessThanOrEqual(1);
          if (fresh) expect(await row.count()).toBe(0);
          expect(await page.getByRole('button', { name: 'Save', exact: true }).count()).toBe(0);
        }
        if (directory)
          await writeFile(
            join(directory, `geometry-${colorScheme}-${width}.json`),
            JSON.stringify(observations, null, 2),
          );
      } finally {
        await page.close();
      }
    },
  );
});
