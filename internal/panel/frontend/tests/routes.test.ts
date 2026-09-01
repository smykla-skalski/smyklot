import { describe, expect, it } from 'vitest';

import {
  DIRECT_PANEL_VIEWS,
  PANEL_VIEWS,
  QUEUE_SECTIONS,
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
  it('reads workspace routes at the public root', () => {
    expect(parsePanelRoute('', '/workspace/smykla-skalski/repositories')).toEqual({
      account: 'smykla-skalski',
      view: 'repositories',
    });
  });

  it('reads routes below a configured panel mount', () => {
    expect(parsePanelRoute('/panel', '/panel/workspace/bartsmykla/history/')).toEqual({
      account: 'bartsmykla',
      view: 'history',
    });
  });

  it('keeps access datasets inside workspace routes', () => {
    expect(parsePanelRoute('', '/help')).toBeNull();
    expect(parsePanelRoute('/panel', '/panel/help/')).toBeNull();
    expect(parsePanelRoute('', '/workspace/smykla-skalski/help')).toBeNull();
    expect(parsePanelRoute('', '/users')).toBeNull();
    expect(parsePanelRoute('', '/invitations')).toBeNull();
    expect(parsePanelRoute('', '/workspace/smykla-skalski/access/users')).toEqual({
      account: 'smykla-skalski',
      view: 'users',
    });
    expect(parsePanelRoute('', '/workspace/smykla-skalski/access/invitations')).toEqual({
      account: 'smykla-skalski',
      view: 'invitations',
    });
    expect(parsePanelRoute('', '/workspace/smykla-skalski/access/owners')).toBeNull();
  });

  it('parses sync sections and refuses what is not one', () => {
    // The bare view is the overview - the section is not written into the path.
    expect(parsePanelRoute('', '/workspace/smykla-skalski/sync')).toEqual({
      account: 'smykla-skalski',
      view: 'sync',
    });
    expect(parsePanelRoute('', '/workspace/smykla-skalski/sync/plan')).toEqual({
      account: 'smykla-skalski',
      view: 'sync',
      sync: 'plan',
    });
    expect(parsePanelRoute('', '/workspace/smykla-skalski/sync/labels')).toEqual({
      account: 'smykla-skalski',
      view: 'sync',
      sync: 'labels',
    });
    // `overview` is never written, so an address naming it does not resolve.
    expect(parsePanelRoute('', '/workspace/smykla-skalski/sync/overview')).toBeNull();
    expect(parsePanelRoute('', '/workspace/smykla-skalski/sync/nonsense')).toBeNull();
    expect(parsePanelRoute('', '/workspace/smykla-skalski/sync/plan/extra')).toBeNull();
  });

  it('parses Queue pages and refuses non-pages', () => {
    expect(parsePanelRoute('', '/workspace/smykla-skalski/queue')).toEqual({
      account: 'smykla-skalski',
      view: 'queue',
    });
    expect(parsePanelRoute('', '/workspace/smykla-skalski/queue/approvals')).toEqual({
      account: 'smykla-skalski',
      view: 'queue',
      queue: 'approvals',
    });
    expect(parsePanelRoute('', '/workspace/smykla-skalski/queue/history')).toEqual({
      account: 'smykla-skalski',
      view: 'queue',
      queue: 'history',
    });
    expect(parsePanelRoute('', '/workspace/smykla-skalski/queue/active')).toBeNull();
    expect(parsePanelRoute('', '/workspace/smykla-skalski/queue/nonsense')).toBeNull();
    expect(parsePanelRoute('', '/workspace/smykla-skalski/queue/history/extra')).toBeNull();
  });

  it('parses one ruleset page and refuses names anywhere else', () => {
    expect(parsePanelRoute('', '/workspace/smykla-skalski/sync/rulesets/main-protection')).toEqual({
      account: 'smykla-skalski',
      view: 'sync',
      sync: 'rulesets',
      syncRuleset: 'main-protection',
    });
    // Encoded names come back decoded, like a repository's do.
    expect(parsePanelRoute('', '/workspace/smykla-skalski/sync/rulesets/main%20guard')).toEqual({
      account: 'smykla-skalski',
      view: 'sync',
      sync: 'rulesets',
      syncRuleset: 'main guard',
    });
    // Only rulesets lists named objects - a name after any other section is
    // an address that does not resolve.
    expect(parsePanelRoute('', '/workspace/smykla-skalski/sync/labels/anything')).toBeNull();
    expect(parsePanelRoute('', '/workspace/smykla-skalski/sync/rulesets/')).toEqual({
      account: 'smykla-skalski',
      view: 'sync',
      sync: 'rulesets',
    });
    expect(parsePanelRoute('', '/workspace/smykla-skalski/sync/rulesets/a/b')).toBeNull();
  });

  it('parses Root routes without treating them as workspaces', () => {
    expect(parsePanelRoute('', '/root')).toEqual({ rootView: 'overview' });
    expect(parsePanelRoute('', '/root/schedules')).toEqual({ rootView: 'schedules' });
    expect(parsePanelRoute('', '/root/queue/approvals')).toEqual({
      rootView: 'queue-approvals',
    });
    expect(parsePanelRoute('', '/root/queue/history')).toEqual({ rootView: 'queue-history' });
    expect(parsePanelRoute('/panel', '/panel/root/workspaces')).toEqual({
      rootView: 'workspaces',
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
    expect(parsePanelRoute('', '/root/workspaces/smykla-skalski/repositories')).toEqual({
      rootView: 'workspace',
      account: 'smykla-skalski',
      view: 'repositories',
    });
  });

  it('rejects removed compatibility addresses', () => {
    /* `defaults` is what this page was addressed while the console's own settings page
       was `/root/settings`. That collision is gone - the console's is `/root/runtime/
       settings` - and the word is one the dictionary retires, so the page is `settings`
       again and `defaults` is the removed one. */
    expect(parsePanelRoute('', '/workspace/acme/defaults')).toBeNull();
    expect(parsePanelRoute('', '/root/workspaces/acme/defaults')).toBeNull();
    expect(parsePanelRoute('', '/root/settings')).toBeNull();
    expect(parsePanelRoute('', '/workspace/acme/users')).toBeNull();
    expect(parsePanelRoute('', '/workspace/acme/invitations')).toBeNull();
  });

  it('treats the panel root as an unresolved destination', () => {
    expect(parsePanelRoute('', '/')).toBeNull();
    expect(parsePanelRoute('/panel', '/panel/')).toBeNull();
  });

  it('rejects unknown tabs and paths outside the panel mount', () => {
    expect(parsePanelRoute('', '/workspace/smykla-skalski/billing')).toBeNull();
    expect(parsePanelRoute('', '/smykla-skalski/settings')).toBeNull();
    expect(parsePanelRoute('', '/auth/settings')).toBeNull();
    expect(parsePanelRoute('', '/webhook/history')).toBeNull();
    expect(parsePanelRoute('', '/@smykla-skalski/settings')).toBeNull();
    expect(parsePanelRoute('/panel', '/workspace/smykla-skalski/settings')).toBeNull();
    expect(parsePanelRoute('/panel', '/panel/too/many/parts')).toBeNull();
    expect(parsePanelRoute('', '/root/access/owners')).toBeNull();
    expect(parsePanelRoute('', '/root/history/unknown')).toBeNull();
    expect(parsePanelRoute('', '/root/runtime/unknown')).toBeNull();
    expect(parsePanelRoute('', '/root/settings/database')).toBeNull();
    expect(parsePanelRoute('', '/root/workspaces/smykla-skalski/unknown')).toBeNull();
  });

  it('recognizes only exact invitation review routes', () => {
    expect(parseInvitationToken('', '/invite/single-use-token')).toBe('single-use-token');
    expect(parseInvitationToken('/panel', '/panel/invite/token%5F1')).toBe('token_1');
    expect(parseInvitationToken('/panel', '/invite/token')).toBeNull();
    expect(parseInvitationToken('', '/invite/token/more')).toBeNull();
  });

  it('encodes account slugs when building links', () => {
    expect(panelAddress({ account: 'smykla skalski', view: 'history' })).toBe(
      `${basePath}/workspace/smykla%20skalski/history`,
    );
    expect(panelAddress({ account: 'bartsmykla', view: 'settings' })).toBe(
      `${basePath}/workspace/bartsmykla/settings`,
    );
    expect(panelAddress({ account: 'bartsmykla', view: 'queue' })).toBe(
      `${basePath}/workspace/bartsmykla/queue`,
    );
    expect(panelAddress({ account: 'bartsmykla', view: 'queue', queue: 'approvals' })).toBe(
      `${basePath}/workspace/bartsmykla/queue/approvals`,
    );
    expect(panelAddress({ rootView: 'queue-history' })).toBe(`${basePath}/root/queue/history`);
    expect(panelAddress({ account: 'bartsmykla', view: 'users' })).toBe(
      `${basePath}/workspace/bartsmykla/access/users`,
    );
    expect(panelAddress({ rootView: 'overview' })).toBe(`${basePath}/root`);
    expect(panelAddress({ rootView: 'schedules' })).toBe(`${basePath}/root/schedules`);
    expect(panelAddress({ rootView: 'access-users' })).toBe(`${basePath}/root/access/users`);
    expect(
      panelAddress({
        rootView: 'workspace',
        account: 'smykla-skalski',
        view: 'history',
      }),
    ).toBe(`${basePath}/root/workspaces/smykla-skalski/history`);
    expect(panelAddress({ account: 'bartsmykla', view: 'invitations' })).toBe(
      `${basePath}/workspace/bartsmykla/access/invitations`,
    );
  });
});

describe('panel document titles', () => {
  it.each([
    [{ account: 'acme', view: 'settings' }, 'Workspace settings | SMYKLOT'],
    [{ account: 'acme', view: 'repositories' }, 'Repositories | SMYKLOT'],
    [{ account: 'acme', view: 'users' }, 'Users | Access | SMYKLOT'],
    [{ account: 'acme', view: 'invitations' }, 'Invitations | Access | SMYKLOT'],
    [{ account: 'acme', view: 'history', section: 'audit' }, 'Audit | History | SMYKLOT'],
    [{ account: 'acme', view: 'history', section: 'failures' }, 'Failures | History | SMYKLOT'],
    [{ account: 'acme', view: 'queue', queue: 'approvals' }, 'Approvals | Queue | SMYKLOT'],
    [{ personal: 'inbox' }, 'Inbox | SMYKLOT'],
    [{ rootView: 'overview' }, 'Overview | Root Console | SMYKLOT'],
    [{ rootView: 'workspaces' }, 'Workspaces | Root Console | SMYKLOT'],
    [{ rootView: 'access-users' }, 'Users | Access | Root Console | SMYKLOT'],
    [{ rootView: 'access-invitations' }, 'Invitations | Access | Root Console | SMYKLOT'],
    [{ rootView: 'history-audit' }, 'Audit | History | Root Console | SMYKLOT'],
    [{ rootView: 'history-failures' }, 'Failures | History | Root Console | SMYKLOT'],
    [{ rootView: 'queue-history' }, 'History | Queue | Root Console | SMYKLOT'],
    [{ rootView: 'runtime-service' }, 'Service health | Root Console | SMYKLOT'],
    [{ rootView: 'runtime-settings' }, 'Service settings | Root Console | SMYKLOT'],
    [
      { rootView: 'workspace', account: 'acme', view: 'repositories' },
      'Repositories | Root Console | SMYKLOT',
    ],
    [
      {
        rootView: 'workspace',
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

  it('orders and groups the Runtime leaves while opening the section on service', () => {
    expect(ROOT_RUNTIME_SECTIONS).toEqual(['service', 'settings']);
    expect(rootSection({ rootView: 'runtime-service' })).toBe('runtime');
    expect(rootSection({ rootView: 'runtime-settings' })).toBe('runtime');
    expect(rootSectionRoute('runtime')).toEqual({ rootView: 'runtime-service' });
  });

  /* The database was a page and is a card on Service health. Its address is kept
     and answered, because an operator's bookmark is not a reason to 404. */
  it('reads the retired database address as the page it lands on', () => {
    expect(parsePanelRoute('', '/root/runtime/database')).toEqual({ rootView: 'runtime-service' });
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

  it('keeps nested views under their own routes', () => {
    for (const view of PANEL_VIEWS.filter(
      (candidate) => !(DIRECT_PANEL_VIEWS as readonly string[]).includes(candidate),
    )) {
      expect(accepts('panelView', view), `${view} escaped the Access route`).toBe(false);
    }
  });

  it('accepts only Queue sections written into addresses', () => {
    for (const section of QUEUE_SECTIONS.filter((candidate) => candidate !== 'active')) {
      expect(accepts('queueSection', section)).toBe(true);
    }
    expect(accepts('queueSection', 'active')).toBe(false);
    expect(accepts('queueSection', 'everything')).toBe(false);
  });

  it('refuses a segment that is not a view', () => {
    expect(accepts('panelView', 'everything')).toBe(false);
    expect(accepts('panelView', '')).toBe(false);
  });
});

describe('resolvePanelRoute', () => {
  const accounts = ['bartsmykla', 'smykla-skalski'];

  it('lets an explicit route win over the remembered workspace', () => {
    const requested: PanelRoute = { account: 'SMYKLA-SKALSKI', view: 'history' };

    /* History resolves with a section: the address never sits on a bare
       /history that a reload would have to guess at. */
    expect(resolvePanelRoute(accounts, requested, 'bartsmykla')).toEqual({
      account: 'smykla-skalski',
      view: 'history',
      section: 'audit',
    });
  });

  /* A workspace opens on its overview - what needs somebody, not the least
     urgent page it holds. */
  it('restores the remembered workspace at its overview', () => {
    expect(resolvePanelRoute(accounts, null, 'smykla-skalski')).toEqual({
      account: 'smykla-skalski',
      view: 'overview',
    });
  });

  it('preserves the requested tab when its workspace is unavailable', () => {
    expect(
      resolvePanelRoute(accounts, { account: 'removed-org', view: 'repositories' }, 'bartsmykla'),
    ).toEqual({ account: 'bartsmykla', view: 'repositories' });
  });

  it('preserves the requested Queue page across workspaces', () => {
    expect(
      resolvePanelRoute(
        accounts,
        { account: 'removed-org', view: 'queue', queue: 'approvals' },
        'bartsmykla',
      ),
    ).toEqual({ account: 'bartsmykla', view: 'queue', queue: 'approvals' });
  });

  it('falls back to the first available workspace', () => {
    expect(resolvePanelRoute(accounts, null, 'removed-org')).toEqual({
      account: 'bartsmykla',
      view: 'overview',
    });
  });

  it('has no destination without a workspace', () => {
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
    expect(parsePanelRoute('', '/workspace/acme/inbox')).toBeNull();
    expect(parsePanelRoute('', '/root/inbox')).toBeNull();
  });

  /* The name is only read in the first segment, so an account called `inbox` is
     still an account. Reading it anywhere else would take a workspace away from
     whoever owns that name. */
  it('leaves an account of the same name alone', () => {
    expect(parsePanelRoute('', '/workspace/inbox/settings')).toEqual({
      account: 'inbox',
      view: 'settings',
    });
    expect(parsePanelRoute('', '/root/workspaces/inbox/repositories')).toEqual({
      rootView: 'workspace',
      account: 'inbox',
      view: 'repositories',
    });
  });

  /* The Root user table offers no history - decisions are made inside a
     workspace - so an address naming one does not resolve, rather than
     resolving to the table with nothing open on it. */
  it('refuses a Root user history that nothing opens', () => {
    expect(parsePanelRoute('', '/root/access/users/octocat/history')).toBeNull();
    expect(parsePanelRoute('', '/root/access/users/octocat/ban')).toEqual({
      rootView: 'access-users',
      dialog: { name: 'root-user-action', params: { user: 'octocat', action: 'ban' } },
    });
    // The same person inside a workspace still has one.
    expect(parsePanelRoute('', '/root/workspaces/acme/access/users/octocat/history')).toEqual({
      rootView: 'workspace',
      account: 'acme',
      view: 'users',
      dialog: { name: 'decision-history', params: { user: 'octocat' } },
    });
  });
});

describe('history sections are addressable', () => {
  it('parses the section off a workspace history path', () => {
    expect(parsePanelRoute('', '/workspace/acme/history/failures')).toEqual({
      account: 'acme',
      view: 'history',
      section: 'failures',
    });
    expect(parsePanelRoute('', '/workspace/acme/history/audit')).toEqual({
      account: 'acme',
      view: 'history',
      section: 'audit',
    });
  });

  it('leaves a bare history path sectionless, and resolves it to audit', () => {
    expect(parsePanelRoute('', '/workspace/acme/history')).toEqual({
      account: 'acme',
      view: 'history',
    });
    expect(resolvePanelRoute(['acme'], { account: 'acme', view: 'history' }, null)).toEqual({
      account: 'acme',
      view: 'history',
      section: 'audit',
    });
  });

  it('refuses a section on a view that has none, and an unknown section', () => {
    expect(parsePanelRoute('', '/workspace/acme/settings/audit')).toBeNull();
    expect(parsePanelRoute('', '/workspace/acme/history/everything')).toBeNull();
  });

  it('writes the section back into the path', () => {
    expect(panelAddress({ account: 'acme', view: 'history', section: 'failures' })).toBe(
      `${basePath}/workspace/acme/history/failures`,
    );
    expect(panelAddress({ account: 'acme', view: 'history' })).toBe(
      `${basePath}/workspace/acme/history`,
    );
    expect(panelAddress({ account: 'acme', view: 'settings' })).toBe(
      `${basePath}/workspace/acme/settings`,
    );
  });

  it('resolves a bare root section path to that section default', () => {
    expect(parsePanelRoute('', '/root/history')).toEqual({ rootView: 'history-audit' });
    expect(parsePanelRoute('', '/root/access')).toEqual({ rootView: 'access-users' });
  });

  it('carries the section through a root workspace route', () => {
    expect(parsePanelRoute('', '/root/workspaces/acme/history/failures')).toEqual({
      rootView: 'workspace',
      account: 'acme',
      view: 'history',
      section: 'failures',
    });
    expect(
      panelAddress({
        rootView: 'workspace',
        account: 'acme',
        view: 'history',
        section: 'failures',
      }),
    ).toBe(`${basePath}/root/workspaces/acme/history/failures`);
  });
});
