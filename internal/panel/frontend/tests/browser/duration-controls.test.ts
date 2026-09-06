import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import type { Locator, Page } from 'playwright-core';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';

import type { ScheduleRequest } from '../../src/lib/types';

import { calloutGeometry } from './callout-geometry';

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

async function inspectModal(dialog: Locator, page: Page, name: string): Promise<void> {
  await page.mouse.move(0, 0);
  await dialog.evaluate((node) => {
    if (node.contains(document.activeElement)) (document.activeElement as HTMLElement).blur();
  });
  await expect
    .poll(() =>
      dialog.evaluate((node) => {
        const fields = Array.from(
          node.querySelectorAll<HTMLInputElement | HTMLSelectElement>(
            'input:not([type="checkbox"]),select',
          ),
        );
        return fields.every((field) => {
          const probe = document.createElement('input');
          probe.className = 'text-input';
          field.parentElement!.append(probe);
          const actual = getComputedStyle(field);
          const expected = getComputedStyle(probe);
          const matches = [
            'borderTopWidth',
            'borderRadius',
            'backgroundColor',
            'fontSize',
            'height',
          ].every(
            (key) =>
              actual[key as keyof CSSStyleDeclaration] ===
              expected[key as keyof CSSStyleDeclaration],
          );
          probe.remove();
          return matches && field.getBoundingClientRect().height === 34;
        });
      }),
    )
    .toBe(true);
  const result = await dialog.evaluate((node) => ({
    proseFonts: Array.from(node.querySelectorAll('textarea.text-input:not(.mono)')).map(
      (field) => ({
        actual: getComputedStyle(field).fontFamily,
        expected: getComputedStyle(node).fontFamily,
      }),
    ),
    overflow: node.scrollWidth - node.clientWidth,
    nativeChecks: Array.from(node.querySelectorAll('input[type="checkbox"]')).filter(
      (input) => !input.closest('.switch,.check-item'),
    ).length,
    nativeSelects: Array.from(node.querySelectorAll('select')).filter(
      (select) => !select.closest('.select-wrap'),
    ).length,
    rows: Array.from(node.querySelectorAll('.form-row')).map((row) => {
      const label = row.querySelector('.form-label,.setting-say')!.getBoundingClientRect();
      const track = row.querySelector('.switch-track')!.getBoundingClientRect();
      return label.y + label.height / 2 - track.y - track.height / 2;
    }),
  }));
  for (const font of result.proseFonts) expect(font.actual).toBe(font.expected);
  expect(result.overflow).toBeLessThanOrEqual(1);
  expect(result.nativeChecks).toBe(0);
  expect(result.nativeSelects).toBe(0);
  for (const center of result.rows) expect(Math.abs(center)).toBeLessThanOrEqual(1);
  for (const callout of await calloutGeometry(dialog.locator('.callout:has(svg)'))) {
    expect(Math.abs(callout.difference), JSON.stringify(callout)).toBeLessThanOrEqual(0.25);
    for (const gap of callout.lineGaps)
      expect(Math.abs(gap - 8), JSON.stringify(callout)).toBeLessThanOrEqual(0.25);
    expect(
      Math.abs(callout.topSpace - callout.bottomSpace),
      JSON.stringify(callout),
    ).toBeLessThanOrEqual(0.25);
  }
  const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
  if (directory) {
    await mkdir(directory, { recursive: true });
    await dialog.screenshot({ path: join(directory, `${name}.png`), animations: 'disabled' });
  }
}

describe('shared duration field style contract [Browser]', () => {
  it.each([
    { colorScheme: 'light', width: 1440 },
    { colorScheme: 'dark', width: 1440 },
    { colorScheme: 'light', width: 375 },
    { colorScheme: 'dark', width: 375 },
  ] as const)(
    'keeps action, consent and invitation controls coherent at $colorScheme $width',
    async ({ colorScheme, width }) => {
      const page = await panel.browser.newPage({ colorScheme, viewport: { width, height: 1000 } });
      page.setDefaultTimeout(8_000);
      try {
        await visit(page, `${panel.origin}/root/queue`, { ready: '[data-queue-item]' });
        for (const action of ['Schedule exact time', 'Change priority']) {
          await page
            .getByRole('button', { name: 'Actions for Scan for new commands', exact: true })
            .click();
          await page.getByRole('menuitem').filter({ hasText: action }).click();
          const dialog = page.getByRole('dialog', { name: action, exact: true });
          await dialog.waitFor();
          if (action === 'Schedule exact time') {
            await dialog
              .getByRole('checkbox', { name: "Allow this run outside the job's hours" })
              .locator('xpath=ancestor::label[1]')
              .click();
            await dialog
              .getByRole('textbox', { name: 'Reason', exact: true })
              .fill('Release window exception');
          }
          await inspectModal(
            dialog,
            page,
            `queue-${action === 'Schedule exact time' ? 'schedule' : 'priority'}-${colorScheme}-${width}`,
          );
          await page.keyboard.press('Escape');
        }
        await visit(page, `${panel.origin}/root/access/invitations`);
        await page.getByRole('button', { name: 'Invite an operator', exact: true }).first().click();
        const invitation = page.getByRole('dialog', { name: 'Invite an operator', exact: true });
        await invitation.waitFor();
        await inspectModal(invitation, page, `invitation-${colorScheme}-${width}`);
        await page.keyboard.press('Escape');

        await page.route('**/api/v1/root/workspaces/2001/settings', async (route) => {
          const response = await route.fetch();
          const data = await response.json();
          await route.fulfill({ response, json: { ...data, access_source: 'operator' } });
        });
        await visit(page, `${panel.origin}/root/workspaces/${panel.account}/settings`);
        await page.getByRole('button', { name: 'Visit as an operator', exact: true }).click();
        const consent = page.getByRole('dialog', { name: /Visit .* as an operator/ });
        const consentGap = await consent.evaluate((node) => {
          const body = node.querySelector('.modal-body')!;
          const warning = body.querySelector('.callout')!.getBoundingClientRect();
          const fields = body.querySelector('.form-stack')!.getBoundingClientRect();
          return {
            actual: fields.top - warning.bottom,
            expected: parseFloat(getComputedStyle(body).rowGap),
          };
        });
        expect(consentGap.actual).toBe(consentGap.expected);
        const start = consent.getByRole('button', { name: 'Start a 15-minute visit' });
        expect(await start.isDisabled()).toBe(true);
        const checkbox = consent.getByRole('checkbox');
        await checkbox.check();
        expect(await start.isEnabled()).toBe(true);
        expect(await checkbox.evaluate((input) => getComputedStyle(input).opacity)).toBe('0');
        const checkItem = consent.locator('.check-item');
        await checkItem.hover();
        await expect
          .poll(() => checkItem.evaluate((item) => getComputedStyle(item).backgroundColor))
          .not.toBe('rgba(0, 0, 0, 0)');
        await page.mouse.down();
        await expect
          .poll(() =>
            checkItem.evaluate((item) => ({
              background: getComputedStyle(item).backgroundColor,
              inset: getComputedStyle(item).boxShadow !== 'none',
              translate: getComputedStyle(item).translate,
            })),
          )
          .toEqual({ background: 'rgba(0, 0, 0, 0)', inset: true, translate: '0px 1px' });
        await page.mouse.up();
        await checkbox.focus();
        await page.keyboard.press('Tab');
        await page.keyboard.press('Shift+Tab');
        expect(
          await consent.locator('.check-box').evaluate((box) => getComputedStyle(box).outlineStyle),
        ).toBe('solid');
        await checkbox.check();
        await inspectModal(consent, page, `consent-${colorScheme}-${width}`);
        await page.keyboard.press('Escape');

        await page.route('**/api/v1/root/schedule-requests', async (route) => {
          const response = await route.fetch();
          const data: { requests: ScheduleRequest[] } = await response.json();
          const requests = data.requests.map((request, index) => {
            if (index !== 0) return request;
            const custom: ScheduleRequest = {
              ...request,
              custom_profile: {
                id: '',
                name: 'Release hours',
                timezone: 'UTC',
                system: false,
                revision: 0,
                windows: [{ weekday: 1, start_minute: 540, end_minute: 1020 }],
                exceptions: [],
              },
            };
            delete custom.profile_id;
            return custom;
          });
          await route.fulfill({ response, json: { requests } });
        });
        await visit(page, `${panel.origin}/root/schedules`);
        await page.getByRole('button', { name: /Show all \d+ jobs/ }).click();
        for (const title of ['Webhook intake', 'CI re-checks', 'Workspace sync scan']) {
          await page.getByRole('button', { name: `Edit schedule - ${title}`, exact: true }).click();
          const dialog = page.getByRole('dialog', { name: 'Configure job', exact: true });
          await dialog.waitFor();
          await inspectModal(
            dialog,
            page,
            `policy-${title.toLowerCase().replaceAll(' ', '-')}-${colorScheme}-${width}`,
          );
          await page.keyboard.press('Escape');
        }
        await page.getByRole('button', { name: 'Approve', exact: true }).click();
        const approval = page.getByRole('dialog', {
          name: 'Approve schedule request',
          exact: true,
        });
        await approval
          .getByRole('textbox', { name: 'Decision reason' })
          .fill('Useful for release preparation');
        const reuse = approval.getByRole('checkbox', { name: 'Reuse these hours' });
        expect(await reuse.isChecked()).toBe(false);
        await reuse.locator('xpath=ancestor::label[1]').click();
        expect(await reuse.isChecked()).toBe(true);
        expect(
          await approval.getByRole('button', { name: 'Approve', exact: true }).isEnabled(),
        ).toBe(true);
        await inspectModal(approval, page, `schedule-request-${colorScheme}-${width}`);
        await page.keyboard.press('Escape');
      } finally {
        await page.close();
      }
    },
  );

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
