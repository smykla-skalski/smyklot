import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const source = (path: string): string =>
  readFileSync(new URL(`../src/${path}`, import.meta.url), 'utf8');

function token(css: string, name: string): number {
  const value = new RegExp(`${name}:\\s*(\\d+)`, 'u').exec(css)?.[1];
  if (value === undefined) throw new Error(`${name} is not declared`);
  return Number(value);
}

describe('the responsive shell [Unit]', () => {
  it('keeps the rail below dialogs and above the mobile drawer', () => {
    const css = source('app.css');
    const rail = source('lib/components/Rail.svelte');

    expect(token(css, '--layer-rail')).toBeGreaterThan(40);
    expect(token(css, '--layer-rail')).toBeLessThan(token(css, '--layer-dialog'));
    expect(rail).toMatch(/\.rail\s*\{[^}]*z-index:\s*var\(--layer-rail\)/su);
  });

  it('lets a stacked Root access row grow around all of its cells', () => {
    const access = source('lib/components/RootAccess.svelte');

    expect(access).toMatch(
      /@media \(max-width: 64rem\)[^{]*\{\s*:global\(\.table-scroll tbody tr\)\s*\{[^}]*height:\s*auto/su,
    );
  });

  it('gives the mobile NightPage switch a row of its own', () => {
    const night = source('lib/components/NightPage.svelte');

    expect(night).toMatch(
      /@media \(max-width: 36rem\)[^{]*\{\s*\.night-head\s*\{[^}]*flex-direction:\s*column/su,
    );
  });

  it('renders route failures through the shared recovery page', () => {
    const errorRoute = source('routes/+error.svelte');

    expect(errorRoute).toMatch(/import ErrorPage from/u);
    expect(errorRoute).toMatch(/<ErrorPage\b/u);
    expect(errorRoute).not.toContain('style="padding: 2rem');
  });
});
