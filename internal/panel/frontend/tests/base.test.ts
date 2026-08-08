import { describe, expect, it } from 'vitest';

import {
  BASE_PATH_SENTINEL,
  normalizeBasePath,
  panelUrl,
  readBasePath,
  readPanelBuild,
} from '../src/lib/base';

function documentWithMeta(tags: Record<string, string | null>): Document {
  return {
    querySelector(selector: string) {
      const name = /^meta\[name="(?<name>[^"]+)"\]$/u.exec(selector)?.groups?.name;
      if (name === undefined || !(name in tags)) {
        return null;
      }
      return { getAttribute: () => tags[name] };
    },
  } as unknown as Document;
}

function documentWithBase(content: string | null): Document {
  return documentWithMeta({ 'harness-panel-base': content });
}

describe('normalizeBasePath', () => {
  it('gives one spelling to the mount points an operator might write', () => {
    for (const raw of ['/panel', 'panel', '/panel/', ' /panel// ']) {
      expect(normalizeBasePath(raw)).toBe('/panel');
    }
  });

  it('treats a root mount as the empty prefix so joining never doubles the slash', () => {
    expect(normalizeBasePath('/')).toBe('');
    expect(panelUrl('/', '/api/me')).toBe('/api/me');
  });
});

describe('panelUrl', () => {
  it('builds an absolute path under the mount point', () => {
    expect(panelUrl('/panel', '/api/me')).toBe('/panel/api/me');
  });

  // A relative URL would resolve against whichever route the browser is
  // showing, so a deep link would send the request to the wrong path.
  it('stays absolute when the caller omits the leading slash', () => {
    expect(panelUrl('/panel', 'api/me')).toBe('/panel/api/me');
  });
});

describe('readBasePath', () => {
  it('reads the prefix the serving binary injected', () => {
    expect(readBasePath(documentWithBase('/pairing'))).toBe('/pairing');
  });

  // `vite dev` serves the unsubstituted page, where the sentinel is the real
  // mount point rather than a placeholder to work around.
  it('accepts the build-time sentinel as an ordinary prefix', () => {
    expect(readBasePath(documentWithBase(BASE_PATH_SENTINEL))).toBe(BASE_PATH_SENTINEL);
  });

  // A panel at the origin root has the sentinel replaced with nothing, so an
  // empty value is a real answer rather than a page served wrong.
  it('reads an empty prefix as the origin root', () => {
    expect(readBasePath(documentWithBase(''))).toBe('');
  });

  it('fails loudly when the page was not served by the panel', () => {
    expect(() => readBasePath(documentWithBase(null))).toThrow(/harness-panel-base/);
    expect(() => readBasePath(documentWithMeta({}))).toThrow(/harness-panel-base/);
  });
});

describe('readPanelBuild', () => {
  it('reads what the serving binary said about itself', () => {
    expect(
      readPanelBuild(
        documentWithMeta({
          'harness-panel-version': '0.3.0',
          'harness-panel-daemon': 'harness.example.com',
        }),
      ),
    ).toEqual({ version: '0.3.0', daemonHost: 'harness.example.com' });
  });

  // Under `vite dev` nothing substitutes these, and a footer reading
  // `__harness_panel_version__` is worse than one that omits the line.
  it('reports nothing for a page no panel has filled in', () => {
    expect(
      readPanelBuild(
        documentWithMeta({
          'harness-panel-version': '__harness_panel_version__',
          'harness-panel-daemon': '__harness_panel_daemon__',
        }),
      ),
    ).toEqual({ version: null, daemonHost: null });
    expect(readPanelBuild(documentWithMeta({}))).toEqual({ version: null, daemonHost: null });
    expect(readPanelBuild(documentWithMeta({ 'harness-panel-version': '  ' }))).toEqual({
      version: null,
      daemonHost: null,
    });
  });
});
