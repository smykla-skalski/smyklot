import { describe, expect, it } from 'vitest';

import {
  DIRECT_PANEL_VIEWS,
  PANEL_VIEWS,
  panelDocumentTitle,
  panelViewSection,
  parseInvitationToken,
  parsePanelRoute,
  ROOT_RUNTIME_SECTIONS,
  resolvePanelRoute,
  rootSection,
  rootSectionRoute,
  routeSegmentLabel,
  type PanelRoute,
} from '../src/lib/routes';
import { panelAddress } from '../src/lib/addresses.ts';
import { basePath } from '../src/lib/paths.ts';
import { patterns } from '../src/params.ts';

/**
 * Asks the matcher's pattern, which is the same string the route manifest hands the Go
 * server - so this asserts what both the router and the server will do, not one of them.
 */
function accepts(name: keyof typeof patterns, value: string): boolean {
  return new RegExp(patterns[name]).test(value);
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
    expect(parsePanelRoute('', '/i/smykla-skalski/access/users')).toEqual({
      account: 'smykla-skalski',
      view: 'users',
    });
    expect(parsePanelRoute('', '/i/smykla-skalski/access/invitations')).toEqual({
      account: 'smykla-skalski',
      view: 'invitations',
    });
    expect(parsePanelRoute('', '/i/smykla-skalski/access/owners')).toBeNull();
  });

  it('parses sync sections and refuses what is not one', () => {
    // The bare view is the overview - the section is not written into the path.
    expect(parsePanelRoute('', '/i/smykla-skalski/sync')).toEqual({
      account: 'smykla-skalski',
      view: 'sync',
    });
    expect(parsePanelRoute('', '/i/smykla-skalski/sync/plan')).toEqual({
      account: 'smykla-skalski',
      view: 'sync',
      sync: 'plan',
    });
    expect(parsePanelRoute('', '/i/smykla-skalski/sync/labels')).toEqual({
      account: 'smykla-skalski',
      view: 'sync',
      sync: 'labels',
    });
    // `overview` is never written, so an address naming it does not resolve.
    expect(parsePanelRoute('', '/i/smykla-skalski/sync/overview')).toBeNull();
    expect(parsePanelRoute('', '/i/smykla-skalski/sync/nonsense')).toBeNull();
    expect(parsePanelRoute('', '/i/smykla-skalski/sync/plan/extra')).toBeNull();
  });

  it('parses one ruleset page and refuses names anywhere else', () => {
    expect(parsePanelRoute('', '/i/smykla-skalski/sync/rulesets/main-protection')).toEqual({
      account: 'smykla-skalski',
      view: 'sync',
      sync: 'rulesets',
      syncRuleset: 'main-protection',
    });
    // Encoded names come back decoded, like a repository's do.
    expect(parsePanelRoute('', '/i/smykla-skalski/sync/rulesets/main%20guard')).toEqual({
      account: 'smykla-skalski',
      view: 'sync',
      sync: 'rulesets',
      syncRuleset: 'main guard',
    });
    // Only rulesets lists named objects - a name after any other section is
    // an address that does not resolve.
    expect(parsePanelRoute('', '/i/smykla-skalski/sync/labels/anything')).toBeNull();
    expect(parsePanelRoute('', '/i/smykla-skalski/sync/rulesets/')).toEqual({
      account: 'smykla-skalski',
      view: 'sync',
      sync: 'rulesets',
    });
    expect(parsePanelRoute('', '/i/smykla-skalski/sync/rulesets/a/b')).toBeNull();
  });

  it('parses Root routes without treating them as installations', () => {
    expect(parsePanelRoute('', '/root')).toEqual({ rootView: 'overview' });
    expect(parsePanelRoute('', '/root/schedules')).toEqual({ rootView: 'schedules' });
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
    expect(parsePanelRoute('', '/root/runtime')).toEqual({ rootView: 'runtime-service' });
    expect(parsePanelRoute('', '/root/runtime/settings')).toEqual({
      rootView: 'runtime-settings',
    });
    expect(parsePanelRoute('', '/root/runtime/service')).toEqual({
      rootView: 'runtime-service',
    });
    expect(parsePanelRoute('', '/root/runtime/database')).toEqual({
      rootView: 'runtime-database',
    });
    expect(parsePanelRoute('', '/root/installations/smykla-skalski/repositories')).toEqual({
      rootView: 'installation',
      account: 'smykla-skalski',
      view: 'repositories',
    });
  });

  it('rejects removed compatibility addresses', () => {
    expect(parsePanelRoute('', '/i/acme/settings')).toBeNull();
    expect(parsePanelRoute('', '/root/installations/acme/settings')).toBeNull();
    expect(parsePanelRoute('', '/root/settings')).toBeNull();
    expect(parsePanelRoute('', '/i/acme/users')).toBeNull();
    expect(parsePanelRoute('', '/i/acme/invitations')).toBeNull();
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
    expect(parsePanelRoute('', '/root/runtime/unknown')).toBeNull();
    expect(parsePanelRoute('', '/root/settings/database')).toBeNull();
    expect(parsePanelRoute('', '/root/installations/smykla-skalski/unknown')).toBeNull();
  });

  it('recognizes only exact invitation review routes', () => {
    expect(parseInvitationToken('', '/invite/single-use-token')).toBe('single-use-token');
    expect(parseInvitationToken('/panel', '/panel/invite/token%5F1')).toBe('token_1');
    expect(parseInvitationToken('/panel', '/invite/token')).toBeNull();
    expect(parseInvitationToken('', '/invite/token/more')).toBeNull();
  });

  it('encodes account slugs when building links', () => {
    expect(panelAddress({ account: 'smykla skalski', view: 'history' })).toBe(
      `${basePath}/i/smykla%20skalski/history`,
    );
    expect(panelAddress({ account: 'bartsmykla', view: 'defaults' })).toBe(
      `${basePath}/i/bartsmykla/defaults`,
    );
    expect(panelAddress({ account: 'bartsmykla', view: 'users' })).toBe(
      `${basePath}/i/bartsmykla/access/users`,
    );
    expect(panelAddress({ rootView: 'overview' })).toBe(`${basePath}/root`);
    expect(panelAddress({ rootView: 'schedules' })).toBe(`${basePath}/root/schedules`);
    expect(panelAddress({ rootView: 'access-users' })).toBe(`${basePath}/root/access/users`);
    expect(
      panelAddress({
        rootView: 'installation',
        account: 'smykla-skalski',
        view: 'history',
      }),
    ).toBe(`${basePath}/root/installations/smykla-skalski/history`);
    expect(panelAddress({ account: 'bartsmykla', view: 'invitations' })).toBe(
      `${basePath}/i/bartsmykla/access/invitations`,
    );
  });
});

describe('panel document titles', () => {
  it.each([
    [{ account: 'acme', view: 'defaults' }, 'Defaults | SMYKLOT'],
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
    [{ rootView: 'runtime-service' }, 'Service | Runtime | Root Console | SMYKLOT'],
    [{ rootView: 'runtime-database' }, 'Database | Runtime | Root Console | SMYKLOT'],
    [{ rootView: 'runtime-settings' }, 'Settings | Runtime | Root Console | SMYKLOT'],
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

  it('orders and groups the Runtime leaves while opening the section on settings', () => {
    expect(ROOT_RUNTIME_SECTIONS).toEqual(['service', 'database', 'settings']);
    expect(rootSection({ rootView: 'runtime-service' })).toBe('runtime');
    expect(rootSection({ rootView: 'runtime-database' })).toBe('runtime');
    expect(rootSection({ rootView: 'runtime-settings' })).toBe('runtime');
    expect(rootSectionRoute('runtime')).toEqual({ rootView: 'runtime-service' });
  });
});

/**
 * The router's own list of views, which is the one that decides whether an
 * address exists at all. It kept a second copy of PANEL_VIEWS and drifted: a
 * view added everywhere else was still refused here, so the row in the
 * navigation led to the not-found page and so did a reload.
 */
describe('the direct panel view matcher', () => {
  it('accepts every view written directly after an account', () => {
    for (const view of DIRECT_PANEL_VIEWS) {
      expect(accepts('panelView', view), `the router refuses the ${view} view`).toBe(true);
    }
  });

  it('keeps Access leaves under the Access route', () => {
    for (const view of PANEL_VIEWS.filter(
      (candidate) => !(DIRECT_PANEL_VIEWS as readonly string[]).includes(candidate),
    )) {
      expect(accepts('panelView', view), `${view} escaped the Access route`).toBe(false);
    }
  });

  it('refuses a segment that is not a view', () => {
    expect(accepts('panelView', 'everything')).toBe(false);
    expect(accepts('panelView', '')).toBe(false);
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

  it('restores the remembered installation at the defaults route', () => {
    expect(resolvePanelRoute(accounts, null, 'smykla-skalski')).toEqual({
      account: 'smykla-skalski',
      view: 'defaults',
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
      view: 'defaults',
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
    expect(panelAddress({ personal: 'inbox' })).toBe(`${basePath}/inbox`);
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
    expect(parsePanelRoute('', '/i/inbox/defaults')).toEqual({
      account: 'inbox',
      view: 'defaults',
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
    expect(parsePanelRoute('', '/root/installations/acme/access/users/octocat/history')).toEqual({
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
    expect(parsePanelRoute('', '/i/acme/defaults/audit')).toBeNull();
    expect(parsePanelRoute('', '/i/acme/settings/audit')).toBeNull();
    expect(parsePanelRoute('', '/i/acme/history/everything')).toBeNull();
  });

  it('writes the section back into the path', () => {
    expect(panelAddress({ account: 'acme', view: 'history', section: 'failures' })).toBe(
      `${basePath}/i/acme/history/failures`,
    );
    expect(panelAddress({ account: 'acme', view: 'history' })).toBe(`${basePath}/i/acme/history`);
    expect(panelAddress({ account: 'acme', view: 'defaults' })).toBe(`${basePath}/i/acme/defaults`);
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
      panelAddress({
        rootView: 'installation',
        account: 'acme',
        view: 'history',
        section: 'failures',
      }),
    ).toBe(`${basePath}/root/installations/acme/history/failures`);
  });
});
