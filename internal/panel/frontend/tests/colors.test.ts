import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

type Palette = Record<string, string>;

const css = readFileSync(new URL('../src/app.css', import.meta.url), 'utf8');
const palettes = [...css.matchAll(/:root\s*\{(?<body>[^}]*)\}/gu)].map((match) => {
  const entries = [
    ...(match.groups?.body ?? '').matchAll(/--(?<name>[\w-]+):\s*(?<value>#[\da-f]{6})/giu),
  ];
  return Object.fromEntries(
    entries.map((entry) => [entry.groups?.name ?? '', entry.groups?.value ?? '']),
  );
});

function color(palette: Palette, name: string): string {
  const value = palette[name];
  if (value === undefined || !/^#[\da-f]{6}$/iu.test(value)) {
    throw new Error(`palette is missing a six-digit --${name} color`);
  }
  return value;
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
    ['text', 'strip'],
    ['text', 'control-surface'],
    ['text', 'input-surface'],
    ['dim', 'strip'],
    ['dim', 'control-surface'],
    ['dim', 'input-surface'],
    ['dim', 'strip-lift'],
    ['signal', 'strip'],
    ['signal', 'signal-tint'],
    ['clear', 'clear-tint'],
    ['warning', 'warning-tint'],
    ['stop', 'stop-tint'],
    ['accent', 'strip'],
    ['accent', 'accent-tint'],
    ['on-admin', 'admin'],
    ['on-signal', 'signal'],
  ])('keeps %s readable on %s', (foreground, background) => {
    expect(contrast(color(palette, foreground), color(palette, background))).toBeGreaterThanOrEqual(
      4.5,
    );
  });

  it('keeps structural rules deliberately subtle', () => {
    const ratio = contrast(color(palette, 'rule'), color(palette, 'strip'));
    expect(ratio).toBeGreaterThanOrEqual(1.3);
    expect(ratio).toBeLessThan(2);
  });

  it('uses the quiet structural rule for idle control borders', () => {
    expect(color(palette, 'control-border')).toBe(color(palette, 'rule'));
  });

  it('separates control fills from the card without turning them into focal surfaces', () => {
    const ratio = contrast(color(palette, 'control-surface'), color(palette, 'strip'));
    expect(ratio).toBeGreaterThanOrEqual(1.15);
    expect(ratio).toBeLessThan(1.5);
  });

  it('gives editable command fields a distinct inset surface', () => {
    const ratio = contrast(color(palette, 'input-surface'), color(palette, 'strip-lift'));
    expect(ratio).toBeGreaterThanOrEqual(1.1);
    expect(ratio).toBeLessThan(1.3);
  });
});
