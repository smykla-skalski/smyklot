import { describe, expect, it } from 'vitest';

import {
  panelDocumentTitle,
  panelViewSection,
  panelRoutePath,
  parseInvitationToken,
  parsePanelRoute,
  resolvePanelRoute,
  routeSegmentLabel,
  type PanelRoute,
} from '../src/lib/routes';

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

describe('panel document titles', () => {
  it.each([
    [{ account: 'acme', view: 'settings' }, 'Settings | SMYKLOT'],
    [{ account: 'acme', view: 'repositories' }, 'Repositories | SMYKLOT'],
    [{ account: 'acme', view: 'users' }, 'Users | Access | SMYKLOT'],
    [{ account: 'acme', view: 'invitations' }, 'Invitations | Access | SMYKLOT'],
    [{ account: 'acme', view: 'history', section: 'audit' }, 'Audit | History | SMYKLOT'],
    [{ account: 'acme', view: 'history', section: 'failures' }, 'Failures | History | SMYKLOT'],
    [{ personal: 'inbox' }, 'Inbox | SMYKLOT'],
    [{ rootView: 'overview' }, 'Overview | Root Console | SMYKLOT'],
    [{ rootView: 'installations' }, 'Installations | Root Console | SMYKLOT'],
    [{ rootView: 'access-users' }, 'Users | Access | Root Console | SMYKLOT'],
    [{ rootView: 'access-invitations' }, 'Invitations | Access | Root Console | SMYKLOT'],
    [{ rootView: 'history-audit' }, 'Audit | History | Root Console | SMYKLOT'],
    [{ rootView: 'history-failures' }, 'Failures | History | Root Console | SMYKLOT'],
    [{ rootView: 'settings' }, 'Settings | Root Console | SMYKLOT'],
    [
      { rootView: 'installation', account: 'acme', view: 'repositories' },
      'Repositories | Root Console | SMYKLOT',
    ],
    [
      {
        rootView: 'installation',
        account: 'acme',
        view: 'history',
        section: 'audit',
      },
      'Audit | History | Root Console | SMYKLOT',
    ],
  ] satisfies ReadonlyArray<[PanelRoute, string]>)('formats %j', (route, title) => {
    expect(panelDocumentTitle(route)).toBe(title);
  });

  it('derives labels and title hierarchy from route segments', () => {
    expect(panelViewSection('users')).toBe('access');
    expect(panelViewSection('history')).toBe('history');
    expect(routeSegmentLabel('root-console')).toBe('Root Console');
  });
});

describe('resolvePanelRoute', () => {
  const accounts = ['bartsmykla', 'smykla-skalski'];

  it('lets an explicit route win over the remembered installation', () => {
    const requested: PanelRoute = { account: 'SMYKLA-SKALSKI', view: 'history' };

    /* History resolves with a section: the address never sits on a bare
       /history that a reload would have to guess at. */
    expect(resolvePanelRoute(accounts, requested, 'bartsmykla')).toEqual({
      account: 'smykla-skalski',
      view: 'history',
      section: 'audit',
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

describe('personal routes', () => {
  it('reads the inbox at the top of the panel, under any mount', () => {
    expect(parsePanelRoute('', '/inbox')).toEqual({ personal: 'inbox' });
    expect(parsePanelRoute('/panel', '/panel/inbox/')).toEqual({ personal: 'inbox' });
    expect(panelRoutePath('/panel', { personal: 'inbox' })).toBe('/panel/inbox');
  });

  it('refuses anything hanging off it, or scoped to a workspace', () => {
    expect(parsePanelRoute('', '/inbox/security')).toBeNull();
    expect(parsePanelRoute('', '/i/acme/inbox')).toBeNull();
    expect(parsePanelRoute('', '/root/inbox')).toBeNull();
  });

  /* The name is only read in the first segment, so an account called `inbox` is
     still an account. Reading it anywhere else would take a workspace away from
     whoever owns that name. */
  it('leaves an account of the same name alone', () => {
    expect(parsePanelRoute('', '/i/inbox/settings')).toEqual({
      account: 'inbox',
      view: 'settings',
    });
    expect(parsePanelRoute('', '/root/installations/inbox/repositories')).toEqual({
      rootView: 'installation',
      account: 'inbox',
      view: 'repositories',
    });
  });

  /* The Root user table offers no history - decisions are made inside an
     installation - so an address naming one does not resolve, rather than
     resolving to the table with nothing open on it. */
  it('refuses a Root user history that nothing opens', () => {
    expect(parsePanelRoute('', '/root/access/users/octocat/history')).toBeNull();
    expect(parsePanelRoute('', '/root/access/users/octocat/ban')).toEqual({
      rootView: 'access-users',
      dialog: { name: 'root-user-action', params: { user: 'octocat', action: 'ban' } },
    });
    // The same person inside an installation still has one.
    expect(parsePanelRoute('', '/root/installations/acme/users/octocat/history')).toEqual({
      rootView: 'installation',
      account: 'acme',
      view: 'users',
      dialog: { name: 'decision-history', params: { user: 'octocat' } },
    });
  });
});

describe('history sections are addressable', () => {
  it('parses the section off an installation history path', () => {
    expect(parsePanelRoute('', '/i/acme/history/failures')).toEqual({
      account: 'acme',
      view: 'history',
      section: 'failures',
    });
    expect(parsePanelRoute('', '/i/acme/history/audit')).toEqual({
      account: 'acme',
      view: 'history',
      section: 'audit',
    });
  });

  it('leaves a bare history path sectionless, and resolves it to audit', () => {
    expect(parsePanelRoute('', '/i/acme/history')).toEqual({ account: 'acme', view: 'history' });
    expect(resolvePanelRoute(['acme'], { account: 'acme', view: 'history' }, null)).toEqual({
      account: 'acme',
      view: 'history',
      section: 'audit',
    });
  });

  it('refuses a section on a view that has none, and an unknown section', () => {
    expect(parsePanelRoute('', '/i/acme/settings/audit')).toBeNull();
    expect(parsePanelRoute('', '/i/acme/history/everything')).toBeNull();
  });

  it('writes the section back into the path', () => {
    expect(panelRoutePath('', { account: 'acme', view: 'history', section: 'failures' })).toBe(
      '/i/acme/history/failures',
    );
    expect(panelRoutePath('', { account: 'acme', view: 'history' })).toBe('/i/acme/history');
    expect(panelRoutePath('', { account: 'acme', view: 'settings' })).toBe('/i/acme/settings');
  });

  it('resolves a bare root section path to that section default', () => {
    expect(parsePanelRoute('', '/root/history')).toEqual({ rootView: 'history-audit' });
    expect(parsePanelRoute('', '/root/access')).toEqual({ rootView: 'access-users' });
  });

  it('carries the section through a root installation route', () => {
    expect(parsePanelRoute('', '/root/installations/acme/history/failures')).toEqual({
      rootView: 'installation',
      account: 'acme',
      view: 'history',
      section: 'failures',
    });
    expect(
      panelRoutePath('', {
        rootView: 'installation',
        account: 'acme',
        view: 'history',
        section: 'failures',
      }),
    ).toBe('/root/installations/acme/history/failures');
  });
});
