import { describe, expect, it } from 'vitest';

import {
  createPanelRouter,
  panelRoutePath,
  parsePanelRoute,
  resolvePanelRoute,
  type PanelRoute,
} from '../src/lib/routes';

function fakeBrowser(initialPath: string) {
  let pathname = initialPath;
  const listeners = new Set<() => void>();
  const location = {
    get pathname(): string {
      return pathname;
    },
  };
  const setPath = (_data: unknown, _unused: string, url?: string | URL | null): void => {
    if (url !== undefined && url !== null) pathname = String(url);
  };

  return {
    browser: {
      location,
      history: { pushState: setPath, replaceState: setPath },
      addEventListener: (_type: 'popstate', listener: () => void) => listeners.add(listener),
      removeEventListener: (_type: 'popstate', listener: () => void) => listeners.delete(listener),
    },
    navigateFromHistory(nextPath: string): void {
      pathname = nextPath;
      for (const listener of listeners) listener();
    },
  };
}

describe('panel routes', () => {
  it('reads installation routes at the public root', () => {
    expect(parsePanelRoute('', '/i/smykla-skalski/repositories')).toEqual({
      account: 'smykla-skalski',
      view: 'repositories',
    });
  });

  it('reads routes below a configured panel mount', () => {
    expect(parsePanelRoute('/panel', '/panel/i/bartsmykla/history/')).toEqual({
      account: 'bartsmykla',
      view: 'history',
    });
  });

  it('keeps help global', () => {
    expect(parsePanelRoute('', '/help')).toEqual({ view: 'help' });
    expect(parsePanelRoute('/panel', '/panel/help/')).toEqual({ view: 'help' });
    expect(parsePanelRoute('', '/i/smykla-skalski/help')).toBeNull();
  });

  it('treats the panel root as an unresolved destination', () => {
    expect(parsePanelRoute('', '/')).toBeNull();
    expect(parsePanelRoute('/panel', '/panel/')).toBeNull();
  });

  it('rejects unknown tabs and paths outside the panel mount', () => {
    expect(parsePanelRoute('', '/i/smykla-skalski/billing')).toBeNull();
    expect(parsePanelRoute('', '/smykla-skalski/settings')).toBeNull();
    expect(parsePanelRoute('', '/auth/settings')).toBeNull();
    expect(parsePanelRoute('', '/webhook/history')).toBeNull();
    expect(parsePanelRoute('', '/@smykla-skalski/settings')).toBeNull();
    expect(parsePanelRoute('/panel', '/i/smykla-skalski/settings')).toBeNull();
    expect(parsePanelRoute('/panel', '/panel/too/many/parts')).toBeNull();
  });

  it('encodes account slugs when building links', () => {
    expect(panelRoutePath('', { account: 'smykla skalski', view: 'history' })).toBe(
      '/i/smykla%20skalski/history',
    );
    expect(panelRoutePath('/panel/', { account: 'bartsmykla', view: 'settings' })).toBe(
      '/panel/i/bartsmykla/settings',
    );
    expect(panelRoutePath('/panel', { view: 'help' })).toBe('/panel/help');
  });
});

describe('resolvePanelRoute', () => {
  const accounts = ['bartsmykla', 'smykla-skalski'];

  it('lets an explicit route win over the remembered installation', () => {
    const requested: PanelRoute = { account: 'SMYKLA-SKALSKI', view: 'history' };

    expect(resolvePanelRoute(accounts, requested, 'bartsmykla')).toEqual({
      account: 'smykla-skalski',
      view: 'history',
    });
  });

  it('restores the remembered installation at the settings route', () => {
    expect(resolvePanelRoute(accounts, null, 'smykla-skalski')).toEqual({
      account: 'smykla-skalski',
      view: 'settings',
    });
  });

  it('opens global help with the remembered installation selected', () => {
    expect(resolvePanelRoute(accounts, { view: 'help' }, 'smykla-skalski')).toEqual({
      account: 'smykla-skalski',
      view: 'help',
    });
  });

  it('preserves the requested tab when its installation is unavailable', () => {
    expect(
      resolvePanelRoute(accounts, { account: 'removed-org', view: 'repositories' }, 'bartsmykla'),
    ).toEqual({ account: 'bartsmykla', view: 'repositories' });
  });

  it('falls back to the first available installation', () => {
    expect(resolvePanelRoute(accounts, null, 'removed-org')).toEqual({
      account: 'bartsmykla',
      view: 'settings',
    });
  });

  it('has no destination without an installation', () => {
    expect(resolvePanelRoute([], null, 'bartsmykla')).toBeNull();
  });
});

describe('browser panel router', () => {
  it('writes canonical links and reports browser history navigation', () => {
    const fixture = fakeBrowser('/panel/i/bartsmykla/settings');
    const router = createPanelRouter('/panel', fixture.browser);
    const visited: Array<PanelRoute | null> = [];
    const unsubscribe = router.subscribe((route) => visited.push(route));

    router.push({ account: 'smykla-skalski', view: 'repositories' });
    expect(fixture.browser.location.pathname).toBe('/panel/i/smykla-skalski/repositories');

    fixture.navigateFromHistory('/panel/i/bartsmykla/history');
    expect(visited).toEqual([{ account: 'bartsmykla', view: 'history' }]);

    router.push({ view: 'help' });
    expect(fixture.browser.location.pathname).toBe('/panel/help');

    unsubscribe();
    fixture.navigateFromHistory('/panel/i/smykla-skalski/repositories');
    expect(visited).toHaveLength(1);
  });
});
