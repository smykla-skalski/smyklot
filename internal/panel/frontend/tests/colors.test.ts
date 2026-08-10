import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

type Palette = Record<string, string>;

const css = readFileSync(new URL('../src/app.css', import.meta.url), 'utf8');
const roots = [...css.matchAll(/:root\s*\{(?<body>[^}]*)\}/gu)].map((match) =>
  declarations(match.groups?.body ?? ''),
);
const basePalette = roots[0] ?? {};
const palettes = [basePalette, { ...basePalette, ...(roots[1] ?? {}) }];

function declarations(body: string): Palette {
  return Object.fromEntries(
    [...body.matchAll(/--(?<name>[\w-]+):\s*(?<value>[^;]+);/gu)].map((entry) => [
      entry.groups?.name ?? '',
      entry.groups?.value?.trim() ?? '',
    ]),
  );
}

function color(palette: Palette, name: string): string {
  const value = resolve(palette, name, new Set());
  if (value === undefined || !/^#[\da-f]{6}$/iu.test(value)) {
    throw new Error(`palette is missing a six-digit --${name} color`);
  }
  return value;
}

function resolve(palette: Palette, name: string, seen: Set<string>): string | undefined {
  if (seen.has(name)) throw new Error(`palette contains a circular --${name} reference`);
  seen.add(name);
  const value = palette[name];
  const reference = value?.match(/^var\(--(?<name>[\w-]+)\)$/u)?.groups?.name;
  return reference === undefined ? value : resolve(palette, reference, seen);
}

function contrast(left: string, right: string): number {
  const leftLuminance = luminance(left);
  const rightLuminance = luminance(right);
  const lighter = Math.max(leftLuminance, rightLuminance);
  const darker = Math.min(leftLuminance, rightLuminance);
  return (lighter + 0.05) / (darker + 0.05);
}

function luminance(hex: string): number {
  const channels = [1, 3, 5].map(
    (offset) => Number.parseInt(hex.slice(offset, offset + 2), 16) / 255,
  );
  const [red = 0, green = 0, blue = 0] = channels.map((value) =>
    value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4,
  );
  return 0.2126 * red + 0.7152 * green + 0.0722 * blue;
}

describe.each([
  ['light', palettes[0]],
  ['dark', palettes[1]],
] as const)('%s panel palette', (_theme, palette) => {
  if (palette === undefined) throw new Error('app.css must define light and dark :root palettes');

  it.each([
    ['text-primary', 'surface-base'],
    ['text-primary', 'surface-control'],
    ['text-primary', 'input-bg'],
    ['text-muted', 'surface-base'],
    ['text-muted', 'surface-control'],
    ['text-muted', 'input-bg'],
    ['text-muted', 'surface-raised'],
    ['focus', 'surface-base'],
    ['info', 'surface-base'],
    ['info', 'info-tint'],
    ['success', 'success-tint'],
    ['warning', 'warning-tint'],
    ['danger', 'danger-tint'],
    ['brand-action-text', 'surface-base'],
    ['brand-action-text', 'brand-action-tint'],
    ['on-brand-action', 'brand-action'],
    ['on-info', 'info'],
    ['sidebar-text', 'sidebar-bg'],
    ['sidebar-text-muted', 'sidebar-bg'],
  ])('keeps %s readable on %s', (foreground, background) => {
    expect(contrast(color(palette, foreground), color(palette, background))).toBeGreaterThanOrEqual(
      4.5,
    );
  });

  it('keeps structural rules deliberately subtle', () => {
    const ratio = contrast(color(palette, 'border-subtle'), color(palette, 'surface-base'));
    expect(ratio).toBeGreaterThanOrEqual(1.2);
    expect(ratio).toBeLessThan(2);
  });

  it('keeps resting control boundaries quiet but visible', () => {
    const controlRatio = contrast(color(palette, 'control-border'), color(palette, 'control-bg'));
    const inputRatio = contrast(color(palette, 'control-border'), color(palette, 'input-bg'));
    expect(controlRatio).toBeGreaterThanOrEqual(1.5);
    expect(controlRatio).toBeLessThan(2.25);
    expect(inputRatio).toBeGreaterThanOrEqual(1.4);
    expect(inputRatio).toBeLessThan(2.25);
  });

  it.each(['canvas', 'surface-base', 'surface-control', 'input-bg'])(
    'keeps the focus indicator visible on %s',
    (background) => {
      expect(contrast(color(palette, 'focus'), color(palette, background))).toBeGreaterThanOrEqual(
        3,
      );
    },
  );

  it('keeps active navigation legible without an extra rail', () => {
    expect(
      contrast(color(palette, 'sidebar-item-active-text'), color(palette, 'sidebar-item-active')),
    ).toBeGreaterThanOrEqual(4.5);
  });

  it('keeps control fills from becoming focal surfaces', () => {
    const ratio = contrast(color(palette, 'control-bg'), color(palette, 'surface-base'));
    expect(ratio).toBeLessThan(1.5);
  });

  it('gives editable command fields a distinct inset surface', () => {
    const ratio = contrast(color(palette, 'input-bg'), color(palette, 'surface-base'));
    expect(ratio).toBeGreaterThanOrEqual(1.05);
    expect(ratio).toBeLessThan(1.3);
  });

  it('keeps table filler subtly distinct from rows and table chrome', () => {
    const filler = color(palette, 'table-filler-bg');
    const rowRatio = contrast(filler, color(palette, 'surface-base'));
    const chromeRatio = contrast(filler, color(palette, 'table-header-bg'));

    expect(rowRatio).toBeGreaterThan(1.02);
    expect(rowRatio).toBeLessThan(1.3);
    expect(chromeRatio).toBeGreaterThan(1.02);
    expect(chromeRatio).toBeLessThan(1.3);
  });

  it('keeps active header filters distinct from table chrome', () => {
    expect(
      contrast(color(palette, 'brand-action'), color(palette, 'table-header-bg')),
    ).toBeGreaterThanOrEqual(3);
  });

  it('keeps segmented-control labels readable and selected fills restrained', () => {
    expect(
      contrast(color(palette, 'text-secondary'), color(palette, 'surface-inset')),
    ).toBeGreaterThanOrEqual(4.5);
    for (const fill of ['surface-base', 'brand-action-tint', 'success-tint', 'danger-tint']) {
      const ratio = contrast(color(palette, fill), color(palette, 'surface-inset'));
      expect(ratio).toBeGreaterThan(1);
      expect(ratio).toBeLessThan(1.5);
    }
  });
});
