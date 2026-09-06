import { mkdir, writeFile } from 'node:fs/promises';
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

describe('themed shared picker [Browser]', () => {
  it('keeps the composer outside navigation throughout normal-motion layout changes', async () => {
    const page = await panel.browser.newPage({
      viewport: { width: 1153, height: 1000 },
      reducedMotion: 'no-preference',
    });
    page.setDefaultTimeout(10_000);
    try {
      await visit(page, addressOf(panel, 'workspace/sync/rulesets/main-protection'));
      await page
        .getByRole('combobox', { name: /Bypass mode for/ })
        .last()
        .click();
      await page.getByRole('option', { name: 'Always allow', exact: true }).click();
      await page.locator('.settings-composer').waitFor();
      await expect
        .poll(() =>
          page.locator('.settings-composer').evaluate((node) => node.getAnimations().length),
        )
        .toBe(0);
      const timelines = [];
      async function sample(action: () => Promise<unknown>) {
        const pending = page.evaluate(
          () =>
            new Promise<
              Array<{
                left: number;
                right: number;
                sideRight: number;
                paneLeft: number;
                paneRight: number;
              }>
            >((resolve) => {
              const frames: Array<{
                left: number;
                right: number;
                sideRight: number;
                paneLeft: number;
                paneRight: number;
              }> = [];
              const end = performance.now() + 700;
              const frame = () => {
                const composer = document
                  .querySelector('.settings-composer')!
                  .getBoundingClientRect();
                const side = document.querySelector('.side')!.getBoundingClientRect();
                const pane = document.querySelector('.workspace')!.getBoundingClientRect();
                frames.push({
                  left: composer.left,
                  right: composer.right,
                  sideRight: side.right,
                  paneLeft: pane.left,
                  paneRight: pane.right,
                });
                if (performance.now() < end) requestAnimationFrame(frame);
                else resolve(frames);
              };
              frame();
            }),
        );
        await action();
        return pending;
      }
      timelines.push({
        change: 'collapse',
        frames: await sample(() =>
          page.getByRole('button', { name: 'Collapse pages', exact: true }).click(),
        ),
      });
      timelines.push({
        change: 'expand',
        frames: await sample(() =>
          page.getByRole('button', { name: 'Expand pages', exact: true }).click(),
        ),
      });
      await page.setViewportSize({ width: 1024, height: 1000 });
      timelines.push({
        change: 'breakpoint',
        frames: await sample(() => page.setViewportSize({ width: 1025, height: 1000 })),
      });
      const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
      if (directory) {
        await mkdir(directory, { recursive: true });
        await writeFile(
          join(directory, 'composer-normal-motion.json'),
          JSON.stringify(timelines, null, 2),
        );
      }
      for (const { change, frames } of timelines) {
        expect(frames.length, change).toBeGreaterThan(3);
        for (const frame of frames) {
          expect(frame.left, change).toBeGreaterThanOrEqual(frame.sideRight + 15);
          expect(frame.left, change).toBeGreaterThanOrEqual(frame.paneLeft + 15);
          expect(frame.right, change).toBeLessThanOrEqual(frame.paneRight - 15);
        }
      }
    } finally {
      await page.close();
    }
  });

  it('preserves labels, keyboard navigation, typed values, required forms and reset', async () => {
    const page = await panel.browser.newPage();
    try {
      await visit(page, addressOf(panel, 'workspace/'));
      await page.addScriptTag({
        type: 'module',
        url: `${panel.origin}/tests/support/select-harness.ts`,
      });
      const picker = page.getByRole('combobox', { name: 'Typed choice', exact: true });
      await picker.waitFor();
      expect(await page.locator('select').count()).toBe(0);
      await picker.focus();
      await picker.press('ArrowDown');
      await page.getByRole('listbox').waitFor();
      await picker.press('End');
      await picker.press('Enter');
      await expect.poll(() => page.getByLabel('Typed result').textContent()).toBe('string:z');
      expect(await picker.evaluate((node) => node === document.activeElement)).toBe(true);
      await picker.press('n');
      await expect.poll(() => page.getByLabel('Typed result').textContent()).toBe('number:1');
      for (const [label, result] of [
        ['Text one', 'string:1'],
        ['Empty value', 'string:'],
        ['No override', 'null'],
      ]) {
        await picker.click();
        await page.getByRole('option', { name: label, exact: true }).click();
        await expect.poll(() => page.getByLabel('Typed result').textContent()).toBe(result);
      }
      await picker.click();
      await picker.press('Home');
      await picker.press('Escape');
      expect(await page.getByLabel('Typed result').textContent()).toBe('null');
      expect(await picker.evaluate((node) => node === document.activeElement)).toBe(true);
      expect(await page.getByLabel('Disabled picker').isDisabled()).toBe(true);
      expect(await page.getByLabel('Saved unavailable value').textContent()).toContain('retired');
      await page.getByRole('button', { name: 'Submit', exact: true }).click();
      expect(await page.getByLabel('Required choice').getAttribute('aria-invalid')).toBe('true');
      await page.getByLabel('Required choice').click();
      await page.getByRole('option', { name: 'Confirmed', exact: true }).click();
      await picker.click();
      await page.getByRole('option', { name: 'Text one', exact: true }).click();
      await page.getByRole('button', { name: 'Submit', exact: true }).click();
      await expect
        .poll(() => page.getByLabel('Form result').textContent())
        .toBe('[["choice","1"],["required","yes"]]');
      await page.getByRole('button', { name: 'Reset', exact: true }).click();
      await expect.poll(() => page.getByLabel('Typed result').textContent()).toBe('number:1');
      expect(await page.getByLabel('Required choice').textContent()).toContain('Choose an option');
    } finally {
      await page.close();
    }
  });

  it.each([
    { colorScheme: 'light', width: 1440 },
    { colorScheme: 'dark', width: 1440 },
    { colorScheme: 'light', width: 1024 },
    { colorScheme: 'dark', width: 1024 },
    { colorScheme: 'light', width: 768 },
    { colorScheme: 'dark', width: 768 },
    { colorScheme: 'light', width: 375 },
    { colorScheme: 'dark', width: 375 },
  ] as const)(
    'themes live bypass and operator menus at $colorScheme $width',
    async ({ colorScheme, width }) => {
      const page = await panel.browser.newPage({
        colorScheme,
        viewport: { width, height: 1100 },
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(10_000);
      try {
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (directory) await mkdir(directory, { recursive: true });
        async function checkComposer() {
          const composer = page.locator('.settings-composer');
          const geometry = await composer.evaluate((node) => {
            const bounds = node.getBoundingClientRect();
            const pane = document.querySelector('.workspace')!.getBoundingClientRect();
            const children = Array.from(node.querySelectorAll('.composer-copy,.composer-actions'));
            return {
              left: bounds.left,
              right: bounds.right,
              paneLeft: pane.left,
              paneRight: pane.right,
              center: (bounds.left + bounds.right - pane.left - pane.right) / 2,
              contained: children.every((child) => {
                const childBounds = child.getBoundingClientRect();
                return childBounds.left >= bounds.left && childBounds.right <= bounds.right;
              }),
            };
          });
          expect(geometry.left).toBeGreaterThanOrEqual(geometry.paneLeft + 16);
          expect(geometry.right).toBeLessThanOrEqual(geometry.paneRight - 16);
          expect(Math.abs(geometry.center)).toBeLessThanOrEqual(1);
          expect(geometry.contained).toBe(true);
        }
        async function checkMenu(name: string) {
          const menu = page.getByRole('listbox');
          await menu.waitFor();
          await expect
            .poll(() => menu.evaluate((node) => getComputedStyle(node).visibility))
            .toBe('visible');
          const geometry = await menu.evaluate((node) => {
            const rect = node.getBoundingClientRect();
            const style = getComputedStyle(node);
            const probe = document.createElement('span');
            node.append(probe);
            probe.style.color = 'var(--popover-bg)';
            const expected = getComputedStyle(probe).color;
            probe.remove();
            return {
              background: style.backgroundColor,
              expected,
              radius: parseFloat(style.borderRadius),
              top: rect.top,
              bottom: rect.bottom,
              hit: document.elementFromPoint(
                rect.x + rect.width / 2,
                rect.y + Math.min(rect.height / 2, 20),
              )?.outerHTML,
              left: rect.left,
              right: rect.right,
              width: innerWidth,
              heights: Array.from(node.querySelectorAll('[role="option"]')).map(
                (option) => option.getBoundingClientRect().height,
              ),
              topElement:
                document
                  .elementFromPoint(rect.x + rect.width / 2, rect.y + Math.min(rect.height / 2, 20))
                  ?.closest('[role="listbox"]') === node,
            };
          });
          if (directory) {
            await page.screenshot({
              path: join(directory, `${name}-${colorScheme}-${width}.png`),
              animations: 'disabled',
            });
            await writeFile(
              join(directory, `${name}-${colorScheme}-${width}.json`),
              JSON.stringify(geometry, null, 2),
            );
          }
          expect(geometry.background).toBe(geometry.expected);
          expect(geometry.radius).toBeGreaterThan(0);
          expect(geometry.left).toBeGreaterThanOrEqual(0);
          expect(geometry.right).toBeLessThanOrEqual(geometry.width);
          await expect
            .poll(() =>
              menu.evaluate((node) => {
                const rect = node.getBoundingClientRect();
                return (
                  document
                    .elementFromPoint(
                      rect.x + rect.width / 2,
                      rect.y + Math.min(rect.height / 2, 20),
                    )
                    ?.closest('[role="listbox"]') === node
                );
              }),
            )
            .toBe(true);
          expect(geometry.heights.every((height) => height === 34)).toBe(true);
        }
        await visit(page, addressOf(panel, 'workspace/sync/rulesets/main-protection'));
        const mode = page.getByRole('combobox', { name: /Bypass mode for/ }).last();
        await mode.scrollIntoViewIfNeeded();
        const beforeMenuScroll = await page.evaluate(() => scrollY);
        await mode.click();
        await checkMenu('bypass-mode');
        expect(await page.evaluate(() => scrollY)).toBe(beforeMenuScroll);
        await page.getByRole('option', { name: 'Always allow', exact: true }).click();
        await page.getByRole('button', { name: 'Add an actor', exact: true }).click();
        await page.getByLabel('Who', { exact: true }).click();
        await checkMenu('bypass-actor-type');
        await page.keyboard.press('Escape');
        const actorList = page.getByRole('list', { name: 'Bypass exceptions' });
        for (const remove of await actorList.getByRole('button', { name: /^Remove / }).all()) {
          const size = await remove.boundingBox();
          expect(size?.width).toBe(34);
          expect(size?.height).toBe(34);
        }
        const form = page.locator('.actor-form');
        await form.getByLabel('App name or slug').fill('re');
        await page.getByRole('list', { name: 'Matching actors' }).waitFor();
        await form.getByText('Looking for actors', { exact: true }).waitFor({ state: 'hidden' });
        const geometry = await form.evaluate((node) => {
          const list = node.previousElementSibling!;
          const last = Array.from(list.querySelectorAll('.actor-row')).at(-1);
          const field = node.querySelector('.actor-field')!;
          const results = Array.from(node.querySelectorAll('.result-row')).map((row) =>
            row.getBoundingClientRect(),
          );
          const bounds = node.getBoundingClientRect();
          const card = node.closest('.card')!;
          const add = card.querySelector('.card-head button')!.getBoundingClientRect();
          const cancel = node.querySelector('.actor-form-actions button')!.getBoundingClientRect();
          return {
            topInset: add.top - card.getBoundingClientRect().top,
            bottomInset: card.getBoundingClientRect().bottom - cancel.bottom,
            balance:
              add.top -
              card.getBoundingClientRect().top -
              (card.getBoundingClientRect().bottom - cancel.bottom),
            gap:
              field.getBoundingClientRect().top -
              (last?.getBoundingClientRect().bottom ?? bounds.top),
            sides: results.map((row) => ({
              left: bounds.left - row.left,
              right: row.right - bounds.right,
            })),
          };
        });
        await actorList.evaluate((node) =>
          node.closest('.card')!.scrollIntoView({ block: 'center' }),
        );
        if (directory) {
          await page.screenshot({
            path: join(directory, `bypass-add-form-${colorScheme}-${width}.png`),
            animations: 'disabled',
          });
          await writeFile(
            join(directory, `bypass-add-form-${colorScheme}-${width}.json`),
            JSON.stringify(geometry, null, 2),
          );
        }
        expect(geometry.gap).toBeGreaterThanOrEqual(16);
        expect(Math.abs(geometry.balance)).toBeLessThanOrEqual(1);
        await checkComposer();
        for (const side of geometry.sides)
          expect(Math.abs(side.left - side.right)).toBeLessThanOrEqual(1);
        await form.getByLabel('App name or slug').fill('');
        expect(await form.getByRole('button', { name: 'Add smyklot', exact: true }).count()).toBe(
          0,
        );
        expect(await form.getByRole('button', { name: /^Add / }).count()).toBeGreaterThan(1);
        await actorList.evaluate((node) =>
          node.closest('.card')!.scrollIntoView({ block: 'center' }),
        );
        if (directory)
          await page.screenshot({
            path: join(directory, `bypass-remaining-${colorScheme}-${width}.png`),
            animations: 'disabled',
          });
        if (width === 1440) {
          await page.getByRole('button', { name: 'Collapse pages', exact: true }).click();
          await checkComposer();
          if (directory)
            await page.screenshot({
              path: join(directory, `bypass-collapsed-${colorScheme}-${width}.png`),
              animations: 'disabled',
            });
          await page.getByRole('button', { name: 'Expand pages', exact: true }).click();
        }
        await form.getByLabel('App name or slug').fill('smyklot');
        await form.getByText('Looking for actors', { exact: true }).waitFor({ state: 'hidden' });
        await form.getByText('Matching actors are already added', { exact: true }).waitFor();
        expect(await form.getByRole('button', { name: /^Add / }).count()).toBe(0);
        if (directory)
          await page.screenshot({
            path: join(directory, `bypass-already-added-${colorScheme}-${width}.png`),
            animations: 'disabled',
          });
        await actorList.getByRole('button', { name: 'Remove smyklot', exact: true }).click();
        await form.getByRole('button', { name: 'Add smyklot', exact: true }).waitFor();
        await form.getByRole('button', { name: 'Add smyklot', exact: true }).click();
        expect(await form.getByRole('button', { name: 'Add smyklot', exact: true }).count()).toBe(
          0,
        );
        const add = page.getByRole('button', { name: 'Add an actor', exact: true });
        await form.waitFor({ state: 'hidden' });
        await add.click();
        await form.waitFor();
        await add.click();
        await form.waitFor({ state: 'hidden' });
        await add.click();
        await page.getByRole('button', { name: 'Cancel', exact: true }).click();
        expect(
          await page
            .getByRole('button', { name: 'Add an actor', exact: true })
            .evaluate((node) => node === document.activeElement),
        ).toBe(true);
        // Rapid toggles share one tick: an obsolete open callback must not focus
        // a detached field or scroll after the reader has already closed it.
        await page.evaluate(() => scrollTo(0, document.documentElement.scrollHeight));
        await add.scrollIntoViewIfNeeded();
        const beforeToggle = await page.evaluate(() => scrollY);
        await add.evaluate((node) => {
          if (!(node instanceof HTMLButtonElement)) throw new Error('Expected the add button');
          node.click();
          node.click();
        });
        await form.waitFor({ state: 'hidden' });
        await expect.poll(() => add.evaluate((node) => node === document.activeElement)).toBe(true);
        expect(await page.evaluate(() => scrollY)).toBe(beforeToggle);
        await add.evaluate((node) => {
          if (!(node instanceof HTMLButtonElement)) throw new Error('Expected the add button');
          node.click();
          node.click();
          node.click();
        });
        const who = page.getByLabel('Who', { exact: true });
        await expect.poll(() => who.evaluate((node) => node === document.activeElement)).toBe(true);
        const focusBounds = await who.evaluate((node) => ({
          top: node.getBoundingClientRect().top,
          bottom: node.getBoundingClientRect().bottom,
          ceiling: document.querySelector('.top-bar')?.getBoundingClientRect().bottom ?? 0,
          floor:
            document.querySelector('.settings-composer')?.getBoundingClientRect().top ??
            innerHeight,
        }));
        expect(focusBounds.top).toBeGreaterThanOrEqual(focusBounds.ceiling);
        expect(focusBounds.bottom).toBeLessThanOrEqual(focusBounds.floor - 8);
        await page.getByRole('button', { name: 'Cancel', exact: true }).click();
        await expect.poll(() => add.evaluate((node) => node === document.activeElement)).toBe(true);
        await visit(page, addressOf(panel, 'root/schedules'), { ready: '.view-frame .object-row' });
        await page
          .getByRole('button', { name: /Edit schedule -/ })
          .first()
          .click();
        const dialog = page.getByRole('dialog', { name: 'Configure job', exact: true });
        await dialog.getByLabel('Default priority', { exact: true }).click();
        await checkMenu('operator-priority');
        await page.getByRole('option', { name: 'High', exact: true }).click();
        expect(
          await dialog.getByLabel('Default priority', { exact: true }).textContent(),
        ).toContain('High');
        await dialog.getByLabel('Wait before retrying unit', { exact: true }).click();
        await checkMenu('operator-duration');
        await page.keyboard.press('Escape');
        await page.keyboard.press('Escape');
        await page
          .getByRole('button', { name: /^Edit - the / })
          .first()
          .click();
        const windowControls = await page
          .locator('.window-row')
          .first()
          .evaluate((node) => {
            const controls = Array.from(node.querySelectorAll('input,button')).map((control) => {
              const bounds = control.getBoundingClientRect();
              return { height: bounds.height, middle: bounds.top + bounds.height / 2 };
            });
            const fields = Array.from(node.querySelectorAll('.form-field')).map((field) => {
              const label = field.querySelector('.form-label')!.getBoundingClientRect();
              const control = field.querySelector('input,button')!.getBoundingClientRect();
              return control.top - label.bottom;
            });
            return { controls, fields };
          });
        expect(windowControls.controls.every((control) => control.height === 34)).toBe(true);
        expect(windowControls.fields.every((gap) => Math.abs(gap - 8) <= 1)).toBe(true);
        // Day occupies its own row only in the narrow layout; the two time fields
        // still share a center, and every desktop control shares that same center.
        const aligned =
          width === 375 ? windowControls.controls.slice(1, 3) : windowControls.controls;
        expect(
          Math.max(...aligned.map((control) => control.middle)) -
            Math.min(...aligned.map((control) => control.middle)),
        ).toBeLessThanOrEqual(1);
        await page.getByRole('dialog').getByRole('combobox', { name: /^Day/ }).first().click();
        await checkMenu('schedule-weekday');
        await page.getByRole('option', { name: 'Tuesday', exact: true }).click();
        await page.keyboard.press('Escape');
        await visit(page, addressOf(panel, 'root/access/invitations'));
        await page.getByRole('button', { name: 'Invite an operator', exact: true }).first().click();
        await page.getByRole('dialog').getByLabel('Invitation expiry', { exact: true }).click();
        await checkMenu('operator-invitation-expiry');
        await page.keyboard.press('Escape');
        await page.keyboard.press('Escape');
        await visit(page, addressOf(panel, 'workspace/access/users'));
        await page.getByRole('button', { name: 'Add someone', exact: true }).click();
        await page.getByRole('dialog').getByLabel('Role', { exact: true }).click();
        await checkMenu('workspace-membership-role');
      } finally {
        await page.close();
      }
    },
  );
});
