import { readdirSync, readFileSync } from 'node:fs';
import { join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

import { normalizeBasePath, panelUrl, readPanelBuild, type MetaSource } from '../src/lib/base';

const SOURCE_ROOT = fileURLToPath(new URL('../src', import.meta.url));

/**
 * What the server writes into a served page, and what it leaves behind until it
 * does. Spelled here rather than in the module under test on purpose - see the
 * sweep at the bottom of this file.
 */
const BASE_PATH_SENTINEL = '/__smyklot_panel_base__';
const VERSION_SENTINEL = '__smyklot_panel_version__';
const SERVICE_SENTINEL = '__smyklot_panel_service__';

function documentWithMeta(tags: Record<string, string | null>): MetaSource {
  return {
    querySelector(selector: string) {
      const name = /^meta\[name="(?<name>[^"]+)"\]$/u.exec(selector)?.groups?.name;
      if (name === undefined || !(name in tags)) return null;
      return { getAttribute: () => tags[name] };
    },
  };
}

function sourceFiles(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.name === 'node_modules') return [];
    if (entry.isDirectory()) return sourceFiles(path);
    return /\.(?:ts|svelte)$/u.test(entry.name) ? [path] : [];
  });
}

describe('panel base path', () => {
  it('normalizes operator input and joins absolute routes', () => {
    for (const raw of ['/panel', 'panel', '/panel/', ' /panel// ']) {
      expect(normalizeBasePath(raw)).toBe('/panel');
    }
    expect(normalizeBasePath('/')).toBe('');
    expect(panelUrl('/panel', 'api/v1/session')).toBe('/panel/api/v1/session');
  });
});

describe('panel build metadata', () => {
  it('reads the served version and host', () => {
    expect(
      readPanelBuild(
        documentWithMeta({
          'smyklot-panel-version': '1.13.0',
          'smyklot-panel-service': 'smyklot.example.com',
        }),
      ),
    ).toEqual({ version: '1.13.0', serviceHost: 'smyklot.example.com' });
  });

  it('hides unsubstituted development sentinels', () => {
    expect(
      readPanelBuild(
        documentWithMeta({
          'smyklot-panel-version': VERSION_SENTINEL,
          'smyklot-panel-service': SERVICE_SENTINEL,
        }),
      ),
    ).toEqual({ version: null, serviceHost: null });
  });
});

/**
 * The panel shipped without a footer because the server rewrote the frontend's
 * own copy of a sentinel into the value that sentinel was there to detect, so
 * `readPanelBuild` reported every release as having no version and no host.
 *
 * Nothing under `src` may name a sentinel, because everything under `src` is
 * built into an asset the server rewrites. A placeholder is recognised by its
 * shape instead. `vite.config.ts` still names one - that is SvelteKit's build
 * version, a substitution the server is meant to make, and it is not shipped
 * source. This is the only place the rule can be checked: the corruption
 * happens after the bundle is built, so no test of the module's behaviour can
 * see it.
 */
describe('panel sentinels [Unit]', () => {
  it('are named nowhere the server would rewrite them', () => {
    const sentinels = [BASE_PATH_SENTINEL, VERSION_SENTINEL, SERVICE_SENTINEL];
    const offenders = sourceFiles(SOURCE_ROOT)
      .filter((path) => {
        const source = readFileSync(path, 'utf8');
        return sentinels.some((sentinel) => source.includes(sentinel));
      })
      .map((path) => relative(SOURCE_ROOT, path));

    expect(offenders).toEqual([]);
  });
});
