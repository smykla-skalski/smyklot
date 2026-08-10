import { describe, expect, it } from 'vitest';

import {
  createPanelRouter,
  panelRoutePath,
  parseInvitationToken,
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

  it('keeps access datasets inside installation routes', () => {
    expect(parsePanelRoute('', '/help')).toBeNull();
    expect(parsePanelRoute('/panel', '/panel/help/')).toBeNull();
    expect(parsePanelRoute('', '/i/smykla-skalski/help')).toBeNull();
    expect(parsePanelRoute('', '/users')).toBeNull();
    expect(parsePanelRoute('', '/invitations')).toBeNull();
    expect(parsePanelRoute('', '/i/smykla-skalski/users')).toEqual({
      account: 'smykla-skalski',
      view: 'users',
    });
    expect(parsePanelRoute('', '/i/smykla-skalski/invitations')).toEqual({
      account: 'smykla-skalski',
      view: 'invitations',
    });
  });

  it('parses Root routes without treating them as installations', () => {
    expect(parsePanelRoute('', '/root')).toEqual({ rootView: 'overview' });
    expect(parsePanelRoute('/panel', '/panel/root/installations')).toEqual({
      rootView: 'installations',
    });
    expect(parsePanelRoute('', '/root/access/users')).toEqual({ rootView: 'access-users' });
    expect(parsePanelRoute('', '/root/access/invitations')).toEqual({
      rootView: 'access-invitations',
    });
    expect(parsePanelRoute('', '/root/history/audit')).toEqual({ rootView: 'history-audit' });
    expect(parsePanelRoute('', '/root/history/failures')).toEqual({
      rootView: 'history-failures',
    });
    expect(parsePanelRoute('', '/root/settings')).toEqual({ rootView: 'settings' });
    expect(parsePanelRoute('', '/root/installations/smykla-skalski/repositories')).toEqual({
      rootView: 'installation',
      account: 'smykla-skalski',
      view: 'repositories',
    });
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
    expect(parsePanelRoute('', '/root/access/owners')).toBeNull();
    expect(parsePanelRoute('', '/root/history/unknown')).toBeNull();
    expect(parsePanelRoute('', '/root/installations/smykla-skalski/unknown')).toBeNull();
  });

  it('recognizes only exact invitation review routes', () => {
    expect(parseInvitationToken('', '/invite/single-use-token')).toBe('single-use-token');
    expect(parseInvitationToken('/panel', '/panel/invite/token%5F1')).toBe('token_1');
    expect(parseInvitationToken('/panel', '/invite/token')).toBeNull();
    expect(parseInvitationToken('', '/invite/token/more')).toBeNull();
  });

  it('encodes account slugs when building links', () => {
    expect(panelRoutePath('', { account: 'smykla skalski', view: 'history' })).toBe(
      '/i/smykla%20skalski/history',
    );
    expect(panelRoutePath('/panel/', { account: 'bartsmykla', view: 'settings' })).toBe(
      '/panel/i/bartsmykla/settings',
    );
    expect(panelRoutePath('/panel', { account: 'bartsmykla', view: 'users' })).toBe(
      '/panel/i/bartsmykla/users',
    );
    expect(panelRoutePath('/panel', { rootView: 'overview' })).toBe('/panel/root');
    expect(panelRoutePath('', { rootView: 'access-users' })).toBe('/root/access/users');
    expect(
      panelRoutePath('/panel', {
        rootView: 'installation',
        account: 'smykla-skalski',
        view: 'history',
      }),
    ).toBe('/panel/root/installations/smykla-skalski/history');
    expect(panelRoutePath('/panel', { account: 'bartsmykla', view: 'invitations' })).toBe(
      '/panel/i/bartsmykla/invitations',
    );
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

    router.push({ rootView: 'overview' });
    expect(fixture.browser.location.pathname).toBe('/panel/root');

    fixture.navigateFromHistory('/panel/i/bartsmykla/history');
    expect(visited).toEqual([{ account: 'bartsmykla', view: 'history' }]);

    unsubscribe();
    fixture.navigateFromHistory('/panel/i/smykla-skalski/repositories');
    expect(visited).toHaveLength(1);
  });
});
