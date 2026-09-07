import type { Locator, Page } from 'playwright-core';
import { expect } from 'vitest';

/** Check the real shared control in any pointer or disabled state. */
export async function expectAddPill(control: Locator): Promise<void> {
  const geometry = await control.evaluate((node) => {
    const style = getComputedStyle(node);
    return {
      shared: node.classList.contains('btn-add'),
      height: node.getBoundingClientRect().height,
      border: style.borderTopStyle,
      width: style.borderTopWidth,
      radius: Number.parseFloat(style.borderTopLeftRadius),
      gap: style.columnGap,
      icon: node.querySelector(':scope > svg') !== null,
      label: node.querySelector(':scope > .button-label')?.textContent?.trim(),
    };
  });
  expect(geometry.shared).toBe(true);
  expect(geometry.height).toBe(34);
  expect(geometry.border).toBe('dashed');
  expect(geometry.width).toBe('1px');
  expect(geometry.radius).toBeGreaterThanOrEqual(geometry.height / 2);
  expect(geometry.gap).toBe('8px');
  expect(geometry.icon).toBe(true);
  expect(geometry.label).not.toBe('');
}

export async function expectAdditionPicker(picker: Locator): Promise<void> {
  const geometry = await picker.evaluate((node) => {
    const copy = node.firstElementChild!.getBoundingClientRect();
    const choices = node.querySelector('.addition-choices')!;
    const start = choices.getBoundingClientRect().left;
    const buttons = [...choices.querySelectorAll('button')];
    const boxes = buttons.map((button) => button.getBoundingClientRect());
    const cancel = buttons.at(-1)!;
    const cancelStyle = getComputedStyle(cancel);
    return {
      copyGap: Math.min(...boxes.map((box) => box.top)) - copy.bottom,
      firstStart: boxes[0]!.left - start,
      cancel: {
        height: boxes.at(-1)!.height,
        border: cancelStyle.borderTopStyle,
        radius: cancelStyle.borderTopLeftRadius,
        rightGap: choices.getBoundingClientRect().right - boxes.at(-1)!.right,
      },
      gaps: boxes.slice(1).map((box, index) => {
        const previous = boxes[index]!;
        const sameLine = Math.abs(box.top - previous.top) < 1;
        return {
          cancel: index === boxes.length - 2,
          gap: sameLine ? box.left - previous.right : box.top - previous.bottom,
          lineStart: sameLine ? null : box.left - start,
        };
      }),
    };
  });
  expect(geometry.copyGap).toBeCloseTo(16, 1);
  expect(geometry.firstStart).toBeCloseTo(0, 1);
  expect(geometry.cancel).toEqual({ height: 34, border: 'solid', radius: '8px', rightGap: 0 });
  for (const item of geometry.gaps) {
    if (item.cancel) expect(item.gap).toBeGreaterThanOrEqual(7.9);
    else {
      expect(item.gap).toBeCloseTo(8, 1);
      if (item.lineStart !== null) expect(item.lineStart).toBeCloseTo(0, 1);
    }
  }
}

/** Opening a choice overlay must retain the exact trigger and all document geometry. */
export async function expectStableExpansion(
  page: Page,
  trigger: Locator,
  menu: Locator,
): Promise<void> {
  await page.evaluate(() => document.fonts.ready);
  await trigger.evaluate((node) => node.scrollIntoView({ block: 'center' }));
  const original = await trigger.elementHandle();
  if (!original) throw new Error('Expansion trigger is missing');
  const snapshot = () =>
    original.evaluate((node) => {
      const row = node.closest('.policy-row,.group-rest')!;
      const bounds = (element: Element | null | undefined) => {
        if (!element) return null;
        const rect = element.getBoundingClientRect();
        return { x: rect.x, y: rect.y, width: rect.width, height: rect.height };
      };
      return {
        connected: node.isConnected,
        trigger: bounds(node),
        row: bounds(row),
        previous: bounds(row?.previousElementSibling),
        next: bounds(row?.nextElementSibling),
        scrollX: window.scrollX,
        scrollY: window.scrollY,
        pageHeight: document.documentElement.scrollHeight,
      };
    });
  const before = await snapshot();
  await trigger.click();
  const pressed = await snapshot();
  expect({ ...pressed, trigger: before.trigger }).toEqual(before);
  expect({ ...pressed.trigger, y: before.trigger!.y }).toEqual(before.trigger);
  expect(Math.abs(pressed.trigger!.y - before.trigger!.y)).toBeLessThanOrEqual(1);
  await menu.waitFor();
  await expect.poll(() => menu.getAttribute('data-starting-style')).toBeNull();
  await expect.poll(snapshot).toEqual(before);
  expect(await trigger.getAttribute('aria-expanded')).toBe('true');
  const choices = menu.locator('.btn-add');
  await expect
    .poll(() =>
      choices.first().evaluate(async (node) => {
        await new Promise(requestAnimationFrame);
        return document.activeElement === node;
      }),
    )
    .toBe(true);
  await expect.poll(snapshot).toEqual(before);
  await page.keyboard.press('ArrowDown');
  await expect
    .poll(() => choices.nth(1).evaluate((node) => document.activeElement === node))
    .toBe(true);
  await trigger.click();
  await menu.waitFor({ state: 'hidden' });
  await expect.poll(snapshot).toEqual(before);
  await expect.poll(() => original.evaluate((node) => document.activeElement === node)).toBe(true);
  expect(await trigger.getAttribute('aria-expanded')).toBe('false');
  await trigger.press('Enter');
  await menu.waitFor();
  await expect.poll(snapshot).toEqual(before);
  await page.keyboard.press('Escape');
  await menu.waitFor({ state: 'hidden' });
  await expect.poll(snapshot).toEqual(before);
  await expect.poll(() => original.evaluate((node) => document.activeElement === node)).toBe(true);
  await trigger.click();
  await menu.waitFor();
  await expect.poll(snapshot).toEqual(before);
  await menu.getByRole('button', { name: 'Cancel', exact: true }).click();
  await menu.waitFor({ state: 'hidden' });
  await expect.poll(snapshot).toEqual(before);
  await expect.poll(() => original.evaluate((node) => document.activeElement === node)).toBe(true);
  await trigger.click();
  await menu.waitFor();
  await expect
    .poll(() => choices.first().evaluate((node) => document.activeElement === node))
    .toBe(true);
  await expect.poll(() => menu.getAttribute('data-starting-style')).toBeNull();
  const outside = await original.evaluate((node) => {
    const copy = node
      .closest('.policy-row,.group-rest')!
      .querySelector('.setting-name,.rest-say')!
      .getBoundingClientRect();
    const point = { x: copy.left + 4, y: copy.top + copy.height / 2 };
    const target = document.elementFromPoint(point.x, point.y);
    return {
      ...point,
      target: target?.outerHTML.slice(0, 400),
      inMenu: Boolean(target?.closest('[data-popover-content]')),
      inTrigger: Boolean(target?.closest('[data-popover-trigger]')),
    };
  });
  expect(outside.inMenu).toBe(false);
  expect(outside.inTrigger).toBe(false);
  await page.mouse.click(outside.x, outside.y);
  await menu.waitFor({ state: 'hidden' });
  await expect.poll(snapshot).toEqual(before);
  await expect.poll(() => original.evaluate((node) => document.activeElement === node)).toBe(true);
  await trigger.click();
  await menu.waitFor();
  await expect.poll(snapshot).toEqual(before);
}
