import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import type { Locator, Page } from 'playwright-core';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';

import { addressOf, startPanel, visit, type Panel } from './harness';

let panel: Panel;
beforeAll(async () => {
  panel = await startPanel();
});
afterAll(async () => {
  await panel?.close();
});

async function sharedFieldStyle(field: Locator) {
  return field.evaluate((node) => {
    const input = node.querySelector<HTMLInputElement>('input')!;
    const select = node.querySelector<HTMLSelectElement>('select')!;
    // A real shared field in the same inherited theme gives us the style contract,
    // including future token changes. This caught extraction losing page-scoped CSS.
    const reference = document.createElement('input');
    reference.className = 'text-input';
    reference.disabled = input.disabled;
    node.append(reference);
    const read = (element: HTMLElement) => {
      const style = getComputedStyle(element);
      return {
        border: style.borderTopWidth,
        borderStyle: style.borderTopStyle,
        radius: style.borderRadius,
        background: style.backgroundColor,
        color: style.color,
        opacity: style.opacity,
        height: element.getBoundingClientRect().height,
      };
    };
    const actual = read(input);
    const expected = read(reference);
    const unitHeight = select.getBoundingClientRect().height;
    reference.remove();
    return { actual, expected, unitHeight };
  });
}

async function expectSharedDurationStyles(page: Page): Promise<void> {
  const fields = page.locator('.duration-field:visible');
  expect(await fields.count()).toBeGreaterThan(0);
  for (const field of await fields.all()) {
    const { actual, expected, unitHeight } = await sharedFieldStyle(field);
    await expect
      .poll(async () => {
        const settled = await sharedFieldStyle(field);
        return settled.actual;
      })
      .toEqual(expected);
    expect(actual.border).toBe('1px');
    expect(actual.radius).not.toBe('0px');
    expect(actual.height).toBe(unitHeight);

    const input = field.locator('input');
    await input.focus();
    await expect
      .poll(() =>
        input.evaluate((node) => {
          const style = getComputedStyle(node);
          return (
            node.matches(':focus-visible') &&
            style.outlineStyle === 'solid' &&
            Number.parseFloat(style.outlineWidth) > 0 &&
            style.borderTopColor === style.outlineColor
          );
        }),
      )
      .toBe(true);
    await input.blur();
    await field.evaluate((node) => {
      node.querySelector<HTMLInputElement>('input')!.disabled = true;
      node.querySelector<HTMLSelectElement>('select')!.disabled = true;
    });
    await expect
      .poll(async () => {
        const result = await sharedFieldStyle(field);
        return JSON.stringify(result.actual) === JSON.stringify(result.expected);
      })
      .toBe(true);
  }
}

async function settledDisclosurePaint(summary: Locator, active: boolean) {
  const read = () =>
    summary.evaluate((node) => {
      const style = getComputedStyle(node);
      return {
        background: style.backgroundColor,
        inset: style.boxShadow,
        hovered: node.matches(':hover'),
        active: node.matches(':active'),
        settled: node.getAnimations().every((animation) => animation.playState === 'finished'),
      };
    });

  // Pointer events arrive before the next rendered frame. Wait for the actual
  // shared transition, otherwise both reads can still be the idle/hover color.
  await expect.poll(read).toMatchObject({ hovered: true, active, settled: true });
  return read();
}

describe('shared duration field style contract [Browser]', () => {
  it.each(['light', 'dark'] as const)(
    'aligns the formatting disclosure with its card text and frame in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1440, height: 1000 },
        colorScheme,
      });
      try {
        await visit(page, addressOf(panel, 'workspace/settings'));
        const origin = page.locator('.formatting-origin').first();
        const card = origin.locator('..');
        const samples = [];
        for (const width of [1440, 768, 375]) {
          await page.setViewportSize({ width, height: 1000 });
          for (const open of [false, true]) {
            if (
              await origin.evaluate(
                (node, expanded) => (node as HTMLDetailsElement).open !== expanded,
                open,
              )
            ) {
              await origin.locator('summary').click();
            }
            await card.scrollIntoViewIfNeeded();
            await page.mouse.move(0, 0);
            const auditDirectory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
            if (auditDirectory !== undefined) {
              await mkdir(auditDirectory, { recursive: true });
              await card.screenshot({
                path: join(
                  auditDirectory,
                  `formatting-${colorScheme}-${width}-${open ? 'open' : 'closed'}.png`,
                ),
              });
            }
            samples.push(
              await origin.evaluate((node) => {
                const card = node.parentElement!;
                const summary = node.querySelector('summary')!;
                const label = summary.querySelector('.band-trim')!;
                const icon = summary.querySelector('svg')!;
                const heading = card.querySelector('h2')!;
                const cardStyle = getComputedStyle(card);
                const frame = Number.parseFloat(cardStyle.paddingBottom);
                const border = Number.parseFloat(cardStyle.borderBottomWidth);
                const open = (node as HTMLDetailsElement).open;
                const bottom = open ? node.querySelector('ol')! : label;
                return {
                  width: innerWidth,
                  open,
                  textOffset:
                    label.getBoundingClientRect().left - heading.getBoundingClientRect().left,
                  iconInsideRow:
                    icon.getBoundingClientRect().left >= summary.getBoundingClientRect().left &&
                    icon.getBoundingClientRect().right <= summary.getBoundingClientRect().right,
                  rowWidthDifference:
                    summary.getBoundingClientRect().width -
                    card.querySelector('.policy-row')!.getBoundingClientRect().width,
                  rowLeftDifference:
                    summary.getBoundingClientRect().left -
                    card.querySelector('.policy-row')!.getBoundingClientRect().left,
                  rowGap:
                    summary.getBoundingClientRect().top -
                    card.querySelector('.policy-row')!.getBoundingClientRect().bottom,
                  bottomGap:
                    card.getBoundingClientRect().bottom -
                    border -
                    bottom.getBoundingClientRect().bottom,
                  frame,
                  overflow: document.documentElement.scrollWidth > innerWidth,
                };
              }),
            );
            const summary = origin.locator('summary');
            await summary.hover();
            const hovered = await settledDisclosurePaint(summary, false);
            if (auditDirectory !== undefined) {
              await card.screenshot({
                path: join(
                  auditDirectory,
                  `formatting-${colorScheme}-${width}-${open ? 'open' : 'closed'}-hover.png`,
                ),
              });
            }
            await page.mouse.down();
            const pressed = await settledDisclosurePaint(summary, true);
            expect(pressed.background).not.toBe(hovered.background);
            expect(pressed.inset).toContain('inset');
            if (auditDirectory !== undefined) {
              await card.screenshot({
                path: join(
                  auditDirectory,
                  `formatting-${colorScheme}-${width}-${open ? 'open' : 'closed'}-pressed.png`,
                ),
              });
            }
            await page.mouse.up();
            const released = await settledDisclosurePaint(summary, false);
            expect(released.background).toBe(hovered.background);
            expect(released.inset).not.toContain('inset');
          }
        }
        for (const sample of samples) {
          expect(sample.textOffset, JSON.stringify(sample)).toBe(0);
          expect(sample.iconInsideRow, JSON.stringify(sample)).toBe(true);
          expect(sample.rowWidthDifference, JSON.stringify(sample)).toBeCloseTo(0, 1);
          expect(sample.rowLeftDifference, JSON.stringify(sample)).toBeCloseTo(0, 1);
          expect(sample.rowGap, JSON.stringify(sample)).toBeGreaterThanOrEqual(4);
          expect(sample.bottomGap, JSON.stringify(sample)).toBeLessThanOrEqual(sample.frame + 2);
          expect(sample.bottomGap, JSON.stringify(sample)).toBeGreaterThanOrEqual(sample.frame - 2);
          expect(sample.overflow, JSON.stringify(sample)).toBe(false);
        }
      } finally {
        await page.close();
      }
    },
  );

  it.each(['light', 'dark'] as const)(
    'keeps alias lanes and formatting fields on shared geometry in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1440, height: 1000 },
        colorScheme,
      });
      try {
        await visit(page, addressOf(panel, 'workspace/settings'));
        for (const width of [1440, 768, 375]) {
          await page.setViewportSize({ width, height: 1000 });
          const fields = page.locator('.prefix-inline, .number-input');
          expect(await fields.count()).toBeGreaterThan(1);
          for (const field of await fields.all()) {
            const style = await field.evaluate((node) => {
              const reference = document.createElement('input');
              reference.className = 'text-input';
              node.parentElement!.append(reference);
              const paint = (element: Element) => {
                const css = getComputedStyle(element);
                return [
                  css.backgroundColor,
                  css.borderTopWidth,
                  css.borderRadius,
                  element.getBoundingClientRect().height,
                ];
              };
              const actual = paint(node);
              const expected = paint(reference);
              reference.remove();
              return { actual, expected };
            });
            expect(style.actual).toEqual(style.expected);
            expect(style.actual.at(-1)).toBe(34);
          }
          const lane = page.locator('.pair-entry').first();
          const geometry = await lane.evaluate((node) => {
            const box = node.getBoundingClientRect();
            const halves = [...node.querySelectorAll<HTMLInputElement>('.pair-input')].map(
              (input) => {
                const css = getComputedStyle(input);
                const rect = input.getBoundingClientRect();
                return {
                  background: css.backgroundColor,
                  border: css.borderTopWidth,
                  center: rect.y + rect.height / 2,
                };
              },
            );
            return { height: box.height, center: box.y + box.height / 2, halves };
          });
          expect(geometry.height).toBe(34);
          const resetCenter = await page.locator('.alias-controls > .btn').evaluate((button) => {
            const box = button.getBoundingClientRect();
            return box.y + box.height / 2;
          });
          expect(Math.abs(resetCenter - geometry.center)).toBeLessThanOrEqual(1);
          for (const half of geometry.halves) {
            expect(half.background).toBe('rgba(0, 0, 0, 0)');
            expect(half.border).toBe('0px');
            expect(half.center).toBeCloseTo(geometry.center, 0);
          }
          expect(
            await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
          ).toBe(true);
        }
        const command = page.getByRole('combobox', { name: 'Command for alias ship' });
        await command.focus();
        await command.fill('squ');
        await page.getByRole('option', { name: /^squash / }).click();
        await expect.poll(() => command.inputValue()).toBe('squash');
        await command.focus();
        await command.fill('no-such-command');
        await page.getByText('No matching command', { exact: true }).waitFor();
        await command.press('Escape');
        await expect.poll(() => command.inputValue()).toBe('squash');
      } finally {
        await page.close();
      }
    },
  );

  it.each(['light', 'dark'] as const)(
    'preserves shared styling, equal heights, focus and disabled states in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1440, height: 1000 },
        colorScheme,
      });
      try {
        for (const route of [
          'workspace/settings',
          'workspace/repositories/api-gateway',
          'root/workspaces/{account}/settings',
          'root/runtime/settings',
        ]) {
          await visit(page, addressOf(panel, route));
          if (route === 'root/runtime/settings') {
            await page
              .getByRole('button', { name: 'Override the deployment session lifetime' })
              .click();
            const composer = page.getByRole('complementary', { name: 'Settings draft' });
            const paint = await composer.evaluate((node) => {
              const style = getComputedStyle(node);
              const canvas = document.createElement('canvas');
              canvas.width = 1;
              canvas.height = 1;
              const context = canvas.getContext('2d')!;
              context.fillStyle = style.backgroundColor;
              context.fillRect(0, 0, 1, 1);
              return {
                alpha: context.getImageData(0, 0, 1, 1).data[3],
                backdrop: style.backdropFilter,
                border: style.borderTopWidth,
              };
            });
            expect(paint).toEqual({ alpha: 255, backdrop: 'none', border: '2px' });
            for (const width of [1440, 768, 375]) {
              await page.setViewportSize({ width, height: 1000 });
              const rhythm = await composer.locator('.composer-copy').evaluate((copy) => {
                const title = copy.querySelector('strong')!;
                const subtitle = copy.querySelector('span')!;
                return {
                  gap: subtitle.getBoundingClientRect().top - title.getBoundingClientRect().bottom,
                  expected: Number.parseFloat(
                    getComputedStyle(copy).getPropertyValue('--row-copy-gap'),
                  ),
                  titleTrim: getComputedStyle(title).textBoxTrim,
                  subtitleTrim: getComputedStyle(subtitle).textBoxTrim,
                };
              });
              expect(rhythm.gap).toBeCloseTo(rhythm.expected, 1);
              expect(rhythm.titleTrim).toBe('trim-both');
              expect(rhythm.subtitleTrim).toBe('trim-both');
            }
            await page.setViewportSize({ width: 1440, height: 1000 });
          }
          await expectSharedDurationStyles(page);
        }
      } finally {
        await page.close();
      }
    },
  );
});
