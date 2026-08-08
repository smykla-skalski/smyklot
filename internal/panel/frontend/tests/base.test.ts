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
      if (name === undefined || !(name in tags)) return null;
      return { getAttribute: () => tags[name] };
    },
  } as unknown as Document;
}

describe('panel base path', () => {
  it('normalizes operator input and joins absolute routes', () => {
    for (const raw of ['/panel', 'panel', '/panel/', ' /panel// ']) {
      expect(normalizeBasePath(raw)).toBe('/panel');
    }
    expect(normalizeBasePath('/')).toBe('');
    expect(panelUrl('/panel', 'api/v1/session')).toBe('/panel/api/v1/session');
  });

  it('reads root and sentinel mount points', () => {
    expect(readBasePath(documentWithMeta({ 'smyklot-panel-base': '' }))).toBe('');
    expect(readBasePath(documentWithMeta({ 'smyklot-panel-base': BASE_PATH_SENTINEL }))).toBe(
      BASE_PATH_SENTINEL,
    );
  });

  it('fails when the server did not inject panel metadata', () => {
    expect(() => readBasePath(documentWithMeta({}))).toThrow(/smyklot-panel-base/);
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
          'smyklot-panel-version': '__smyklot_panel_version__',
          'smyklot-panel-service': '__smyklot_panel_service__',
        }),
      ),
    ).toEqual({ version: null, serviceHost: null });
  });
});
