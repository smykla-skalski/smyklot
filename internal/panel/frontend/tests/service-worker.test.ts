import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const source = readFileSync(new URL('../src/service-worker.ts', import.meta.url), 'utf8');

describe('the panel service worker', () => {
  it('names and removes caches only within its panel scope', () => {
    expect(source).toContain('smyklot-panel:${encodeURIComponent(SCOPE_PATH)}:');
    expect(source).toContain('key.startsWith(CACHE_PREFIX) && key !== CACHE');
    expect(source).not.toMatch(/keys\.filter\(\(key\) => key !== CACHE\)/u);
  });

  it('installs a canonical shell fallback and refreshes it after navigation', () => {
    expect(source).toContain("new Request(SCOPE_PATH, { credentials: 'same-origin' })");
    expect(source).toContain('cache.addAll([...ASSETS, SHELL_REQUEST])');
    expect(source).toContain('cache.put(SHELL_REQUEST, fetched.clone())');
    expect(source).toContain('cache.match(SHELL_REQUEST)');
  });

  it('never replaces the shell fallback with a navigated API response', () => {
    expect(source).toContain("contentType.startsWith('text/html')");
    expect(source).toMatch(
      /fetched\.ok && fetched\.type === 'basic' && contentType\.startsWith\('text\/html'\)/u,
    );
  });

  it('does not intercept same-origin traffic outside the panel mount', () => {
    expect(source).toContain('!url.pathname.startsWith(SCOPE_PATH)');
  });
});
