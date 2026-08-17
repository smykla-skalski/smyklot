import { describe, expect, it } from 'vitest';

import { panelAddress, panelRouteAt } from '../src/lib/addresses.ts';
import { basePath } from '../src/lib/paths.ts';
import { parsePanelRoute, type PanelRoute } from '../src/lib/routes.ts';

/**
 * Every shape the panel can be looking at, with the route SvelteKit matches it to.
 *
 * Written out rather than derived, because deriving it would mean re-running the very
 * mapping under test. What proves the ids are the ones SvelteKit actually reports is the
 * browser suite, which drives real navigation; what this proves is that reading a route
 * back gives the shape that wrote it, and that the reading agrees with the hand parser
 * the mock server still uses. Two implementations, checked against each other.
 */
const CASES: Array<{ route: PanelRoute; id: string; params: Record<string, string> }> = [
  { route: { personal: 'inbox' }, id: '/inbox', params: {} },
  {
    route: { account: 'acme', view: 'repositories' },
    id: '/i/[account]/[view=panelView]',
    params: { account: 'acme', view: 'repositories' },
  },
  {
    route: { account: 'acme', view: 'sync' },
    id: '/i/[account]/[view=panelView]',
    params: { account: 'acme', view: 'sync' },
  },
  {
    route: { account: 'smykla skalski', view: 'repositories' },
    id: '/i/[account]/[view=panelView]',
    params: { account: 'smykla skalski', view: 'repositories' },
  },
  {
    route: { account: 'acme', view: 'history', section: 'failures' },
    id: '/i/[account]/history/[[section=historySection]]',
    params: { account: 'acme', section: 'failures' },
  },
  {
    route: {
      account: 'acme',
      view: 'repositories',
      // A bare repository segment means the dialog's first section, which reading it
      // back says out loud - so the shape that comes out is the normalised one.
      dialog: {
        name: 'repository-settings',
        params: { repository: 'api-gateway', section: 'file' },
      },
    },
    id: '/i/[account]/[view=dialogHostView]/[...rest=dialogPath]',
    params: { account: 'acme', view: 'repositories', rest: 'api-gateway' },
  },
  {
    route: {
      account: 'acme',
      view: 'users',
      dialog: { name: 'user-action', params: { user: 'octocat', action: 'suspend' } },
    },
    id: '/i/[account]/[view=dialogHostView]/[...rest=dialogPath]',
    params: { account: 'acme', view: 'users', rest: 'octocat/suspend' },
  },
  { route: { rootView: 'overview' }, id: '/root', params: {} },
  { route: { rootView: 'installations' }, id: '/root/installations', params: {} },
  { route: { rootView: 'queue' }, id: '/root/queue', params: {} },
  { route: { rootView: 'queue-recent' }, id: '/root/queue/recent', params: {} },
  {
    route: { rootView: 'queue-request', request: 'req-1' },
    id: '/root/queue/request/[id]',
    params: { id: 'req-1' },
  },
  { route: { rootView: 'settings' }, id: '/root/settings', params: {} },
  {
    route: { rootView: 'history-audit' },
    id: '/root/history/[[section=historySection]]',
    params: { section: 'audit' },
  },
  {
    route: { rootView: 'history-failures' },
    id: '/root/history/[[section=historySection]]',
    params: { section: 'failures' },
  },
  {
    route: { rootView: 'access-users' },
    id: '/root/access/[section=accessSection]/[...rest=dialogPath]',
    params: { section: 'users', rest: '' },
  },
  {
    route: { rootView: 'access-invitations' },
    id: '/root/access/[section=accessSection]/[...rest=dialogPath]',
    params: { section: 'invitations', rest: '' },
  },
  {
    route: {
      rootView: 'access-users',
      dialog: { name: 'root-user-action', params: { user: 'octocat', action: 'ban' } },
    },
    id: '/root/access/[section=accessSection]/[...rest=dialogPath]',
    params: { section: 'users', rest: 'octocat/ban' },
  },
  {
    route: { rootView: 'installation', account: 'acme', view: 'repositories' },
    id: '/root/installations/[account]/[view=rootInstallationView]',
    params: { account: 'acme', view: 'repositories' },
  },
  {
    route: { rootView: 'installation', account: 'acme', view: 'history', section: 'failures' },
    id: '/root/installations/[account]/history/[[section=historySection]]',
    params: { account: 'acme', section: 'failures' },
  },
  {
    route: {
      rootView: 'installation',
      account: 'acme',
      view: 'repositories',
      dialog: {
        name: 'repository-settings',
        params: { repository: 'api-gateway', section: 'file' },
      },
    },
    id: '/root/installations/[account]/[view=dialogHostView]/[...rest=dialogPath]',
    params: { account: 'acme', view: 'repositories', rest: 'api-gateway' },
  },
];

/** Drops the keys `panelRouteAt` leaves undefined, so `toEqual` compares like with like. */
function defined(route: PanelRoute | null): PanelRoute | null {
  if (route === null) return null;

  return Object.fromEntries(
    Object.entries(route).filter(([, value]) => value !== undefined),
  ) as PanelRoute;
}

describe('panel addresses [Unit]', () => {
  it.each(CASES.map((entry) => [entry.id, entry] as const))(
    'reads %s back into the route that wrote it',
    (_id, { route, id, params }) => {
      expect(defined(panelRouteAt(id, params))).toEqual(route);
    },
  );

  it('reads the same route the hand parser reads from the address', () => {
    for (const { route, id, params } of CASES) {
      const address = panelAddress(route);

      expect(defined(panelRouteAt(id, params)), address).toEqual(
        parsePanelRoute(basePath, address),
      );
    }
  });

  it('has no route for an address outside the panel', () => {
    expect(panelRouteAt('/', {})).toBeNull();
    expect(panelRouteAt('/invite/[token=invitationToken]', { token: 'x' })).toBeNull();
    expect(panelRouteAt(null, {})).toBeNull();
  });
});
