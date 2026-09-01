import { readFileSync } from 'node:fs';
import { runInNewContext } from 'node:vm';

import { describe, expect, it } from 'vitest';

const source = readFileSync(new URL('../static/theme-boot.js', import.meta.url), 'utf8');
const template = readFileSync(new URL('../src/app.html', import.meta.url), 'utf8');

function bootTheme({
  stored,
  path,
  systemDark = false,
}: {
  stored: Record<string, string>;
  path: string;
  systemDark?: boolean;
}): { theme: string; colors: string[] } {
  const dataset: Record<string, string> = {};
  const colors: string[] = [];

  runInNewContext(source, {
    localStorage: { getItem: (key: string) => stored[key] ?? null },
    matchMedia: () => ({ matches: systemDark }),
    location: { pathname: path },
    document: {
      documentElement: { dataset },
      querySelector: () => ({ getAttribute: () => '/panel' }),
      querySelectorAll: () => [
        { setAttribute: (_name: string, value: string) => colors.push(value) },
        { setAttribute: (_name: string, value: string) => colors.push(value) },
      ],
    },
  });

  return { theme: dataset.theme ?? '', colors };
}

describe('theme boot script', () => {
  it('loads synchronously from the same origin without an inline nonce', () => {
    expect(template).toContain(
      '<script src="/__smyklot_panel_base__/theme-boot.js?v=__smyklot_panel_version__"></script>',
    );
    expect(template).not.toContain('%sveltekit.nonce%');
  });

  it('uses the pending preference before paint and selects the Root surface', () => {
    const result = bootTheme({
      stored: {
        'smyklot.panel.prefs': JSON.stringify({
          shadow: { theme: 'light' },
          pending: { theme: 'dark' },
        }),
      },
      path: '/panel/root',
    });

    expect(result).toEqual({ theme: 'dark', colors: ['#0f0d14', '#0f0d14'] });
  });

  it('falls back to the system preference when stored data is invalid', () => {
    const result = bootTheme({
      stored: { 'smyklot.panel.prefs': '{not-json' },
      path: '/panel/workspace/acme/settings',
      systemDark: true,
    });

    expect(result).toEqual({ theme: 'dark', colors: ['#0e1116', '#0e1116'] });
  });
});
